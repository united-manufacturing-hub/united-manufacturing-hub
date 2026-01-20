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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
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
// Actions use deps.Metrics().IncrementCounter/SetGauge to record metrics.
// CollectObservedState calls Metrics().Drain() to merge into ObservedState.Metrics.
//
// Access is protected by mu for thread-safety.
//
// Field ordering: Fields are ordered by decreasing size to minimize struct padding.
// 24-byte fields first, then 16-byte, then 8-byte, then 1-byte (bools) at the end.
type CommunicatorDependencies struct {
	// 24-byte fields
	mu             sync.RWMutex            // Mutex for thread-safe access
	jwtExpiry      time.Time               // JWT expiry (set by AuthenticateAction)
	pulledMessages []*transport.UMHMessage // Pulled messages (set by SyncAction)

	// 16-byte fields
	transport    transport.Transport // HTTP transport for push/pull operations
	jwtToken     string              // JWT token (set by AuthenticateAction)
	instanceUUID string              // Instance UUID from backend (set by AuthenticateAction)
	instanceName string              // Instance name from backend (set by AuthenticateAction)

	// 8-byte fields
	*fsmv2.BaseDependencies
	metrics         *fsmv2.MetricsRecorder       // Standard metrics recorder for actions
	lastPullLatency time.Duration                // Per-tick pull latency
	lastPushLatency time.Duration                // Per-tick push latency
	inboundChan     chan<- *transport.UMHMessage // Write received messages to router
	outboundChan    <-chan *transport.UMHMessage // Read messages from router to push
	consecutiveErrors int                        // Error counter (incremented by RecordError)
	lastPullCount     int                        // Per-tick pull message count
	lastPushCount     int                        // Per-tick push message count

	// 1-byte fields (bools grouped at end to minimize padding)
	lastPullSuccess   bool // Per-tick pull success flag
	lastPushSuccess   bool // Per-tick push success flag
	syncTickCompleted bool // True if a sync tick ran this cycle
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
		metrics:          fsmv2.NewMetricsRecorder(),
		inboundChan:      inbound,
		outboundChan:     outbound,
	}
}

// Metrics returns the MetricsRecorder for actions to record metrics.
// Actions call IncrementCounter/SetGauge with typed constants.
// CollectObservedState calls Drain() to merge buffered metrics.
func (d *CommunicatorDependencies) Metrics() *fsmv2.MetricsRecorder {
	return d.metrics
}

// SetTransport sets the transport instance (mutex protected).
// Called by AuthenticateAction on first execution.
func (d *CommunicatorDependencies) SetTransport(t transport.Transport) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.transport = t
}

// GetTransport returns the transport (mutex protected).
// Returns nil if not yet created - callers MUST check for nil.
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
// This is called by actions after an operation fails.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordError() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.consecutiveErrors++
}

// RecordSuccess resets the consecutive error counter to 0.
// This is called by actions after an operation succeeds.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordSuccess() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.consecutiveErrors = 0
}

// GetConsecutiveErrors returns the current consecutive error count.
// This is called by CollectObservedState to populate the observed state.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetConsecutiveErrors() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.consecutiveErrors
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

// RecordPullSuccess records a successful pull operation with its latency and message count.
// Stores per-tick results; CollectObservedState accumulates into metrics.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordPullSuccess(latency time.Duration, msgCount int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastPullLatency = latency
	d.lastPullCount = msgCount
	d.lastPullSuccess = true
	d.syncTickCompleted = true
}

// RecordPullFailure records a failed pull operation with its latency.
// Stores per-tick results; CollectObservedState accumulates into metrics.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordPullFailure(latency time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastPullLatency = latency
	d.lastPullCount = 0
	d.lastPullSuccess = false
	d.syncTickCompleted = true
}

// RecordPushSuccess records a successful push operation with its latency and message count.
// Stores per-tick results; CollectObservedState accumulates into metrics.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordPushSuccess(latency time.Duration, msgCount int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastPushLatency = latency
	d.lastPushCount = msgCount
	d.lastPushSuccess = true
}

// RecordPushFailure records a failed push operation with its latency.
// Stores per-tick results; CollectObservedState accumulates into metrics.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) RecordPushFailure(latency time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastPushLatency = latency
	d.lastPushCount = 0
	d.lastPushSuccess = false
}

// SyncTickResult contains the results of a single pull or push operation.
type SyncTickResult struct {
	Latency time.Duration
	Count   int
	Success bool
}

// GetLastSyncResults returns the per-tick sync results from the most recent sync operation.
// Returns pull result, push result, and whether a sync tick completed this cycle.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) GetLastSyncResults() (pull SyncTickResult, push SyncTickResult, completed bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	pull = SyncTickResult{
		Latency: d.lastPullLatency,
		Count:   d.lastPullCount,
		Success: d.lastPullSuccess,
	}

	push = SyncTickResult{
		Latency: d.lastPushLatency,
		Count:   d.lastPushCount,
		Success: d.lastPushSuccess,
	}

	completed = d.syncTickCompleted

	return
}

// ClearSyncResults clears the per-tick sync results after CollectObservedState has accumulated them.
// Thread-safe: uses mutex for concurrent access protection.
func (d *CommunicatorDependencies) ClearSyncResults() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastPullLatency = 0
	d.lastPullCount = 0
	d.lastPullSuccess = false
	d.lastPushLatency = 0
	d.lastPushCount = 0
	d.lastPushSuccess = false
	d.syncTickCompleted = false
}

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
