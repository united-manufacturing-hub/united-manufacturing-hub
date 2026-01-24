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
)

const SyncActionName = "sync"

// SyncAction performs bidirectional message sync via HTTP transport.
//
// Pull: HTTPTransport.Pull() → inboundChan (backend → edge).
// Push: outboundChan → HTTPTransport.Push() (edge → backend).
//
// Records metrics and error counts for health monitoring. See worker.go C5 (syncing loop).
type SyncAction struct {
	JWTToken           string
	MessagesToBePushed []*transport.UMHMessage
}

// SyncActionResult contains pushed (edge->backend) and pulled (backend->edge) messages.
type SyncActionResult struct {
	PushedMessages []*transport.UMHMessage
	PulledMessages []*transport.UMHMessage
}

func NewSyncAction(jwtToken string) *SyncAction {
	return &SyncAction{
		JWTToken: jwtToken,
	}
}

// Execute performs a sync tick: pull, write to inbound, drain outbound, push.
func (a *SyncAction) Execute(ctx context.Context, depsAny any) error {
	deps := depsAny.(CommunicatorDependencies)

	pullStart := time.Now()
	messages, err := deps.GetTransport().Pull(ctx, a.JWTToken)
	pullLatency := time.Since(pullStart)

	if err != nil {
		// ENG-3600: RecordTypedError only on actual failure, not before
		deps.RecordPullFailure(pullLatency)

		var transportErr *httpTransport.TransportError
		if errors.As(err, &transportErr) {
			deps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
			deps.MetricsRecorder().IncrementCounter(counterForErrorType(transportErr.Type), 1)
		} else {
			deps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			deps.MetricsRecorder().IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)
		}

		deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullFailures, 1)
		deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, float64(pullLatency.Milliseconds()))

		return fmt.Errorf("pull failed: %w", err)
	}

	deps.RecordPullSuccess(pullLatency, len(messages))

	var bytesPulled int64

	for _, msg := range messages {
		if msg != nil {
			bytesPulled += int64(len(msg.InstanceUUID) + len(msg.Content) + len(msg.Email))
		}
	}

	deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
	deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullSuccess, 1)
	deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, int64(len(messages)))
	deps.MetricsRecorder().IncrementCounter(depspkg.CounterBytesPulled, bytesPulled)
	deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, float64(pullLatency.Milliseconds()))

	deps.SetPulledMessages(messages)

	if inChan := deps.GetInboundChan(); inChan != nil {
		for _, msg := range messages {
			select {
			case inChan <- msg:
			case <-ctx.Done():
				return ctx.Err()
			default:
				deps.GetLogger().Warnw("Inbound channel full, dropping message")
			}
		}
	}

	var messagesToPush []*transport.UMHMessage
	if outChan := deps.GetOutboundChan(); outChan != nil {
	drainLoop:
		for {
			select {
			case msg := <-outChan:
				messagesToPush = append(messagesToPush, msg)
			default:
				break drainLoop
			}
		}
	}

	if len(messagesToPush) == 0 && len(a.MessagesToBePushed) > 0 { // Fallback for tests

		messagesToPush = a.MessagesToBePushed
	}

	if len(messagesToPush) > 0 {
		pushStart := time.Now()
		if err := deps.GetTransport().Push(ctx, a.JWTToken, messagesToPush); err != nil {
			pushLatency := time.Since(pushStart)
			deps.RecordPushFailure(pushLatency)

			var transportErr *httpTransport.TransportError
			if errors.As(err, &transportErr) {
				deps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
				deps.MetricsRecorder().IncrementCounter(counterForErrorType(transportErr.Type), 1)
			} else {
				deps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)
			}

			deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushOps, 1)
			deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushFailures, 1)
			deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPushLatencyMs, float64(pushLatency.Milliseconds()))

			return fmt.Errorf("push failed: %w", err)
		}

		pushLatency := time.Since(pushStart)
		deps.RecordPushSuccess(pushLatency, len(messagesToPush))

		var bytesPushed int64

		for _, msg := range messagesToPush {
			if msg != nil {
				bytesPushed += int64(len(msg.InstanceUUID) + len(msg.Content) + len(msg.Email))
			}
		}

		deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushOps, 1)
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushSuccess, 1)
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPushed, int64(len(messagesToPush)))
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterBytesPushed, bytesPushed)
		deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPushLatencyMs, float64(pushLatency.Milliseconds()))
	}

	// ENG-3600: Counter resets only after confirmed success to prevent oscillation
	deps.RecordSuccess()

	return nil
}

func (a *SyncAction) Name() string {
	return SyncActionName
}
