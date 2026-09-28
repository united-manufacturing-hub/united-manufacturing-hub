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

// FailingScenarioV2 runs the three failing workers from the v1 scenario. The
// recovery worker fails three times and then connects. The permanent worker
// never reaches its failure limit of 999999, so it never connects. The
// restart worker fails forever and is restarted after five consecutive
// failures, and the scenario waits until a new worker of the same name is
// trying to connect again.
var FailingScenarioV2 = ScenarioV2{
	Name:        "failing",
	Description: "Demonstrates action failure handling with recovery vs permanent failure patterns",

	ExpectedWarnings: []string{"connect_failed_simulated"},

	ExpectedErrorCauses: []error{example_failing_action.ErrSimulatedFailure},

	Run: func(ctx context.Context, env Env) error {
		recoveryRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "failing-worker-recovery"}
		permanentRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "failing-worker-permanent"}
		restartRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "failing-worker-restart"}

		env.Step("create the recovery worker, which fails three times before it connects")

		if err := env.Client.Upsert(recoveryRef, map[string]any{
			"state":        "running",
			"should_fail":  true,
			"max_failures": 3,
		}); err != nil {
			return fmt.Errorf("upsert recovery worker: %w", err)
		}

		env.Step("create the permanent worker, which never reaches its failure limit")

		if err := env.Client.Upsert(permanentRef, map[string]any{
			"state":        "running",
			"should_fail":  true,
			"max_failures": 999999,
		}); err != nil {
			return fmt.Errorf("upsert permanent worker: %w", err)
		}

		env.Step("create the restart worker, which is restarted after five failures")

		if err := env.Client.Upsert(restartRef, map[string]any{
			"state":                  "running",
			"should_fail":            true,
			"max_failures":           999999,
			"restart_after_failures": 5,
		}); err != nil {
			return fmt.Errorf("upsert restart worker: %w", err)
		}

		// The recovery worker reaches Connected only after its attempts ran
		// past its failure limit.
		if err := env.WaitFor(ctx, "the recovery worker reaches Connected after more than 3 attempts",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, recoveryRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				done := obs.State == "Connected" && obs.Status.ConnectAttempts > 3

				return done, fmt.Sprintf("state=%s attempts=%d", obs.State, obs.Status.ConnectAttempts), nil
			}); err != nil {
			return err
		}

		// The permanent worker never connects, so after three failed attempts
		// one more reading must still show it outside Connected.
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

		// The restart worker first runs its failures up to the restart
		// threshold.
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

		// After the restart, only a newly created worker is trying to
		// connect again with a low attempt count. Waiting for a low count
		// alone would pass on the old worker's last reading, Stopped with 0
		// attempts, taken after it stopped and before the new worker
		// exists.
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
