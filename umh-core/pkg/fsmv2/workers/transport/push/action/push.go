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

	outChan := pushDeps.GetOutboundChan()
	if outChan == nil {
		return nil
	}

	var messagesToPush []*transport.UMHMessage

drainLoop:
	for {
		select {
		case msg := <-outChan:
			messagesToPush = append(messagesToPush, msg)
		default:
			break drainLoop
		}
	}

	if len(messagesToPush) == 0 {
		return nil
	}

	t := pushDeps.GetTransport()
	if t == nil {
		return errors.New("transport is nil")
	}

	jwtToken := pushDeps.GetJWTToken()
	metrics := pushDeps.MetricsRecorder()

	pushStart := time.Now()
	if err := t.Push(ctx, jwtToken, messagesToPush); err != nil {
		pushLatency := time.Since(pushStart)

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

	return nil
}

func (a *PushAction) String() string {
	return PushActionName
}

func (a *PushAction) Name() string {
	return PushActionName
}

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
	case httpTransport.ErrorTypeNetwork:
		return depspkg.CounterNetworkErrorsTotal
	case httpTransport.ErrorTypeUnknown:
		return depspkg.CounterNetworkErrorsTotal
	default:
		return depspkg.CounterNetworkErrorsTotal
	}
}
