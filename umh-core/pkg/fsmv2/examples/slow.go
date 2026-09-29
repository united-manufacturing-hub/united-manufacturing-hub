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
	example_slow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
)

// SlowScenarioV2 checks that a slow worker spends its connect delay in
// TryingToConnect.
var SlowScenarioV2 = ScenarioV2{
	Name:        "slow",
	Description: "A worker whose connect takes two seconds; checks that it spent them in TryingToConnect",

	Run: func(ctx context.Context, env Env) error {
		slowRef := dynamicchildren.Ref{WorkerType: "exampleslow", Name: "slow-worker-1"}

		env.Step("create the slow worker with a two-second connect delay")

		if err := env.Client.Upsert(slowRef, map[string]any{
			"state":        "running",
			"delaySeconds": 2,
		}); err != nil {
			return fmt.Errorf("upsert slow worker: %w", err)
		}

		// The connect action sleeps its whole delay before it reports
		// success, so the worker spends at least that long in
		// TryingToConnect. CumulativeTimeByStateMs only grows, so a slow
		// machine delays this reading but cannot shrink it.
		return env.WaitFor(ctx, "the slow worker is Connected after at least 1.9 s in TryingToConnect",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_slow.ExampleslowStatus](ctx, env.Client, slowRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				timeTrying := obs.Metrics.Framework.CumulativeTimeByStateMs["TryingToConnect"]

				done := obs.State == "Connected" && timeTrying >= 1900

				return done, fmt.Sprintf("state=%s trying_to_connect_ms=%d", obs.State, timeTrying), nil
			})
	},
}
