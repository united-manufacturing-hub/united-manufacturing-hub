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

// CascadeScenarioV2 runs one exampleparent whose two children are
// examplefailing workers that each fail three times, in two cycles. The
// first cycle's failures happen while the parent is still starting, so the
// parent reaches Running. The second cycle's failures happen while the
// parent is Running, and only then does the parent report Degraded. When
// the children recover, the parent returns to Running.
var CascadeScenarioV2 = ScenarioV2{
	Name:        "cascade",
	Description: "Shows cascade failure: child failures propagate to parent state, parent recovery when children heal",

	ExpectedWarnings: []string{"connect_failed_simulated"},

	ExpectedErrorCauses: []error{example_failing_action.ErrSimulatedFailure},

	Run: func(ctx context.Context, env Env) error {
		parentRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "cascade-parent"}

		childConfig := "should_fail: true\n" +
			"max_failures: 3\n" +
			"failure_cycles: 2\n" +
			"recovery_delay_observations: 3\n"

		env.Step("create the parent with two failing children")

		if err := env.Client.Upsert(parentRef, map[string]any{
			"state":             "running",
			"children_count":    2,
			"child_worker_type": "examplefailing",
			"child_config":      childConfig,
		}); err != nil {
			return fmt.Errorf("upsert parent: %w", err)
		}

		// Degraded can only follow Running, so this wait also carries the
		// parent through its start: Stopped, then TryingToStart while the
		// children run their first failure cycle, then Running, and into
		// the second cycle that makes the parent degraded.
		if err := env.WaitFor(ctx, "the parent reaches Degraded",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the parent has not published an observation yet", nil
					}

					return false, "", err
				}

				return obs.State == "Degraded", "state=" + obs.State, nil
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

			if err := env.WaitFor(ctx, "the child "+name+" reaches Connected",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, childRef)
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

		return nil
	},
}
