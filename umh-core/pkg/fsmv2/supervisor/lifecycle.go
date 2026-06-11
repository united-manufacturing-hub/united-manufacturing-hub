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

	s.startWorkerRunners(supervisorCtx)

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

	s.startWorkerRunners(supervisorCtx)
}

// startWorkerRunners starts each worker's observation collector and action
// executor. The collector is what re-observes the worker's world every tick;
// the executor drains queued actions. Both must be running before the
// supervisor is ticked.
//
// Lock ordering: takes only mu.RLock internally. Callers must NOT hold ctxMu
// when calling this (advisory order is mu -> ctxMu).
func (s *Supervisor[TObserved, TDesired]) startWorkerRunners(ctx context.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		if err := workerCtx.collector.Start(ctx); err != nil {
			s.logger.SentryError(deps.FeatureFSMv2, workerCtx.identity.HierarchyPath, err, "collector_start_failed")
		}

		workerCtx.executor.Start(ctx)
	}
}

// Run starts the supervisor and blocks until ctx is canceled or Shutdown is
// called externally. When ctx is canceled, Run drives a graceful shutdown via
// Shutdown() before returning. Returns nil on a clean shutdown.
//
// Cancelling ctx does NOT stop the tick loop directly: the loop runs on a
// context detached from the caller's cancellation (context.WithoutCancel),
// and the cancel is the trigger for an orderly Shutdown executed against the
// still-live loop. The drain depends on that liveness — only the tick loop
// reaps workers that signal removal, so a loop sharing the caller's ctx dies
// on the first cancel and the drain has nothing to reap until the budget
// (sampled once at Shutdown entry as base × subtree height) expires
// (ENG-4971). If a worker never signals removal, the supervisor logs
// graceful_shutdown_timeout and removes it when the level's drain budget
// expires.
//
// Run is the recommended entry point for top-level supervisors (e.g.,
// cmd/main.go). For child supervisors, parents use StartAsChild instead.
//
// Composition:
//
//	done := s.Start(context.WithoutCancel(ctx)) // tick loop detached from caller cancel
//	<-ctx.Done()   // OR <-done if an external Shutdown() stops the supervisor first
//	s.Shutdown()   // graceful drain against the live tick loop
//	<-done         // Shutdown waits for tickLoop; this read is a safe no-op
func (s *Supervisor[TObserved, TDesired]) Run(ctx context.Context) error {
	// Values are preserved; cancellation AND any caller deadline are discarded.
	// Shutdown Phase 4 is the sole canceller of the supervisor context (ENG-4971).
	done := s.Start(context.WithoutCancel(ctx))

	select {
	case <-ctx.Done():
		// External cancellation (e.g., SIGTERM via signal.NotifyContext).
		// Shutdown drives graceful drain before cancelling the supervisor context.
		s.Shutdown()
		// done is already closed when Shutdown returns; this read is a safe no-op.
		<-done

		return nil
	case <-done:
		// tickLoop exited on its own: an external Shutdown() cancelled the
		// supervisor context (Phase 4) — the detached loop cannot die via
		// caller-ctx cancellation. Call Shutdown to clean up executor +
		// collectors (idempotent).
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

	// Sample the cascaded drain budget once, at entry (pkg/fsmv2/CLAUDE.md
	// §"Graceful Shutdown Cascading"): base × subtree height, so a leaf keeps
	// the base budget and every level above adds one base. Sampling before
	// Phase 2 matters twice over: the synchronous child drains below spend
	// from this same budget (Phase 3 arms only the remainder, so per-level
	// budgets do not sum across a chain), and the tickLoop keeps running
	// during the drain — its shutdown reap (reconcileChildren with nil specs)
	// prunes s.children mid-cascade, so a later sample could see a shrunken
	// tree and undersize the budget. The tree can also GROW between sampling
	// and Phase 2 — the still-running tickLoop can spawn a child that is
	// absent from both the childrenToShutdown snapshot and the sampled
	// height, so its later tick-driven drain eats the Phase-3 remainder
	// uncounted: a known, bounded residual (deferred to ENG-5141).
	drainBudget := s.gracefulShutdownTimeout * time.Duration(s.calculateSubtreeHeight())
	drainStart := time.Now()

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
			// and synchronous — bounded by the child's own cascaded drain budget
			// (gracefulShutdownTimeout × the child's subtree height). Calls
			// from this cascade and from later tick-driven reconcileChildren(nil) paths
			// converge on the same early-return — which makes this budget blind to
			// in-flight reap drains: when a tick-goroutine reap entered the child
			// first, this call returns immediately while the child's real drain
			// completes inside the tickLoop, absorbed by the Phase-4 join below.
			// The post-join budget re-check after Phase 4 is what surfaces that
			// spend.
			child.Shutdown()

			s.logTrace("lifecycle",
				deps.String("lifecycle_event", "child_shutdown_complete"),
				deps.String("child_name", childName),
				deps.String("parent_worker_type", s.workerType))
		}

		s.logger.Debug("graceful_shutdown_children_complete")
	}

	// Phase-2 spend, captured once for both the Phase-3 remainder and the
	// timeout warn: when child drains pre-spend the budget, the warn must show
	// how much of the window this level's own workers actually had left —
	// otherwise an exhausted-by-children level is indistinguishable from a
	// stuck worker.
	childDrainElapsed := time.Since(drainStart)

	// Event-name rule, shared by all three warn sites below:
	// graceful_shutdown_budget_exhausted means the budget was already spent
	// by child drains before this level's own workers had a window (or, at
	// the post-join re-check, by drains the Phase-3 timer never saw);
	// graceful_shutdown_timeout is reserved for a level whose OWN workers
	// exhausted a budget that was actually available to them. budgetWarned
	// keeps the post-join re-check from double-reporting an overrun a
	// drain-loop branch already warned about.
	budgetWarned := false

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

		// Arm what is left of the cascaded budget at arming time — re-measured
		// here rather than reusing childDrainElapsed, so the per-worker
		// requestShutdown store round-trips above are also charged against the
		// budget. The height factor exists because deeper subtrees imply
		// parent workers whose own graceful stops take intrinsically longer,
		// and because on the tick-driven removal cascade (reconciliation.go,
		// the pendingRemoval reap) a child supervisor's full drain genuinely
		// nests inside this window. A level that exhausts its budget still
		// warns and breaks out: a spent budget arms a non-positive timer,
		// which fires immediately.
		gracefulTimer := time.NewTimer(drainBudget - time.Since(drainStart))
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

				// Event name per the rule above childDrainElapsed: a budget the
				// children pre-spent is exhaustion, not an own-worker timeout.
				event := "graceful_shutdown_timeout"
				if childDrainElapsed >= drainBudget {
					event = "graceful_shutdown_budget_exhausted"
				}

				s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPath(), event,
					deps.Duration("timeout", drainBudget),
					deps.Duration("child_drain_elapsed", childDrainElapsed),
					deps.Int("remaining_worker_count", remainingCount))

				budgetWarned = true

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

				// The operator forced the exit; the post-join budget re-check
				// must not re-report the truncated drain as an overrun.
				budgetWarned = true

				break drainLoop
			}
		}
	} else if childDrainElapsed >= drainBudget {
		// No own workers to drain, but the Phase-2 child drains exhausted this
		// level's budget. Zero workers on a started supervisor is a normal
		// transient (see tick's no-worker skip in reconciliation.go), so the
		// exhaustion warn must not hide behind the worker drain: a shutdown
		// overrun that blows the process's SIGTERM grace would otherwise leave
		// no Sentry signal at the level that overran. The worker count is
		// re-read live (mirroring the timer branch): a worker raced in by
		// handleWorkerRestart after the entry snapshot stays registered and
		// undrained, and the warn must not claim a clean zero.
		// Children pre-spent the budget, so the event name is the
		// exhaustion one (see the rule above childDrainElapsed).
		s.mu.RLock()
		remainingCount := len(s.workers)
		s.mu.RUnlock()

		s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPath(), "graceful_shutdown_budget_exhausted",
			deps.Duration("timeout", drainBudget),
			deps.Duration("child_drain_elapsed", childDrainElapsed),
			deps.Int("remaining_worker_count", remainingCount))

		budgetWarned = true
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

	// Budget re-check after the Phase-4 joins: a child drain that a
	// tick-goroutine reap entered first early-returns in Phase 2 (see the
	// child.Shutdown comment above) and its real spend completes inside the
	// tickLoop, invisible to the Phase-3 timer — only the total elapsed
	// measured here catches it. An overrun that reaches this point without a
	// drain-loop warn never came from this level's own workers exhausting an
	// available window, so per the event-name rule above childDrainElapsed it
	// reports as budget exhaustion.
	if totalDrainElapsed := time.Since(drainStart); !budgetWarned && totalDrainElapsed > drainBudget {
		s.logger.SentryWarn(deps.FeatureFSMv2, s.GetHierarchyPath(), "graceful_shutdown_budget_exhausted",
			deps.Duration("timeout", drainBudget),
			deps.Duration("child_drain_elapsed", childDrainElapsed),
			deps.Duration("total_drain_elapsed", totalDrainElapsed))
	}

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

// calculateSubtreeHeight returns the number of supervisor levels in this
// supervisor's subtree, including itself. A supervisor with no child
// supervisors has height 1.
func (s *Supervisor[TObserved, TDesired]) calculateSubtreeHeight() int {
	// Copy the children under RLock, then recurse unlocked: LOCK ORDER rule 3
	// (supervisor.go) forbids holding Supervisor.mu across child method calls.
	s.mu.RLock()

	children := make([]SupervisorInterface, 0, len(s.children))
	for _, child := range s.children {
		children = append(children, child)
	}

	s.mu.RUnlock()

	height := 1

	for _, child := range children {
		if h := 1 + child.calculateSubtreeHeight(); h > height {
			height = h
		}
	}

	return height
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
