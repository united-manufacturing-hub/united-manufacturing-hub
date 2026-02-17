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

package transport

import (
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry"
	communicator_transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// ChannelProvider interface and singleton functions are defined in channel_provider.go

// TransportDependencies provides transport and channel access for transport worker actions.
type TransportDependencies struct {
	jwtExpiry         time.Time
	lastAuthAttemptAt time.Time

	transport communicator_transport.Transport

	*deps.BaseDependencies
	inboundChan  chan<- *communicator_transport.UMHMessage
	outboundChan <-chan *communicator_transport.UMHMessage
	jwtToken     string
	instanceUUID string

	lastErrorType httpTransport.ErrorType

	mu sync.RWMutex
}

// NewTransportDependencies creates dependencies for the transport worker.
// Panics if SetChannelProvider was not called first.
func NewTransportDependencies(t communicator_transport.Transport, logger deps.FSMLogger, stateReader deps.StateReader, identity deps.Identity) *TransportDependencies {
	provider := GetChannelProvider()
	if provider == nil {
		panic(fmt.Sprintf("ChannelProvider must be set before creating dependencies (worker=%s). "+
			"Call SetChannelProvider() in main() before starting FSMv2 supervisor.",
			identity.ID))
	}

	inbound, outbound := provider.GetChannels(identity.ID)

	return &TransportDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		transport:        t,
		inboundChan:      inbound,
		outboundChan:     outbound,
	}
}

// SetTransport sets the transport instance.
func (d *TransportDependencies) SetTransport(t communicator_transport.Transport) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.transport = t
}

// GetTransport returns the transport. Nil only before AuthenticateAction runs.
func (d *TransportDependencies) GetTransport() communicator_transport.Transport {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.transport
}

// SetJWT stores the JWT token and expiry from authentication response.
func (d *TransportDependencies) SetJWT(token string, expiry time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.jwtToken = token
	d.jwtExpiry = expiry
}

// GetJWTToken returns the stored JWT token.
func (d *TransportDependencies) GetJWTToken() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.jwtToken
}

// GetJWTExpiry returns the stored JWT expiry time.
func (d *TransportDependencies) GetJWTExpiry() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.jwtExpiry
}

// RecordError increments consecutive errors and records when degraded mode started.
// Error tracking is used by the supervisor for retry backoff policy.
func (d *TransportDependencies) RecordError() {
	d.RetryTracker().RecordError()
}

// RecordSuccess resets all error tracking state.
func (d *TransportDependencies) RecordSuccess() {
	d.mu.Lock()
	d.lastErrorType = 0
	d.lastAuthAttemptAt = time.Time{}
	d.mu.Unlock()

	d.RetryTracker().RecordSuccess()
}

// GetConsecutiveErrors returns the current consecutive error count.
// Delegates to RetryTracker for single source of truth.
func (d *TransportDependencies) GetConsecutiveErrors() int {
	return d.RetryTracker().ConsecutiveErrors()
}

// GetDegradedEnteredAt returns when degraded mode started, or zero if not degraded.
func (d *TransportDependencies) GetDegradedEnteredAt() time.Time {
	degradedSince, _ := d.RetryTracker().DegradedSince()

	return degradedSince
}

// GetInboundChan returns channel to write received messages, or nil if no provider set.
func (d *TransportDependencies) GetInboundChan() chan<- *communicator_transport.UMHMessage {
	return d.inboundChan
}

// GetOutboundChan returns channel to read messages for pushing, or nil if no provider set.
func (d *TransportDependencies) GetOutboundChan() <-chan *communicator_transport.UMHMessage {
	return d.outboundChan
}

// GetInboundChanStats returns the capacity and current length of the inbound channel.
// Returns (0, 0) if no channel provider is set.
func (d *TransportDependencies) GetInboundChanStats() (capacity int, length int) {
	provider := GetChannelProvider()
	if provider == nil {
		d.GetLogger().SentryWarn(deps.FeatureCommunicator, d.GetHierarchyPath(), "channel_provider_not_initialized",
			deps.WorkerID(d.GetWorkerID()))

		return 0, 0
	}

	return provider.GetInboundStats(d.GetWorkerID())
}

// RecordTypedError increments consecutive errors and records error type and retry-after.
// Error tracking is used by the supervisor for retry backoff policy.
func (d *TransportDependencies) RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration) {
	d.mu.Lock()
	d.lastErrorType = errType
	d.mu.Unlock()

	d.RetryTracker().RecordError(retry.WithClass(errType.String()), retry.WithRetryAfter(retryAfter))
}

// GetLastErrorType returns the last recorded error type.
func (d *TransportDependencies) GetLastErrorType() httpTransport.ErrorType {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastErrorType
}

// GetLastRetryAfter returns the Retry-After duration from the last error.
func (d *TransportDependencies) GetLastRetryAfter() time.Duration {
	return d.RetryTracker().LastError().RetryAfter
}

// SetLastAuthAttemptAt records the timestamp of the last authentication attempt.
func (d *TransportDependencies) SetLastAuthAttemptAt(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastAuthAttemptAt = t
}

// GetLastAuthAttemptAt returns the timestamp of the last authentication attempt.
func (d *TransportDependencies) GetLastAuthAttemptAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastAuthAttemptAt
}

// SetAuthenticatedUUID stores the UUID returned from the backend after authentication.
func (d *TransportDependencies) SetAuthenticatedUUID(uuid string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.instanceUUID = uuid
}

// GetAuthenticatedUUID returns the stored UUID from backend authentication.
func (d *TransportDependencies) GetAuthenticatedUUID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.instanceUUID
}
