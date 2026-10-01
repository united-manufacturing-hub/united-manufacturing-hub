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
	example_failing "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	example_failing_action "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/action"
	example_parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
)

// CascadeScenarioV2 runs one exampleparent whose examplefailing children fail their connects in repeated cycles.
var CascadeScenarioV2 = ScenarioV2{
	Name:        "cascade",
	Description: "A parent goes Degraded while its failing children reconnect, and returns to Running when both are Connected",

	ExpectedWarnings: []string{"connect_failed_simulated"},

	ExpectedErrorCauses: []error{example_failing_action.ErrSimulatedFailure},

	Run: func(ctx context.Context, env Env) error {
		parentRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "cascade-parent"}

		// recovery_delay_ms keeps each failed child unhealthy for longer than the
		// parent's observation interval, so the parent sees it.
		childConfig := "should_fail: true\n" +
			"max_failures: 3\n" +
			"failure_cycles: 2\n" +
			"recovery_delay_ms: 700\n"

		env.Step("create the parent with two children that each fail 3 connects, then connect, and repeat for 2 cycles; the parent waits 5 s before it creates them")

		if err := env.Client.Upsert(parentRef, map[string]any{
			"state":             "running",
			"children_count":    2,
			"child_worker_type": "examplefailing",
			"child_config":      childConfig,
		}); err != nil {
			return fmt.Errorf("upsert parent: %w", err)
		}

		// Degraded can only follow Running, so this wait also covers the parent's start.
		if err := env.WaitFor(ctx, "the parent has been Degraded once",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the parent has not published an observation yet", nil
					}

					return false, "", err
				}

				degradedCount := timesEntered(obs, "Degraded")

				return degradedCount >= 1, fmt.Sprintf("state=%s degraded_transitions=%d", obs.State, degradedCount), nil
			}); err != nil {
			return err
		}

		if err := env.WaitFor(ctx, "the parent returns to Running",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					return false, "", fmt.Errorf("read the parent again: %w", err)
				}

				return obs.State == "Running", "state=" + obs.State, nil
			}); err != nil {
			return err
		}

		for _, name := range []string{"child-0", "child-1"} {
			childRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: name}

			// AllCyclesComplete stays true until the parent removes the child after RunningDuration.
			if err := env.WaitFor(ctx, "the child "+name+" completes all its failure cycles",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, childRef)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the child has not published an observation yet", nil
						}

						return false, "", err
					}

					return obs.Status.AllCyclesComplete, fmt.Sprintf("state=%s cycle=%d complete=%t attempts=%d", obs.State, obs.Status.CurrentCycle, obs.Status.AllCyclesComplete, obs.Status.ConnectAttempts), nil
				}); err != nil {
				return err
			}
		}

		return nil
	},
}
