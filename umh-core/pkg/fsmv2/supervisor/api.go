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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// AddWorker adds a new worker to the supervisor's registry.
// Returns error if worker with same ID already exists.
// Multi-layer validation strategy:
//
// FSMv2 validates data at multiple layers (not one centralized validator).
// This is intentional, not redundant:
//
// Layer 1: API entry (AddWorker) - Fast fail on obvious errors
// Layer 2: Reconciliation entry (reconcileChildren) - Catch runtime edge cases
// Layer 3: Factory (worker creation) - Validate WorkerType exists
// Layer 4: Worker constructor - Validate dependencies
//
// Why multiple layers:
//   - Security: Never trust data, even from internal callers
//   - Debuggability: Errors caught closest to source
//   - Reliability: One layer failing doesn't compromise system
//
// Each layer has different validation concerns:
//   - Layer 1: Public API validation (protect against bad calls)
//   - Layer 2: Runtime state validation (data evolved since layer 1)
//   - Layer 3: Registry validation (WorkerType registered?)
//   - Layer 4: Logical validation (dependencies compatible?)
func (s *Supervisor[TObserved, TDesired]) AddWorker(identity deps.Identity, worker fsmv2.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[identity.ID]; exists {
		s.logger.SentryWarn(deps.FeatureFSMv2, identity.HierarchyPath, "worker_add_rejected",
			deps.Reason("already_exists"))

		return errors.New("worker already exists")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	initialDesired, err := s.deriveInitialDesired(worker, identity)
	if err != nil {
		return err
	}

	observed, err := s.collectInitialObservation(ctx, worker, identity, initialDesired)
	if err != nil {
		return err
	}

	if err := s.saveIdentity(ctx, identity); err != nil {
		return err
	}

	startupCount := s.nextStartupCount(identity)

	if err := s.saveInitialState(ctx, worker, identity, observed, initialDesired, startupCount); err != nil {
		return err
	}

	// Use baseLogger (un-enriched) to prevent duplicate "worker" fields.
	workerLogger := s.baseLogger.With(deps.String("worker", identity.String()))
	workerLogger.Info("identity_created")

	workerCtx := s.newWorkerContext(worker, identity, workerLogger, startupCount)

	workerCtx.collector = s.newCollector(worker, identity, workerLogger, workerCtx)

	s.registerWorker(workerCtx, identity, workerLogger)

	return nil
}

// deriveInitialDesired passes s.userSpec to DeriveDesiredState when its
// Config is set, nil otherwise.
// Caller must hold s.mu.
func (s *Supervisor[TObserved, TDesired]) deriveInitialDesired(worker fsmv2.Worker, identity deps.Identity) (fsmv2.DesiredState, error) {
	var ddsSpec interface{}
	if s.userSpec.Config != "" {
		ddsSpec = s.userSpec
	}

	initialDesired, err := worker.DeriveDesiredState(ddsSpec)
	if err != nil && ddsSpec != nil && errors.Is(err, config.ErrVariablesNotPropagated) {
		// Template rendering failed because parent variables (IP/PORT, auth token, …)
		// haven't propagated yet. Fall back to nil so the worker gets a valid default
		// desired state; the tick loop re-derives with the full spec on the next
		// reconciliation cycle. Hard errors (parse failures, type mismatches, YAML
		// validation issues) are NOT wrapped with ErrVariablesNotPropagated and bubble
		// up below so they don't get silently swallowed at startup.
		s.logger.SentryWarn(deps.FeatureFSMv2, identity.HierarchyPath, "worker_add_derive_desired_fallback_to_nil",
			deps.Err(err))
		initialDesired, err = worker.DeriveDesiredState(nil)
	}

	if err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, identity.HierarchyPath, err, "worker_add_derive_desired_failed")

		return nil, fmt.Errorf("failed to derive initial desired state: %w", err)
	}

	return initialDesired, nil
}

