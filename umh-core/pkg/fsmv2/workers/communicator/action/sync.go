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
	// Dependencies are received via Execute() parameter, not stored in struct
}

// SyncActionResult contains the results of a sync operation.
// PushedMessages are the messages that were sent to the backend.
// PulledMessages are the messages that were received from the backend.
type SyncActionResult struct {
	PushedMessages []*transport.UMHMessage
	PulledMessages []*transport.UMHMessage
}

// NewSyncAction creates a new sync action. Dependencies injected via Execute().
func NewSyncAction(jwtToken string) *SyncAction {
	return &SyncAction{
		JWTToken: jwtToken,
	}
}

// Execute performs a sync tick: pull messages, write to inbound channel,
// drain outbound channel, push messages. Records metrics and errors.
func (a *SyncAction) Execute(ctx context.Context, depsAny any) error {
	// Cast dependencies from supervisor-injected parameter
	deps := depsAny.(CommunicatorDependencies)

	// 1. Pull messages from backend with timing
	pullStart := time.Now()
	messages, err := deps.GetTransport().Pull(ctx, a.JWTToken)
	pullLatency := time.Since(pullStart)

	if err != nil {
		// BUG #1 (ENG-3600): Old communicator reported OK before verifying success.
		// We call RecordTypedError() ONLY on actual failure, never before.
		deps.RecordPullFailure(pullLatency)

		// Extract error type and record typed error (with Retry-After if present)
		var transportErr *httpTransport.TransportError
		if errors.As(err, &transportErr) {
			deps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
			// Record metric for error type
			deps.MetricsRecorder().IncrementCounter(counterForErrorType(transportErr.Type), 1)
		} else {
			// Non-transport error (e.g., context canceled) - treat as network error
			deps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			deps.MetricsRecorder().IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)
		}

		// Record metrics with typed constants
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullFailures, 1)
		deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, float64(pullLatency.Milliseconds()))

		return fmt.Errorf("pull failed: %w", err)
	}

	deps.RecordPullSuccess(pullLatency, len(messages))

	// Calculate bytes pulled
	var bytesPulled int64

	for _, msg := range messages {
		if msg != nil {
			bytesPulled += int64(len(msg.InstanceUUID) + len(msg.Content) + len(msg.Email))
		}
	}

	// Record successful pull metrics with typed constants
	deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullOps, 1)
	deps.MetricsRecorder().IncrementCounter(depspkg.CounterPullSuccess, 1)
	deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPulled, int64(len(messages)))
	deps.MetricsRecorder().IncrementCounter(depspkg.CounterBytesPulled, bytesPulled)
	deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPullLatencyMs, float64(pullLatency.Milliseconds()))

	// 2. Store pulled messages (they will be available in next observed state)
	deps.SetPulledMessages(messages)

	// 3. Write pulled messages to inbound channel (if available)
	if inChan := deps.GetInboundChan(); inChan != nil {
		for _, msg := range messages {
			select {
			case inChan <- msg:
				// Message sent to router
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Channel full, log warning but don't block
				deps.GetLogger().Warnw("Inbound channel full, dropping message")
			}
		}
	}

	// 4. Read messages from outbound channel (if available)
	var messagesToPush []*transport.UMHMessage

	if outChan := deps.GetOutboundChan(); outChan != nil {
		// Non-blocking drain of all available messages
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

	// Fallback: use messages from action struct (for test scenarios)
	if len(messagesToPush) == 0 && len(a.MessagesToBePushed) > 0 {
		messagesToPush = a.MessagesToBePushed
	}

	// 5. Push batch to backend if we have messages (with timing)
	if len(messagesToPush) > 0 {
		pushStart := time.Now()
		if err := deps.GetTransport().Push(ctx, a.JWTToken, messagesToPush); err != nil {
			pushLatency := time.Since(pushStart)
			deps.RecordPushFailure(pushLatency)

			// Extract error type and record typed error (with Retry-After if present)
			var transportErr *httpTransport.TransportError
			if errors.As(err, &transportErr) {
				deps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
				// Record metric for error type
				deps.MetricsRecorder().IncrementCounter(counterForErrorType(transportErr.Type), 1)
			} else {
				// Non-transport error (e.g., context canceled) - treat as network error
				deps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
				deps.MetricsRecorder().IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)
			}

			// Record push failure metrics with typed constants
			deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushOps, 1)
			deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushFailures, 1)
			deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPushLatencyMs, float64(pushLatency.Milliseconds()))

			return fmt.Errorf("push failed: %w", err)
		}

		pushLatency := time.Since(pushStart)
		deps.RecordPushSuccess(pushLatency, len(messagesToPush))

		// Calculate bytes pushed
		var bytesPushed int64

		for _, msg := range messagesToPush {
			if msg != nil {
				bytesPushed += int64(len(msg.InstanceUUID) + len(msg.Content) + len(msg.Email))
			}
		}

		// Record successful push metrics with typed constants
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushOps, 1)
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterPushSuccess, 1)
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterMessagesPushed, int64(len(messagesToPush)))
		deps.MetricsRecorder().IncrementCounter(depspkg.CounterBytesPushed, bytesPushed)
		deps.MetricsRecorder().SetGauge(depspkg.GaugeLastPushLatencyMs, float64(pushLatency.Milliseconds()))
	}

	// 6. Success - call RecordSuccess()
	// BUG #1 (ENG-3600): Counter only resets AFTER confirmed success,
	// preventing oscillation 0→1→0→1.
	deps.RecordSuccess()

	return nil
}

func (a *SyncAction) Name() string {
	return SyncActionName
}
