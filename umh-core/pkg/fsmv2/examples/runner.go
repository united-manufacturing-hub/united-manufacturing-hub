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

	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
)

// RunResult contains the result of running a scenario.
type RunResult struct {
	// Done channel that closes when the supervisor stops
	Done <-chan struct{}

	// Shutdown initiates graceful shutdown of the supervisor.
	// This first requests shutdown on all workers (sets ShutdownRequested=true),
	// waits for workers to complete their FSM shutdown transitions,
	// then cancels the supervisor context.
	Shutdown func()
}

// Run executes a scenario with the given configuration.
//
// This function creates an ApplicationSupervisor with the scenario's YAML config
// and starts it. The supervisor runs until the context is cancelled or the
// specified duration elapses.
//
// If DumpStore is enabled, outputs a human-readable summary of all store changes
// and final worker states after the scenario completes.
//
// Parameters:
//   - ctx: Context for cancellation
//   - cfg: RunConfig with scenario, duration, tick interval, logger, store, and trace logging options
//
// Returns:
//   - *RunResult: Contains done channel and shutdown function
//   - error: If supervisor creation fails
//
// Example:
//
//	result, err := examples.Run(ctx, examples.RunConfig{
//	    Scenario:     examples.SimpleScenario,
//	    Duration:     10 * time.Second,
//	    TickInterval: 100 * time.Millisecond,
//	    Logger:       logger,
//	    Store:        store,
//	})
//	// On signal, call result.Shutdown() for graceful shutdown
func Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	// Capture start syncID before running scenario (for dump)
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

	// Create shutdown function that initiates graceful shutdown
	shutdownFn := func() {
		appSup.Shutdown()
	}

	// If dump is enabled, wrap the done channel to dump on completion
	if cfg.DumpStore {
		wrappedDone := make(chan struct{})
		go func() {
			<-done
			// Use background context for dump since original ctx may be cancelled
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
//
// This is a convenience function that creates a memory-backed store,
// useful for scenarios that don't require persistent storage.
//
// Parameters:
//   - logger: Logger for store operations (use zap.NewNop().Sugar() for silent operation)
//
// Returns:
//   - storage.TriangularStoreInterface: In-memory store ready for use
//
// Example:
//
//	logger := zap.NewDevelopment().Sugar()
//	store := examples.SetupStore(logger)
//	defer store.Close()
func SetupStore(logger *zap.SugaredLogger) storage.TriangularStoreInterface {
	basicStore := memory.NewInMemoryStore()
	return storage.NewTriangularStore(basicStore, logger)
}
