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

package communicator

import (
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"go.uber.org/zap"
)

// CommunicatorDependencies provides transport and channel access for communicator worker actions.
type CommunicatorDependencies struct {
	jwtExpiry         time.Time
	degradedEnteredAt time.Time
	lastAuthAttemptAt time.Time

	transport transport.Transport

	// retryTracker provides framework-level retry tracking.
	// Used by actions like ResetTransportAction to advance past modulo-N triggers.
	retryTracker retry.Tracker

	*deps.BaseDependencies
	inboundChan  chan<- *transport.UMHMessage
	outboundChan <-chan *transport.UMHMessage
	jwtToken     string
	instanceUUID string
	instanceName string

	pulledMessages []*transport.UMHMessage

	lastRetryAfter    time.Duration
	consecutiveErrors int
	lastErrorType     httpTransport.ErrorType

	// backpressured tracks hysteresis state for backpressure detection.
	// When true, a higher threshold (low water mark) is required to resume pulling.
	backpressured bool

	mu sync.RWMutex
}

// NewCommunicatorDependencies creates dependencies for the communicator worker.
// Panics if SetChannelProvider was not called first.
func NewCommunicatorDependencies(t transport.Transport, logger *zap.SugaredLogger, stateReader deps.StateReader, identity deps.Identity) *CommunicatorDependencies {
	provider := GetChannelProvider()
	if provider == nil {
		panic("ChannelProvider must be set before creating communicator dependencies. " +
			"Call SetChannelProvider() in main.go before starting the FSMv2 supervisor.")
	}

	inbound, outbound := provider.GetChannels(identity.ID)

	return &CommunicatorDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		transport:        t,
		inboundChan:      inbound,
		outboundChan:     outbound,
		retryTracker:     retry.New(),
	}
}

// SetTransport sets the transport instance.
func (d *CommunicatorDependencies) SetTransport(t transport.Transport) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.transport = t
}

// GetTransport returns the transport. Nil only before AuthenticateAction runs.
func (d *CommunicatorDependencies) GetTransport() transport.Transport {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.transport
}

// SetJWT stores the JWT token and expiry from authentication response.
func (d *CommunicatorDependencies) SetJWT(token string, expiry time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.jwtToken = token
	d.jwtExpiry = expiry
}

// GetJWTToken returns the stored JWT token.
func (d *CommunicatorDependencies) GetJWTToken() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.jwtToken
}

// GetJWTExpiry returns the stored JWT expiry time.
func (d *CommunicatorDependencies) GetJWTExpiry() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.jwtExpiry
}

// SetPulledMessages stores the messages retrieved from the backend.
func (d *CommunicatorDependencies) SetPulledMessages(messages []*transport.UMHMessage) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pulledMessages = messages
}

// GetPulledMessages returns a shallow copy of the pulled messages. Treat as read-only.
func (d *CommunicatorDependencies) GetPulledMessages() []*transport.UMHMessage {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.pulledMessages == nil {
		return nil
	}

	result := make([]*transport.UMHMessage, len(d.pulledMessages))
	copy(result, d.pulledMessages)

	return result
}

// RecordError increments consecutive errors and records when degraded mode started.
// Transport reset is handled by ResetTransportAction from DegradedState, not here,
// to avoid duplicate resets and maintain single responsibility.
func (d *CommunicatorDependencies) RecordError() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.consecutiveErrors == 0 {
		d.degradedEnteredAt = time.Now()
	}

	d.consecutiveErrors++

	// Keep retry tracker in sync
	d.retryTracker.RecordError()
}

// RecordSuccess resets all error tracking state.
func (d *CommunicatorDependencies) RecordSuccess() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.consecutiveErrors = 0
	d.degradedEnteredAt = time.Time{}
	d.lastErrorType = 0
	d.lastRetryAfter = 0
	d.lastAuthAttemptAt = time.Time{}

	// Keep retry tracker in sync
	d.retryTracker.RecordSuccess()
}

// RecordTypedError increments consecutive errors and records error type and retry-after.
// Transport reset is handled by ResetTransportAction from DegradedState, not here,
// to avoid duplicate resets and maintain single responsibility.
func (d *CommunicatorDependencies) RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.consecutiveErrors == 0 {
		d.degradedEnteredAt = time.Now()
	}

	d.consecutiveErrors++
	d.lastErrorType = errType
	d.lastRetryAfter = retryAfter

	// Keep retry tracker in sync (convert ErrorType to string class)
	d.retryTracker.RecordError(retry.WithClass(errType.String()), retry.WithRetryAfter(retryAfter))
}

// GetLastErrorType returns the last recorded error type.
func (d *CommunicatorDependencies) GetLastErrorType() httpTransport.ErrorType {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastErrorType
}

// GetLastRetryAfter returns the Retry-After duration from the last error.
func (d *CommunicatorDependencies) GetLastRetryAfter() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastRetryAfter
}