// collectInitialObservation collects the observed state a worker is added
// with. If CollectObservedState returned a NewObservation (zero CollectedAt),
// set it now: AddWorker bypasses the collector, so without a timestamp here
// the tick loop's checkDataFreshness would treat the first observation as
// timed out.
// Caller must hold s.mu.
func (s *Supervisor[TObserved, TDesired]) collectInitialObservation(ctx context.Context, worker fsmv2.Worker, identity deps.Identity, initialDesired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	observed, err := worker.CollectObservedState(ctx, initialDesired)
	if err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, identity.HierarchyPath, err, "worker_add_collect_observed_failed")

		return nil, fmt.Errorf("failed to collect initial observed state: %w", err)
	}

	if observed.GetTimestamp().IsZero() {
		if setter, ok := observed.(interface {
			SetCollectedAt(time.Time) fsmv2.ObservedState
		}); ok {
			observed = setter.SetCollectedAt(time.Now())
		} else {
			s.logger.SentryWarn(deps.FeatureFSMv2, identity.HierarchyPath,
				"worker_add_zero_timestamp_not_settable",
				deps.String("type", fmt.Sprintf("%T", observed)))
		}
	}

	return observed, nil
}

// saveIdentity writes a worker's identity document.
// Caller must hold s.mu.
func (s *Supervisor[TObserved, TDesired]) saveIdentity(ctx context.Context, identity deps.Identity) error {
	identityDoc := persistence.Document{
		"id":             identity.ID,
		"name":           identity.Name,
		"worker_type":    identity.WorkerType,
		"hierarchy_path": identity.HierarchyPath,
	}
	if err := s.store.SaveIdentity(ctx, s.workerType, identity.ID, identityDoc); err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, identity.HierarchyPath, err, "worker_add_save_identity_failed")

		return fmt.Errorf("failed to save identity: %w", err)
	}

	s.logger.Debug("identity_saved")

	return nil
}

// nextStartupCount returns the worker's next StartupCount, advanced by one
// from the previous observation when it recorded one. It runs before the
// SaveObserved in saveInitialState, which would otherwise overwrite the
// count; the ordering is pinned by "StartupCount persistence advances across
// a worker respawn instead of resetting to 1". A load error other than
// "not yet present" is logged, and the count restarts at 1.
// Caller must hold s.mu.
func (s *Supervisor[TObserved, TDesired]) nextStartupCount(identity deps.Identity) int64 {
	var startupCount int64 = 1

	loadCtx, loadCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer loadCancel()

	var prevObserved TObserved

	loadErr := s.store.LoadObservedTyped(loadCtx, s.workerType, identity.ID, &prevObserved)
	if loadErr == nil {
		if holder, ok := any(prevObserved).(deps.MetricsHolder); ok {
			fm := holder.GetFrameworkMetrics()
			if fm.StartupCount > 0 {
				startupCount = fm.StartupCount + 1
			}
		}
	} else if !errors.Is(loadErr, persistence.ErrNotFound) {
		s.logger.SentryWarn(deps.FeatureFSMv2, identity.HierarchyPath, "worker_add_load_prev_observation_failed", deps.Err(loadErr))
	}

	return startupCount
}

// documentConversion names the error prefix and the Sentry event for each
// stage of a toDocument conversion. Set both prefixes; an empty event skips
// Sentry.
type documentConversion struct {
	marshalEvent       string
	unmarshalEvent     string
	marshalErrPrefix   string
	unmarshalErrPrefix string
}

// toDocument converts a state value into its store document, with the id
// set. A failed stage wraps its error with that stage's prefix and reports
// that stage's Sentry event on hierarchyPath when the event is non-empty.
func (s *Supervisor[TObserved, TDesired]) toDocument(
	v any,
	id string,
	hierarchyPath string,
	conv documentConversion,
) (persistence.Document, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		if conv.marshalEvent != "" {
			s.logger.SentryError(deps.FeatureFSMv2, hierarchyPath, err, conv.marshalEvent)
		}

		return nil, fmt.Errorf("%s: %w", conv.marshalErrPrefix, err)
	}

	doc := make(persistence.Document)
	if err := json.Unmarshal(encoded, &doc); err != nil {
		if conv.unmarshalEvent != "" {
			s.logger.SentryError(deps.FeatureFSMv2, hierarchyPath, err, conv.unmarshalEvent)
		}

		return nil, fmt.Errorf("%s: %w", conv.unmarshalErrPrefix, err)
	}

	// TriangularStore validation rejects a document without the id.
	doc[FieldID] = id

	return doc, nil
}

