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

package push

import (
	"errors"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry/failurerate"
	communicator_transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
)

const maxPendingMessages = 1000

var _ snapshot.PushDependencies = (*PushDependencies)(nil)

type PushDependencies struct {
	*deps.BaseDependencies
	parentDeps              *transport_pkg.TransportDependencies
	failureRate             *failurerate.Tracker
	pendingMessages         []*communicator_transport.UMHMessage
	errorMu                 sync.RWMutex
	pendingMu               sync.RWMutex
	lastSeenResetGeneration uint64
	lastErrorType           httpTransport.ErrorType
}

func NewPushDependencies(parentDeps *transport_pkg.TransportDependencies, identity deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader) (*PushDependencies, error) {
	if parentDeps == nil {
		return nil, errors.New("parentDeps must not be nil")
	}

	return &PushDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		parentDeps:       parentDeps,
		failureRate: failurerate.New(transport_pkg.ChildFailureRateConfig),
	}, nil
}

func (d *PushDependencies) GetOutboundChan() <-chan *communicator_transport.UMHMessage {
	return d.parentDeps.GetOutboundChan()
}

func (d *PushDependencies) GetTransport() communicator_transport.Transport {
	return d.parentDeps.GetTransport()
}

func (d *PushDependencies) GetJWTToken() string {
	return d.parentDeps.GetJWTToken()
}

func (d *PushDependencies) GetAuthenticatedUUID() string {
	return d.parentDeps.GetAuthenticatedUUID()
}

func (d *PushDependencies) RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration) {
	d.errorMu.Lock()
	d.lastErrorType = errType
	d.errorMu.Unlock()

	d.RetryTracker().RecordError(retry.WithClass(errType.String()), retry.WithRetryAfter(retryAfter))
	d.parentDeps.RecordTypedError(errType, retryAfter)

	if d.failureRate.RecordOutcome(false) {
		d.BaseDependencies.GetLogger().SentryWarn(deps.FeatureCommunicator, d.GetHierarchyPath(), "persistent_push_failure",
			deps.String("error_type", errType.String()),
			deps.Float64("failure_rate", d.failureRate.FailureRate()))
	}
}

// RecordSuccess resets the child's error state. It intentionally does NOT
// propagate to the parent tracker. The parent tracker is only reset by auth
// success (authenticate.go). This prevents a push success from masking pull
// errors (or vice versa) on the shared parent counter.
func (d *PushDependencies) RecordSuccess() {
	d.errorMu.Lock()
	d.lastErrorType = 0
	d.errorMu.Unlock()

	d.RetryTracker().RecordSuccess()
	d.failureRate.RecordOutcome(true)
}

func (d *PushDependencies) RecordError() {
	d.RetryTracker().RecordError()
	d.parentDeps.RecordError()
	d.failureRate.RecordOutcome(false)
}

func (d *PushDependencies) GetConsecutiveErrors() int {
	return d.RetryTracker().ConsecutiveErrors()
}

func (d *PushDependencies) GetLastErrorType() httpTransport.ErrorType {
	d.errorMu.RLock()
	defer d.errorMu.RUnlock()
	return d.lastErrorType
}

func (d *PushDependencies) StorePendingMessages(msgs []*communicator_transport.UMHMessage) {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	for _, msg := range msgs {
		if msg != nil {
			d.pendingMessages = append(d.pendingMessages, msg)
		}
	}
	if len(d.pendingMessages) > maxPendingMessages {
		dropped := len(d.pendingMessages) - maxPendingMessages
		d.pendingMessages = d.pendingMessages[len(d.pendingMessages)-maxPendingMessages:]
		d.BaseDependencies.GetLogger().SentryWarn(deps.FeatureCommunicator, d.GetHierarchyPath(), "pending_buffer_overflow",
			deps.Int("dropped", dropped), deps.Int("cap", maxPendingMessages))
		d.MetricsRecorder().IncrementCounter(deps.CounterMessagesDropped, int64(dropped))
	}
}

func (d *PushDependencies) DrainPendingMessages() []*communicator_transport.UMHMessage {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	msgs := d.pendingMessages
	d.pendingMessages = nil

	return msgs
}

func (d *PushDependencies) PendingMessageCount() int {
	d.pendingMu.RLock()
	defer d.pendingMu.RUnlock()

	return len(d.pendingMessages)
}

func (d *PushDependencies) IsTokenValid() bool {
	token := d.parentDeps.GetJWTToken()
	if token == "" {
		return false
	}

	expiry := d.parentDeps.GetJWTExpiry()
	if expiry.IsZero() {
		return false
	}

	const safetyBuffer = 1 * time.Minute

	return !time.Now().Add(safetyBuffer).After(expiry)
}

func (d *PushDependencies) GetLastRetryAfter() time.Duration {
	return d.RetryTracker().LastError().RetryAfter
}

func (d *PushDependencies) GetDegradedEnteredAt() time.Time {
	degradedSince, _ := d.RetryTracker().DegradedSince()
	return degradedSince
}

func (d *PushDependencies) GetLastErrorAt() time.Time {
	return d.RetryTracker().LastError().OccurredAt
}

func (d *PushDependencies) GetResetGeneration() uint64 {
	return d.parentDeps.GetResetGeneration()
}

// CheckAndClearOnReset checks if parent has done a transport reset.
// If resetGeneration changed, clears all pending messages, resets the failure
// rate tracker, and returns true.
func (d *PushDependencies) CheckAndClearOnReset() bool {
	currentGen := d.parentDeps.GetResetGeneration()

	d.pendingMu.Lock()

	changed := currentGen != d.lastSeenResetGeneration
	if changed {
		d.pendingMessages = nil
		d.lastSeenResetGeneration = currentGen
	}

	d.pendingMu.Unlock()

	if changed {
		d.failureRate.Reset()
	}

	return changed
}

// IsPersistentFailureEscalated reports whether the failure rate meets or exceeds
// the escalation threshold over the rolling window.
func (d *PushDependencies) IsPersistentFailureEscalated() bool {
	return d.failureRate.IsEscalated()
}

// SetPersistentFailureEscalatedForTest sets the escalation flag directly for tests.
func (d *PushDependencies) SetPersistentFailureEscalatedForTest(v bool) {
	d.failureRate.SetEscalatedForTest(v)
}
