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

	pushStart := time.Now()
	if err := t.Push(ctx, jwtToken, messagesToPush); err != nil {
		pushLatency := time.Since(pushStart)

		pushDeps.StorePendingMessages(messagesToPush)

		var transportErr *httpTransport.TransportError
		if errors.As(err, &transportErr) {
			pushDeps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
			metrics.IncrementCounter(counterForErrorType(transportErr.Type), 1)
		} else {
			pushDeps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			metrics.IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)
		}

		metrics.IncrementCounter(depspkg.CounterPushOps, 1)
		metrics.IncrementCounter(depspkg.CounterPushFailures, 1)
		metrics.SetGauge(depspkg.GaugeLastPushLatencyMs, float64(pushLatency.Milliseconds()))
		metrics.SetGauge(depspkg.GaugePendingMessages, float64(pushDeps.PendingMessageCount()))

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

func (a *PushAction) retryPending(ctx context.Context, t transport.Transport, pushDeps snapshot.PushDependencies, pending []*transport.UMHMessage, metrics *depspkg.MetricsRecorder) ([]*transport.UMHMessage, error) {
	jwtToken := pushDeps.GetJWTToken()

	for i, msg := range pending {
		select {
		case <-ctx.Done():
			return pending[i:], ctx.Err()
		default:
		}

		if err := t.Push(ctx, jwtToken, []*transport.UMHMessage{msg}); err != nil {
			var transportErr *httpTransport.TransportError

			if errors.As(err, &transportErr) {
				pushDeps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
				metrics.IncrementCounter(counterForErrorType(transportErr.Type), 1)

				if isRecoverableByParent(transportErr.Type) {
					return pending[i:], fmt.Errorf("pending retry failed (recoverable by parent): %w", err)
				}

				pushDeps.GetLogger().SentryWarn(depspkg.FeatureCommunicator, pushDeps.GetHierarchyPath(), "dropping_poison_message",
					depspkg.String("errorType", transportErr.Type.String()),
					depspkg.Err(transportErr),
					depspkg.Int("remaining", len(pending)-i-1))
				metrics.IncrementCounter(depspkg.CounterMessagesDropped, 1)

				continue
			}

			pushDeps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			metrics.IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)

			return pending[i:], fmt.Errorf("pending retry failed: %w", err)
		}

		pushDeps.RecordSuccess()
		metrics.IncrementCounter(depspkg.CounterPushOps, 1)
		metrics.IncrementCounter(depspkg.CounterPushSuccess, 1)
		metrics.IncrementCounter(depspkg.CounterMessagesPushed, 1)
	}

	return nil, nil
}

func counterForErrorType(t httpTransport.ErrorType) depspkg.CounterName {
	switch t {
	case httpTransport.ErrorTypeCloudflareChallenge:
		return depspkg.CounterCloudflareErrorsTotal
	case httpTransport.ErrorTypeBackendRateLimit:
		return depspkg.CounterBackendRateLimitErrorsTotal
	case httpTransport.ErrorTypeInvalidToken:
		return depspkg.CounterAuthFailuresTotal
	// ErrorTypeInstanceDeleted: reachable via HTTP 404 from classifyError().
	// Rare in practice (push endpoint uses session auth, not instance lookup),
	// but kept for correct metric attribution if backend returns 404.
	case httpTransport.ErrorTypeInstanceDeleted:
		return depspkg.CounterInstanceDeletedTotal
	case httpTransport.ErrorTypeServerError:
		return depspkg.CounterServerErrorsTotal
	case httpTransport.ErrorTypeProxyBlock:
		return depspkg.CounterProxyBlockErrorsTotal
	case httpTransport.ErrorTypeNetwork:
		return depspkg.CounterNetworkErrorsTotal
	case httpTransport.ErrorTypeUnknown:
		return depspkg.CounterNetworkErrorsTotal
	default:
		return depspkg.CounterNetworkErrorsTotal
	}
}

// isRecoverableByParent returns true for error types where the message should be
// preserved in the pending buffer rather than dropped. These are transient errors
// (network, server, auth, rate limit, proxy) where the message itself is valid.
// Recovery happens via parent actions (re-authentication, transport reset) or
// child-level backoff (rate limit delay), not by discarding the message.
func isRecoverableByParent(errType httpTransport.ErrorType) bool {
	switch errType {
	case httpTransport.ErrorTypeNetwork,
		httpTransport.ErrorTypeServerError,
		httpTransport.ErrorTypeCloudflareChallenge,
		httpTransport.ErrorTypeBackendRateLimit,
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