// saveInitialState saves the desired document, then the observation with its
// StartupCount, then clears the tombstone. A failed attempt stores no new
// count, so a retried AddWorker does not count it; a failed ClearTombstone
// does count. Caller must hold s.mu.
func (s *Supervisor[TObserved, TDesired]) saveInitialState(ctx context.Context, worker fsmv2.Worker, identity deps.Identity, observed fsmv2.ObservedState, initialDesired fsmv2.DesiredState, startupCount int64) error {
	// Persist the computed StartupCount on the initial observation so a crash
	// between this save and the first collector tick does not reset it. The
	// collector still owns the full framework metrics on every later tick.
	if setter, ok := observed.(interface {
		SetFrameworkMetrics(deps.FrameworkMetrics) fsmv2.ObservedState
	}); ok {
		observed = setter.SetFrameworkMetrics(deps.FrameworkMetrics{StartupCount: startupCount})
	}

	observedDoc, err := s.toDocument(observed, identity.ID, identity.HierarchyPath, documentConversion{
		marshalEvent:       "worker_add_marshal_observed_failed",
		unmarshalEvent:     "worker_add_unmarshal_observed_failed",
		marshalErrPrefix:   "failed to marshal observed state",
		unmarshalErrPrefix: "failed to unmarshal observed state to document",
	})
	if err != nil {
		return err
	}

	// Inject the initial FSM state name so the store never contains state="".
	// CollectObservedState runs before the collector's StateProvider closure is
	// wired up, so the Observation struct leaves State="" at this point. The
	// StateProvider fires on every subsequent collection tick, but if the
	// scenario ends before the first tick fires (e.g., during the last cycle's
	// shutdown), the store would retain state="" and fail the
	// verifyObservedStateHasState check. Injecting the registered initial state
	// here closes that window.
	if initialStateForDoc := worker.GetInitialState(); initialStateForDoc != nil {
		observedDoc["state"] = initialStateForDoc.String()
	}

	_, err = s.store.SaveObserved(ctx, s.workerType, identity.ID, observedDoc)
	if err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, identity.HierarchyPath, err, "worker_add_save_observed_failed")

		return fmt.Errorf("failed to save initial observation: %w", err)
	}

	s.logger.Debug("initial_observation_saved")

	desiredDoc, err := s.toDocument(initialDesired, identity.ID, identity.HierarchyPath, documentConversion{
		marshalEvent:       "worker_add_marshal_desired_failed",
		unmarshalEvent:     "worker_add_unmarshal_desired_failed",
		marshalErrPrefix:   "failed to marshal desired state",
		unmarshalErrPrefix: "failed to unmarshal desired state to document",
	})
	if err != nil {
		return err
	}

	_, err = s.store.SaveDesired(ctx, s.workerType, identity.ID, desiredDoc)
	if err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, identity.HierarchyPath, err, "worker_add_save_desired_failed")

		return fmt.Errorf("failed to save initial desired state: %w", err)
	}

	s.logger.Debug("initial_desired_state_saved")

	// This ID may belong to a removed worker; its tombstone must not apply to the new one.
	if err := s.store.ClearTombstone(ctx, s.workerType, identity.ID); err != nil {
		s.logger.SentryError(deps.FeatureFSMv2, identity.HierarchyPath, err, "worker_add_clear_tombstone_failed")

		// The store error already names the operation and the worker.
		return err
	}

	return nil
}

