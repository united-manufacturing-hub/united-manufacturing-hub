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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
)

const PushActionName = "push"

type PushAction struct{}

func (a *PushAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	pushDeps, ok := depsAny.(snapshot.PushDependencies)
	if !ok {
		return errors.New("invalid dependencies type: expected PushDependencies")
	}

	if !pushDeps.IsTokenValid() {
		return errors.New("token not valid, skipping push")
	}

	t := pushDeps.GetTransport()
	if t == nil {
		return errors.New("transport is nil")
	}

	metrics := pushDeps.MetricsRecorder()

	pendingBeforeReset := pushDeps.PendingMessageCount()
	if pushDeps.CheckAndClearOnReset() {
		pushDeps.GetLogger().Info("push_reset_cleared",
			depspkg.Int("pending_dropped", pendingBeforeReset))

		if pendingBeforeReset > 0 {
			metrics.IncrementCounter(depspkg.CounterMessagesDropped, int64(pendingBeforeReset))
		}
	}

	// Phase 1: Retry pending messages one-by-one
	pending := pushDeps.DrainPendingMessages()
	if len(pending) > 0 {
		remaining, err := a.retryPending(ctx, t, pushDeps, pending, metrics)
		if len(remaining) > 0 {
			pushDeps.StorePendingMessages(remaining)
		}

		metrics.SetGauge(depspkg.GaugePendingMessages, float64(pushDeps.PendingMessageCount()))

		return err
	}

	// Phase 2: Drain and batch-push new messages
	outChan := pushDeps.GetOutboundChan()
	if outChan == nil {
		return errors.New("outbound channel is nil")
	}

	var messagesToPush []*transport.UMHMessage

drainLoop:
	for {
		select {
		case msg, ok := <-outChan:
			if !ok {
				break drainLoop
			}

			messagesToPush = append(messagesToPush, msg)
		default:
			break drainLoop
		}
	}

	if len(messagesToPush) == 0 {
		return nil
	}

	jwtToken := pushDeps.GetJWTToken()
	authenticatedUUID := pushDeps.GetAuthenticatedUUID()

	for _, msg := range messagesToPush {
		if msg != nil && authenticatedUUID != "" {
			msg.InstanceUUID = authenticatedUUID
		}
	}

	pushStart := time.Now()
	if err := t.Push(ctx, jwtToken, messagesToPush); err != nil {
		pushLatency := time.Since(pushStart)

		pushDeps.StorePendingMessages(messagesToPush)

		errType, retryAfter := httpTransport.ExtractErrorType(err)
		pushDeps.RecordTypedError(errType, retryAfter)
		metrics.IncrementCounter(httpTransport.CounterForErrorType(errType), 1)

		metrics.IncrementCounter(depspkg.CounterPushOps, 1)
		metrics.IncrementCounter(depspkg.CounterPushFailures, 1)
		metrics.SetGauge(depspkg.GaugeLastPushLatencyMs, float64(pushLatency.Milliseconds()))
		metrics.SetGauge(depspkg.GaugePendingMessages, float64(pushDeps.PendingMessageCount()))

		if ctx.Err() != nil {
			return fmt.Errorf("push failed (context canceled): %w", ctx.Err())
		}

		if errType.IsTransient() {
			return nil
		}

		return fmt.Errorf("push failed: %w", err)
	}

	pushLatency := time.Since(pushStart)

	pushDeps.RecordSuccess()

	var bytesPushed int64

	for _, msg := range messagesToPush {
		if msg != nil {
			bytesPushed += int64(len(msg.InstanceUUID) + len(msg.Content) + len(msg.Email))
		}
	}

	metrics.IncrementCounter(depspkg.CounterPushOps, 1)
	metrics.IncrementCounter(depspkg.CounterPushSuccess, 1)
	metrics.IncrementCounter(depspkg.CounterMessagesPushed, int64(len(messagesToPush)))
	metrics.IncrementCounter(depspkg.CounterBytesPushed, bytesPushed)
	metrics.SetGauge(depspkg.GaugeLastPushLatencyMs, float64(pushLatency.Milliseconds()))
	metrics.SetGauge(depspkg.GaugePendingMessages, 0)

	return nil
}

