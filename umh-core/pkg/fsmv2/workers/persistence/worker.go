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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
	persistencepkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

const workerType = "persistence"

const (
	DefaultCompactionInterval  = 5 * time.Minute
	DefaultRetentionWindow     = 1 * time.Hour
	DefaultMaintenanceInterval = 7 * 24 * time.Hour
)

type PersistenceWorker struct {
	*helpers.BaseWorker[*PersistenceDependencies]
}

// NewPersistenceWorker creates a new persistence worker. If the typed
// dependencies argument is non-nil, it is used as-is (its Store field must be
// non-nil). When nil, the constructor builds default dependencies from the
// package-level Store() singleton populated by cmd/main.go via SetStore. This
// replaces the prior extraDeps["store"] seam with a typed plumbing path.
// Returns fsmv2.Worker to align with the register.Worker constructor
// signature used by transport/push/pull.
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

	return &PersistenceWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
	}, nil
}

func (w *PersistenceWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	var prev snapshot.PersistenceObservedState

	var prevWorkerMetrics deps.Metrics

	stateReader := d.GetStateReader()
	if stateReader != nil {
		if err := stateReader.LoadObservedTyped(ctx, d.GetWorkerType(), d.GetWorkerID(), &prev); err == nil {
			prevWorkerMetrics = prev.Metrics.Worker

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
		lastCompactionAt = prev.LastCompactionAt
	}

	lastMaintenanceAt := d.GetLastMaintenanceAt()
	if lastMaintenanceAt.IsZero() {
		lastMaintenanceAt = prev.LastMaintenanceAt
	}

	newWorkerMetrics := prevWorkerMetrics
	if newWorkerMetrics.Counters == nil {
		newWorkerMetrics.Counters = make(map[string]int64)
	}

	if newWorkerMetrics.Gauges == nil {
		newWorkerMetrics.Gauges = make(map[string]float64)
	}

	tickMetrics := d.MetricsRecorder().Drain()

	for name, delta := range tickMetrics.Counters {
		newWorkerMetrics.Counters[name] += delta
	}

	for name, value := range tickMetrics.Gauges {
		newWorkerMetrics.Gauges[name] = value
	}

	metricsContainer := deps.MetricsContainer{
		Worker: newWorkerMetrics,
	}

	if fm := d.GetFrameworkState(); fm != nil {
		metricsContainer.Framework = *fm
	}

	actionResults := d.GetActionHistory()

	consecutiveErrors := prev.ConsecutiveActionErrors

	for _, result := range actionResults {
		if result.Success {
			consecutiveErrors = 0
		} else {
			consecutiveErrors++
		}
	}

	now := time.Now()
	scheduler := d.GetScheduler()

	observed := snapshot.PersistenceObservedState{
		CollectedAt:                   now,
		LastCompactionAt:              lastCompactionAt,
		LastMaintenanceAt:             lastMaintenanceAt,
		IsPreferredMaintenanceWindow:  scheduler.IsPreferredMaintenanceWindow(now),
		IsAcceptableMaintenanceWindow: scheduler.IsAcceptableMaintenanceWindow(now),
		ConsecutiveActionErrors:       consecutiveErrors,
		LastActionResults:             actionResults,
		MetricsEmbedder:               deps.MetricsEmbedder{Metrics: metricsContainer},
	}

	return observed, nil
}

func (w *PersistenceWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &snapshot.PersistenceDesiredState{
			BaseDesiredState: fsmv2config.BaseDesiredState{
				State: "running",
			},
			CompactionInterval:  DefaultCompactionInterval,
			RetentionWindow:     DefaultRetentionWindow,
			MaintenanceInterval: DefaultMaintenanceInterval,
		}, nil
	}

	userSpec, ok := spec.(fsmv2config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	var persSpec PersistenceUserSpec
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &persSpec); err != nil {
			return nil, fmt.Errorf("failed to parse persistence config: %w", err)
		}
	}

	compactionInterval := persSpec.CompactionInterval
	if compactionInterval == 0 {
		compactionInterval = DefaultCompactionInterval
	}

	retentionWindow := persSpec.RetentionWindow
	if retentionWindow == 0 {
		retentionWindow = DefaultRetentionWindow
	}

	maintenanceInterval := persSpec.MaintenanceInterval
	if maintenanceInterval == 0 {
		maintenanceInterval = DefaultMaintenanceInterval
	}

	return &snapshot.PersistenceDesiredState{
		BaseDesiredState:    fsmv2config.BaseDesiredState{State: persSpec.GetState()},
		CompactionInterval:  compactionInterval,
		RetentionWindow:     retentionWindow,
		MaintenanceInterval: maintenanceInterval,
	}, nil
}

func (w *PersistenceWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

// init registers the persistence worker via factory.RegisterWorkerType. The
// factory closure obtains the triangular store from the persistence.Store()
// package-level singleton (populated by cmd/main.go via SetStore), replacing
// the prior extraDeps["store"] seam with a typed plumbing path. Persistence
// retains the legacy 2-generic factory registration rather than register.Worker
// because its CSE types (PersistenceObservedState / PersistenceDesiredState)
// are custom and not the Observation[Status] / WrappedDesiredState[Config]
// pair that register.Worker hardcodes.
func init() {
	if err := factory.RegisterWorkerType[snapshot.PersistenceObservedState, *snapshot.PersistenceDesiredState](
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			worker, err := NewPersistenceWorker(id, logger, stateReader, nil)
			if err != nil {
				panic(fmt.Sprintf("failed to create persistence worker: %v", err))
			}

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.PersistenceObservedState, *snapshot.PersistenceDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register persistence worker: %v", err))
	}
}