// newCollector builds the collector for a newly added worker.
// Caller must hold s.mu.
func (s *Supervisor[TObserved, TDesired]) newCollector(worker fsmv2.Worker, identity deps.Identity, workerLogger deps.FSMLogger, workerCtx *WorkerContext[TObserved, TDesired]) *collection.Collector[TObserved] {
	// A worker type may register a custom collection cadence (simple.MonitorSpec.Interval);
	// fall back to the default when it did not.
	observationInterval := DefaultObservationInterval
	if iv, ok := fsmv2.ObservationIntervalFor(s.workerType); ok {
		observationInterval = iv
	}

	return collection.NewCollector[TObserved](collection.CollectorConfig[TObserved]{
		Worker:              worker,
		Identity:            identity,
		Store:               s.store,
		Logger:              workerLogger,
		ObservationInterval: observationInterval,
		ObservationTimeout:  s.collectorHealth.observationTimeout,
		StateProvider: func() string {
			workerCtx.mu.RLock()
			defer workerCtx.mu.RUnlock()

			if workerCtx.currentState == nil {
				return "unknown"
			}

			return workerCtx.currentState.String()
		},
		ShutdownRequestedProvider: func() bool {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			var desired TDesired
			if err := s.store.LoadDesiredTyped(ctx, s.workerType, identity.ID, &desired); err != nil {
				s.logger.SentryWarn(deps.FeatureFSMv2, identity.HierarchyPath, "shutdown_requested_load_failed",
					deps.Err(err))

				return true
			}

			return desired.IsShutdownRequested()
		},
		// A child is healthy ONLY if in PhaseRunningHealthy (fully stable).
		// PhaseRunningDegraded is operational but NOT healthy.
		// Uses lifecycle phase enum for type-safe health checks.
		ChildrenCountsProvider: func() (healthy int, unhealthy int) {
			s.mu.RLock()
			defer s.mu.RUnlock()

			for _, child := range s.children {
				phase := child.GetLifecyclePhase()
				if phase.IsHealthy() {
					healthy++
				} else if !phase.IsStopped() {
					// Everything except healthy and stopped is unhealthy
					// This includes: PhaseUnknown, PhaseStarting, PhaseRunningDegraded, PhaseStopping
					unhealthy++
				}
				// Stopped states are neither healthy nor unhealthy
			}

			return healthy, unhealthy
		},
		ChildrenViewProvider: func() config.ChildrenView {
			return config.NewChildrenView(s.childInfoSlice())
		},
		// Called BEFORE CollectObservedState to compute metrics.
		FrameworkMetricsProvider: func() *deps.FrameworkMetrics {
			workerCtx.mu.RLock()
			defer workerCtx.mu.RUnlock()

			// Copy stateTransitions to avoid race condition with reconciliation goroutine.
			// Without this copy, the map reference escapes the lock and can be read
			// while reconciliation writes to it.
			transitionsCopy := make(map[string]int64, len(workerCtx.stateTransitions))
			for state, count := range workerCtx.stateTransitions {
				transitionsCopy[state] = count
			}

			// Convert stateDurations (map[string]time.Duration) to milliseconds
			cumulativeTimeMs := make(map[string]int64, len(workerCtx.stateDurations))
			for state, duration := range workerCtx.stateDurations {
				cumulativeTimeMs[state] = duration.Milliseconds()
			}

			return &deps.FrameworkMetrics{
				TimeInCurrentStateMs:    time.Since(workerCtx.stateEnteredAt).Milliseconds(),
				StateEnteredAtUnix:      workerCtx.stateEnteredAt.Unix(),
				StateTransitionsTotal:   workerCtx.totalTransitions,
				TransitionsByState:      transitionsCopy,
				CumulativeTimeByStateMs: cumulativeTimeMs,
				CollectorRestarts:       workerCtx.collectorRestarts,
				StartupCount:            workerCtx.startupCount,
				StateReason:             workerCtx.currentStateReason,
			}
		},
		FrameworkMetricsSetter: func(fm *deps.FrameworkMetrics) {
			// Must use GetDependenciesAny() (returns any), not GetDependencies() (returns D).
			// This write feeds a worker that reads deps.GetFrameworkState() during
			// CollectObservedState, so it reaches only a bound deps that implements
			// SetFrameworkState (a deps embedding *deps.BaseDependencies). It is
			// separate from the Observation injection, which the collector performs
			// from its own local in wrapNewObservation regardless of the deps shape —
			// a struct{}-deps worker (nmap) still carries framework metrics on its
			// Observation. Application and configworker bind no deps and so simply
			// get no pre-COS write; returning nil or struct{}{} from
			// GetDependenciesAny is equivalent and neither is overridden.
			type depsGetter interface {
				GetDependenciesAny() any
			}
			if dg, ok := worker.(depsGetter); ok {
				workerDeps := dg.GetDependenciesAny()
				if setter, ok := workerDeps.(interface{ SetFrameworkState(*deps.FrameworkMetrics) }); ok {
					setter.SetFrameworkState(fm)
				}
			}
		},
		// Called BEFORE CollectObservedState to drain action history buffer.
		ActionHistoryProvider: func() []deps.ActionResult {
			if workerCtx.actionHistory == nil {
				return nil
			}

			return workerCtx.actionHistory.Drain()
		},
		ActionHistorySetter: func(history []deps.ActionResult) {
			type depsGetter interface {
				GetDependenciesAny() any
			}
			if dg, ok := worker.(depsGetter); ok {
				workerDeps := dg.GetDependenciesAny()
				if setter, ok := workerDeps.(interface{ SetActionHistory([]deps.ActionResult) }); ok {
					setter.SetActionHistory(history)
				}
			}
		},
		DesiredStateProvider: func() (fsmv2.DesiredState, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			var desired TDesired
			if err := s.store.LoadDesiredTyped(ctx, s.workerType, identity.ID, &desired); err != nil {
				// ErrNotFound is expected on first boot before the initial desired state
				// has been written to CSE storage. Return ErrNoDesiredState to signal
				// the collector to skip collection without flooding Sentry with expected
				// startup noise (persistence.ErrNotFound is the store-level sentinel).
				if errors.Is(err, persistence.ErrNotFound) {
					return nil, fsmv2.ErrNoDesiredState
				}

				s.logger.SentryWarn(deps.FeatureFSMv2, identity.HierarchyPath, "desired_state_load_failed",
					deps.Err(err))

				return nil, err
			}

			return desired, nil
		},
	})
}

