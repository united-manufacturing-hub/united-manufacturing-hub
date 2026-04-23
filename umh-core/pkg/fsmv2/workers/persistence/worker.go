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

package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	persistencepkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"

	// Blank import for side effects: registers the "Stopped" initial state
	// via fsmv2.RegisterInitialState in state/stopped.go init(). WorkerBase's
	// GetInitialState looks up the state from the registry, so the state
	// package must be loaded whenever the worker is imported — otherwise the
	// registry lookup returns nil and the supervisor panics at first tick.
	// This import is safe because state/ depends on snapshot/ (not on the
	// worker package), so no import cycle is introduced.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
)

const workerType = "persistence"

const (
	DefaultCompactionInterval  = 5 * time.Minute
	DefaultRetentionWindow     = 1 * time.Hour
	DefaultMaintenanceInterval = 7 * 24 * time.Hour
)

// Compile-time interface check: PersistenceWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*PersistenceWorker)(nil)

// PersistenceWorker implements the FSM Worker interface for the edge persistence
// layer. It drives compaction and maintenance against the triangular store.
type PersistenceWorker struct {
	fsmv2.WorkerBase[PersistenceConfig, PersistenceStatus]
	deps *PersistenceDependencies
}

// NewPersistenceWorker creates a new persistence worker. If the typed
// dependencies argument is non-nil, it is used as-is (its Store field must be
// non-nil). When nil, the constructor builds default dependencies from the
// package-level Store() singleton populated by cmd/main.go via SetStore. This
// replaces the prior extraDeps["store"] seam with a typed plumbing path.
// Returns fsmv2.Worker to align with the constructor signature used by
// transport/push/pull.
func NewPersistenceWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *PersistenceDependencies,
) (fsmv2.Worker, error) {
	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	if dependencies == nil {
		store := Store()
		if store == nil {
			return nil, errors.New("persistence worker requires a store via persistence.SetStore(); cmd/main.go must publish the store before the application supervisor starts")
		}

		dependencies = NewPersistenceDependencies(store, deps.DefaultScheduler{}, logger, stateReader, identity)
	} else if dependencies.GetStore() == nil {
		return nil, errors.New("persistence worker: dependencies.Store must not be nil")
	}

	w := &PersistenceWorker{deps: dependencies}
	w.InitBase(identity, logger, stateReader)

	// Apply persistence-specific defaults after config parsing. Zero values in
	// the parsed config (no user-supplied value) are replaced with package
	// defaults so the rest of the worker can rely on non-zero intervals.
	w.SetPostParseHook(func(cfg *PersistenceConfig) error {
		if cfg.CompactionInterval == 0 {
			cfg.CompactionInterval = DefaultCompactionInterval
		}
		if cfg.RetentionWindow == 0 {
			cfg.RetentionWindow = DefaultRetentionWindow
		}
		if cfg.MaintenanceInterval == 0 {
			cfg.MaintenanceInterval = DefaultMaintenanceInterval
		}
		return nil
	})

	return w, nil
}

// GetDependencies returns the typed persistence dependencies.
// Used by tests and by external callers that need to observe worker state.
func (w *PersistenceWorker) GetDependencies() *PersistenceDependencies {
	return w.deps
}

// GetDependenciesAny returns the custom PersistenceDependencies.
// Overrides WorkerBase's default which returns *BaseDependencies.
// Required by architecture test: custom deps must be visible to the supervisor.
func (w *PersistenceWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState returns the current observed state of the persistence
// worker. Returns fsmv2.NewObservation — the collector handles CollectedAt,
// framework metrics, action history, and metric accumulation automatically
// after COS returns.
func (w *PersistenceWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.deps

	var prev fsmv2.Observation[PersistenceStatus]

	stateReader := d.GetStateReader()
	if stateReader != nil {
		if err := stateReader.LoadObservedTyped(ctx, d.GetWorkerType(), d.GetWorkerID(), &prev); err == nil {
			d.SetObservedStateLoaded()
		} else if errors.Is(err, persistencepkg.ErrNotFound) && !d.IsObservedStateLoaded() {
			d.GetLogger().Debug("no previous observed state found, using zero-value defaults")
		} else {
			d.GetLogger().SentryWarn(deps.FeatureForWorker(d.GetWorkerType()), d.GetHierarchyPath(), "previous_observed_load_failed",
				deps.Err(err),
				deps.String("worker_type", d.GetWorkerType()),
				deps.String("worker_id", d.GetWorkerID()))
		}
	}

	lastCompactionAt := d.GetLastCompactionAt()
	if lastCompactionAt.IsZero() {
		lastCompactionAt = prev.Status.LastCompactionAt
	}

	lastMaintenanceAt := d.GetLastMaintenanceAt()
	if lastMaintenanceAt.IsZero() {
		lastMaintenanceAt = prev.Status.LastMaintenanceAt
	}

	actionResults := d.GetActionHistory()

	consecutiveErrors := prev.Status.ConsecutiveActionErrors

	for _, result := range actionResults {
		if result.Success {
			consecutiveErrors = 0
		} else {
			consecutiveErrors++
		}
	}

	now := time.Now()
	scheduler := d.GetScheduler()

	return fsmv2.NewObservation(PersistenceStatus{
		LastCompactionAt:              lastCompactionAt,
		LastMaintenanceAt:             lastMaintenanceAt,
		IsPreferredMaintenanceWindow:  scheduler.IsPreferredMaintenanceWindow(now),
		IsAcceptableMaintenanceWindow: scheduler.IsAcceptableMaintenanceWindow(now),
		ConsecutiveActionErrors:       consecutiveErrors,
	}), nil
}

// init registers the persistence worker via factory.RegisterWorkerAndSupervisorFactoryByType.
// The factory closure obtains the triangular store from the persistence.Store()
// package-level singleton (populated by cmd/main.go via SetStore), replacing
// the prior extraDeps["store"] seam with a typed plumbing path.
//
// Persistence retains the factory-based registration path (rather than
// register.Worker) until PR2 C10 completes the register.Worker migration. This
// commit is a type-shape refactor only.
func init() {
	workerFactory := func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
		worker, err := NewPersistenceWorker(id, logger, stateReader, nil)
		if err != nil {
			panic(fmt.Sprintf("failed to create persistence worker: %v", err))
		}

		return worker
	}

	supervisorFactory := func(cfg interface{}) interface{} {
		return supervisor.NewSupervisor[
			fsmv2.Observation[PersistenceStatus],
			*fsmv2.WrappedDesiredState[PersistenceConfig],
		](cfg.(supervisor.Config))
	}

	if err := factory.RegisterWorkerAndSupervisorFactoryByType(workerType, workerFactory, supervisorFactory); err != nil {
		panic(fmt.Sprintf("failed to register persistence worker: %v", err))
	}
}
