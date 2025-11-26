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

// Package main demonstrates a simple, runnable example of FSM v2 using the application worker pattern.
// This example shows how to:
// 1. Create an application worker
// 2. Register a parent worker with children
// 3. Start the application worker
// 4. Run for 10 seconds with visible status output
// 5. Gracefully shut down
package main

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child"
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

func main() {
	// Use INFO level by default, set LOG_LEVEL=debug for DEBUG level
	logLevel := zap.InfoLevel
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = zap.DebugLevel
	}

	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(logLevel)
	logger, err := config.Build()
	if err != nil {
		sugar := zap.NewNop().Sugar()
		sugar.Errorf("Failed to create logger: %v", err)
		os.Exit(1)
	}

	sugar := logger.Sugar()
	defer logger.Sync()

	sugar.Info("=== FSM v2 Application Worker - Simple Example ===")
	sugar.Info("")

	sugar.Info("Step 1: Creating storage...")

	basicStore := memory.NewInMemoryStore()

	// NOTE: InMemoryStore automatically creates collections on first write.
	// No explicit CreateCollection() calls are needed. Collections will be created
	// automatically when workers insert their identity, desired, and observed state documents.

	store := storage.NewTriangularStore(basicStore, sugar)

	sugar.Info("Storage created successfully")
	sugar.Info("")

	yamlConfig := `
children:
  - name: "parent-1"
    workerType: "parent"
    userSpec:
      config: |
        children_count: 2
`

	sugar.Info("Step 2: Creating application supervisor...")
	sugar.Info("This supervisor will manage a parent worker with 2 child workers")

	// Read ENABLE_TRACE_LOGGING environment variable
	enableTraceLogging := false
	if envVal := os.Getenv("ENABLE_TRACE_LOGGING"); envVal != "" {
		if parsed, err := strconv.ParseBool(envVal); err == nil {
			enableTraceLogging = parsed
		}
	}

	if enableTraceLogging {
		sugar.Info("Trace logging ENABLED - verbose mutex/tick logs will be shown")
	} else {
		sugar.Info("Trace logging DISABLED - clean logs for normal operation")
	}

	sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
		ID:                     "app-001",
		Name:                   "Simple Application",
		Store:                  store,
		Logger:                 sugar,
		TickInterval:           100 * time.Millisecond,
		YAMLConfig:             yamlConfig,
		EnableTraceLogging: enableTraceLogging,
	})
	if err != nil {
		sugar.Errorf("Failed to create application supervisor: %v", err)
		os.Exit(1)
	}

	sugar.Info("Application supervisor created successfully")
	sugar.Info("")

	sugar.Info("Step 3: Starting the application worker...")

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := sup.Start(runCtx)

	sugar.Info("Application worker started!")
	sugar.Info("")

	sugar.Info("Step 4: Running for 10 seconds...")
	sugar.Info("The FSM is now managing the parent and child workers.")
	sugar.Info("Watch the logs above to see state transitions and reconciliation.")
	sugar.Info("")

	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-timeout:
			sugar.Info("")
			sugar.Info("Step 5: Gracefully shutting down...")
			cancel()
			<-done
			sugar.Info("Application shut down successfully!")
			sugar.Info("")
			sugar.Info("=== Example Complete ===")
			sugar.Info("You've successfully:")
			sugar.Info("  1. Created an application worker")
			sugar.Info("  2. Registered a parent worker with 2 children")
			sugar.Info("  3. Started and ran the FSM")
			sugar.Info("  4. Gracefully shut down")
			sugar.Info("")
			sugar.Info("Next steps:")
			sugar.Info("  - Check the FSM v2 documentation: pkg/fsmv2/README.md")
			sugar.Info("  - Explore example workers: pkg/fsmv2/workers/example/")
			sugar.Info("  - Build your own worker based on the patterns shown")

			return

		case <-done:
			sugar.Warn("Application stopped unexpectedly")

			return
		}
	}
}
