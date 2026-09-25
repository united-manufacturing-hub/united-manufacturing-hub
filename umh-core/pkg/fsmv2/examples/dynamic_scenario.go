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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	hello_state "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

const (
	// dynamicHelloChildName is the helloworld child the dynamic scenario drives
	// through create -> update -> delete.
	dynamicHelloChildName = "dynamic-hello"

	// dynamicHelloInitialMood is the mood the CREATE leg's moodFilePath points
	// at, distinct from the updated mood so the UPDATE leg's observed change is
	// unambiguous. Neither value is "sad", which would drive the worker to
	// Degraded instead of Running.
	dynamicHelloInitialMood = "happy"

	// dynamicHelloUpdatedMood is the mood the UPDATE leg's new moodFilePath
	// points at. Observing this exact value in the child's persisted status is
	// the load-bearing proof that a runtime Upsert reached a live child.
	dynamicHelloUpdatedMood = "cheerful"

	// configWorkerName is the application worker's kernel child's name, from
	// workers/application/state/children.go.
	configWorkerName = "config-worker"
)

// DynamicScenarioV2 drives one helloworld child through the migration-API
// client: create it to Running, Upsert an observable config change (a new
// moodFilePath whose file contents land in observed status), then Delete it.
// The kernel-only supervisor and its config worker run the whole time; each
// leg is announced with Step and its result is checked through WaitFor, and
// the config worker is read once more after the child's lifecycle ends.
var DynamicScenarioV2 = ScenarioV2{
	Name:        "dynamic",
	Description: "Drives a helloworld child through create/update/delete via the migration-API client (v2)",
	Run:         runDynamicHello,
}

// runDynamicHello is DynamicScenarioV2's Run. The UPDATE leg points
// moodFilePath at a different file, so the observed mood changes only when
// the Upsert reached the child.
func runDynamicHello(ctx context.Context, env Env) error {
	ref := dynamicchildren.Ref{WorkerType: "helloworld", Name: dynamicHelloChildName}

	dir, err := os.MkdirTemp("", "dynamic-hello-mood")
	if err != nil {
		return fmt.Errorf("create mood dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(dir) }()

	initialMoodPath := filepath.Join(dir, "mood-initial")
	if err := os.WriteFile(initialMoodPath, []byte(dynamicHelloInitialMood), 0o600); err != nil {
		return fmt.Errorf("write initial mood file: %w", err)
	}

	updatedMoodPath := filepath.Join(dir, "mood-updated")
	if err := os.WriteFile(updatedMoodPath, []byte(dynamicHelloUpdatedMood), 0o600); err != nil {
		return fmt.Errorf("write updated mood file: %w", err)
	}

	// CREATE: Upsert the child pointing at the initial mood file, wait until it
	// reaches Running.
	env.Step("create the child with the initial mood file")

	if err := env.Client.Upsert(ref, map[string]any{
		"state":        "running",
		"moodFilePath": initialMoodPath,
	}); err != nil {
		return fmt.Errorf("upsert create: %w", err)
	}

	if err := env.WaitFor(ctx, "the child reaches Running",
		func(ctx context.Context) (bool, string, error) {
			obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, env.Client, ref)
			if err != nil {
				if errors.Is(err, fsmv2client.ErrNotObserved) {
					return false, "the child has not published an observation yet", nil
				}

				return false, "", err
			}

			return obs.State == hello_state.StateNameRunning, "state=" + obs.State, nil
		}); err != nil {
		return err
	}

	// UPDATE: Upsert a changed config field (a different moodFilePath), wait
	// until the new mood lands in observed status. The config field itself
	// changes here; the worker re-reads the new path in CollectObservedState.
	env.Step("upsert a mood file with updated contents")

	if err := env.Client.Upsert(ref, map[string]any{
		"state":        "running",
		"moodFilePath": updatedMoodPath,
	}); err != nil {
		return fmt.Errorf("upsert update: %w", err)
	}

	if err := env.WaitFor(ctx, "the child shows the updated mood",
		func(ctx context.Context) (bool, string, error) {
			obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, env.Client, ref)
			if err != nil {
				if errors.Is(err, fsmv2client.ErrNotObserved) {
					return false, "the child has not published an observation yet", nil
				}

				return false, "", err
			}

			return obs.Status.Mood == dynamicHelloUpdatedMood, "mood=" + obs.Status.Mood, nil
		}); err != nil {
		return err
	}

	// DELETE: remove the child, exercising the despawn path. Run only
	// calls Delete; proving the store-side reap (the worker gone from the store)
	// is deferred to ENG-5107, which builds the despawn-tombstone subsystem.
	env.Step("delete the child")
	env.Client.Delete(ref)

	if _, err := fsmv2client.Get[snapshot.ConfigworkerStatus](ctx, env.Client, dynamicchildren.Ref{
		WorkerType: configworker.WorkerTypeName,
		Name:       configWorkerName,
	}); err != nil {
		return fmt.Errorf("check the config worker is still readable after the child's lifecycle: %w", err)
	}

	return nil
}