// newWorkerContext builds the executor, the action history and the worker
// context around them.
// Caller must hold s.mu.
func (s *Supervisor[TObserved, TDesired]) newWorkerContext(worker fsmv2.Worker, identity deps.Identity, workerLogger deps.FSMLogger, startupCount int64) *WorkerContext[TObserved, TDesired] {
	executor := execution.NewActionExecutor(10, s.workerType, identity, workerLogger)

	actionHistoryBuffer := deps.NewInMemoryActionHistoryRecorder()

	initialState := worker.GetInitialState()

	// lastLifecyclePhase starts at the worker's initial phase so
	// GetLifecyclePhase reports it before the first tick runs, not the
	// PhaseUnknown zero value.
	var initialPhase config.LifecyclePhase
	if initialState != nil {
		initialPhase = initialState.LifecyclePhase()
	}

	workerCtx := &WorkerContext[TObserved, TDesired]{
		mu:                 s.lockManager.NewLock(lockNameWorkerContextMu, lockLevelWorkerContextMu),
		identity:           identity,
		worker:             worker,
		currentState:       initialState,
		currentStateReason: "initial",
		lastLifecyclePhase: initialPhase,
		executor:           executor,
		actionHistory:      actionHistoryBuffer,
		stateEnteredAt:     time.Now(),
		stateTransitions:   make(map[string]int64),
		stateDurations:     make(map[string]time.Duration),
		totalTransitions:   0,
		collectorRestarts:  0,
		startupCount:       startupCount,
	}

	executor.SetOnActionComplete(func(result deps.ActionResult) {
		actionHistoryBuffer.Record(result)

		// This eliminates the delay between action and FSM progression.
		if workerCtx.collector.IsRunning() {
			workerCtx.collector.TriggerNow()
		}
	})

	return workerCtx
}

// registerWorker puts a built worker context into the supervisor's registry.
// Caller must hold s.mu.
func (s *Supervisor[TObserved, TDesired]) registerWorker(workerCtx *WorkerContext[TObserved, TDesired], identity deps.Identity, workerLogger deps.FSMLogger) {
	s.workers[identity.ID] = workerCtx

	// Cache the first worker ID for lock-free access in GetHierarchyPathUnlocked()
	if s.cachedFirstWorkerID.Load() == nil {
		s.cachedFirstWorkerID.Store(identity.ID)
	}

	if len(s.workers) == 1 && identity.HierarchyPath != "" {
		s.logger = workerLogger
	}

	s.logger.Info("worker_added")
}

// stopWorker tears down a worker that just left the registry, deleting the
// state-duration series of the worker's final state.
func (s *Supervisor[TObserved, TDesired]) stopWorker(
	ctx context.Context,
	workerCtx *WorkerContext[TObserved, TDesired],
) {
	workerCtx.collector.Stop(ctx)
	workerCtx.executor.Shutdown()

	workerCtx.mu.RLock()

	if workerCtx.currentState != nil {
		metrics.CleanupStateDuration(s.GetHierarchyPathUnlocked(), workerCtx.currentState.String())
	}

	workerCtx.mu.RUnlock()
}

