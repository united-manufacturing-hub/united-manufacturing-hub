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
//
// Backpressure check: Before pulling messages from the backend (which deletes them there),
// we check if the local inbound channel has enough capacity. If not, we skip pulling
// to prevent message loss. This is flow control, NOT an error.
//
// ACCEPTED RISK: There's a small race window between the capacity check and message delivery.
// If another goroutine fills the channel during this window, partial delivery may fail.
// This is acceptable because:
// 1. The communicator is the sole producer to inbound channel (no other goroutines writing)
// 2. The existing safety net (lines below) handles partial delivery failure
// 3. Pre-check prevents the COMMON case (pulling into an already-full channel).
func (a *SyncAction) Execute(ctx context.Context, depsAny any) error {
	deps := depsAny.(CommunicatorDependencies)

	// === BACKPRESSURE CHECK (before destructive Pull) ===
	// Check channel capacity to prevent pulling messages we can't deliver.
	capacity, length := deps.GetInboundChanStats()
	available := capacity - length
	wasBackpressured := deps.IsBackpressured()

	var shouldSkipPull bool
	if wasBackpressured {
		// Low water mark: need more capacity to exit backpressure (hysteresis)
		shouldSkipPull = available < ExpectedBatchSize*LowWaterMarkMultiplier
	} else {
		// High water mark: enter backpressure when capacity is low
		shouldSkipPull = available < ExpectedBatchSize
	}

	// Track hysteresis state transitions
	if shouldSkipPull {
		if !wasBackpressured {
			deps.GetLogger().Warnw("backpressure_entering",
				"available", available,
				"threshold", ExpectedBatchSize)
			deps.SetBackpressured(true)
			deps.MetricsRecorder().SetGauge(depspkg.GaugeBackpressureActive, 1)
			deps.MetricsRecorder().IncrementCounter(depspkg.CounterBackpressureEntryTotal, 1)
		}
		// Skip pull, but continue to push outbound - backpressure is NOT an error
	} else {
		if wasBackpressured {
			deps.GetLogger().Warnw("backpressure_exiting",
				"available", available,
				"low_water_mark", ExpectedBatchSize*LowWaterMarkMultiplier)
			deps.SetBackpressured(false)
			deps.MetricsRecorder().SetGauge(depspkg.GaugeBackpressureActive, 0)
		}
	}

	// === PULL LOGIC (only when not backpressured) ===
	if !shouldSkipPull {
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

		// Deliver pulled messages to inbound channel
		if inChan := deps.GetInboundChan(); inChan != nil {
			for i, msg := range messages {
				select {
				case inChan <- msg:
				case <-ctx.Done():
					return ctx.Err()
				default:
					deps.GetLogger().Warnw("inbound_channel_full_stopping_sync",
						"total_messages", len(messages),
						"delivered", i,
						"pending", len(messages)-i)
					// Record as typed error for proper backoff, then return
					deps.RecordTypedError(httpTransport.ErrorTypeChannelFull, 0)

					return errors.New("inbound channel full: stopping sync to prevent message loss")
				}
			}
		}
	}

	// NOTE: At this point, pulled messages (if any) have been delivered to inbound channel.
	// If Push fails below, those messages are still processed (intended behavior).
	// Push will be retried on next SyncAction.
	// Push also runs even during backpressure - outbound is not affected by inbound pressure.
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
	// Don't call RecordSuccess when backpressured - we're not fully syncing
	if !deps.IsBackpressured() {
		deps.RecordSuccess()
	}

	return nil
}

func (a *SyncAction) Name() string {
	return SyncActionName
}
