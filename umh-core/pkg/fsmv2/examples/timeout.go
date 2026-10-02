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
	example_failing_action "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/action"
	example_slow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
)

// TimeoutScenario runs four workers that connect at different speeds. Its
// longest action, the two-second delay, stays far below the 30 s action
// timeout (defaultActionTimeout in supervisor/internal/execution).
var TimeoutScenario = Scenario{
	Name:        "timeout",
	Description: "Four workers that connect at different speeds; the longest action takes 2 s, far below the 30 s action timeout",

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
			"state":          "running",
			"should_fail":    true,
			"max_failures":   3,
			"failure_cycles": 1,
		}); err != nil {
			return fmt.Errorf("upsert retry worker: %w", err)
		}

		env.Step("create the combined worker: it fails five times and connects on attempt six, before its restart limit of ten")

		if err := env.Client.Upsert(combinedRef, map[string]any{
			"state":                  "running",
			"should_fail":            true,
			"max_failures":           5,
			"restart_after_failures": 10,
			"failure_cycles":         1,
		}); err != nil {
			return fmt.Errorf("upsert combined worker: %w", err)
		}

		if err := waitReconnectedAfterFailureCycle(ctx, env, retryRef); err != nil {
			return err
		}

		if err := waitReconnectedAfterFailureCycle(ctx, env, combinedRef); err != nil {
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