// RemoveWorkerForRestart removes a worker from the registry without
// tombstoning its documents. To remove a worker for good, use
// fsmv2.SignalNeedsRemoval.
func (s *Supervisor[TObserved, TDesired]) RemoveWorkerForRestart(ctx context.Context, workerID string) error {
	s.mu.Lock()

	hierarchyPath := s.GetHierarchyPathUnlocked()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		s.mu.Unlock()

		s.logger.SentryWarn(deps.FeatureFSMv2, hierarchyPath, "worker_remove_not_found",
			deps.String("target_worker_id", workerID))

		return errors.New("worker not found")
	}

	delete(s.workers, workerID)
	s.mu.Unlock()

	s.stopWorker(ctx, workerCtx)

	s.logger.Info("worker_removed")

	return nil
}

// GetWorker returns the worker context for the given ID.
func (s *Supervisor[TObserved, TDesired]) GetWorker(workerID string) (*WorkerContext[TObserved, TDesired], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx, exists := s.workers[workerID]
	if !exists {
		return nil, errors.New("worker not found")
	}

	return ctx, nil
}

// ListWorkers returns all worker IDs currently managed by this supervisor.
func (s *Supervisor[TObserved, TDesired]) ListWorkers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.workers))
	for id := range s.workers {
		ids = append(ids, id)
	}

	return ids
}

// SetGlobalVariables sets the global variables for this supervisor.
// Global variables come from the management system and are fleet-wide settings.
// They are injected into UserSpec.Variables.Global before DeriveDesiredState() is called.
func (s *Supervisor[TObserved, TDesired]) SetGlobalVariables(vars map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.globalVars = vars
}

// GetWorkers returns the identity of each worker currently managed by this supervisor.
func (s *Supervisor[TObserved, TDesired]) GetWorkers() []deps.Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workers := make([]deps.Identity, 0, len(s.workers))
	for id, worker := range s.workers {
		if worker == nil {
			continue
		}

		workers = append(workers, deps.Identity{
			ID:            id,
			Name:          worker.identity.Name,
			WorkerType:    s.workerType,
			HierarchyPath: worker.identity.HierarchyPath,
		})
	}

	return workers
}

// GetCurrentState returns the current state of the first worker.
//
// Deprecated: Use GetCurrentStateName() for interface compatibility, or
// GetWorkerState(workerID) for full state information including reason.
// This method will be removed in a future version.
func (s *Supervisor[TObserved, TDesired]) GetCurrentState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()
		state := workerCtx.currentState.String()
		workerCtx.mu.RUnlock()

		return state
	}

	return "no workers"
}

// GetWorkerState returns the current state name and reason for a worker.
// This method is thread-safe and can be safely called concurrently with tick operations.
// Returns "Unknown" state with reason "current state is nil" if the worker's state is nil.
// Returns an error if the worker is not found.
func (s *Supervisor[TObserved, TDesired]) GetWorkerState(workerID string) (string, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workerCtx, exists := s.workers[workerID]
	if !exists {
		return "", "", errors.New("worker not found")
	}

	workerCtx.mu.RLock()
	defer workerCtx.mu.RUnlock()

	if workerCtx.currentState == nil {
		return "Unknown", "current state is nil", nil
	}

	return workerCtx.currentState.String(), workerCtx.currentStateReason, nil
}

// isCircuitOpen returns true if any circuit breaker is open for this supervisor.
// Used by InfrastructureHealthChecker.CheckChildConsistency() to detect unhealthy children.
func (s *Supervisor[TObserved, TDesired]) isCircuitOpen() bool {
	return s.IsCircuitOpen()
}

// IsCircuitOpen implements SupervisorInterface.
// Returns true if any circuit breaker is open (infrastructure failure or repeated panics).
// Used by ChildInfo to report health status to parents.
func (s *Supervisor[TObserved, TDesired]) IsCircuitOpen() bool {
	return s.circuitOpen.Load() || s.panicCircuitOpen.Load()
}

