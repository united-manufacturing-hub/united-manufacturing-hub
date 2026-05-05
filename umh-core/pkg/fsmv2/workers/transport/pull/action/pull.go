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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// PullActionName identifies the pull action in FSM action results.
const PullActionName = "pull"

// PullAction pulls inbound messages from the backend via long-poll and delivers
// them to the inbound channel. It manages a pending-message buffer for partial
// deliveries and applies backpressure when the channel nears capacity.
type PullAction struct{}

// Execute runs one pull cycle: delivers pending messages, checks backpressure,
// pulls new messages from the backend, and delivers them to the inbound channel.
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
		pullDeps.GetLogger().Info("pull_reset_cleared",
			depspkg.Int("pending_dropped", pendingBeforeReset))

		if pendingBeforeReset > 0 {
			metrics.IncrementCounter(depspkg.CounterMessagesDropped, int64(pendingBeforeReset))
		}
	}

	// Phase 1: Deliver pending messages (if any)
	pending := pullDeps.DrainPendingMessages()
	if len(pending) > 0 {
		inChan := pullDeps.GetInboundChan()
		if inChan == nil {
			pullDeps.GetLogger().SentryWarn(depspkg.FeatureForWorker(pullDeps.GetWorkerType()), pullDeps.GetHierarchyPath(), "pull_skipped_nil_inbound_chan_pending_delivery")
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

		metrics.IncrementCounter(depspkg.CounterPullOps, 1)
		metrics.IncrementCounter(depspkg.CounterPullSuccess, 1)
		metrics.IncrementCounter(depspkg.CounterPendingDelivered, int64(len(pending)))
		metrics.SetGauge(depspkg.GaugePendingMessages, 0)

		return nil
	}

	// Phase 2: Backpressure check
	inChan := pullDeps.GetInboundChan()
	if inChan == nil {
		pullDeps.GetLogger().SentryWarn(depspkg.FeatureForWorker(pullDeps.GetWorkerType()), pullDeps.GetHierarchyPath(), "pull_skipped_nil_inbound_chan")

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
		pullDeps.GetLogger().SentryWarn(depspkg.FeatureForWorker(pullDeps.GetWorkerType()), pullDeps.GetHierarchyPath(), "backpressure_entering",
			depspkg.Int("available", available), depspkg.Int("threshold", ExpectedBatchSize))
		pullDeps.SetBackpressured(true)
		metrics.SetGauge(depspkg.GaugeBackpressureActive, 1)
		metrics.IncrementCounter(depspkg.CounterBackpressureEntryTotal, 1)
	}

	if !shouldSkip && wasBackpressured {
		pullDeps.GetLogger().SentryWarn(depspkg.FeatureForWorker(pullDeps.GetWorkerType()), pullDeps.GetHierarchyPath(), "backpressure_exiting",
			depspkg.Int("available", available), depspkg.Int("low_water_mark", ExpectedBatchSize*LowWaterMarkMultiplier))
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
		errType, retryAfter := types.ExtractErrorType(err)
		pullDeps.RecordTypedError(errType, retryAfter)
		metrics.IncrementCounter(types.CounterForErrorType(errType), 1)

		metrics.IncrementCounter(depspkg.CounterPullOps, 1)
		metrics.IncrementCounter(depspkg.CounterPullFailures, 1)
		metrics.SetGauge(depspkg.GaugeLastPullLatencyMs, float64(pullLatency.Milliseconds()))

		if ctx.Err() != nil {
			return fmt.Errorf("pull failed (context canceled): %w", ctx.Err())
		}

		if errType.IsTransient() {
			return nil
		}

		return fmt.Errorf("pull failed: %w", err)
	}

	var bytesPulled int64
	var nonNilCount int64

	for _, msg := range messages {
		if msg != nil {
			nonNilCount++
			bytesPulled += int64(len(msg.InstanceUUID) + len(msg.Content) + len(msg.Email))
		}
	}

	metrics.IncrementCounter(depspkg.CounterPullOps, 1)
	metrics.IncrementCounter(depspkg.CounterPullSuccess, 1)
	metrics.IncrementCounter(depspkg.CounterMessagesPulled, nonNilCount)
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

func (a *PullAction) deliverToChannel(ctx context.Context, inChan chan<- *types.UMHMessage, messages []*types.UMHMessage) []*types.UMHMessage {
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

// String returns the action name for logging.
func (a *PullAction) String() string {
	return PullActionName
}

// Name returns the action name for FSM registration.
func (a *PullAction) Name() string {
	return PullActionName
}

// Backpressure thresholds for inbound channel capacity management.
// These prevent pulling messages from the backend when the local channel is near full.
const (
	// ExpectedBatchSize is the high water mark: stop pulling when available capacity < this value.
	// This is the expected maximum number of messages that could be pulled in one request.
	ExpectedBatchSize = 50

	// LowWaterMarkMultiplier determines when to resume pulling after backpressure.
	// Resume when available capacity >= ExpectedBatchSize * LowWaterMarkMultiplier.
	// This hysteresis prevents oscillation between backpressured and normal states.
	LowWaterMarkMultiplier = 2
)
