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
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
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
// Example: CommunicatorScenarioEntry uses this pattern - it wraps
// RunCommunicatorScenario which creates a mock server but still runs
// the communicator worker via ApplicationSupervisor.
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
//   - Embedded mock servers with dynamic URLs (CommunicatorScenario)
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

// Registry contains all available scenarios.
// Add new scenarios here to make them available in both tests and CLI.
var Registry = map[string]Scenario{
	"simple":       SimpleScenario,
	"failing":      FailingScenario,
	"panic":        PanicScenario,
	"slow":         SlowScenario,
	"cascade":      CascadeScenario,
	"timeout":      TimeoutScenario,
	"configerror":  ConfigErrorScenario,
	"inheritance":  InheritanceScenario,
	"communicator": CommunicatorScenarioEntry,
}

// CommunicatorScenarioEntry registers the communicator scenario for CLI access.
//
// Uses a CustomRunner that wraps RunCommunicatorScenario:
//  1. Creates an embedded mock relay server
//  2. Builds dynamic YAMLConfig with the mock server URL
//  3. Runs via ApplicationSupervisor
//
// # CLI Usage
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario communicator --duration 5s
//
// # What Gets Tested
//
//   - FSMv2 communicator worker state machine (Stopped -> Authenticating -> Syncing)
//   - Authentication with relay server via HTTPTransport
//   - Message pulling (backend -> edge) and pushing (edge -> backend) via SyncAction
//   - Metrics collection and observability
var CommunicatorScenarioEntry = Scenario{
	Name:        "communicator",
	Description: "Tests FSMv2 communicator worker with embedded mock server (uses ApplicationSupervisor)",
	YAMLConfig:  "", // Config built dynamically by RunCommunicatorScenario with mock server URL
	CustomRunner: func(ctx context.Context, cfg RunConfig) (*RunResult, error) {
		result := RunCommunicatorScenario(ctx, CommunicatorRunConfig{
			Duration:     cfg.Duration,
			TickInterval: cfg.TickInterval,
			Logger:       cfg.Logger,
			// Add test messages for metrics visibility in CLI scenario
			InitialPullMessages: []*transport.UMHMessage{
				{InstanceUUID: "test-instance-uuid", Content: "test-action-1"},
				{InstanceUUID: "test-instance-uuid", Content: "test-action-2"},
				{InstanceUUID: "test-instance-uuid", Content: "test-action-3"},
			},
			InitialOutboundMessages: []*transport.UMHMessage{
				{InstanceUUID: "test-instance-uuid", Content: "test-status-1"},
				{InstanceUUID: "test-instance-uuid", Content: "test-status-2"},
			},
		})

		if result.Error != nil {
			return nil, result.Error
		}

		if cfg.Logger != nil {
			go func() {
				<-result.Done
				cfg.Logger.Infow("scenario_complete",
					"auth_calls", result.AuthCallCount,
					"pushed_messages", len(result.PushedMessages),
					"received_messages", len(result.ReceivedMessages),
				)
			}()
		}

		return &RunResult{
			Done:     result.Done,
			Shutdown: result.Shutdown, // Delegate to RunCommunicatorScenario's Shutdown
		}, nil
	},
}

// RunConfig configures how a scenario is executed.
type RunConfig struct {

	Store            storage.TriangularStoreInterface
	Logger           *zap.SugaredLogger
	Scenario         Scenario
	Duration         time.Duration // 0 means run forever (until context cancelled)
	TickInterval     time.Duration
	EnableTraceLogging bool
	DumpStore        bool // Dump store deltas and final state after completion
}

// ListScenarios returns all registered scenario names and descriptions.
func ListScenarios() map[string]string {
	result := make(map[string]string, len(Registry))
	for name, scenario := range Registry {
		result[name] = scenario.Description
	}

	return result
}