// SetLastAuthAttemptAt records the timestamp of the last authentication attempt.
func (d *CommunicatorDependencies) SetLastAuthAttemptAt(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastAuthAttemptAt = t
}

// GetLastAuthAttemptAt returns the timestamp of the last authentication attempt.
func (d *CommunicatorDependencies) GetLastAuthAttemptAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastAuthAttemptAt
}

// GetConsecutiveErrors returns the current consecutive error count.
// Delegates to RetryTracker for single source of truth.
func (d *CommunicatorDependencies) GetConsecutiveErrors() int {
	return d.retryTracker.ConsecutiveErrors()
}

// GetDegradedEnteredAt returns when degraded mode started, or zero if not degraded.
func (d *CommunicatorDependencies) GetDegradedEnteredAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.degradedEnteredAt
}

// RetryTracker returns the framework-level retry tracker.
// Used by recovery actions (like ResetTransportAction) to advance past modulo-N triggers.
func (d *CommunicatorDependencies) RetryTracker() retry.Tracker {
	return d.retryTracker
}

// GetInboundChan returns channel to write received messages, or nil if no provider set.
func (d *CommunicatorDependencies) GetInboundChan() chan<- *transport.UMHMessage {
	return d.inboundChan
}

// GetOutboundChan returns channel to read messages for pushing, or nil if no provider set.
func (d *CommunicatorDependencies) GetOutboundChan() <-chan *transport.UMHMessage {
	return d.outboundChan
}

// -----------------------------------------------------------------------------
// Legacy Metrics Methods (No-op)
//
// These methods exist for interface compatibility with legacy metric recording
// interfaces. In FSMv2, metrics are recorded through deps.MetricsRecorder which
// is automatically exported to Prometheus by the supervisor.
//
// Actions should use:
//   deps.Metrics().IncrementCounter(deps.CounterPullSuccess, 1)
//   deps.Metrics().SetGauge(deps.GaugeLastPullLatencyMs, float64(latency.Milliseconds()))
//
// The supervisor handles delta computation and Prometheus export.
// -----------------------------------------------------------------------------

// RecordPullSuccess is a no-op. Use MetricsRecorder instead (see above).
func (d *CommunicatorDependencies) RecordPullSuccess(latency time.Duration, msgCount int) {}

// RecordPullFailure is a no-op. Use MetricsRecorder instead (see above).
func (d *CommunicatorDependencies) RecordPullFailure(latency time.Duration) {}

// RecordPushSuccess is a no-op. Use MetricsRecorder instead (see above).
func (d *CommunicatorDependencies) RecordPushSuccess(latency time.Duration, msgCount int) {}

// RecordPushFailure is a no-op. Use MetricsRecorder instead (see above).
func (d *CommunicatorDependencies) RecordPushFailure(latency time.Duration) {}

// SetInstanceInfo stores the instance UUID and name. Deprecated: Use SetAuthenticatedUUID instead.
func (d *CommunicatorDependencies) SetInstanceInfo(uuid, name string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.instanceUUID = uuid
	d.instanceName = name
}

// GetInstanceUUID returns the stored instance UUID from backend authentication.
func (d *CommunicatorDependencies) GetInstanceUUID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceUUID
}

// GetInstanceName returns the stored instance name from backend authentication.
func (d *CommunicatorDependencies) GetInstanceName() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceName
}

// GetInstanceInfo returns both the stored instance UUID and name.
func (d *CommunicatorDependencies) GetInstanceInfo() (uuid, name string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceUUID, d.instanceName
}

// SetAuthenticatedUUID stores the UUID returned from the backend after authentication.
func (d *CommunicatorDependencies) SetAuthenticatedUUID(uuid string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.instanceUUID = uuid
}

// GetAuthenticatedUUID returns the stored UUID from backend authentication.
func (d *CommunicatorDependencies) GetAuthenticatedUUID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceUUID
}

// IsBackpressured returns true if the communicator is in backpressure state.
// Backpressure occurs when the inbound channel doesn't have enough capacity
// for the expected batch size from the backend.
func (d *CommunicatorDependencies) IsBackpressured() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.backpressured
}

// SetBackpressured sets the backpressure state.
// Called by SyncAction when entering or exiting backpressure.
func (d *CommunicatorDependencies) SetBackpressured(backpressured bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.backpressured = backpressured
}

// GetInboundChanStats returns the capacity and current length of the inbound channel.
// Returns (0, 0) if no channel provider is set.
//
// Returning (0, 0) when provider is nil intentionally triggers backpressure as a safe default.
// With capacity=0 and length=0, available = 0 - 0 = 0, which is < ExpectedBatchSize (50),
// so the sync action will skip pulling. This prevents pulling messages when we have no
// channel to deliver them to, avoiding potential message loss.
func (d *CommunicatorDependencies) GetInboundChanStats() (capacity int, length int) {
	provider := GetChannelProvider()
	if provider == nil {
		return 0, 0
	}

	return provider.GetInboundStats(d.GetWorkerID())
}
