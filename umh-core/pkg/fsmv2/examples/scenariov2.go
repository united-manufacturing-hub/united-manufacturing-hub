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

// Env carries the client, the logger and the run's dependency map. Run
// changes its mocks through Dependencies and checks the result through
// Client. Env has no store handle and no supervisor handle, so a check
// reads what a user of the client could read.
type Env struct {
	// Client is the migration-API client wired to the run's dynamicchildren
	// Writer and store, so Runs can Upsert/Delete child specs and read
	// observed state.
	Client *fsmv2client.FSMv2Client

	// Logger is the run's logger (the same logger RunConfig.Logger carries),
	// so Runs log into the same stream the post-run log checks read.
	Logger deps.FSMLogger

	// Dependencies is the map the scenario's Dependencies returned, or nil when
	// the scenario declares none. The supervisor reads the same map while it
	// builds workers, so Run must not write to it: Run reads a mock out with
	// config.LookupDependency and changes the mock itself.
	Dependencies map[string]any
}

// ScenarioV2 defines a Run-based scenario. A scenario builds its mocks in
// Dependencies, creates its workers through env.Client, and in Run changes
// the mocks and checks the store. When a check fails, Run returns an error
// whose text names the check.
type ScenarioV2 struct {
	// Run runs against the started supervisor. After a nil return, the
	// runner waits RunConfig.Duration (or until ctx is cancelled; 0 means
	// ctx-only), then shuts the supervisor down. Runs must honor ctx
	// cancellation: a cancelled ctx is the only stop signal a Run
	// receives, and teardown cannot start until Run returns.
	Run func(ctx context.Context, env Env) error

	// Name is the identifier for this scenario (used in CLI --scenario flag).
	Name string

	// Description explains what this scenario tests (shown in CLI output).
	Description string

	// Dependencies optionally returns a map of named dependencies that the
	// runner injects into the supervisor before it starts, so the map reaches
	// the constructor of every worker Run creates via env.Client.Upsert (see
	// config.DependencyKey for the typed read/write helpers). A nil
	// Dependencies means Run-created workers get no scenario dependencies.
	//
	// When Dependencies returns an error, it releases whatever it built
	// itself, and the runner ignores its other return values and starts
	// nothing. When it succeeds, the runner calls cleanup (if non-nil) once,
	// after the supervisor has stopped, or right away if the supervisor
	// fails to build.
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
