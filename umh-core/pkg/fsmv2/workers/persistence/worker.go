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

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
	persistencepkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"

	// Blank import registers the "persistence" initial state via
	// fsmv2.RegisterInitialState in state/state_stopped.go init().
	// GetInitialState uses the registry, so the state package must be loaded
	// whenever the worker is imported.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
)

// WorkerTypeName is the canonical worker-type identifier for the persistence worker.
const WorkerTypeName = "persistence"

const workerType = WorkerTypeName

// Re-exported defaults for caller convenience. Canonical values live with
// PersistenceConfig in the snapshot package, alongside the GetX accessors.
const (
	DefaultCompactionInterval  = snapshot.DefaultCompactionInterval
	DefaultRetentionWindow     = snapshot.DefaultRetentionWindow
	DefaultMaintenanceInterval = snapshot.DefaultMaintenanceInterval
)

// Compile-time interface check: PersistenceWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*PersistenceWorker)(nil)

// PersistenceWorker implements the FSM Worker interface for the edge persistence
// layer. It drives compaction and maintenance against the triangular store.
type PersistenceWorker struct {
	fsmv2.WorkerBase[snapshot.PersistenceConfig, snapshot.PersistenceStatus, *PersistenceDependencies]
}

// NewPersistenceWorker creates a new persistence worker.
//
// Two supported shapes for dependencies:
//
//   - seed (built via NewStoreOnlyDependencies): the constructor extracts the
//     store and builds full deps with this worker's identity/logger/stateReader.
//   - fully built (via NewPersistenceDependencies): used as-is, preserving
//     the direct-injection contract used by tests.
//
// Returns fsmv2.Worker to align with the factory constructor signature.
func NewPersistenceWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *PersistenceDependencies,
) (fsmv2.Worker, error) {
	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	w := &PersistenceWorker{}
	bd := w.InitBase(identity, logger, stateReader)

	switch {
	case dependencies == nil:
		return nil, errors.New("persistence worker requires a store; pass via NewPersistenceDependencies or NewStoreOnlyDependencies")
	case dependencies.BaseDependencies == nil:
		store := dependencies.GetStore()
		if store == nil {
			return nil, errors.New("persistence worker: seed dependencies.Store must not be nil")
		}

		dependencies = NewPersistenceDependencies(store, deps.DefaultScheduler{}, bd)
	case dependencies.GetStore() == nil:
		return nil, errors.New("persistence worker: dependencies.Store must not be nil")
	}

	w.BindDeps(dependencies)

	return w, nil
}

// GetDependencies returns the typed persistence dependencies.
// Panics with a clear message if BindDeps was not called before this worker is used.
func (w *PersistenceWorker) GetDependencies() *PersistenceDependencies {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*PersistenceDependencies)
	if !ok || d == nil {
		panic("PersistenceWorker: GetDependencies called before BindDeps")
	}

	return d
}

// CollectObservedState returns the current observed state of the persistence
// worker. Returns fsmv2.NewObservation - the collector handles CollectedAt,
// framework metrics, action history, and metric accumulation automatically
// after COS returns.
func (w *PersistenceWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	// Framework state and action history are injected before COS and consumed by
	// the collector wrapper after NewObservation returns. Calling them here satisfies
	// the framework-metrics-copy and action-history-copy invariants enforced by the
	// architecture validator.
	d.GetFrameworkState()

	var prev fsmv2.Observation[snapshot.PersistenceStatus]

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

	return fsmv2.NewObservation(snapshot.PersistenceStatus{
		LastCompactionAt:              lastCompactionAt,
		LastMaintenanceAt:             lastMaintenanceAt,
		IsPreferredMaintenanceWindow:  scheduler.IsPreferredMaintenanceWindow(now),
		IsAcceptableMaintenanceWindow: scheduler.IsAcceptableMaintenanceWindow(now),
		ConsecutiveActionErrors:       consecutiveErrors,
	}), nil
}

// DeriveDesiredState parses UserSpec.Config YAML into a typed WrappedDesiredState[PersistenceConfig].
func (w *PersistenceWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[snapshot.PersistenceConfig]{
			State: fsmv2config.DesiredStateRunning,
			Config: snapshot.PersistenceConfig{
				CompactionInterval:  DefaultCompactionInterval,
				RetentionWindow:     DefaultRetentionWindow,
				MaintenanceInterval: DefaultMaintenanceInterval,
			},
		}, nil
	}

	userSpec, ok := spec.(fsmv2config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	renderedConfig, err := fsmv2config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	var cfg snapshot.PersistenceConfig
	if renderedConfig != "" {
		if err := yaml.Unmarshal([]byte(renderedConfig), &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse persistence config: %w", err)
		}
	}

	if cfg.CompactionInterval == 0 {
		cfg.CompactionInterval = DefaultCompactionInterval
	}

	if cfg.RetentionWindow == 0 {
		cfg.RetentionWindow = DefaultRetentionWindow
	}

	if cfg.MaintenanceInterval == 0 {
		cfg.MaintenanceInterval = DefaultMaintenanceInterval
	}

	state := cfg.GetState()

	return &fsmv2.WrappedDesiredState[snapshot.PersistenceConfig]{
		State:  state,
		Config: cfg,
	}, nil
}

func init() {
	register.Worker[snapshot.PersistenceConfig, snapshot.PersistenceStatus, *PersistenceDependencies](WorkerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			d := register.GetDeps[*PersistenceDependencies](WorkerTypeName)
			return NewPersistenceWorker(id, logger, sr, d)
		})
}
