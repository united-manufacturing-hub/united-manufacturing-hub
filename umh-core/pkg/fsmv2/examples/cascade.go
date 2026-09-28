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
// examplefailing workers that each fail three times in each of two cycles.
// The parent reports Degraded when its children leave Connected for the next
// cycle while it is Running, and the second cycle's failures keep them
// unhealthy until they reconnect.
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

		// Degraded can only follow Running, so this wait also covers the
		// parent's start. The parent stays in TryingToStart until the
		// children finish their first failure cycle.
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

			// ConnectAttempts restarts at zero when examplefailing's
			// AdvanceCycle starts a new cycle, and with max_failures 3 the
			// fourth attempt connects. Connected in cycle 1 (CurrentCycle is
			// zero based) with more than 3 attempts therefore means the child
			// failed three times again before it reconnected. A child that
			// never cycles again stays at CurrentCycle 0.
			if err := env.WaitFor(ctx, "the child "+name+" reconnected in its second cycle after failing again",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, childRef)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the child has not published an observation yet", nil
						}

						return false, "", err
					}

					done := obs.State == "Connected" &&
						obs.Status.CurrentCycle == 1 &&
						obs.Status.ConnectAttempts > 3

					return done, fmt.Sprintf("state=%s cycle=%d attempts=%d", obs.State, obs.Status.CurrentCycle, obs.Status.ConnectAttempts), nil
				}); err != nil {
				return err
			}
		}

		return nil
	},
}
