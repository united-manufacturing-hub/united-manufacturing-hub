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
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

const maxPendingMessages = 1000

var _ snapshot.PushDependencies = (*PushDependencies)(nil)

// PushDependencies holds runtime state for the push worker, including a pending-message
// buffer. It delegates transport, token, and error tracking to the parent
// TransportDependencies.
type PushDependencies struct {
	*deps.BaseDependencies
	parentDeps *transport_pkg.TransportDependencies
	// TODO(ENG-5018): failureRate and the embedded RetryTracker (from BaseDependencies)
	// are per-child health counters that live in deps, not in observed status. The store
	// does not cover them, so both reset to zero when this child is re-created or the
	// process restarts, even once a durable (SQLite) store backend lands. To preserve
	// counters across re-creates, model them into status.
	failureRate     *failurerate.Tracker
	lastErrorDetail string
	// TODO(ENG-5018): pendingMessages holds messages destructively drained from the
	// outbound channel but not yet delivered. It lives in deps, not in observed status,
	// so it is not covered by the store: up to maxPendingMessages messages are discarded
	// on child re-create or process restart, even once a durable (SQLite) store backend
	// lands. To survive re-creates, back this buffer with the durable queue.
	pendingMessages         []*types.UMHMessage
	lastSeenResetGeneration uint64
	lastErrorType           types.ErrorType
	lastStatusCode          int
	errorMu                 sync.RWMutex
	pendingMu               sync.RWMutex
}

// NewPushDependencies creates a PushDependencies backed by the given parent transport dependencies.
// bd is the shared BaseDependencies returned by WorkerBase.InitBase.
func NewPushDependencies(parentDeps *transport_pkg.TransportDependencies, bd *deps.BaseDependencies) (*PushDependencies, error) {
	if parentDeps == nil {
		return nil, errors.New("parentDeps must not be nil")
	}

	return &PushDependencies{
		BaseDependencies: bd,
		parentDeps:       parentDeps,
		failureRate:      failurerate.New(transport_pkg.ChildFailureRateConfig),
	}, nil
}

// GetOutboundChan returns the parent's outbound message channel for read access
// by the push action.
func (d *PushDependencies) GetOutboundChan() <-chan *types.UMHMessage {
	return d.parentDeps.GetOutboundChan()
}

// GetTransport returns the parent's transport implementation.
func (d *PushDependencies) GetTransport() types.Transport {
	return d.parentDeps.GetTransport()
}

// RecordTypedError records a typed error for this child, propagates it to the parent
// transport tracker, and emits a Sentry warning when the failure rate escalates.
// statusCode and errorDetail carry the HTTP status code and sanitized body from the
// failed operation so they are available on the persistent_push_failure event.
func (d *PushDependencies) RecordTypedError(errType types.ErrorType, retryAfter time.Duration, statusCode int, errorDetail string) {
	d.errorMu.Lock()
	d.lastErrorType = errType
	d.lastStatusCode = statusCode
	d.lastErrorDetail = errorDetail
	d.errorMu.Unlock()

	d.RetryTracker().RecordError(retry.WithClass(errType.String()), retry.WithRetryAfter(retryAfter))
	d.parentDeps.RecordTypedError(errType, retryAfter)

	if d.failureRate.RecordOutcome(false) {
		fields := []deps.Field{
			deps.String("error_type", errType.String()),
			deps.Float64("failure_rate", d.failureRate.FailureRate()),
		}
		if statusCode > 0 {
			fields = append(fields, deps.Int("status_code", statusCode))
		}
		if errorDetail != "" {
			fields = append(fields, deps.String("error_detail", errorDetail))
		}
		d.BaseDependencies.GetLogger().SentryWarn(deps.FeatureForWorker(d.GetWorkerType()), d.GetHierarchyPath(), "persistent_push_failure",
			fields...)
	}
}

// RecordSuccess resets the child's error state. It intentionally does NOT
// propagate to the parent tracker. The parent tracker is only reset by auth
// success (authenticate.go). This prevents a push success from masking pull
// errors (or vice versa) on the shared parent counter.
func (d *PushDependencies) RecordSuccess() {
	d.errorMu.Lock()
	d.lastErrorType = 0
	d.lastStatusCode = 0
	d.lastErrorDetail = ""
	d.errorMu.Unlock()

	d.RetryTracker().RecordSuccess()
	d.failureRate.RecordOutcome(true)
}