// IsObservationStale implements SupervisorInterface.
// Returns true if the last observation is older than the stale threshold.
// Used by ChildInfo to report infrastructure status to parents.
func (s *Supervisor[TObserved, TDesired]) IsObservationStale() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		if workerCtx.collector == nil {
			return true
		}

		workerCtx.mu.RLock()
		lastCollectedAt := workerCtx.lastObservationCollectedAt
		workerCtx.mu.RUnlock()

		if lastCollectedAt.IsZero() {
			return true // Never collected = stale
		}

		age := time.Since(lastCollectedAt)

		return age > s.collectorHealth.staleThreshold
	}

	return true
}

// updateUserSpec implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) updateUserSpec(spec config.UserSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.userSpec = spec
}

// getUserSpec implements SupervisorInterface.
// Returns a deep copy to prevent callers from modifying internal state.
func (s *Supervisor[TObserved, TDesired]) getUserSpec() config.UserSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.userSpec.Clone()
}

// setParent implements SupervisorInterface.
func (s *Supervisor[TObserved, TDesired]) setParent(parent SupervisorInterface, parentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.parent = parent
	s.parentID = parentID
}

// GetChildren returns a copy of the children map for inspection.
// This method is thread-safe and can be used in tests to verify hierarchical composition.
func (s *Supervisor[TObserved, TDesired]) GetChildren() map[string]SupervisorInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()

	children := make(map[string]SupervisorInterface, len(s.children))
	for name, child := range s.children {
		children[name] = child
	}

	return children
}

// GetHierarchyPath returns the full hierarchy path from root to this supervisor.
// Format: "workerID(workerType)/childID(childType)/..."
// Example: "scenario123(application)/parent-123(parent)/child001(child)"
// Returns "unknown(workerType)" if no workers are registered yet.
func (s *Supervisor[TObserved, TDesired]) GetHierarchyPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.GetHierarchyPathUnlocked()
}

// GetHierarchyPathUnlocked computes the hierarchy path without s.mu. Safe
// because the cached first worker ID is written once (registerWorker) and
// s.parent is written once (setParent) before the first tick; after both,
// the path is constant.
func (s *Supervisor[TObserved, TDesired]) GetHierarchyPathUnlocked() string {
	// Use cached first worker ID for lock-free access
	workerID := "unknown"
	if cached := s.cachedFirstWorkerID.Load(); cached != nil {
		workerID = cached.(string)
	}

	segment := fmt.Sprintf("%s(%s)", workerID, s.workerType)

	if s.parent == nil {
		return segment
	}

	parentPath := s.parent.GetHierarchyPathUnlocked()

	return parentPath + "/" + segment
}

// GetCurrentStateName returns the current FSM state name for this supervisor's worker.
// Returns "unknown" if no worker or state is set.
// Used by parent supervisors to track children's health status.
func (s *Supervisor[TObserved, TDesired]) GetCurrentStateName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()

		stateName := "unknown"
		if workerCtx.currentState != nil {
			stateName = workerCtx.currentState.String()
		}

		workerCtx.mu.RUnlock()

		return stateName
	}

	return "unknown"
}

// GetCurrentStateNameAndReason returns the current FSM state name and reason.
// Returns ("unknown", "") if no worker or state is set.
// Used by ChildInfo to populate StateReason field for parent workers.
func (s *Supervisor[TObserved, TDesired]) GetCurrentStateNameAndReason() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()

		stateName := "unknown"
		stateReason := ""

		if workerCtx.currentState != nil {
			stateName = workerCtx.currentState.String()
			stateReason = workerCtx.currentStateReason
		}

		workerCtx.mu.RUnlock()

		return stateName, stateReason
	}

	return "unknown", ""
}

// GetObservedStateName returns the observed state's State field (e.g., "running_healthy_connected").
// Use config.ParseLifecyclePhase() to convert this to a LifecyclePhase for health checks.
// Returns "unknown" if no observation has been collected yet.
// Used by parent supervisors to determine child health based on lifecycle phase.
func (s *Supervisor[TObserved, TDesired]) GetObservedStateName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()
		stateName := workerCtx.lastObservedStateName
		workerCtx.mu.RUnlock()

		if stateName == "" {
			return "unknown"
		}

		return stateName
	}

	return "unknown"
}

