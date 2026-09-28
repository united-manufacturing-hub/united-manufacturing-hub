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
//	    Scenario:     examples.InheritanceScenario,
//	    Duration:     10 * time.Second,
//	    TickInterval: 100 * time.Millisecond,
//	    Logger:       testLogger,
//	    Store:        store,
//	})
//
// CLI uses the same scenarios via pkg/fsmv2/cmd/runner.
package examples

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ScenarioRunner executes scenarios that need infrastructure setup beyond
// what YAMLConfig alone can provide.
//
// # When to Use CustomRunner
//
// Use a CustomRunner when your scenario needs external infrastructure (mock servers,
// test fixtures) that must be running BEFORE the FSMv2 workers start.
//
// IMPORTANT: CustomRunner should STILL use ApplicationSupervisor for worker
// execution when possible. There are two valid patterns:
//
// ## Pattern 1: Infrastructure Orchestration (PREFERRED)
//
// CustomRunner sets up infrastructure, then delegates to ApplicationSupervisor.
// The FSMv2 workers still run through the standard supervisor flow.
//
//	CustomRunner: func(ctx, cfg) (*RunResult, error) {
//	    mockServer := startMockServer()
//	    defer mockServer.Close()
//	    return RunScenarioThatUsesApplicationSupervisor(ctx, cfg)
//	}
//
// ## Pattern 2: Full Custom Execution (USE SPARINGLY)
//
// CustomRunner implements its own execution loop without ApplicationSupervisor.
// Only use this when ApplicationSupervisor fundamentally cannot support your use case.
//
// # When to Use YAMLConfig Instead
//
// Use standard YAMLConfig-based scenarios when you are:
//   - Testing FSM worker hierarchies (parent/child relationships)
//   - Testing ApplicationSupervisor behavior directly
//   - Testing worker state transitions and persistence
//   - No external infrastructure is needed
//
// # Contract
//
// A ScenarioRunner must:
//   - Return a RunResult with a Done channel that closes when execution completes
//   - Handle context cancellation for graceful shutdown
//   - Clean up all resources it creates (mock servers, etc.) before Done closes
//   - Return errors only for setup failures; log runtime errors instead
//
// # Example (Infrastructure Orchestration Pattern)
//
//	var CustomScenarioEntry = Scenario{
//	    Name:        "custom",
//	    Description: "Custom scenario with mock server (uses ApplicationSupervisor)",
//	    CustomRunner: func(ctx context.Context, cfg RunConfig) (*RunResult, error) {
//	        mockServer := startMockServer()
//	        defer mockServer.Close()
//
//	        // Delegate to helper that uses ApplicationSupervisor internally
//	        result := RunCustomScenario(ctx, CustomConfig{
//	            Duration:  cfg.Duration,
//	            ServerURL: mockServer.URL(),
//	        })
//	        return &RunResult{Done: result.Done, Shutdown: func(){}}, result.Error
//	    },
//	}
type ScenarioRunner func(ctx context.Context, cfg RunConfig) (*RunResult, error)

// Scenario defines a test scenario configuration.
//
// Scenarios specify:
//   - Name: identifier used in CLI (--scenario=simple)
//   - Description: human-readable explanation shown in CLI output
//   - YAMLConfig: the application configuration that defines the worker hierarchy
//   - CustomRunner: optional custom execution function for infrastructure orchestration
//
// # Execution Modes
//
// Scenarios execute in two modes:
//
// ## Standard Mode (YAMLConfig)
//
// When you set YAMLConfig and leave CustomRunner nil, the scenario uses
// ApplicationSupervisor to manage worker hierarchies. This is the common case
// for testing FSM state machines.
//
// ## Custom Mode (CustomRunner)
//
// When you set CustomRunner, Run() delegates to your function. CustomRunner
// can still use ApplicationSupervisor internally (preferred), or implement
// custom execution when necessary. Use this for scenarios that need:
//   - Embedded mock servers with dynamic URLs
//   - Test fixtures that must exist before workers start
//   - External resource lifecycle management
//
// See ScenarioRunner documentation for the two CustomRunner patterns.
//
// Run() returns an error if you set both YAMLConfig and CustomRunner, or if you set neither.
type Scenario struct {

	// CustomRunner, if set, handles scenario execution.
	// This enables scenarios that need infrastructure setup (like mock servers)
	// before workers can start. CustomRunner should still use ApplicationSupervisor
	// internally when possible (see ScenarioRunner for the preferred pattern).
	//
	// When you set CustomRunner:
	//   - YAMLConfig must be empty
	//   - Run() delegates directly to CustomRunner
	//   - CustomRunner is responsible for all resource lifecycle management
	//
	// See ScenarioRunner documentation for the full contract and patterns.
	CustomRunner ScenarioRunner
	// Name is the identifier for this scenario (used in CLI --scenario flag)
	Name string

	// Description explains what this scenario tests (shown in CLI output)
	Description string

	// YAMLConfig defines the worker hierarchy for this scenario.
	// This is the same format used in production configurations.
	// Do not set this if you set CustomRunner.
	YAMLConfig string
}

// Registry holds the v1 scenarios not yet moved to v2 (ENG-5114). New
// scenarios go in RegistryV2; see pkg/fsmv2/CLAUDE.md, "Writing a scenario".
var Registry = map[string]Scenario{
	"inheritance": InheritanceScenario,
}

// RunConfig configures how a scenario is executed.
type RunConfig struct {
	Store        storage.TriangularStoreInterface
	Logger       deps.FSMLogger
	Scenario     Scenario
	ScenarioV2   ScenarioV2    // When ScenarioV2.Run is set, examples.Run takes the v2 kernel-only path
	Duration     time.Duration // 0 means run forever (until context cancelled)
	TickInterval time.Duration
	// Dependencies is the dependency map Run passes to the application
	// supervisor for a v1 scenario without a CustomRunner. v2 scenarios build
	// theirs in ScenarioV2.Dependencies.
	Dependencies map[string]any
	// GracefulShutdownTimeout is the per-level drain base propagated to the
	// supervisor subtree. Zero falls back to the supervisor default (5s). A
	// tiny value forces a degraded drain, which is how tests exercise the
	// ShutdownClean=false path deterministically.
	GracefulShutdownTimeout time.Duration
	EnableTraceLogging      bool
	DumpStore               bool // Dump store deltas and final state after completion
}

// ListScenarios returns all registered scenario names and descriptions,
// merging the v1 Registry and the v2 RegistryV2 into one listing.
func ListScenarios() map[string]string {
	result := make(map[string]string, len(Registry)+len(RegistryV2))
	for name, scenario := range Registry {
		result[name] = scenario.Description
	}

	for name, scenario := range RegistryV2 {
		result[name] = scenario.Description
	}

	return result
}
