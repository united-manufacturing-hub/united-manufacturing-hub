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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// Start starts the supervisor goroutines.
// Returns a channel that will be closed when the supervisor stops.
func (s *Supervisor[TObserved, TDesired]) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})

	// Create a child context that we can cancel in Shutdown()
	// This allows Shutdown() to stop the tick loop even if parent context is still active
	s.ctxMu.Lock()
	s.ctx, s.ctxCancel = context.WithCancel(ctx)
	s.ctxMu.Unlock()
	s.started.Store(true)

	s.logger.Debugw("supervisor_started")

	// Use the child context for all goroutines so they stop when Shutdown() is called
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
// This method is called when the supervisor is being removed from its parent.
// This method is idempotent - calling it multiple times is safe.
//
// IMPORTANT: Shutdown order is children FIRST, then own workers, then cancel context.
// This ensures children can still tick and process their graceful shutdown while
// the parent's context is still active.
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

	// We use a temporary context that won't be cancelled during graceful shutdown.
	gracefulCtx := context.Background()

	// LOCK SAFETY: Extract children and done channels while holding lock,
	// then release lock before recursively shutting down children.
	// This prevents deadlock where:
	// 1. Parent holds write lock
	// 2. Parent calls child.Shutdown() which tries to acquire child's write lock
	// 3. Child's Tick() goroutine is blocked waiting for parent's read lock
	// By releasing parent lock before calling child.Shutdown(), we allow
	// child Tick() goroutines to complete and exit cleanly.
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

	// GRACEFUL SHUTDOWN PHASE 1: Shutdown children FIRST (while context still active)
	// Children's tick loops depend on the parent's context. By shutting down children
	// before cancelling our context, children can still tick and process their
	// graceful shutdown (FSM transitions, disconnect actions, emit SignalNeedsRemoval).
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

			// child.Shutdown() will recursively:
			// 1. Shutdown grandchildren first
			// 2. Request graceful shutdown on child's workers
			// 3. Cancel child's context last
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

	// GRACEFUL SHUTDOWN PHASE 2: Request graceful shutdown on OWN workers
	// Now that children are done, handle our own workers.
	// The tick loop is still running, so workers can process their state machines.
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

	// GRACEFUL SHUTDOWN PHASE 3: Cancel context
	// Only after children AND own workers are done do we cancel our context.
	// This must happen BEFORE waiting for metrics reporter, because the metrics
	// reporter goroutine waits on <-ctx.Done() to exit.
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

	// Shutdown all per-worker executors and collectors (any remaining after graceful shutdown)
	// These have their own goroutines that need to be stopped to prevent leaks.
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

	metrics.RecordHierarchyDepth(s.workerType, depth)
	metrics.RecordHierarchySize(s.workerType, size)
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

// isStarted returns whether the supervisor has been started.
//
//nolint:unused // Part of API design, may be used in future
func (s *Supervisor[TObserved, TDesired]) isStarted() bool {
	return s.started.Load()
}

// getContext returns the current context for the supervisor.
//
//nolint:unused // Part of API design, may be used in future
func (s *Supervisor[TObserved, TDesired]) getContext() context.Context {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()

	return s.ctx
}

// getStartedContext atomically checks if started and returns context.
// This prevents TOCTOU races between isStarted() and getContext() calls.
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
		return fmt.Errorf("worker %s not found", workerID)
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

	// Convert to Document via JSON marshaling (required by SaveDesired)
	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("failed to marshal desired state: %w", err)
	}
	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(desiredJSON, &desiredDoc); err != nil {
		return fmt.Errorf("failed to unmarshal to document: %w", err)
	}

	// Add 'id' field required by TriangularStore validation
	// JSON marshaling doesn't preserve this field since typed structs don't have an 'id' field
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
