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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"go.uber.org/zap"
)

// CommunicatorDependencies provides access to tools needed by communicator worker actions.
// It extends BaseDependencies with communicator-specific tools (transport).
//
// JWT storage:
// The jwtToken and jwtExpiry fields store authentication results from AuthenticateAction.
// These are read by CollectObservedState to populate the observed state.
//
// Message storage:
// The pulledMessages field stores messages retrieved by SyncAction.Pull().
// These are read by CollectObservedState to populate the observed state.
//
// Error tracking:
// The consecutiveErrors field tracks consecutive errors for health monitoring.
// This is incremented by RecordError() and reset by RecordSuccess().
//
// Metrics recording:
// Actions use deps.MetricsRecorder().IncrementCounter/SetGauge to record metrics.
// CollectObservedState calls MetricsRecorder().Drain() to merge into ObservedState.Metrics.
//
// Access is protected by mu for thread-safety.
//
// Field ordering: Fields are ordered by decreasing size to minimize struct padding.
// 24-byte fields first, then 16-byte, then 8-byte, then 1-byte (bools) at the end.
type CommunicatorDependencies struct {
	jwtExpiry         time.Time // JWT expiry (set by AuthenticateAction)
	degradedEnteredAt time.Time // When we entered degraded mode (first error after success)
	lastAuthAttemptAt time.Time // Last authentication attempt timestamp (for backoff)

	// 16-byte fields
	transport transport.Transport // HTTP transport for push/pull operations

	// 8-byte fields
	*fsmv2.BaseDependencies
	inboundChan  chan<- *transport.UMHMessage // Write received messages to router
	outboundChan <-chan *transport.UMHMessage // Read messages from router to push
	jwtToken     string                       // JWT token (set by AuthenticateAction)
	instanceUUID string                       // Instance UUID from backend (set by AuthenticateAction)
	instanceName string                       // Instance name from backend (set by AuthenticateAction)

	pulledMessages []*transport.UMHMessage // Pulled messages (set by SyncAction)

	lastRetryAfter    time.Duration // Retry-After from last error (from server)
	consecutiveErrors int           // Error counter (incremented by RecordError)

	// 4-byte fields
	lastErrorType httpTransport.ErrorType // Last error type for intelligent backoff
	// 24-byte fields
	mu sync.RWMutex // Mutex for thread-safe access
}

// NewCommunicatorDependencies creates a new dependencies for the communicator worker.
//
// Phase 1 Architecture: ChannelProvider singleton is THE ONLY way to get channels.
// This function will PANIC if the global ChannelProvider singleton is not set.
// The singleton MUST be set via SetChannelProvider() BEFORE calling this function.
//
// Expected call flow:
//  1. main.go: adapter := NewLegacyChannelBridge(...)
//  2. main.go: SetChannelProvider(adapter)  // Single point of setup
//  3. FSMv2 starts supervisor
//  4. Communicator worker created, NewCommunicatorDependencies calls GetChannelProvider()
//  5. If GetChannelProvider() returns nil -> panic
func NewCommunicatorDependencies(t transport.Transport, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, identity fsmv2.Identity) *CommunicatorDependencies {
	// Phase 1: ChannelProvider singleton is REQUIRED
	provider := GetChannelProvider()
	if provider == nil {
		panic("ChannelProvider must be set before creating communicator dependencies. " +
			"Call SetChannelProvider() in main.go before starting the FSMv2 supervisor.")
	}

	// Get channels from the singleton provider
	inbound, outbound := provider.GetChannels(identity.ID)

	return &CommunicatorDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, stateReader, identity),
		transport:        t,
		inboundChan:      inbound,
		outboundChan:     outbound,
	}
}

// SetTransport sets the transport instance (mutex protected).
// Called by AuthenticateAction on first execution.
func (d *CommunicatorDependencies) SetTransport(t transport.Transport) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.transport = t
}

// GetTransport returns the transport (mutex protected).
//
// Transport nil safety:
//   - Returns nil only if transport was not passed to NewCommunicatorDependencies
//     AND AuthenticateAction.Execute() has not yet run
//   - After AuthenticateAction.Execute() completes (even with error), transport is non-nil
//   - All actions that execute AFTER TryingToAuthenticateState (SyncAction, ResetTransportAction)
//     are guaranteed to have a non-nil transport
//
// State machine guarantees:
//   - TryingToAuthenticateState -> SyncingState transition requires successful authentication
//   - SyncingState -> DegradedState transition happens only after sync operations
//   - DegradedState can only call ResetTransportAction, which is safe
//   - Therefore, only AuthenticateAction may see nil transport (and it handles this by creating one)
func (d *CommunicatorDependencies) GetTransport() transport.Transport {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.transport
}

// SetJWT stores the JWT token and expiry from authentication response.
// This is called by AuthenticateAction after successful authentication.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) SetJWT(token string, expiry time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.jwtToken = token
	d.jwtExpiry = expiry
}

// GetJWTToken returns the stored JWT token.
// This is called by CollectObservedState to populate the observed state.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetJWTToken() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.jwtToken
}

// GetJWTExpiry returns the stored JWT expiry time.
// This is called by CollectObservedState to populate the observed state.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetJWTExpiry() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.jwtExpiry
}

// SetPulledMessages stores the messages retrieved from the backend.
// This is called by SyncAction after successful pull operation.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) SetPulledMessages(messages []*transport.UMHMessage) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pulledMessages = messages
}

// GetPulledMessages returns a shallow copy of the stored pulled messages slice.
// This is called by CollectObservedState to populate the observed state.
//
// Thread-safety notes:
// - The slice itself is copied: adding/removing elements won't affect internal state
// - The message pointers are shared: modifying message contents WILL affect internal state
// - Callers should treat returned messages as read-only.
func (d *CommunicatorDependencies) GetPulledMessages() []*transport.UMHMessage {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.pulledMessages == nil {
		return nil
	}

	// Return a shallow copy of the slice (pointers are still shared)
	result := make([]*transport.UMHMessage, len(d.pulledMessages))
	copy(result, d.pulledMessages)

	return result
}

