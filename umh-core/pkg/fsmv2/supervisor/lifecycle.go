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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// Start starts the supervisor goroutines.
// Returns a channel that will be closed when the supervisor stops.
func (s *Supervisor[TObserved, TDesired]) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})

	// Create a child context that we can cancel via Shutdown().
	s.ctxMu.Lock()
	s.ctx, s.ctxCancel = context.WithCancel(ctx)
	supervisorCtx := s.ctx
	s.ctxMu.Unlock()
	s.started.Store(true)

	// Store done so Shutdown() can wait for tickLoop to exit in Phase 4.
	s.tickLoopDone = done

	s.logger.Debug("supervisor_started")

	s.actionExecutor.Start(supervisorCtx)

	s.startMetricsReporter(supervisorCtx)

	// Start observation collectors and action executors for all workers
	s.mu.RLock()

	for _, workerCtx := range s.workers {
		if err := workerCtx.collector.Start(supervisorCtx); err != nil {
			s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "collector_start_failed")
		}

		workerCtx.executor.Start(supervisorCtx)
	}

	s.mu.RUnlock()

	// Start main tick loop
	go func() {
		defer close(done)

		s.tickLoop(supervisorCtx)
	}()

	return done
}

// StartAsChild starts the supervisor without a tick loop.
// The parent supervisor will call tick() synchronously during its own tick.
// Collectors and executors still run in goroutines (they handle async I/O).
//
// This method is used by parent supervisors when spawning child supervisors
// to avoid double-ticking (where children would be ticked both by their own
// goroutine AND by the parent's synchronous tick() call).
func (s *Supervisor[TObserved, TDesired]) StartAsChild(ctx context.Context) {
	s.ctxMu.Lock()
	s.ctx, s.ctxCancel = context.WithCancel(ctx)
	supervisorCtx := s.ctx
	s.ctxMu.Unlock()
	s.started.Store(true)

	s.logger.Debug("supervisor_started_as_child")

	s.actionExecutor.Start(supervisorCtx)

	s.startMetricsReporter(supervisorCtx)

	s.mu.RLock()

	for _, workerCtx := range s.workers {
		if err := workerCtx.collector.Start(supervisorCtx); err != nil {
			s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "collector_start_failed")
		}

		workerCtx.executor.Start(supervisorCtx)
	}

	s.mu.RUnlock()
}

// Run starts the supervisor and blocks until ctx is canceled, the supervisor
// stops on its own, or Shutdown is called externally. When ctx is canceled,
// Run drives a graceful shutdown via Shutdown() before returning. Returns nil
// on a clean shutdown.
//
// Run is the recommended entry point for top-level supervisors (e.g.,
// cmd/main.go). For child supervisors, parents use StartAsChild instead.
//
// Composition:
//
//	done := s.Start(ctx)
//	<-ctx.Done()   // OR <-done if supervisor stops on its own first
//	s.Shutdown()
//	<-done         // Shutdown waits for tickLoop; this read is a safe no-op
func (s *Supervisor[TObserved, TDesired]) Run(ctx context.Context) error {
	done := s.Start(ctx)

	select {
	case <-ctx.Done():
		// External cancellation (e.g., SIGTERM via signal.NotifyContext).
		// Shutdown drives graceful drain before cancelling the supervisor context.
		s.Shutdown()
		// done is already closed when Shutdown returns; this read is a safe no-op.
		<-done

		return nil
	case <-done:
		// tickLoop exited on its own (e.g., panic-circuit open or external stop).
		// Call Shutdown to clean up executor + collectors (idempotent).
		s.Shutdown()

		return nil
	}
}

// tickLoop is the main FSM loop.
// Calls tick() which includes hierarchical composition (Phase 0) and worker state transitions.
func (s *Supervisor[TObserved, TDesired]) tickLoop(ctx context.Context) {
	s.logger.Debug("tick_loop_initializing")

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	s.logger.Debug("tick_loop_started",
		deps.Duration("interval", s.tickInterval))

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("tick_loop_stopped")

			return
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				if errors.Is(err, ErrPanicCircuitOpen) || errors.Is(err, ErrInfraCircuitOpen) {
					s.logger.Debug("tick_suppressed_circuit_open",
						deps.HierarchyPath(s.GetHierarchyPath()),
						deps.Err(err))
				} else {
					s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPath(), err, "tick_error")
				}
			}
		}
	}
}

