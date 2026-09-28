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

// Env is what a scenario's Run receives. It has no store handle and no
// supervisor handle, so a check reads only what a user of the client could read.
type Env struct {
	// Client is the migration-API client wired to the run's dynamicchildren
	// Writer and store, so Run can Upsert/Delete child specs and read
	// observed state.
	Client *fsmv2client.FSMv2Client

	// Logger is the run's logger (the same logger RunConfig.Logger carries),
	// so Run logs into the same stream the post-run log checks read.
	Logger deps.FSMLogger

	// Dependencies is the map the scenario's Dependencies returned, or nil when
	// the scenario declares none. The supervisor reads the same map while it
	// builds workers, so Run must not write to it: Run reads a mock out with
	// config.LookupDependency and changes the mock itself.
	Dependencies map[string]any
}

// ScenarioV2 is a scenario that drives the kernel-only supervisor.
// Dependencies builds its mocks. Run creates workers through env.Client,
// changes the mocks, and checks the result through env.Client. When a
// check fails, Run returns an error that names the check.
type ScenarioV2 struct {
	// Run runs against the started supervisor. After a nil return, the
	// runner waits RunConfig.Duration (or until ctx is cancelled; 0 means
	// ctx-only), then shuts the supervisor down. Run must honor ctx
	// cancellation: a cancelled ctx is the only stop signal a Run
	// receives, and teardown cannot start until Run returns.
	Run func(ctx context.Context, env Env) error

	// Name is the identifier for this scenario (used in CLI --scenario flag).
	Name string

	// Description explains what this scenario tests (shown in CLI output).
	Description string

	// Dependencies optionally builds the scenario's mocks. The runner passes
	// the returned map to the supervisor before it starts. Every worker Run
	// upserts through env.Client receives the map in its constructor.
	// config.DependencyKey has the typed read and write helpers.
	//
	// On error, Dependencies must release what it built. The runner ignores
	// the other return values and starts nothing. On success, the runner calls
	// cleanup once if it is non-nil: after the supervisor stops, or at once if
	// the supervisor fails to build.
	Dependencies func() (depsMap map[string]any, cleanup func(), err error)
}

// NoopScenarioV2 starts the kernel-only supervisor and drives nothing: the
// application worker spawns only its config worker kernel child.
//
// This scenario is kept permanently as the copy-paste template for scenario
// authors: copy it, rename it, put your mocks in Dependencies, and put the
// steps and checks in Run.
var NoopScenarioV2 = ScenarioV2{
	Name:        "noop",
	Description: "Runs the kernel-only supervisor and changes nothing (v2)",
	Run: func(_ context.Context, _ Env) error {
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
