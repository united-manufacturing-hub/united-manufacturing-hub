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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
)

// Env carries exactly the user-facing API a ScenarioV2 driver may touch: the
// client and a logger. The driver plays the role FSMv1 plays in production:
// it drives the system but must not assert on it. Correctness is judged
// after the run by log checks and store checks, which is why Env
// deliberately has no store handle and no supervisor handle.
type Env struct {
	// Client is the migration-API client wired to the run's dynamicchildren
	// Writer and store, so drivers can Upsert/Delete child specs and read
	// observed state.
	Client *fsmv2client.FSMv2Client

	// Logger is the run's logger (the same logger RunConfig.Logger carries),
	// so drivers log into the same stream the post-run log checks read.
	Logger deps.FSMLogger
}

// ScenarioV2 defines a driver-based scenario. Instead of declaring children
// via YAMLConfig, a v2 scenario receives an Env and drives the running
// kernel-only supervisor through the fsmv2client. The Driver only simulates
// the user; correctness is judged after the run by log checks and store
// checks, so the Driver gets no handle that could assert on internals.
type ScenarioV2 struct {
	// Driver runs against the started supervisor. After a nil return, the
	// runner waits RunConfig.Duration (or until ctx is cancelled; 0 means
	// ctx-only), then shuts the supervisor down. Drivers must honor ctx
	// cancellation: a cancelled ctx is the only stop signal a driver
	// receives, and teardown cannot start until the Driver returns.
	Driver func(ctx context.Context, env Env) error

	// Name is the identifier for this scenario (used in CLI --scenario flag).
	Name string

	// Description explains what this scenario tests (shown in CLI output).
	Description string
}

// NoopScenarioV2 starts the kernel-only supervisor and drives nothing: the
// application worker spawns only its config worker kernel child.
//
// This scenario is kept permanently as the copy-paste template for scenario
// authors: copy it, rename it, and put your driving logic in Driver.
var NoopScenarioV2 = ScenarioV2{
	Name:        "noop",
	Description: "Runs the kernel-only supervisor with no driver actions (v2)",
	Driver: func(_ context.Context, _ Env) error {
		return nil
	},
}

// RegistryV2 contains all available v2 scenarios, merged into ListScenarios
// alongside the v1 Registry. Names must not collide with v1 Registry names
// (enforced by the disjointness test in scenariov2_test.go, which documents
// what breaks on a collision).
var RegistryV2 = map[string]ScenarioV2{
	"noop":    NoopScenarioV2,
	"dynamic": DynamicScenarioV2,
}