// Shutdown gracefully shuts down this supervisor and all its workers.
//
// Phases:
//  1. Set accepting-work flag to false. Do NOT cancel context.
//  2. Cascade Shutdown() to children.
//  3. Drain: request worker shutdown, then wait for tickLoop to reap them
//     (via SignalNeedsRemoval). Exit on len(workers)==0 or gracefulTimeout.
//  4. Cancel context. Wait for tickLoop to exit. Stop executor + collectors.
//
// s.ctx remains valid until the shutdown sequence completes; workers and
// actions all share this context throughout.
//
// Idempotent.
func (s *Supervisor[TObserved, TDesired]) Shutdown() {
	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "shutdown_start"))

	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "mutex_lock_acquire"),
		deps.String("mutex_name", "supervisor.mu"),
		deps.String("lock_type", "write"))

	s.mu.Lock()

	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "mutex_lock_acquired"),
		deps.String("mutex_name", "supervisor.mu"))

	// Make idempotent - check if already shut down
	if !s.started.Load() {
		s.mu.Unlock()

		s.logTrace("lifecycle",
			deps.String("lifecycle_event", "shutdown_skip"),
			deps.Reason("already_shutdown"))

		s.logger.Debug("supervisor_already_shutdown")

		return
	}

	s.started.Store(false)

	s.logger.Info("supervisor_shutting_down")

	// Extract children before releasing lock to avoid deadlock with child Tick() goroutines.
	childrenToShutdown := make(map[string]SupervisorInterface, len(s.children))

	for name, child := range s.children {
		childrenToShutdown[name] = child
	}

	// Get worker IDs under lock
	workerIDs := make([]string, 0, len(s.workers))
	for workerID := range s.workers {
		workerIDs = append(workerIDs, workerID)
	}

	// Release lock before graceful shutdown operations
	s.mu.Unlock()

	if len(childrenToShutdown) > 0 {
		s.logger.Debug("graceful_shutdown_children_starting",
			deps.Int("child_count", len(childrenToShutdown)))

		for childName, child := range childrenToShutdown {
			s.logTrace("lifecycle",
				deps.String("lifecycle_event", "child_shutdown_start"),
				deps.String("child_name", childName),
				deps.String("parent_worker_type", s.workerType))

			s.logger.Debug("child_shutting_down",
				deps.String("child_name", childName))

			// child.Shutdown() is idempotent (lifecycle.go: see Shutdown's started-flag guard)
			// and synchronous — bounded by the child's own gracefulShutdownTimeout. Calls
			// from this cascade and from later tick-driven reconcileChildren(nil) paths
			// converge on the same early-return.
			child.Shutdown()

			s.logTrace("lifecycle",
				deps.String("lifecycle_event", "child_shutdown_complete"),
				deps.String("child_name", childName),
				deps.String("parent_worker_type", s.workerType))
		}

		s.logger.Debug("graceful_shutdown_children_complete")
	}

	if len(workerIDs) > 0 {
		s.logger.Debug("graceful_shutdown_workers_starting",
			deps.Int("worker_count", len(workerIDs)))

		// Capture s.ctx once under ctxMu. The invariant on ctxMu (see Supervisor
		// definition) covers cancel-vs-use races in getStartedContext; this read
		// is on the same goroutine that will later cancel ctx in Phase 4, but
		// locking here keeps the access uniformly disciplined.
		s.ctxMu.RLock()
		shutdownCtx := s.ctx
		s.ctxMu.RUnlock()

		// Request graceful shutdown on all workers
		for _, workerID := range workerIDs {
			if err := s.requestShutdown(shutdownCtx, workerID, "supervisor_shutdown"); err != nil {
				s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPath(), "graceful_shutdown_request_failed",
					deps.Err(err))
			}
		}

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		gracefulTimer := time.NewTimer(s.gracefulShutdownTimeout)
		defer gracefulTimer.Stop()

		// Phase 3 drain loop intentionally does NOT watch s.ctx.Done() — Phase 4
		// below is what cancels s.ctx; escaping here would short-circuit the drain.
		// The forceExit arm gives operators an explicit fast-exit: cmd/main.go
		// closes the channel on a second SIGTERM (Go's signal.NotifyContext is
		// single-shot and silently drops repeats). The channel is shared across
		// all supervisor levels via Config propagation, so closing it breaks the
		// drain at every level simultaneously. A nil forceExit blocks that case
		// forever (Go semantics for nil-channel receive), preserving the previous
		// "drain to gracefulShutdownTimeout" behaviour for callers that don't opt in.
	drainLoop:
		for {
			select {
			case <-gracefulTimer.C:
				s.mu.RLock()
				remainingCount := len(s.workers)
				s.mu.RUnlock()

				s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPath(), "graceful_shutdown_timeout",
					deps.Duration("timeout", s.gracefulShutdownTimeout),
					deps.Int("remaining_worker_count", remainingCount))

				break drainLoop
			case <-ticker.C:
				s.mu.RLock()
				remaining := len(s.workers)
				s.mu.RUnlock()

				if remaining == 0 {
					s.logger.Debug("graceful_shutdown_workers_removed")

					break drainLoop
				}
			case <-s.forceExit:
				s.mu.RLock()
				remainingCount := len(s.workers)
				s.mu.RUnlock()

				s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPath(), "graceful_shutdown_force_exit",
					deps.Int("remaining_worker_count", remainingCount))

				break drainLoop
			}
		}
	}

	// Phase 4: Terminate context, wait for tickLoop, stop executor + collectors.
	s.ctxMu.Lock()

	if s.ctxCancel != nil {
		s.ctxCancel()
		s.ctxCancel = nil // Prevent double-cancel
	}

	s.ctxMu.Unlock()

	// Wait for tickLoop to exit (it exits on ctx.Done()).
	// tickLoopDone is nil for StartAsChild supervisors (no owned tickLoop).
	if s.tickLoopDone != nil {
		<-s.tickLoopDone
	}

	// Wait for metrics reporter to finish (it exits on ctx.Done())
	s.metricsWg.Wait()

	// Re-acquire lock for cleanup
	s.mu.Lock()

	// Shutdown action executor (doesn't block)
	s.actionExecutor.Shutdown()

	// Shutdown remaining per-worker executors and collectors.
	shutdownCtx := context.Background()

	for _, workerCtx := range s.workers {
		s.logger.Debug("worker_shutting_down",
			deps.HierarchyPath(workerCtx.identity.HierarchyPath))

		// Stop the collector's observation loop
		workerCtx.collector.Stop(shutdownCtx)

		// Shutdown the per-worker action executor
		workerCtx.executor.Shutdown()
	}

	s.mu.Unlock()

	s.logTrace("lifecycle",
		deps.String("lifecycle_event", "shutdown_complete"))
}

