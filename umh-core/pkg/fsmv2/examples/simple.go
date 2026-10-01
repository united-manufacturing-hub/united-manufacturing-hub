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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	example_parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	parentstate "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

// SimpleScenarioV2 runs one exampleparent with two examplechild children through start and stop.
var SimpleScenarioV2 = ScenarioV2{
	Name:        "simple",
	Description: "One exampleparent starts two examplechild workers, reports both healthy, then stops them and reaches Stopped",

	Run: func(ctx context.Context, env Env) error {
		parentRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "parent-1"}

		env.Step(fmt.Sprintf("create the parent with two children; the parent waits %s in Stopped before it creates them", parentstate.StoppedWaitDuration))

		if err := env.Client.Upsert(parentRef, map[string]any{
			"state":          "running",
			"children_count": 2,
		}); err != nil {
			return fmt.Errorf("upsert parent: %w", err)
		}

		if err := env.WaitFor(ctx, "the parent reaches Running",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the parent has not published an observation yet", nil
					}

					return false, "", err
				}

				return obs.State == "Running", "state=" + obs.State, nil
			}); err != nil {
			return err
		}

		for _, name := range []string{"child-0", "child-1"} {
			childRef := dynamicchildren.Ref{WorkerType: "examplechild", Name: name}

			if err := env.WaitFor(ctx, "the child "+name+" reaches Connected",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_child.ExamplechildStatus](ctx, env.Client, childRef)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the child has not published an observation yet", nil
						}

						return false, "", err
					}

					return obs.State == "Connected", "state=" + obs.State, nil
				}); err != nil {
				return err
			}
		}

		var stoppedCountWhileRunning int64

		if err := env.WaitFor(ctx, "the parent reports both children healthy",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					return false, "", fmt.Errorf("read the parent again: %w", err)
				}

				stoppedCountWhileRunning = timesEntered(obs, "Stopped")

				return obs.ChildrenHealthy == 2, fmt.Sprintf("state=%s healthy=%d", obs.State, obs.ChildrenHealthy), nil
			}); err != nil {
			return err
		}

		env.Step("let the parent stop on its own: after RunningDuration in Running it removes its children and reaches Stopped")

		// TryingToStop enters Stopped only when the parent counts no child as healthy
		// or unhealthy, and the supervisor counts a stopped child as neither.
		return env.WaitFor(ctx, "the parent has stopped its children and entered Stopped",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					return false, "", fmt.Errorf("read the parent again: %w", err)
				}

				stopped := timesEntered(obs, "Stopped")

				return stopped > stoppedCountWhileRunning, fmt.Sprintf("state=%s stopped_transitions=%d (was %d)", obs.State, stopped, stoppedCountWhileRunning), nil
			})
	},
}