// retryPending sends previously failed messages one at a time. Messages are
// sent individually (not batched) because a prior batch push already failed,
// and we need per-message error handling to isolate poison messages.
//
// For each message the error cascade is:
//  1. Context canceled (pre-push) → return remaining messages immediately.
//  2. Push fails, context expired during push → return remaining with
//     cancellation error.
//  3. Recoverable by parent (auth failure, proxy block, etc.) → return
//     remaining messages with error so the parent triggers re-authentication
//     or transport reset.
//  4. Poison message (unrecoverable) → log SentryWarn, drop the message,
//     continue to next.
//  5. Success → record metrics, continue.
//
// Returns (nil, nil) when all messages are sent successfully. Otherwise
// returns the unsent tail of pending so the caller can buffer them for
// the next tick.
func (a *PushAction) retryPending(ctx context.Context, t transport.Transport, pushDeps snapshot.PushDependencies, pending []*transport.UMHMessage, metrics *depspkg.MetricsRecorder) ([]*transport.UMHMessage, error) {
	jwtToken := pushDeps.GetJWTToken()
	authenticatedUUID := pushDeps.GetAuthenticatedUUID()

	for i, msg := range pending {
		if msg != nil && authenticatedUUID != "" {
			msg.InstanceUUID = authenticatedUUID
		}

		select {
		case <-ctx.Done():
			return pending[i:], ctx.Err()
		default:
		}

		if err := t.Push(ctx, jwtToken, []*transport.UMHMessage{msg}); err != nil {
			errType, retryAfter := httpTransport.ExtractErrorType(err)
			pushDeps.RecordTypedError(errType, retryAfter)
			metrics.IncrementCounter(httpTransport.CounterForErrorType(errType), 1)

			if ctx.Err() != nil {
				return pending[i:], fmt.Errorf("context canceled during retry: %w", ctx.Err())
			}

			if errType.IsTransient() {
				return pending[i:], nil
			}

			if isRecoverableByParent(errType) {
				return pending[i:], fmt.Errorf("pending retry failed (recoverable by parent): %w", err)
			}

			pushDeps.GetLogger().SentryWarn(depspkg.FeatureForWorker(pushDeps.GetWorkerType()), pushDeps.GetHierarchyPath(), "dropping_poison_message",
				depspkg.String("errorType", errType.String()),
				depspkg.Err(err),
				depspkg.Int("remaining", len(pending)-i-1))
			metrics.IncrementCounter(depspkg.CounterMessagesDropped, 1)

			continue
		}

		pushDeps.RecordSuccess()
		metrics.IncrementCounter(depspkg.CounterPushOps, 1)
		metrics.IncrementCounter(depspkg.CounterPushSuccess, 1)
		metrics.IncrementCounter(depspkg.CounterMessagesPushed, 1)
	}

	return nil, nil
}

// isRecoverableByParent returns true for persistent error types where the
// message is valid but delivery failed due to an external condition requiring
// parent action (re-authentication, transport reset). Messages are preserved
// in the pending buffer for retry rather than dropped.
//
// Only persistent types reach here — transient errors (network, server, rate
// limit, channel full) are intercepted by IsTransient() earlier in retryPending.
func isRecoverableByParent(errType httpTransport.ErrorType) bool {
	switch errType {
	case httpTransport.ErrorTypeCloudflareChallenge,
		httpTransport.ErrorTypeInvalidToken,
		httpTransport.ErrorTypeProxyBlock:
		return true
	default:
		return false
	}
}

func (a *PushAction) String() string {
	return PushActionName
}

func (a *PushAction) Name() string {
	return PushActionName
}