// startMetricsReporter starts a goroutine that periodically records hierarchy metrics.
// Metrics are recorded at the configured metricsReportInterval to avoid excessive Prometheus cardinality.
// The goroutine stops when the context is cancelled.
func (s *Supervisor[TObserved, TDesired]) startMetricsReporter(ctx context.Context) {
	s.metricsWg.Add(1)

	go func() {
		defer s.metricsWg.Done()

		ticker := time.NewTicker(s.metricsReportInterval)
		defer ticker.Stop()

		s.recordHierarchyMetrics()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.recordHierarchyMetrics()
			}
		}
	}()
}

// recordHierarchyMetrics records current hierarchy depth and size metrics.
func (s *Supervisor[TObserved, TDesired]) recordHierarchyMetrics() {
	// Get hierarchy path under lock first (GetHierarchyPathUnlocked iterates s.workers)
	s.mu.RLock()
	path := s.GetHierarchyPathUnlocked()
	s.mu.RUnlock()

	depth := s.calculateHierarchyDepth()
	size := s.calculateHierarchySize()

	metrics.RecordHierarchyDepth(path, depth)
	metrics.RecordHierarchySize(path, size)
}

func (s *Supervisor[TObserved, TDesired]) calculateHierarchyDepth() int {
	if s.parent == nil {
		return 0
	}

	return 1 + s.parent.calculateHierarchyDepth()
}