// RecordError increments the consecutive error counter.
// On the first error (transitioning from 0 errors), it also sets degradedEnteredAt
// to track when we entered degraded mode.
// When consecutive errors reach TransportResetThreshold (or any multiple thereof),
// the transport's Reset() method is called to flush stale connections.
// This is called by actions after an operation fails.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordError() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// If this is the first error, record when we entered degraded mode
	if d.consecutiveErrors == 0 {
		d.degradedEnteredAt = time.Now()
	}

	d.consecutiveErrors++

	// Trigger transport reset when threshold is reached (or at multiples of threshold)
	if d.consecutiveErrors%backoff.TransportResetThreshold == 0 && d.transport != nil {
		d.transport.Reset()
	}
}

// RecordSuccess resets the consecutive error counter to 0 and clears all error tracking state.
// This is called by actions after an operation succeeds.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordSuccess() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.consecutiveErrors = 0
	d.degradedEnteredAt = time.Time{} // Clear degraded entry time
	d.lastErrorType = 0               // Clear error type
	d.lastRetryAfter = 0              // Clear retry-after
	d.lastAuthAttemptAt = time.Time{} // Clear auth attempt time
}

// RecordTypedError increments the consecutive error counter and records error type and retry-after.
// This is called by actions after an operation fails with a classified error.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// If this is the first error, record when we entered degraded mode
	if d.consecutiveErrors == 0 {
		d.degradedEnteredAt = time.Now()
	}

	d.consecutiveErrors++
	d.lastErrorType = errType
	d.lastRetryAfter = retryAfter

	// Trigger transport reset when threshold is reached (or at multiples of threshold)
	if d.consecutiveErrors%backoff.TransportResetThreshold == 0 && d.transport != nil {
		d.transport.Reset()
	}
}

// GetLastErrorType returns the last recorded error type.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetLastErrorType() httpTransport.ErrorType {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastErrorType
}

// GetLastRetryAfter returns the Retry-After duration from the last error.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetLastRetryAfter() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastRetryAfter
}

// SetLastAuthAttemptAt records the timestamp of the last authentication attempt.
// This is used for backoff calculation in TryingToAuthenticateState.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) SetLastAuthAttemptAt(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastAuthAttemptAt = t
}

// GetLastAuthAttemptAt returns the timestamp of the last authentication attempt.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetLastAuthAttemptAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastAuthAttemptAt
}

// GetConsecutiveErrors returns the current consecutive error count.
// This is called by CollectObservedState to populate the observed state.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetConsecutiveErrors() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.consecutiveErrors
}

// GetDegradedEnteredAt returns when we entered degraded mode (first error after success).
// Returns zero time if not currently in degraded mode (no consecutive errors).
// This is called by CollectObservedState to populate the observed state.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetDegradedEnteredAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.degradedEnteredAt
}

// GetInboundChan returns channel to write received messages.
// May return nil if no channel provider was set.
func (d *CommunicatorDependencies) GetInboundChan() chan<- *transport.UMHMessage {
	return d.inboundChan
}

// GetOutboundChan returns channel to read messages for pushing.
// May return nil if no channel provider was set.
func (d *CommunicatorDependencies) GetOutboundChan() <-chan *transport.UMHMessage {
	return d.outboundChan
}

// RecordPullSuccess is a no-op kept for interface compatibility.
// Metrics are recorded via MetricsRecorder instead.
func (d *CommunicatorDependencies) RecordPullSuccess(latency time.Duration, msgCount int) {}

// RecordPullFailure is a no-op kept for interface compatibility.
// Metrics are recorded via MetricsRecorder instead.
func (d *CommunicatorDependencies) RecordPullFailure(latency time.Duration) {}

// RecordPushSuccess is a no-op kept for interface compatibility.
// Metrics are recorded via MetricsRecorder instead.
func (d *CommunicatorDependencies) RecordPushSuccess(latency time.Duration, msgCount int) {}

// RecordPushFailure is a no-op kept for interface compatibility.
// Metrics are recorded via MetricsRecorder instead.
func (d *CommunicatorDependencies) RecordPushFailure(latency time.Duration) {}

// SetInstanceInfo stores the instance UUID and name from authentication response.
// Called by AuthenticateAction after successful authentication.
// Thread-safe: uses mutex for concurrent access protection.
//
// Deprecated: Use SetAuthenticatedUUID instead. This method is kept for backward compatibility.
func (d *CommunicatorDependencies) SetInstanceInfo(uuid, name string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.instanceUUID = uuid
	d.instanceName = name
}

// GetInstanceUUID returns the stored instance UUID from backend authentication.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetInstanceUUID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceUUID
}

// GetInstanceName returns the stored instance name from backend authentication.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetInstanceName() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceName
}

// GetInstanceInfo returns both the stored instance UUID and name from backend authentication.
// This is a convenience method that returns both values in a single call.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetInstanceInfo() (uuid, name string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceUUID, d.instanceName
}

// SetAuthenticatedUUID stores the UUID returned from the backend after successful authentication.
// This is called by AuthenticateAction after successful authentication.
// The stored UUID is then exposed via CollectObservedState in ObservedState.AuthenticatedUUID.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) SetAuthenticatedUUID(uuid string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.instanceUUID = uuid
}

// GetAuthenticatedUUID returns the stored UUID from backend authentication.
// This is called by CollectObservedState to populate ObservedState.AuthenticatedUUID.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetAuthenticatedUUID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceUUID
}
