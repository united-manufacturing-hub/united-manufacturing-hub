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
	"fmt"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
)

const (
	DefaultCompactionInterval  = 5 * time.Minute
	DefaultRetentionWindow     = 24 * time.Hour
	DefaultMaintenanceInterval = 7 * 24 * time.Hour
)

type PersistenceWorker struct {
	*helpers.BaseWorker[*PersistenceDependencies]
}

func NewPersistenceWorker(
	identity deps.Identity,
	store storage.TriangularStoreInterface,
	logger *zap.SugaredLogger,
	stateReader deps.StateReader,
) (*PersistenceWorker, error) {
	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.PersistenceObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	d := NewPersistenceDependencies(store, logger, stateReader, identity)

	return &PersistenceWorker{
		BaseWorker: helpers.NewBaseWorker(d),
	}, nil
}

func (w *PersistenceWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
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
		} else {
			d.GetLogger().Warnw("previous_observed_load_failed",
				"error", err,
				"worker_type", d.GetWorkerType(),
				"worker_id", d.GetWorkerID())
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

	observed := snapshot.PersistenceObservedState{
		CollectedAt:             time.Now(),
		LastCompactionAt:        lastCompactionAt,
		LastMaintenanceAt:       lastMaintenanceAt,
		ConsecutiveActionErrors: consecutiveErrors,
		LastActionResults:       actionResults,
		MetricsEmbedder:        deps.MetricsEmbedder{Metrics: metricsContainer},
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

func init() {
	if err := factory.RegisterWorkerType[snapshot.PersistenceObservedState, *snapshot.PersistenceDesiredState](
		func(id deps.Identity, logger *zap.SugaredLogger, stateReader deps.StateReader, params map[string]any) fsmv2.Worker {
			store, ok := params["store"].(storage.TriangularStoreInterface)
			if !ok || store == nil {
				panic("persistence worker factory: 'store' parameter must be a TriangularStoreInterface")
			}

			worker, err := NewPersistenceWorker(id, store, logger, stateReader)
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
