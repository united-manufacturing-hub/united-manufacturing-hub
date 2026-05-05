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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry/failurerate"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// ChildFailureRateConfig is the shared failurerate.Config for push and pull
// child workers. WindowSize=600 at the 1-second production tick rate covers
// roughly 10 minutes. Threshold=0.9 triggers escalation at 90% failure rate.
// MinSamples=100 suppresses spurious alerts during startup.
var ChildFailureRateConfig = failurerate.Config{
	WindowSize: 600,
	Threshold:  0.9,
	MinSamples: 100,
}

// AuthFailureRateConfig controls when the tracker fires a one-shot
// SentryWarn("persistent_auth_failure") during auth failure episodes.
//
// Because auth uses Reset() on success (not RecordOutcome(true)), the
// window only ever contains failures during an episode. The failure rate
// is always 100% during failures and 0% after reset. Threshold is
// therefore irrelevant -- MinSamples alone controls escalation timing.
//
// The tracker escalation only matters for sustained transient errors (Network,
// ServerError, etc.) where the state machine retries indefinitely. For persistent
// errors (InvalidToken, InstanceDeleted), AuthFailedState stops retries after 1
// failure, so the tracker never reaches MinSamples.
// MinSamples=5 at Network's exponential backoff (2-60s) = ~10-30s before escalation.
var AuthFailureRateConfig = failurerate.Config{
	WindowSize: 30,
	Threshold:  0.9,
	MinSamples: 5,
}

// ChannelProvider interface and singleton functions are defined in channel_provider.go

// TransportDependencies provides transport and channel access for transport worker actions.
type TransportDependencies struct {
	jwtExpiry         time.Time
	lastAuthAttemptAt time.Time

	transport types.Transport

	*deps.BaseDependencies
	authFailureRate *failurerate.Tracker
	inboundChan     chan<- *types.UMHMessage
	outboundChan    <-chan *types.UMHMessage
	jwtToken        string
	instanceUUID    string

	failedAuthToken    string
	failedRelayURL     string
	failedInstanceUUID string

	lastErrorType            types.ErrorType
	persistentAuthErrorCount int

	resetGeneration uint64

	mu sync.RWMutex
}

// NewTransportDependencies creates dependencies for the transport worker.
// Panics if SetChannelProvider was not called first.
func NewTransportDependencies(t types.Transport, logger deps.FSMLogger, stateReader deps.StateReader, identity deps.Identity) *TransportDependencies {
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
		authFailureRate:  failurerate.New(AuthFailureRateConfig),
		inboundChan:      inbound,
		outboundChan:     outbound,
	}
}

// SetTransport sets the transport instance.
func (d *TransportDependencies) SetTransport(t types.Transport) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.transport = t
}

