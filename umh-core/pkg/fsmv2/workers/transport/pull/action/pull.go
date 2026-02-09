// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package action

import (
	"context"
	"errors"
	"fmt"
	"time"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
)

const PullActionName = "pull"

type PullAction struct{}

func (a *PullAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	pullDeps, ok := depsAny.(snapshot.PullDependencies)
	if !ok {
		return errors.New("invalid dependencies type: expected PullDependencies")
	}

	if !pullDeps.IsTokenValid() {
		return errors.New("token not valid, skipping pull")
	}

	t := pullDeps.GetTransport()
	if t == nil {
		return errors.New("transport is nil")
	}

	metrics := pullDeps.MetricsRecorder()

	pendingBeforeReset := pullDeps.PendingMessageCount()
	if pullDeps.CheckAndClearOnReset() {
		pullDeps.GetLogger().Infow("pull_reset_cleared",
			"pending_dropped", pendingBeforeReset)

		if pendingBeforeReset > 0 {
			metrics.IncrementCounter(depspkg.CounterMessagesDropped, int64(pendingBeforeReset))
		}
	}

	// Phase 1: Deliver pending messages (if any)
	pending := pullDeps.DrainPendingMessages()
	if len(pending) > 0 {
		inChan := pullDeps.GetInboundChan()
		if inChan == nil {
			pullDeps.GetLogger().Debugw("pull_skipped_nil_inbound_chan_pending_delivery")
			pullDeps.StorePendingMessages(pending)

			return nil
		}

		remaining := a.deliverToChannel(ctx, inChan, pending)
		if len(remaining) > 0 {
			pullDeps.StorePendingMessages(remaining)
			metrics.SetGauge(depspkg.GaugePendingMessages, float64(pullDeps.PendingMessageCount()))
			metrics.IncrementCounter(depspkg.CounterPartialDeliveries, 1)

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		}

		pullDeps.RecordSuccess()
		metrics.IncrementCounter(depspkg.CounterPullOps, 1)
		metrics.IncrementCounter(depspkg.CounterPullSuccess, 1)
		metrics.IncrementCounter(depspkg.CounterPendingDelivered, int64(len(pending)))
		metrics.SetGauge(depspkg.GaugePendingMessages, 0)

		return nil
	}

	// Phase 2: Backpressure check
	inChan := pullDeps.GetInboundChan()
	if inChan == nil {
		pullDeps.GetLogger().Debugw("pull_skipped_nil_inbound_chan")

		return nil
	}

	capacity, length := pullDeps.GetInboundChanStats()
	available := capacity - length
	wasBackpressured := pullDeps.IsBackpressured()

	var shouldSkip bool
	if wasBackpressured {
		shouldSkip = available < ExpectedBatchSize*LowWaterMarkMultiplier
	} else {
		shouldSkip = available < ExpectedBatchSize
	}

	if shouldSkip && !wasBackpressured {
		pullDeps.GetLogger().Warnw("backpressure_entering",
			"available", available, "threshold", ExpectedBatchSize)
		pullDeps.SetBackpressured(true)
		metrics.SetGauge(depspkg.GaugeBackpressureActive, 1)
		metrics.IncrementCounter(depspkg.CounterBackpressureEntryTotal, 1)
	}

	if !shouldSkip && wasBackpressured {
		pullDeps.GetLogger().Warnw("backpressure_exiting",
			"available", available, "low_water_mark", ExpectedBatchSize*LowWaterMarkMultiplier)
		pullDeps.SetBackpressured(false)
		metrics.SetGauge(depspkg.GaugeBackpressureActive, 0)
	}

	if shouldSkip {
		metrics.IncrementCounter(depspkg.CounterBackpressureSkips, 1)

		return nil
	}

	// Phase 3: Pull from backend
	jwtToken := pullDeps.GetJWTToken()

	pullStart := time.Now()
	messages, err := t.Pull(ctx, jwtToken)
	pullLatency := time.Since(pullStart)

	if err != nil {
		var transportErr *httpTransport.TransportError
		if errors.As(err, &transportErr) {
			pullDeps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
			metrics.IncrementCounter(counterForErrorType(transportErr.Type), 1)
		} else {
			pullDeps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			metrics.IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)
		}

		metrics.IncrementCounter(depspkg.CounterPullOps, 1)
		metrics.IncrementCounter(depspkg.CounterPullFailures, 1)
		metrics.SetGauge(depspkg.GaugeLastPullLatencyMs, float64(pullLatency.Milliseconds()))

		return fmt.Errorf("pull failed: %w", err)
	}

	var bytesPulled int64

	for _, msg := range messages {
		if msg != nil {
			bytesPulled += int64(len(msg.InstanceUUID) + len(msg.Content) + len(msg.Email))
		}
	}

	metrics.IncrementCounter(depspkg.CounterPullOps, 1)
	metrics.IncrementCounter(depspkg.CounterPullSuccess, 1)
	metrics.IncrementCounter(depspkg.CounterMessagesPulled, int64(len(messages)))
	metrics.IncrementCounter(depspkg.CounterBytesPulled, bytesPulled)
	metrics.SetGauge(depspkg.GaugeLastPullLatencyMs, float64(pullLatency.Milliseconds()))

	if len(messages) == 0 {
		pullDeps.RecordSuccess()

		return nil
	}

	// Phase 4: Deliver pulled messages to inbound channel
	remaining := a.deliverToChannel(ctx, inChan, messages)
	if len(remaining) > 0 {
		pullDeps.StorePendingMessages(remaining)
		metrics.SetGauge(depspkg.GaugePendingMessages, float64(pullDeps.PendingMessageCount()))
		metrics.IncrementCounter(depspkg.CounterPartialDeliveries, 1)

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	pullDeps.RecordSuccess()

	return nil
}

func (a *PullAction) deliverToChannel(ctx context.Context, inChan chan<- *transport.UMHMessage, messages []*transport.UMHMessage) []*transport.UMHMessage {
	for i, msg := range messages {
		if msg == nil {
			continue
		}

		select {
		case inChan <- msg:
		case <-ctx.Done():
			return messages[i:]
		default:
			return messages[i:]
		}
	}

	return nil
}

func (a *PullAction) String() string {
	return PullActionName
}

func (a *PullAction) Name() string {
	return PullActionName
}

const (
	ExpectedBatchSize      = 50
	LowWaterMarkMultiplier = 2
)

func counterForErrorType(t httpTransport.ErrorType) depspkg.CounterName {
	switch t {
	case httpTransport.ErrorTypeCloudflareChallenge:
		return depspkg.CounterCloudflareErrorsTotal
	case httpTransport.ErrorTypeBackendRateLimit:
		return depspkg.CounterBackendRateLimitErrorsTotal
	case httpTransport.ErrorTypeInvalidToken:
		return depspkg.CounterAuthFailuresTotal
	case httpTransport.ErrorTypeInstanceDeleted:
		return depspkg.CounterInstanceDeletedTotal
	case httpTransport.ErrorTypeServerError:
		return depspkg.CounterServerErrorsTotal
	case httpTransport.ErrorTypeProxyBlock:
		return depspkg.CounterProxyBlockErrorsTotal
	default:
		return depspkg.CounterNetworkErrorsTotal
	}
}
