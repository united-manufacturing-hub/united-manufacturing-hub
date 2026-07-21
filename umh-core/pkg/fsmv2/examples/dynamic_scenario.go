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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
	hello_state "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

const (
	// dynamicHelloChildName is the helloworld child the dynamic driver drives
	// through create -> update -> delete. The dynamic_scenario_v2 battery reads
	// the child back under this same name.
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
)

// DynamicScenarioV2 drives one helloworld child through the migration-API
// client: create it to Running, Upsert an observable config change (a new
// moodFilePath whose file contents land in observed status), then Delete it.
// The kernel-only supervisor and its config worker run the whole time;
// correctness is judged after the run by the dynamic_scenario_v2 battery
// reading the same store.
var DynamicScenarioV2 = ScenarioV2{
	Name:        "dynamic",
	Description: "Drives a helloworld child through create/update/delete via the migration-API client (v2)",
	Driver:      driveDynamicHello,
}

// driveDynamicHello runs the create -> update -> delete lifecycle against the
// running kernel-only supervisor through env.Client. The UPDATE leg changes a
// real helloworld config field (moodFilePath) to a different file, so the
// observed mood change is driven by a config Upsert through the API, not by an
// out-of-band mutation of a fixed file.
func driveDynamicHello(ctx context.Context, env Env) error {
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
	if err := env.Client.Upsert(ref, map[string]any{
		"state":        "running",
		"moodFilePath": initialMoodPath,
	}); err != nil {
		return fmt.Errorf("upsert create: %w", err)
	}

	if err := waitForDynamicHello(ctx, env.Client, ref, func(obs fsmv2.Observation[hello_world.HelloworldStatus]) bool {
		return obs.State == hello_state.StateNameRunning
	}); err != nil {
		return fmt.Errorf("wait for create->Running: %w", err)
	}

	// UPDATE: Upsert a changed config field (a different moodFilePath), wait
	// until the new mood lands in observed status. The config field itself
	// changes here; the worker re-reads the new path in CollectObservedState.
	if err := env.Client.Upsert(ref, map[string]any{
		"state":        "running",
		"moodFilePath": updatedMoodPath,
	}); err != nil {
		return fmt.Errorf("upsert update: %w", err)
	}

	if err := waitForDynamicHello(ctx, env.Client, ref, func(obs fsmv2.Observation[hello_world.HelloworldStatus]) bool {
		return obs.Status.Mood == dynamicHelloUpdatedMood
	}); err != nil {
		return fmt.Errorf("wait for update->observed mood: %w", err)
	}

	// DELETE: remove the child, exercising the despawn path. The driver only
	// calls Delete; proving the store-side reap (the worker gone from the store)
	// is deferred to ENG-5107, which builds the despawn-tombstone subsystem.
	env.Client.Delete(ref)

	return nil
}

// waitForDynamicHello polls the child's typed observation until pred is
// satisfied or ctx is cancelled. ErrNotObserved means the child has not yet
// published an observation this tick, so it is treated as retry-on-a-later-tick;
// any other error is surfaced, so a real read failure is never swallowed.
func waitForDynamicHello(ctx context.Context, client *fsmv2client.FSMv2Client, ref dynamicchildren.Ref, pred func(fsmv2.Observation[hello_world.HelloworldStatus]) bool) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, client, ref)
		switch {
		case err == nil:
			if pred(obs) {
				return nil
			}
		case errors.Is(err, fsmv2client.ErrNotObserved):
			// Not yet observed: retry on a later tick.
		default:
			return fmt.Errorf("get observation: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