// GetTransport returns the transport. Nil only before AuthenticateAction runs.
func (d *TransportDependencies) GetTransport() types.Transport {
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

// RecordAuthError records a typed auth error and feeds the auth failure rate
// tracker. If the tracker's one-shot fires (sustained failures crossing the
// MinSamples threshold), a SentryWarn("persistent_auth_failure") is emitted.
func (d *TransportDependencies) RecordAuthError(errType types.ErrorType, retryAfter time.Duration) {
	d.RecordTypedError(errType, retryAfter)

	if !errType.IsTransient() {
		d.mu.Lock()
		d.persistentAuthErrorCount++
		d.mu.Unlock()
	}

	if d.authFailureRate.RecordOutcome(false) {
		d.BaseDependencies.GetLogger().SentryWarn(deps.FeatureForWorker(d.GetWorkerType()), d.GetHierarchyPath(), "persistent_auth_failure",
			deps.String("error_type", errType.String()),
			deps.Float64("failure_rate", d.authFailureRate.FailureRate()))
	}
}

// RecordSuccess resets all error tracking state including the failed auth config.
// The failed auth config fields are cleared inline (not via SetFailedAuthConfig) to
// avoid a mutex deadlock — this method already holds d.mu.
func (d *TransportDependencies) RecordSuccess() {
	d.mu.Lock()
	d.lastErrorType = types.ErrorTypeUnknown
	d.lastAuthAttemptAt = time.Time{}
	d.persistentAuthErrorCount = 0
	d.failedAuthToken = ""
	d.failedRelayURL = ""
	d.failedInstanceUUID = ""
	d.mu.Unlock()

	d.RetryTracker().RecordSuccess()
	d.authFailureRate.Reset()
}

// SetFailedAuthConfig stores the auth config that caused a permanent auth failure.
// Called by AuthenticateAction when InvalidToken or InstanceDeleted is returned.
func (d *TransportDependencies) SetFailedAuthConfig(token, relayURL, uuid string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.failedAuthToken = token
	d.failedRelayURL = relayURL
	d.failedInstanceUUID = uuid
}

// GetFailedAuthConfig returns the auth config from the last permanent auth failure.
// Returns empty strings if no permanent failure has been recorded (or after RecordSuccess).
func (d *TransportDependencies) GetFailedAuthConfig() (token, relayURL, uuid string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.failedAuthToken, d.failedRelayURL, d.failedInstanceUUID
}

// GetConsecutiveErrors returns the current consecutive error count.
// Delegates to RetryTracker for single source of truth.
func (d *TransportDependencies) GetConsecutiveErrors() int {
	return d.RetryTracker().ConsecutiveErrors()
}

// GetPersistentAuthErrorCount returns the number of non-transient auth errors
// (InvalidToken, InstanceDeleted, ProxyBlock, CloudflareChallenge, Unknown)
// since the last successful auth. Unlike GetConsecutiveErrors which uses the
// shared RetryTracker (contaminated by child push/pull errors and transient
// auth errors), this counter is only incremented in RecordAuthError for
// non-transient errors and reset in RecordSuccess.
func (d *TransportDependencies) GetPersistentAuthErrorCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.persistentAuthErrorCount
}

// GetDegradedEnteredAt returns when degraded mode started, or zero if not degraded.
func (d *TransportDependencies) GetDegradedEnteredAt() time.Time {
	degradedSince, _ := d.RetryTracker().DegradedSince()

	return degradedSince
}

// GetLastErrorAt returns when the last error occurred, or zero if no errors recorded.
func (d *TransportDependencies) GetLastErrorAt() time.Time {
	return d.RetryTracker().LastError().OccurredAt
}

// GetInboundChan returns channel to write received messages, or nil if no provider set.
func (d *TransportDependencies) GetInboundChan() chan<- *types.UMHMessage {
	return d.inboundChan
}

// GetOutboundChan returns channel to read messages for pushing, or nil if no provider set.
func (d *TransportDependencies) GetOutboundChan() <-chan *types.UMHMessage {
	return d.outboundChan
}

// GetInboundChanStats returns the capacity and current length of the inbound channel.
// Returns (0, 0) if no channel provider is set.
func (d *TransportDependencies) GetInboundChanStats() (capacity int, length int) {
	provider := GetChannelProvider()
	if provider == nil {
		d.GetLogger().SentryWarn(deps.FeatureForWorker(d.GetWorkerType()), d.GetHierarchyPath(), "channel_provider_not_initialized",
			deps.WorkerID(d.GetWorkerID()))

		return 0, 0
	}

	return provider.GetInboundStats(d.GetWorkerID())
}

// RecordTypedError increments consecutive errors and records error type and retry-after.
// Error tracking is used by the supervisor for retry backoff policy.
//
// Asymmetry note: child workers propagate errors UP to this tracker via RecordTypedError
// so the parent sees all child failures and can trigger transport reset decisions. However,
// child successes do NOT propagate here -- only successful auth resets the parent tracker.
// This means the parent error count grows monotonically from child errors until re-auth.
func (d *TransportDependencies) RecordTypedError(errType types.ErrorType, retryAfter time.Duration) {
	d.mu.Lock()
	d.lastErrorType = errType
	d.mu.Unlock()

	d.RetryTracker().RecordError(retry.WithClass(errType.String()), retry.WithRetryAfter(retryAfter))
}

// GetLastErrorType returns the last recorded error type.
func (d *TransportDependencies) GetLastErrorType() types.ErrorType {
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

// GetResetGeneration returns the current reset generation counter.
// Children compare this to detect parent transport resets and clear pending buffers.
func (d *TransportDependencies) GetResetGeneration() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.resetGeneration
}

// IncrementResetGeneration bumps the reset generation counter.
// Called by ResetTransportAction after resetting the HTTP transport.
func (d *TransportDependencies) IncrementResetGeneration() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.resetGeneration++
}
