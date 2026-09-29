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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	example_panic "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic"
)

// PanicScenarioV2 runs a worker whose connect action panics on every attempt.
// ActionExecutor (supervisor/internal/execution) recovers each panic and
// records it as a failed connect, and the worker retries on its next tick.
var PanicScenarioV2 = ScenarioV2{
	Name:        "panic",
	Description: "Demonstrates panic recovery in action handlers",

	ExpectedWarnings: []string{"simulating_panic"},

	ExpectedErrors: []string{"action_panic"},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: "examplepanic", Name: "panic-worker-1"}

		env.Step("create the panic worker, which panics on every connect")

		if err := env.Client.Upsert(ref, map[string]any{
			"state":        "running",
			"should_panic": true,
		}); err != nil {
			return fmt.Errorf("upsert panic worker: %w", err)
		}

		if err := env.WaitFor(ctx, "the panic worker is observed in TryingToConnect",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_panic.ExamplepanicStatus](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				return obs.State == "TryingToConnect", "state=" + obs.State, nil
			}); err != nil {
			return err
		}

		// A poll can read the same observation twice, so each failed
		// connect is counted once, by its timestamp.
		failedConnects := make(map[time.Time]bool)

		return env.WaitFor(ctx, "the panic worker fails its connect three times without reaching Connected",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_panic.ExamplepanicStatus](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				if obs.State == "Connected" {
					return false, "", fmt.Errorf("the panic worker reached Connected, so its panics did not keep it out")
				}

				for _, result := range obs.LastActionResults {
					if result.ActionType == "connect" && !result.Success {
						failedConnects[result.Timestamp] = true
					}
				}

				return len(failedConnects) >= 3, fmt.Sprintf("state=%s failed_connects=%d", obs.State, len(failedConnects)), nil
			})
	},
}