// GetLifecyclePhase returns the lifecycle phase of the current state.
// Used by parent supervisors to classify child health via phase.IsHealthy().
// Returns PhaseUnknown if no state is set or no workers are registered.
func (s *Supervisor[TObserved, TDesired]) GetLifecyclePhase() config.LifecyclePhase {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()
		phase := workerCtx.lastLifecyclePhase
		workerCtx.mu.RUnlock()

		return phase
	}

	return config.PhaseUnknown
}

// GetWorkerType returns the type of workers this supervisor manages.
// For example: "examplechild", "exampleparent", "application".
func (s *Supervisor[TObserved, TDesired]) GetWorkerType() string {
	return s.workerType
}

// WorkerDebugInfo contains debug information for a single worker.
type WorkerDebugInfo struct {
	StateEnteredAt      time.Time `json:"state_entered_at"`
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	WorkerType          string    `json:"worker_type"`
	HierarchyPath       string    `json:"hierarchy_path"`
	State               string    `json:"state"`
	StateReason         string    `json:"state_reason"`
	TimeInCurrentStateS float64   `json:"time_in_current_state_s"`
	TotalTransitions    int64     `json:"total_transitions"`
	CollectorRestarts   int64     `json:"collector_restarts"`
	StartupCount        int64     `json:"startup_count"`
	ActionPending       bool      `json:"action_pending"`
}

// SupervisorDebugInfo contains debug information for a supervisor and its hierarchy.
type SupervisorDebugInfo struct {
	Children            map[string]SupervisorDebugInfo `json:"children,omitempty"`
	WorkerType          string                         `json:"worker_type"`
	HierarchyPath       string                         `json:"hierarchy_path"`
	Workers             []WorkerDebugInfo              `json:"workers"`
	CollectedAtUnixNano int64                          `json:"collected_at_unix_nano"`
	CircuitOpen         bool                           `json:"circuit_open"`
	PanicCircuitOpen    bool                           `json:"panic_circuit_open"`
}

// GetDebugInfo returns introspection data for debugging and monitoring.
// This method is thread-safe and provides a snapshot of the supervisor state.
// The returned data is suitable for JSON serialization and /debug/fsmv2 endpoint.
// Returns interface{} to satisfy metrics.FSMv2DebugProvider interface.
func (s *Supervisor[TObserved, TDesired]) GetDebugInfo() interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info := SupervisorDebugInfo{
		WorkerType:          s.workerType,
		HierarchyPath:       s.GetHierarchyPathUnlocked(),
		CircuitOpen:         s.circuitOpen.Load(),
		PanicCircuitOpen:    s.panicCircuitOpen.Load(),
		CollectedAtUnixNano: time.Now().UnixNano(),
		Workers:             make([]WorkerDebugInfo, 0, len(s.workers)),
	}

	for _, workerCtx := range s.workers {
		workerCtx.mu.RLock()

		workerInfo := WorkerDebugInfo{
			ID:                  workerCtx.identity.ID,
			Name:                workerCtx.identity.Name,
			WorkerType:          workerCtx.identity.WorkerType,
			HierarchyPath:       workerCtx.identity.HierarchyPath,
			State:               "unknown",
			StateReason:         workerCtx.currentStateReason,
			StateEnteredAt:      workerCtx.stateEnteredAt,
			TimeInCurrentStateS: time.Since(workerCtx.stateEnteredAt).Seconds(),
			TotalTransitions:    workerCtx.totalTransitions,
			CollectorRestarts:   workerCtx.collectorRestarts,
			StartupCount:        workerCtx.startupCount,
			ActionPending:       workerCtx.actionPending,
		}

		if workerCtx.currentState != nil {
			workerInfo.State = workerCtx.currentState.String()
		}

		workerCtx.mu.RUnlock()

		info.Workers = append(info.Workers, workerInfo)
	}

	// Recursively collect child debug info
	if len(s.children) > 0 {
		info.Children = make(map[string]SupervisorDebugInfo, len(s.children))

		for name, child := range s.children {
			// Use the interface method which returns interface{}, then type assert
			if debuggable, ok := child.(interface{ GetDebugInfo() interface{} }); ok {
				if childInfo, ok := debuggable.GetDebugInfo().(SupervisorDebugInfo); ok {
					info.Children[name] = childInfo
				}
			}
		}
	}

	return info
}