func (s *Supervisor[TObserved, TDesired]) calculateHierarchySize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	size := 1

	for _, child := range s.children {
		size += child.calculateHierarchySize()
	}

	return size
}

// getStartedContext atomically checks if started and returns context (avoids TOCTOU).
func (s *Supervisor[TObserved, TDesired]) getStartedContext() (context.Context, bool) {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()

	if !s.started.Load() {
		return nil, false
	}

	return s.ctx, true
}

func (s *Supervisor[TObserved, TDesired]) logTrace(msg string, fields ...deps.Field) {
	if s.enableTraceLogging {
		s.logger.Debug(msg, fields...)
	}
}

func (s *Supervisor[TObserved, TDesired]) requestShutdown(ctx context.Context, workerID string, reason string) error {
	s.logger.Info("shutdown_requested",
		deps.String("worker_id", workerID),
		deps.Reason(reason))

	s.mu.RLock()
	_, exists := s.workers[workerID]
	s.mu.RUnlock()

	if !exists {
		return errors.New("worker not found")
	}

	// Load current desired state from database
	var desired TDesired

	err := s.store.LoadDesiredTyped(ctx, s.workerType, workerID, &desired)
	if err != nil {
		if !errors.Is(err, persistence.ErrNotFound) {
			return fmt.Errorf("failed to load desired state: %w", err)
		}

		return nil
	}

	// Set ShutdownRequested=true using the ShutdownRequestable interface
	if sr, ok := any(desired).(fsmv2.ShutdownRequestable); ok {
		sr.SetShutdownRequested(true)
	} else if sr, ok := any(&desired).(fsmv2.ShutdownRequestable); ok {
		sr.SetShutdownRequested(true)
	} else {
		return fmt.Errorf("desired state type %T does not implement ShutdownRequestable", desired)
	}

	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("failed to marshal desired state: %w", err)
	}

	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		return fmt.Errorf("failed to unmarshal to document: %w", err)
	}

	// Add 'id' field required by TriangularStore validation.
	desiredDoc[FieldID] = workerID

	// Save updated desired state back to database
	if _, err := s.store.SaveDesired(ctx, s.workerType, workerID, desiredDoc); err != nil {
		return fmt.Errorf("failed to save desired state with shutdown request: %w", err)
	}

	return nil
}

// RequestShutdown sets ShutdownRequested=true on all workers in this supervisor.
// Workers continue ticking and can complete their FSM shutdown transitions.
// This does NOT cancel the context - use Shutdown() for forced stop.
//
// This method is used by parent supervisors to request graceful shutdown of
// child supervisors when children are removed from ChildrenSpecs.
func (s *Supervisor[TObserved, TDesired]) RequestShutdown(ctx context.Context, reason string) error {
	s.mu.RLock()

	workerIDs := make([]string, 0, len(s.workers))

	for workerID := range s.workers {
		workerIDs = append(workerIDs, workerID)
	}

	s.mu.RUnlock()

	for _, workerID := range workerIDs {
		if err := s.requestShutdown(ctx, workerID, reason); err != nil {
			s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPath(), "shutdown_request_failed",
				deps.Err(err))
		}
	}

	return nil
}

