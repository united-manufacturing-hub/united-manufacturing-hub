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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
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

// WorkerTypeName is the canonical worker-type identifier for the persistence
// worker, used in config YAML, CSE storage, and the register.SetDeps key at
// cmd/main.go.
const WorkerTypeName = "persistence"

const workerType = WorkerTypeName

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

// NewPersistenceWorker creates a new persistence worker. The dependencies
// parameter carries the triangular store published by cmd/main.go via
// register.SetDeps; the register.Worker factory closure forwards it here.
//
// Three supported shapes for dependencies:
//
//   - nil: the constructor falls back to the persistence.Store() singleton for
//     backward compatibility until C13 removes the singleton. Errors if the
//     singleton is also unset.
//   - seed (built via NewStoreOnlyDependencies): the constructor extracts the
//     store and builds full deps with this worker's identity/logger/stateReader.
//     This is the path taken by cmd/main.go → register.SetDeps → factory closure.
//   - fully built (via NewPersistenceDependencies): the constructor uses the
//     value as-is, preserving the pre-existing direct-injection contract used
//     by tests.
//
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

	switch {
	case dependencies == nil:
		store := Store()
		if store == nil {
			return nil, errors.New("persistence worker requires a store via register.SetDeps or persistence.SetStore(); cmd/main.go must publish the store before the application supervisor starts")
		}

		dependencies = NewPersistenceDependencies(store, deps.DefaultScheduler{}, logger, stateReader, identity)
	case dependencies.BaseDependencies == nil:
		store := dependencies.GetStore()
		if store == nil {
			return nil, errors.New("persistence worker: seed dependencies.Store must not be nil")
		}

		dependencies = NewPersistenceDependencies(store, deps.DefaultScheduler{}, logger, stateReader, identity)
	case dependencies.GetStore() == nil:
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

// init registers the persistence worker via the generic register.Worker helper
// with typed TDeps = *PersistenceDependencies. Parent wiring at cmd/main.go
// publishes the store via register.SetDeps[*PersistenceDependencies] before
// the application supervisor starts; the factory closure then forwards the
// seed deps to NewPersistenceWorker. When nothing has been published (e.g.
// older test paths), the constructor falls back to the persistence.Store()
// singleton — preserving backward compatibility until C13 removes the
// singleton entirely.
func init() {
	register.Worker[PersistenceConfig, PersistenceStatus, *PersistenceDependencies](WorkerTypeName, NewPersistenceWorker)
}
