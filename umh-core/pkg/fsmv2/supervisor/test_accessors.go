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

package supervisor

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// TestMarkAsStarted sets the supervisor as started with a valid context and
// starts each worker's observation collector and action executor. Starting the
// collectors is what lets the worker re-observe its world every tick (e.g. a
// shared registry losing a ref), so a manually-ticked supervisor reaches the
// same steady state the production tick loop does.
//
// Call-once-only, and mutually exclusive with Start()/StartAsChild() on the
// same supervisor: all three call Collector.Start via startWorkerRunners, and
// Collector.Start panics on a second invocation (Invariant I8). A test that
// double-marks, or marks then starts, panics at runtime.
//
// Unlike Start()/StartAsChild(), this omits actionExecutor.Start and
// startMetricsReporter. The supervisor-level action executor and metrics
// reporter are not needed by a manually-ticked test, which drives reconcile
// directly; per-worker collectors and executors are the load-bearing parts.
//
// DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestMarkAsStarted() {
	ctx, cancel := context.WithCancel(context.Background())

	s.ctxMu.Lock()
	s.ctx = ctx
	s.ctxCancel = cancel
	supervisorCtx := s.ctx
	s.ctxMu.Unlock()

	s.started.Store(true)

	s.startWorkerRunners(supervisorCtx)
}

// TestTick exposes tick() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestTick(ctx context.Context) error {
	return s.tick(ctx)
}

// TestRequestShutdown exposes requestShutdown() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestRequestShutdown(ctx context.Context, workerID string, reason string) error {
	return s.requestShutdown(ctx, workerID, reason)
}

// TestGetRestartCount returns collectorHealth.restartCount for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestGetRestartCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.collectorHealth.restartCount
}

// TestSetRestartCount sets collectorHealth.restartCount for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetRestartCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.collectorHealth.restartCount = count
}

// TestTickAll exposes tickAll() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestTickAll(ctx context.Context) error {
	return s.tickAll(ctx)
}

// TestUpdateUserSpec exposes updateUserSpec() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestUpdateUserSpec(spec config.UserSpec) {
	s.updateUserSpec(spec)
}

// TestSetPendingRestart marks a worker as pending restart for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetPendingRestart(workerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingRestart[workerID] = true
}

// TestSetRestartRequestedAt sets the restart requested timestamp for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetRestartRequestedAt(workerID string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.restartRequestedAt[workerID] = t
}

// TestIsPendingRestart checks if worker is in pendingRestart map. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestIsPendingRestart(workerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.pendingRestart[workerID]
}

// TestGetUserSpec returns the current userSpec for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestGetUserSpec() config.UserSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.userSpec
}

// TestRestartCollector exposes restartCollector() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestRestartCollector(ctx context.Context, workerID string) error {
	return s.restartCollector(ctx, workerID)
}

// TestSetLastRestart sets collectorHealth.lastRestart for testing. DO NOT USE in production code.
// This allows tests to simulate backoff time having elapsed.
func (s *Supervisor[TObserved, TDesired]) TestSetLastRestart(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.collectorHealth.lastRestart = t
}

// TestIsPanicCircuitOpen returns true if the panic circuit breaker is open. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestIsPanicCircuitOpen() bool {
	return s.panicCircuitOpen.Load()
}

// TestSetCircuitOpen sets the infrastructure circuit breaker state for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetCircuitOpen(open bool) {
	s.circuitOpen.Store(open)
}

// TestPanicRecoveryTracker wraps panicRecovery for unit testing. DO NOT USE in production code.
type TestPanicRecoveryTracker struct {
	pr *panicRecovery
}

// NewTestPanicRecoveryTracker creates a panicRecovery tracker for testing. DO NOT USE in production code.
func NewTestPanicRecoveryTracker(window time.Duration, maxPanics int) *TestPanicRecoveryTracker {
	return &TestPanicRecoveryTracker{pr: newPanicRecovery(window, maxPanics)}
}

// RecordPanic records a panic and returns true if the escalation threshold has been reached.
func (t *TestPanicRecoveryTracker) RecordPanic() bool {
	return t.pr.RecordPanic()
}

// PanicCount returns the number of panics within the window.
func (t *TestPanicRecoveryTracker) PanicCount() int {
	return t.pr.PanicCount()
}

// Reset clears all recorded panics.
func (t *TestPanicRecoveryTracker) Reset() {
	t.pr.Reset()
}

// TestIsPendingRemoval checks if a child is in the pendingRemoval map. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestIsPendingRemoval(childName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.pendingRemoval[childName]
}

// TestSetPendingRemovalFlag marks a child as pending removal for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetPendingRemovalFlag(childName string, value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if value {
		s.pendingRemoval[childName] = true
	} else {
		delete(s.pendingRemoval, childName)
	}
}

// TestSetStarted sets the started flag for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSetStarted(value bool) {
	s.started.Store(value)
}

// TestApplyDisableMapping exposes applyDisableMapping() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestApplyDisableMapping(ctx context.Context, specs []config.ChildSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.applyDisableMapping(ctx, specs)
}

// TestWorkerCount returns the number of workers currently registered. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestWorkerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.workers)
}

// TestIsInfraCircuitOpen returns true if the infrastructure circuit breaker is open. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestIsInfraCircuitOpen() bool {
	return s.circuitOpen.Load()
}

// TestLinkChild inserts a child supervisor into the children map for testing,
// modeling a nested tree without going through the spawn path. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestLinkChild(name string, child SupervisorInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.children[name] = child
}

// TestCalculateSubtreeHeight exposes calculateSubtreeHeight() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestCalculateSubtreeHeight() int {
	return s.calculateSubtreeHeight()
}

// TestRecordPostJoinBudgetOverrun exposes recordPostJoinBudgetOverrun() for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestRecordPostJoinBudgetOverrun(totalDrainElapsed, drainBudget, childDrainElapsed time.Duration, budgetWarned bool) {
	s.recordPostJoinBudgetOverrun(totalDrainElapsed, drainBudget, childDrainElapsed, budgetWarned)
}

// TestSaveDesired writes a desired-state document through the supervisor's store for testing. DO NOT USE in production code.
func (s *Supervisor[TObserved, TDesired]) TestSaveDesired(ctx context.Context, workerType, workerID string, doc persistence.Document) error {
	_, err := s.store.SaveDesired(ctx, workerType, workerID, doc)

	return err
}

// TestDrainTickInterval returns the drain ticker granularity for testing. DO NOT USE in production code.
func TestDrainTickInterval() time.Duration {
	return drainTickInterval
}
