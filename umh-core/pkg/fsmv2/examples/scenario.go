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
// This package implements the Scenario Registry Pattern, allowing the same scenarios to be used by:
//   - Integration tests (with log capture and assertions)
//   - CLI runner (for manual verification and debugging)
//   - Future stress tests and failure injection tests
//
// # Usage
//
// Tests use scenarios via the Run function:
//
//	done, err := examples.Run(ctx, examples.RunConfig{
//	    Scenario:     examples.SimpleScenario,
//	    Duration:     10 * time.Second,
//	    TickInterval: 100 * time.Millisecond,
//	    Logger:       testLogger,
//	    Store:        store,
//	})
//
// CLI uses the same scenarios via pkg/fsmv2/cmd/runner.
//
// # Adding New Scenarios
//
// To add a new scenario:
//  1. Create a new file (e.g., stress.go) with your Scenario definition
//  2. Register it in the Registry map in this file
//  3. Reference simple.go for documentation on available options
package examples

import (
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

// Scenario defines a test scenario configuration.
//
// Scenarios specify:
//   - Name: identifier used in CLI (--scenario=simple)
//   - Description: human-readable explanation shown in CLI output
//   - YAMLConfig: the application configuration that defines the worker hierarchy
//
// The YAMLConfig is passed directly to ApplicationSupervisor and follows
// the same format as production configurations.
type Scenario struct {
	// Name is the identifier for this scenario (used in CLI --scenario flag)
	Name string

	// Description explains what this scenario tests (shown in CLI output)
	Description string

	// YAMLConfig defines the worker hierarchy for this scenario.
	// This is the same format used in production configurations.
	YAMLConfig string
}

// Registry contains all available scenarios.
// Add new scenarios here to make them available in both tests and CLI.
var Registry = map[string]Scenario{
	"simple":      SimpleScenario,
	"failing":     FailingScenario,
	"panic":       PanicScenario,
	"slow":        SlowScenario,
	"cascade":     CascadeScenario,
	"timeout":     TimeoutScenario,
	"configerror": ConfigErrorScenario,
	"inheritance": InheritanceScenario,
}

// RunConfig configures how a scenario is executed.
type RunConfig struct {
	// Scenario to run
	Scenario Scenario

	// Duration to run the scenario. 0 means run forever (until context cancelled)
	Duration time.Duration

	// TickInterval for the supervisor tick loop
	TickInterval time.Duration

	// Logger for all FSM output (can be test logger for capture)
	Logger *zap.SugaredLogger

	// Store for triangular state persistence (can be spy store for assertions)
	Store storage.TriangularStoreInterface

	// EnableTraceLogging enables verbose mutex/tick logging
	EnableTraceLogging bool

	// DumpStore enables dumping store deltas and final state after scenario completion.
	// When enabled, outputs a human-readable summary of all changes and final worker states.
	DumpStore bool
}

// ListScenarios returns all registered scenario names and descriptions.
func ListScenarios() map[string]string {
	result := make(map[string]string, len(Registry))
	for name, scenario := range Registry {
		result[name] = scenario.Description
	}
	return result
}
