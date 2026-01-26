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
	s.ctxMu.Unlock()
	s.started.Store(true)

	s.logger.Debugw("supervisor_started")

	s.ctxMu.RLock()
	supervisorCtx := s.ctx
	s.ctxMu.RUnlock()

	s.actionExecutor.Start(supervisorCtx)

	s.startMetricsReporter(supervisorCtx)

	// Start observation collectors and action executors for all workers
	s.mu.RLock()

	for _, workerCtx := range s.workers {
		if err := workerCtx.collector.Start(supervisorCtx); err != nil {
			s.logger.Errorw("collector_start_failed", "error", err)
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

// tickLoop is the main FSM loop.
// Calls Tick() which includes hierarchical composition (Phase 0) and worker state transitions.
func (s *Supervisor[TObserved, TDesired]) tickLoop(ctx context.Context) {
	s.logger.Debug("Starting tick loop for supervisor")

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	s.logger.Debugw("tick_loop_started",
		"interval", s.tickInterval)

	for {
		select {
		case <-ctx.Done():
			s.logger.Debugw("tick_loop_stopped")

			return
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				s.logger.Errorw("tick_error", "error", err)
			}
		}
	}
}

// Shutdown gracefully shuts down this supervisor and all its workers.
// Shutdown order: children first, then own workers, then cancel context.
// Idempotent.
func (s *Supervisor[TObserved, TDesired]) Shutdown() {
	s.logTrace("lifecycle",
		"lifecycle_event", "shutdown_start")

	s.logTrace("lifecycle",
		"lifecycle_event", "mutex_lock_acquire",
		"mutex_name", "supervisor.mu",
		"lock_type", "write")

	s.mu.Lock()

	s.logTrace("lifecycle",
		"lifecycle_event", "mutex_lock_acquired",
		"mutex_name", "supervisor.mu")

	// Make idempotent - check if already shut down
	if !s.started.Load() {
		s.mu.Unlock()

		s.logTrace("lifecycle",
			"lifecycle_event", "shutdown_skip",
			"reason", "already_shutdown")

		s.logger.Debugw("supervisor_already_shutdown")

		return
	}

	s.started.Store(false)

	s.logger.Infow("supervisor_shutting_down")

	gracefulCtx := context.Background()

	// Extract children before releasing lock to avoid deadlock with child Tick() goroutines.
	childrenToShutdown := make(map[string]SupervisorInterface, len(s.children))

	childDoneChans := make(map[string]<-chan struct{}, len(s.childDoneChans))

	for name, child := range s.children {
		childrenToShutdown[name] = child
	}

	for name, done := range s.childDoneChans {
		childDoneChans[name] = done
	}

	// Get worker IDs under lock
	workerIDs := make([]string, 0, len(s.workers))
	for workerID := range s.workers {
		workerIDs = append(workerIDs, workerID)
	}

	// Release lock before graceful shutdown operations
	s.mu.Unlock()

	// Phase 1: Shutdown children first (context still active for FSM transitions).
	if len(childrenToShutdown) > 0 {
		s.logger.Debugw("graceful_shutdown_children_starting",
			"child_count", len(childrenToShutdown))

		for childName, child := range childrenToShutdown {
			s.logTrace("lifecycle",
				"lifecycle_event", "child_shutdown_start",
				"child_name", childName,
				"parent_worker_type", s.workerType)

			s.logger.Debugw("child_shutting_down",
				"child_name", childName)

			child.Shutdown()

			// Wait for child supervisor to fully shut down
			if done, exists := childDoneChans[childName]; exists {
				s.logger.Debugw("waiting_child_shutdown",
					"child_name", childName)
				<-done
				s.logger.Debugw("child_shutdown_complete",
					"child_name", childName)
			}

			s.logTrace("lifecycle",
				"lifecycle_event", "child_shutdown_complete",
				"child_name", childName,
				"parent_worker_type", s.workerType)
		}

		s.logger.Debugw("graceful_shutdown_children_complete")
	}

	// Phase 2: Request graceful shutdown on own workers.
	if len(workerIDs) > 0 {
		s.logger.Debugw("graceful_shutdown_workers_starting",
			"worker_count", len(workerIDs))

		// Request graceful shutdown on all workers
		for _, workerID := range workerIDs {
			if err := s.requestShutdown(gracefulCtx, workerID, "supervisor_shutdown"); err != nil {
				s.logger.Warnw("graceful_shutdown_request_failed",
					"error", err)
			}
		}

		// Wait for workers to complete graceful shutdown (with timeout)
		gracefulTimeout := s.gracefulShutdownTimeout
		ticker := time.NewTicker(100 * time.Millisecond)
		timeoutCh := time.After(gracefulTimeout)

	gracefulWaitLoop:
		for {
			select {
			case <-timeoutCh:
				s.logger.Warnw("graceful_shutdown_timeout",
					"timeout", gracefulTimeout)

				break gracefulWaitLoop
			case <-ticker.C:
				s.mu.RLock()
				remaining := len(s.workers)
				s.mu.RUnlock()

				if remaining == 0 {
					s.logger.Debugw("graceful_shutdown_workers_removed")

					break gracefulWaitLoop
				}
			}
		}

		ticker.Stop()
	}

	// Phase 3: Cancel context (must happen before waiting for metrics reporter).
	s.ctxMu.Lock()

	if s.ctxCancel != nil {
		s.ctxCancel()
		s.ctxCancel = nil // Prevent double-cancel
	}

	s.ctxMu.Unlock()

	// Wait for metrics reporter to finish (it will exit now that context is cancelled)
	s.metricsWg.Wait()

	// Re-acquire lock for cleanup
	s.mu.Lock()

	// Shutdown action executor (doesn't block)
	s.actionExecutor.Shutdown()

	// Shutdown remaining per-worker executors and collectors.
	shutdownCtx := context.Background()

	for _, workerCtx := range s.workers {
		s.logger.Debugw("worker_shutting_down")

		// Stop the collector's observation loop
		workerCtx.collector.Stop(shutdownCtx)

		// Shutdown the per-worker action executor
		workerCtx.executor.Shutdown()
	}

	s.mu.Unlock()

	s.logTrace("lifecycle",
		"lifecycle_event", "shutdown_complete")
}

// startMetricsReporter starts a goroutine that periodically records hierarchy metrics.
// Metrics are recorded every 10 seconds to avoid excessive Prometheus cardinality.
// The goroutine stops when the context is cancelled.
func (s *Supervisor[TObserved, TDesired]) startMetricsReporter(ctx context.Context) {
	s.metricsWg.Add(1)

	go func() {
		defer s.metricsWg.Done()

		ticker := time.NewTicker(10 * time.Second)
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
	depth := s.calculateHierarchyDepth()
	size := s.calculateHierarchySize()

	metrics.RecordHierarchyDepth(s.GetHierarchyPathUnlocked(), depth)
	metrics.RecordHierarchySize(s.GetHierarchyPathUnlocked(), size)
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

func (s *Supervisor[TObserved, TDesired]) logTrace(msg string, fields ...interface{}) {
	if s.enableTraceLogging {
		s.logger.Debugw(msg, fields...)
	}
}

func (s *Supervisor[TObserved, TDesired]) requestShutdown(ctx context.Context, workerID string, reason string) error {
	s.logger.Infow("shutdown_requested", "reason", reason)

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
		// No desired state in DB yet, nothing to update
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
			s.logger.Warnw("shutdown_request_failed", "error", err)
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
//  1. Extract identity from existing worker (before removal)
//  2. RemoveWorker - stops collector, executor, removes from registry
//  3. Clear ShutdownRequested in storage (so new worker starts fresh)
//  4. factory.NewWorkerByType - creates completely new worker instance
//  5. AddWorker - registers new worker, starts collector
func (s *Supervisor[TObserved, TDesired]) handleWorkerRestart(ctx context.Context, workerID string) error {
	// Extract identity and state under lock before removal.
	// NOTE: We hold both s.mu and workerCtx.mu to prevent TOCTOU race when
	// reading currentState - otherwise reconciliation could modify it concurrently.
	s.mu.RLock()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		s.mu.RUnlock()

		s.logger.Errorw("worker_restart_not_found",
			"hierarchy_path", s.GetHierarchyPathUnlocked(),
			"target_worker_id", workerID)

		return errors.New("worker not found for restart")
	}

	identity := workerCtx.identity

	// Hold workerCtx.mu while reading currentState to prevent race with reconciliation
	workerCtx.mu.RLock()
	fromState := workerCtx.currentState.String()
	workerCtx.mu.RUnlock()

	s.mu.RUnlock()

	s.logger.Infow("worker_restart_executing",
		"hierarchy_path", identity.HierarchyPath,
		"from_state", fromState,
		"action", "full_recreation")

	// 1. Remove old worker completely (stops collector, executor, removes from registry)
	if err := s.RemoveWorker(ctx, workerID); err != nil {
		return fmt.Errorf("failed to remove worker for restart: %w", err)
	}

	s.logger.Debugw("worker_restart_old_removed",
		"hierarchy_path", identity.HierarchyPath)

	// 2. Clear shutdown flag in storage BEFORE creating new worker.
	if err := s.clearShutdownRequested(ctx, workerID); err != nil {
		s.logger.Warnw("restart_clear_shutdown_failed",
			"hierarchy_path", identity.HierarchyPath,
			"error", err)
		// Continue anyway - the new worker might still work
	}

	// 3. Create fresh worker instance via factory
	newWorker, err := factory.NewWorkerByType(s.workerType, identity, s.baseLogger, s.store, s.deps)
	if err != nil {
		return fmt.Errorf("failed to create new worker for restart: %w", err)
	}

	s.logger.Debugw("worker_restart_new_created",
		"hierarchy_path", identity.HierarchyPath)

	// 4. Add new worker to supervisor (this also starts the collector if supervisor is running)
	if err := s.AddWorker(identity, newWorker); err != nil {
		return fmt.Errorf("failed to add new worker for restart: %w", err)
	}

	// 5. Start the new worker's collector and executor if supervisor is already running.
	// NOTE: We capture collector and executor under lock to prevent TOCTOU race.
	// Between checking worker existence and starting collector/executor, another
	// goroutine could remove the worker. By capturing the pointers under lock,
	// we ensure we have valid references even if the worker is subsequently removed.
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
				s.logger.Errorw("restart_collector_start_failed",
					"hierarchy_path", identity.HierarchyPath,
					"error", err)
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

	// Get new state for logging.
	// NOTE: We hold both s.mu and workerCtx.mu to prevent TOCTOU race when
	// reading currentState - otherwise reconciliation could modify it concurrently.
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

	s.logger.Infow("worker_restart_complete",
		"hierarchy_path", identity.HierarchyPath,
		"to_state", toState)

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
