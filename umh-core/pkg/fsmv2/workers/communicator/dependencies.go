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
// Access is protected by mu for thread-safety.
type CommunicatorDependencies struct {
	*fsmv2.BaseDependencies
	transport transport.Transport

	// Mutex for thread-safe access to JWT, message, and error tracking storage
	mu sync.RWMutex

	// JWT token storage (set by AuthenticateAction, read by CollectObservedState)
	jwtToken  string
	jwtExpiry time.Time

	// Pulled message storage (set by SyncAction, read by CollectObservedState)
	pulledMessages []*transport.UMHMessage

	// Consecutive error counter (incremented by RecordError, reset by RecordSuccess)
	consecutiveErrors int
}

// NewCommunicatorDependencies creates a new dependencies for the communicator worker.
func NewCommunicatorDependencies(transport transport.Transport, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, identity fsmv2.Identity) *CommunicatorDependencies {
	return &CommunicatorDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, stateReader, identity),
		transport:        transport,
	}
}

// GetTransport returns the transport for HTTP communication.
func (d *CommunicatorDependencies) GetTransport() transport.Transport {
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