// handleWorkerRestart performs FULL worker recreation after graceful shutdown.
// Called when SignalNeedsRemoval is received for a worker marked in pendingRestart.
//
// This destroys the old worker instance completely (including all dependencies) and
// creates a fresh worker via the factory. All worker state resets,
// including any attempt counters, connection pools, or cached data in dependencies.
//
// Flow:
//  1. RemoveWorker - stops collector, executor, removes from registry
//  2. Clear ShutdownRequested in storage (so new worker starts fresh)
//  3. factory.NewWorkerByType - creates completely new worker instance
//  4. AddWorker - registers new worker
//  5. Start collector and executor if supervisor is running
//  6. Reset health counters for fresh start
func (s *Supervisor[TObserved, TDesired]) handleWorkerRestart(ctx context.Context, workerID string) error {
	s.mu.RLock()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		s.mu.RUnlock()

		err := errors.New("worker not found for restart")
		s.logger.SentryError(deps.FeatureFSMv2, s.GetHierarchyPathUnlocked(), err, "worker_restart_not_found",
			deps.String("target_worker_id", workerID))

		return err
	}

	identity := workerCtx.identity

	// Hold workerCtx.mu while reading currentState to prevent race with reconciliation
	workerCtx.mu.RLock()

	fromState := "unknown"
	if workerCtx.currentState != nil {
		fromState = workerCtx.currentState.String()
	}

	workerCtx.mu.RUnlock()

	s.mu.RUnlock()

	s.logger.Info("worker_restart_executing",
		deps.HierarchyPath(identity.HierarchyPath),
		deps.String("from_state", fromState),
		deps.String("action", "full_recreation"))

	// 1. Remove old worker completely (stops collector, executor, removes from registry)
	if err := s.RemoveWorker(ctx, workerID); err != nil {
		return fmt.Errorf("failed to remove worker for restart: %w", err)
	}

	s.logger.Debug("worker_restart_old_removed",
		deps.HierarchyPath(identity.HierarchyPath))

	// Shutdown may have started after processSignal read started=true but
	// before this point. The old worker is already removed; skip re-creation
	// so shutdown can complete. Best-effort — correctness does not depend on it.
	if !s.started.Load() {
		s.logger.Info("worker_restart_cancelled_shutdown",
			deps.HierarchyPath(identity.HierarchyPath),
			deps.String("target_worker_id", workerID))

		return nil
	}

	// 2. Clear shutdown flag in storage BEFORE creating new worker.
	if err := s.clearShutdownRequested(ctx, workerID); err != nil {
		s.logger.SentryWarn(deps.FeatureFSMv2, identity.HierarchyPath, "restart_clear_shutdown_failed",
			deps.Err(err))
		// Continue anyway - the new worker might still work
	}

	// 3. Create fresh worker instance via factory
	newWorker, err := factory.NewWorkerByType(s.workerType, identity, s.baseLogger, s.store, s.deps)
	if err != nil {
		return fmt.Errorf("failed to create new worker for restart: %w", err)
	}

	s.logger.Debug("worker_restart_new_created",
		deps.HierarchyPath(identity.HierarchyPath))

	// 4. Add new worker to supervisor (registers worker in the workers map)
	if err := s.AddWorker(identity, newWorker); err != nil {
		return fmt.Errorf("failed to add new worker for restart: %w", err)
	}

	// 5. Start collector and executor if supervisor is running
	if supervisorCtx, started := s.getStartedContext(); started {
		s.mu.RLock()

		newWorkerCtx, exists := s.workers[workerID]

		var collector *collection.Collector[TObserved]

		var executor *execution.ActionExecutor

		if exists && newWorkerCtx != nil {
			collector = newWorkerCtx.collector
			executor = newWorkerCtx.executor
		}

		s.mu.RUnlock()

		if collector != nil {
			if err := collector.Start(supervisorCtx); err != nil {
				s.logger.SentryError(deps.FeatureFSMv2, identity.HierarchyPath, err, "restart_collector_start_failed")
			}
		}

		if executor != nil {
			executor.Start(supervisorCtx)
		}
	}

	// 6. Reset health counters for fresh start
	s.mu.Lock()
	s.collectorHealth.restartCount = 0
	s.mu.Unlock()

	s.mu.RLock()

	newWorkerCtx := s.workers[workerID]
	toState := "unknown"

	if newWorkerCtx != nil {
		newWorkerCtx.mu.RLock()

		if newWorkerCtx.currentState != nil {
			toState = newWorkerCtx.currentState.String()
		}

		newWorkerCtx.mu.RUnlock()
	}

	s.mu.RUnlock()

	s.logger.Info("worker_restart_complete",
		deps.HierarchyPath(identity.HierarchyPath),
		deps.String("to_state", toState))

	return nil
}

