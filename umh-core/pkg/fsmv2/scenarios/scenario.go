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

// Package scenarios provides the scenario definitions for FSM v2 integration testing.
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
//	result, err := scenarios.Run(ctx, scenariorunner.RunConfig{
//	    Scenario:   scenarios.Registry["helloworld"],
//	    Duration:     10 * time.Second,
//	    TickInterval: 100 * time.Millisecond,
//	    Logger:       testLogger,
//	    Store:        store,
//	})
//
// CLI uses the same scenarios via pkg/fsmv2/cmd/runner.
//
// The runner, Env and Scenario live in package scenariorunner, which imports no
// worker. This package holds the scenario list, the application supervisor
// the runner drives, and the states each worker type may report.
package scenarios

// ListScenarios returns all registered scenario names and descriptions,
// merging every registry into one listing.
func ListScenarios() map[string]string {
	result := make(map[string]string, len(Registry)+len(LiveRegistry))

	for name, scenario := range Registry {
		result[name] = scenario.Description
	}

	for name, scenario := range LiveRegistry {
		result[name] = scenario.Description
	}

	return result
}