// RecordError records an unclassified error for this child, propagates it to the
// parent transport tracker, and emits a Sentry warning when the failure rate escalates.
func (d *PushDependencies) RecordError() {
	d.RetryTracker().RecordError()
	d.parentDeps.RecordError()

	if d.failureRate.RecordOutcome(false) {
		d.BaseDependencies.GetLogger().SentryWarn(deps.FeatureForWorker(d.GetWorkerType()), d.GetHierarchyPath(), "persistent_push_failure",
			deps.Float64("failure_rate", d.failureRate.FailureRate()))
	}
}

// GetConsecutiveErrors returns the number of consecutive errors recorded by the
// child's retry tracker.
func (d *PushDependencies) GetConsecutiveErrors() int {
	return d.RetryTracker().ConsecutiveErrors()
}

// GetLastErrorType returns the most recent error type recorded for this child.
func (d *PushDependencies) GetLastErrorType() types.ErrorType {
	d.errorMu.RLock()
	defer d.errorMu.RUnlock()

	return d.lastErrorType
}

// GetLastStatusCode returns the HTTP status code from the most recent error
// recorded for this child, or 0 if none has been recorded.
//
// TODO(R7): surface onto PushStatus in CollectObservedState so this reaches
// observed status / durable store.
func (d *PushDependencies) GetLastStatusCode() int {
	d.errorMu.RLock()
	defer d.errorMu.RUnlock()

	return d.lastStatusCode
}

// GetLastErrorDetail returns the sanitized detail string from the most recent
// error recorded for this child, or "" if none has been recorded.
//
// TODO(R7): surface onto PushStatus in CollectObservedState so this reaches
// observed status / durable store.
func (d *PushDependencies) GetLastErrorDetail() string {
	d.errorMu.RLock()
	defer d.errorMu.RUnlock()

	return d.lastErrorDetail
}

// StorePendingMessages appends messages to the pending buffer for retry on the next tick.
// Nil messages are filtered out. If the buffer exceeds maxPendingMessages, the oldest
// messages are dropped.
func (d *PushDependencies) StorePendingMessages(msgs []*types.UMHMessage) {
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
		d.BaseDependencies.GetLogger().SentryWarn(deps.FeatureForWorker(d.GetWorkerType()), d.GetHierarchyPath(), "pending_buffer_overflow",
			deps.Int("dropped", dropped), deps.Int("cap", maxPendingMessages))
		d.MetricsRecorder().IncrementCounter(deps.CounterMessagesDropped, int64(dropped))
	}
}

// DrainPendingMessages returns all pending messages and clears the buffer.
func (d *PushDependencies) DrainPendingMessages() []*types.UMHMessage {
	d.pendingMu.Lock()
	defer d.pendingMu.Unlock()

	msgs := d.pendingMessages
	d.pendingMessages = nil

	return msgs
}

// PendingMessageCount returns the number of messages waiting for delivery.
func (d *PushDependencies) PendingMessageCount() int {
	d.pendingMu.RLock()
	defer d.pendingMu.RUnlock()

	return len(d.pendingMessages)
}

// GetLastRetryAfter returns the retry-after duration from the most recent error.
func (d *PushDependencies) GetLastRetryAfter() time.Duration {
	return d.RetryTracker().LastError().RetryAfter
}

// GetDegradedEnteredAt returns the timestamp at which the retry tracker entered
// the degraded state, or the zero time if the child is not currently degraded.
func (d *PushDependencies) GetDegradedEnteredAt() time.Time {
	degradedSince, _ := d.RetryTracker().DegradedSince()

	return degradedSince
}

// GetLastErrorAt returns the timestamp of the most recent error.
func (d *PushDependencies) GetLastErrorAt() time.Time {
	return d.RetryTracker().LastError().OccurredAt
}

// GetResetGeneration returns the parent's current reset-generation counter.
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
