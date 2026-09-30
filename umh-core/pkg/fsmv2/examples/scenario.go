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

// Package examples provides scenario definitions and a shared runner for FSM v2 integration testing.
//
// # Architecture
//
// This package implements the scenario registry pattern, allowing the same scenarios to be used by:
//   - Integration tests (with log capture and assertions)
//   - CLI runner (for manual verification and debugging)
//   - Future stress tests and failure injection tests
//
// # Usage
//
// Tests use scenarios via the Run function:
//
//	result, err := examples.Run(ctx, examples.RunConfig{
//	    ScenarioV2:  examples.RegistryV2["helloworld"],
//	    Duration:    10 * time.Second,
//	    TickInterval: 100 * time.Millisecond,
//	    Logger:      testLogger,
//	    Store:       store,
//	})
//
// CLI uses the same scenarios via pkg/fsmv2/cmd/runner.
package examples

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// RunConfig configures how a scenario is executed.
type RunConfig struct {
	Store        storage.TriangularStoreInterface
	Logger       deps.FSMLogger
	ScenarioV2   ScenarioV2    // When ScenarioV2.Run is set, examples.Run takes the v2 kernel-only path
	Duration     time.Duration // 0 means run forever (until context cancelled)
	TickInterval time.Duration
	// GracefulShutdownTimeout is the per-level drain base propagated to the
	// supervisor subtree. Zero falls back to the supervisor default (5s). A
	// tiny value forces a degraded drain, which is how tests exercise the
	// ShutdownClean=false path deterministically.
	GracefulShutdownTimeout time.Duration
	EnableTraceLogging      bool
	// DumpStore prints the store's changes and final state to stdout after
	// the scenario run.
	DumpStore bool
}

// ListScenarios returns all registered scenario names and descriptions,
// merging every registry into one listing.
func ListScenarios() map[string]string {
	result := make(map[string]string, len(RegistryV2)+len(LiveRegistryV2))

	for name, scenario := range RegistryV2 {
		result[name] = scenario.Description
	}

	for name, scenario := range LiveRegistryV2 {
		result[name] = scenario.Description
	}

	return result
}
