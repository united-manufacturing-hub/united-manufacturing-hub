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
)

// FailingScenarioV2 runs three examplefailing workers. The recovery worker
// fails three connects, connects, stays Connected for 5 s
// (healthyDurationMsBeforeNextCycle in examplefailing/state), disconnects once
// and reconnects. Only that disconnect sets AllCyclesComplete, so its wait
// takes about 7 s. The permanent worker never connects. The restart worker is
// restarted after every five failures and keeps restarting until the run
// ends. Every failed connect logs a warning and an action_failed error, and
// the scenario expects both.
var FailingScenarioV2 = ScenarioV2{
	Name:        "failing",
	Description: "Three workers whose connect fails: one connects after three failures, one never connects, one is restarted after every five failures",

	ExpectedWarnings: []string{"connect_failed_simulated"},

	ExpectedErrorCauses: []error{example_failing_action.ErrSimulatedFailure},

	Run: func(ctx context.Context, env Env) error {
		recoveryRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "failing-worker-recovery"}
		permanentRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "failing-worker-permanent"}
		restartRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "failing-worker-restart"}

		env.Step("create the recovery worker: it fails three times, connects, stays connected 5 s, disconnects once, and reconnects")

		if err := env.Client.Upsert(recoveryRef, map[string]any{
			"state":          "running",
			"should_fail":    true,
			"max_failures":   3,
			"failure_cycles": 1,
		}); err != nil {
			return fmt.Errorf("upsert recovery worker: %w", err)
		}

		env.Step("create the permanent worker: its failure limit is 999999, so it never connects")

		if err := env.Client.Upsert(permanentRef, map[string]any{
			"state":        "running",
			"should_fail":  true,
			"max_failures": 999999,
		}); err != nil {
			return fmt.Errorf("upsert permanent worker: %w", err)
		}

		env.Step("create the restart worker: it is restarted after every five failures, until the run ends")

		if err := env.Client.Upsert(restartRef, map[string]any{
			"state":                  "running",
			"should_fail":            true,
			"max_failures":           999999,
			"restart_after_failures": 5,
		}); err != nil {
			return fmt.Errorf("upsert restart worker: %w", err)
		}

		if err := waitReconnectedAfterFailureCycle(ctx, env, recoveryRef); err != nil {
			return err
		}

		if err := env.WaitFor(ctx, "the permanent worker records three failed attempts",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, permanentRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				return obs.Status.ConnectAttempts >= 3, fmt.Sprintf("state=%s attempts=%d", obs.State, obs.Status.ConnectAttempts), nil
			}); err != nil {
			return err
		}

		obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, permanentRef)
		if err != nil {
			return fmt.Errorf("read the permanent worker once more after three attempts: %w", err)
		}

		if obs.State == "Connected" {
			return fmt.Errorf("the permanent worker reached Connected after %d attempts, although its failure limit is 999999", obs.Status.ConnectAttempts)
		}

		if err := env.WaitFor(ctx, "the restart worker records five failed attempts",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, restartRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				return obs.Status.ConnectAttempts >= 5, fmt.Sprintf("state=%s attempts=%d", obs.State, obs.Status.ConnectAttempts), nil
			}); err != nil {
			return err
		}

		// The old worker's last reading before the restart is Stopped with 0
		// attempts, so a low count alone would match it.
		return env.WaitFor(ctx, "a restarted worker is trying to connect again with a low attempt count",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, restartRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the restarted worker has not published an observation yet", nil
					}

					return false, "", err
				}

				done := obs.State == "TryingToConnect" && obs.Status.ConnectAttempts >= 1 && obs.Status.ConnectAttempts <= 4

				return done, fmt.Sprintf("state=%s attempts=%d", obs.State, obs.Status.ConnectAttempts), nil
			})
	},
}

// waitReconnectedAfterFailureCycle waits until an examplefailing worker is
// Connected with AllCyclesComplete set. The worker sets the flag when it
// disconnects after healthyDurationMsBeforeNextCycle (examplefailing/state) in
// Connected, and the flag stays set after the reconnect.
func waitReconnectedAfterFailureCycle(ctx context.Context, env Env, ref dynamicchildren.Ref) error {
	return env.WaitFor(ctx, "the worker "+ref.Name+" is connected again after its one disconnect",
		func(ctx context.Context) (bool, string, error) {
			obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, ref)
			if err != nil {
				if errors.Is(err, fsmv2client.ErrNotObserved) {
					return false, "the worker has not published an observation yet", nil
				}

				return false, "", err
			}

			done := obs.State == "Connected" && obs.Status.AllCyclesComplete

			return done, fmt.Sprintf("state=%s all_cycles_complete=%t", obs.State, obs.Status.AllCyclesComplete), nil
		})
}
