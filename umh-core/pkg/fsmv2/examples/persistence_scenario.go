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

package examples

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	persistencesnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
)

type PersistenceRunConfig struct {
	Logger       deps.FSMLogger
	Duration     time.Duration
	TickInterval time.Duration
}

type PersistenceRunResult struct {
	Done              <-chan struct{}
	Shutdown          func()
	Error             error
	LastCompactionAt  time.Time
	LastMaintenanceAt time.Time
	Healthy           bool
	CompactionCycles  int64
	MaintenanceCycles int64
}

func RunPersistenceScenario(ctx context.Context, cfg PersistenceRunConfig) *PersistenceRunResult {
	done := make(chan struct{})

	if cfg.Duration < 0 {
		close(done)

		return &PersistenceRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("invalid duration %v: must be non-negative", cfg.Duration),
		}
	}

	if ctx.Err() != nil {
		close(done)

		return &PersistenceRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("context already cancelled: %w", ctx.Err()),
		}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = deps.NewNopFSMLogger()
	}

	tickInterval := cfg.TickInterval
	if tickInterval < 0 {
		close(done)

		return &PersistenceRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("invalid tick interval %v: must be non-negative", cfg.TickInterval),
		}
	}
	if tickInterval == 0 {
		tickInterval = 100 * time.Millisecond
	}

	store := SetupStore(logger)

	yamlConfig := `
children:
  - name: "persistence"
    workerType: "persistence"
`

	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:           "scenario-persistence",
		Name:         "persistence",
		Store:        store,
		Logger:       logger,
		TickInterval: tickInterval,
		YAMLConfig:   yamlConfig,
		Dependencies: map[string]any{
			"store": store,
		},
	})
	if err != nil {
		close(done)

		return &PersistenceRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("failed to create supervisor: %w", err),
		}
	}

	supDone := appSup.Start(ctx)

	result := &PersistenceRunResult{
		Done:     done,
		Shutdown: appSup.Shutdown,
	}

	go func() {
		if cfg.Duration > 0 {
			select {
			case <-time.After(cfg.Duration):
				appSup.Shutdown()
			case <-ctx.Done():
				appSup.Shutdown()
			case <-supDone:
			}
		} else {
			select {
			case <-ctx.Done():
				appSup.Shutdown()
			case <-supDone:
			}
		}

		<-supDone

		loadCtx := context.Background()

		var observed persistencesnapshot.PersistenceObservedState
		if loadErr := store.LoadObservedTyped(loadCtx, "persistence", "persistence-001", &observed); loadErr != nil {
			if !errors.Is(loadErr, context.Canceled) {
				logger.SentryWarn(deps.FeatureExamples, "", "failed to load persistence observed state",
					deps.Err(loadErr))
			}
		} else {
			workerMetrics := observed.GetWorkerMetrics()
			result.CompactionCycles = workerMetrics.Counters[string(deps.CounterCompactionCyclesTotal)]
			result.MaintenanceCycles = workerMetrics.Counters[string(deps.CounterMaintenanceCyclesTotal)]
			result.LastCompactionAt = observed.LastCompactionAt
			result.LastMaintenanceAt = observed.LastMaintenanceAt
			result.Healthy = observed.IsHealthy()
		}

		close(done)
	}()

	return result
}
