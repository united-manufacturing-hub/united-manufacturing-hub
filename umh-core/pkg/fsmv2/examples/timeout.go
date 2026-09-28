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
	example_slow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
)

// TimeoutScenarioV2 waits for exampleslow and examplefailing workers to reach
// Connected. A failing worker must also show an attempt count above its
// failure limit, which proves its failures happened before its connect.
var TimeoutScenarioV2 = ScenarioV2{
	Name:        "timeout",
	Description: "Demonstrates action timeout handling and retry behavior patterns",

	ExpectedWarnings: []string{"connect_failed_simulated"},

	ExpectedErrorCauses: []error{example_failing_action.ErrSimulatedFailure},

	Run: func(ctx context.Context, env Env) error {
		quickRef := dynamicchildren.Ref{WorkerType: "exampleslow", Name: "timeout-quick"}
		slowRef := dynamicchildren.Ref{WorkerType: "exampleslow", Name: "timeout-slow"}
		retryRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "timeout-retry"}
		combinedRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "timeout-combined"}

		env.Step("create the quick worker, which connects at once")

		if err := env.Client.Upsert(quickRef, map[string]any{
			"state":        "running",
			"delaySeconds": 0,
		}); err != nil {
			return fmt.Errorf("upsert quick worker: %w", err)
		}

		env.Step("create the slow worker, which connects after a two-second delay")

		if err := env.Client.Upsert(slowRef, map[string]any{
			"state":        "running",
			"delaySeconds": 2,
		}); err != nil {
			return fmt.Errorf("upsert slow worker: %w", err)
		}

		env.Step("create the retry worker, which fails three times before it connects")

		if err := env.Client.Upsert(retryRef, map[string]any{
			"state":        "running",
			"should_fail":  true,
			"max_failures": 3,
		}); err != nil {
			return fmt.Errorf("upsert retry worker: %w", err)
		}

		env.Step("create the combined worker, which fails five times before it connects")

		if err := env.Client.Upsert(combinedRef, map[string]any{
			"state":                  "running",
			"should_fail":            true,
			"max_failures":           5,
			"restart_after_failures": 10,
		}); err != nil {
			return fmt.Errorf("upsert combined worker: %w", err)
		}

		waitFailingConnected := func(ref dynamicchildren.Ref, maxFailures int) error {
			return env.WaitFor(ctx, fmt.Sprintf("the worker %s reaches Connected after more than %d attempts", ref.Name, maxFailures),
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the worker has not published an observation yet", nil
						}

						return false, "", err
					}

					done := obs.State == "Connected" && obs.Status.ConnectAttempts > maxFailures

					return done, fmt.Sprintf("state=%s attempts=%d", obs.State, obs.Status.ConnectAttempts), nil
				})
		}

		// A failing worker shows its attempts above its limit only for its
		// first five seconds in Connected (healthyDurationMsBeforeNextCycle,
		// examplefailing/state); then its counter drops to 0. The slow
		// workers stay Connected, so the failing ones are waited for first.
		if err := waitFailingConnected(retryRef, 3); err != nil {
			return err
		}

		if err := waitFailingConnected(combinedRef, 5); err != nil {
			return err
		}

		waitSlowConnected := func(ref dynamicchildren.Ref) error {
			return env.WaitFor(ctx, "the worker "+ref.Name+" reaches Connected",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_slow.ExampleslowStatus](ctx, env.Client, ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the worker has not published an observation yet", nil
						}

						return false, "", err
					}

					return obs.State == "Connected", "state=" + obs.State, nil
				})
		}

		if err := waitSlowConnected(quickRef); err != nil {
			return err
		}

		return waitSlowConnected(slowRef)
	},
}