// setDisabled mirrors requestShutdown but uses the Disableable interface.
// The disable-mapping pass is the exclusive caller; no other subsystem may call this.
func (s *Supervisor[TObserved, TDesired]) setDisabled(ctx context.Context, workerID string, disabled bool) error {
	s.mu.RLock()
	_, exists := s.workers[workerID]
	s.mu.RUnlock()

	if !exists {
		return errors.New("worker not found")
	}

	var desired TDesired

	err := s.store.LoadDesiredTyped(ctx, s.workerType, workerID, &desired)
	if err != nil {
		if !errors.Is(err, persistence.ErrNotFound) {
			return fmt.Errorf("failed to load desired state: %w", err)
		}

		return nil
	}

	if d, ok := any(desired).(fsmv2.Disableable); ok {
		d.SetDisabled(disabled)
	} else if d, ok := any(&desired).(fsmv2.Disableable); ok {
		d.SetDisabled(disabled)
	} else {
		return fmt.Errorf("desired state type %T does not implement Disableable", desired)
	}

	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("failed to marshal desired state: %w", err)
	}

	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		return fmt.Errorf("failed to unmarshal to document: %w", err)
	}

	desiredDoc[FieldID] = workerID

	if _, err := s.store.SaveDesired(ctx, s.workerType, workerID, desiredDoc); err != nil {
		return fmt.Errorf("failed to save desired state with disabled flag: %w", err)
	}

	return nil
}

// SetDisabled writes the Disabled flag on every worker. disabled=true keeps
// them resident in Stopped (no resume). The disable-mapping pass calls this per-tick.
// Returns the first error; subsequent workers are still updated.
func (s *Supervisor[TObserved, TDesired]) SetDisabled(ctx context.Context, disabled bool) error {
	s.mu.RLock()

	workerIDs := make([]string, 0, len(s.workers))

	for workerID := range s.workers {
		workerIDs = append(workerIDs, workerID)
	}

	s.mu.RUnlock()

	for _, workerID := range workerIDs {
		if err := s.setDisabled(ctx, workerID, disabled); err != nil {
			return fmt.Errorf("worker %q: %w", workerID, err)
		}
	}

	return nil
}

// clearShutdownRequested clears the ShutdownRequested flag in storage for restart.
func (s *Supervisor[TObserved, TDesired]) clearShutdownRequested(ctx context.Context, workerID string) error {
	// Load current desired state
	var desired TDesired
	if err := s.store.LoadDesiredTyped(ctx, s.workerType, workerID, &desired); err != nil {
		return fmt.Errorf("load desired for clear shutdown: %w", err)
	}

	// Clear shutdown flag via interface
	if sr, ok := any(desired).(fsmv2.ShutdownRequestable); ok {
		sr.SetShutdownRequested(false)
	} else if sr, ok := any(&desired).(fsmv2.ShutdownRequestable); ok {
		sr.SetShutdownRequested(false)
	} else {
		return fmt.Errorf("desired state type %T does not implement ShutdownRequestable", desired)
	}

	// Save back - need to convert to Document
	desiredDoc := make(persistence.Document)

	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("marshal desired: %w", err)
	}

	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		return fmt.Errorf("unmarshal desired to doc: %w", err)
	}

	desiredDoc["id"] = workerID

	if _, err := s.store.SaveDesired(ctx, s.workerType, workerID, desiredDoc); err != nil {
		return fmt.Errorf("save desired: %w", err)
	}

	return nil
}
