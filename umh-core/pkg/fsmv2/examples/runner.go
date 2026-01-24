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
	"fmt"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"

	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
)

// RunResult contains the result of running a scenario.
type RunResult struct {
	Done     <-chan struct{} // Closes when the supervisor stops
	Shutdown func()          // Initiates graceful shutdown of the supervisor
}

// Run executes a scenario with the given configuration.
//
// Creates an ApplicationSupervisor with the scenario's YAML config, or delegates
// to CustomRunner if set. If DumpStore is enabled, outputs store changes summary.
func Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	hasYAML := cfg.Scenario.YAMLConfig != ""
	hasCustom := cfg.Scenario.CustomRunner != nil

	if !hasYAML && !hasCustom {
		return nil, fmt.Errorf("scenario %q is not properly configured: "+
			"neither YAMLConfig nor CustomRunner is set", cfg.Scenario.Name)
	}

	if hasYAML && hasCustom {
		return nil, fmt.Errorf("scenario %q has conflicting configuration: "+
			"both YAMLConfig and CustomRunner are set (only one allowed)", cfg.Scenario.Name)
	}

	if hasCustom {
		return cfg.Scenario.CustomRunner(ctx, cfg)
	}

	var startSyncID int64
	if cfg.DumpStore {
		startSyncID, _ = cfg.Store.GetLatestSyncID(ctx)
	}

	appSup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:                 "scenario-" + cfg.Scenario.Name,
		Name:               cfg.Scenario.Name,
		Store:              cfg.Store,
		Logger:             cfg.Logger,
		TickInterval:       cfg.TickInterval,
		YAMLConfig:         cfg.Scenario.YAMLConfig,
		EnableTraceLogging: cfg.EnableTraceLogging,
	})
	if err != nil {
		return nil, err
	}

	done := appSup.Start(ctx)

	shutdownFn := func() {
		appSup.Shutdown()
	}

	if cfg.DumpStore {
		wrappedDone := make(chan struct{})

		go func() {
			<-done
			dumpCtx := context.Background()

			dump, err := DumpScenario(dumpCtx, cfg.Store, startSyncID)
			if err != nil {
				cfg.Logger.Warnw("Failed to dump scenario", "error", err)
			} else {
				fmt.Print(dump.FormatHuman())
			}

			close(wrappedDone)
		}()

		return &RunResult{Done: wrappedDone, Shutdown: shutdownFn}, nil
	}

	return &RunResult{Done: done, Shutdown: shutdownFn}, nil
}

// SetupStore creates an in-memory TriangularStore for testing and CLI usage.
func SetupStore(logger *zap.SugaredLogger) storage.TriangularStoreInterface {
	basicStore := memory.NewInMemoryStore()

	return storage.NewTriangularStore(basicStore, logger)
}
