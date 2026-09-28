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
	example_slow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow"
)

// SlowScenarioV2 runs one slow worker whose connect action sleeps two
// seconds, next to a control worker whose connect sleeps none. Spawning a
// worker takes about two seconds by itself, so the gap between the two first
// Connected readings is the only proof the delay ran.
var SlowScenarioV2 = ScenarioV2{
	Name:        "slow",
	Description: "Demonstrates a long-running action and checks its delay ran",

	Run: func(ctx context.Context, env Env) error {
		slowRef := dynamicchildren.Ref{WorkerType: "exampleslow", Name: "slow-worker-1"}
		controlRef := dynamicchildren.Ref{WorkerType: "exampleslow", Name: "slow-control"}

		env.Step("create the slow worker and a delay-0 control worker")

		if err := env.Client.Upsert(slowRef, map[string]any{
			"state":        "running",
			"delaySeconds": 2,
		}); err != nil {
			return fmt.Errorf("upsert slow worker: %w", err)
		}

		if err := env.Client.Upsert(controlRef, map[string]any{
			"state":        "running",
			"delaySeconds": 0,
		}); err != nil {
			return fmt.Errorf("upsert control worker: %w", err)
		}

		// firstConnected waits for one worker to reach Connected and returns
		// when its poll first saw that state.
		firstConnected := func(ref dynamicchildren.Ref) (time.Time, error) {
			var first time.Time

			err := env.WaitFor(ctx, "the worker "+ref.Name+" reaches Connected",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_slow.ExampleslowStatus](ctx, env.Client, ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the worker has not published an observation yet", nil
						}

						return false, "", err
					}

					if obs.State != "Connected" {
						return false, "state=" + obs.State, nil
					}

					if first.IsZero() {
						first = time.Now()
					}

					return true, "state=" + obs.State, nil
				})

			return first, err
		}

		// The control worker connects first. Waiting for the slow worker
		// first would push the control's first reading past that wait, and
		// the gap would measure the wait instead of the delay.
		controlConnected, err := firstConnected(controlRef)
		if err != nil {
			return err
		}

		slowConnected, err := firstConnected(slowRef)
		if err != nil {
			return err
		}

		// The slow worker's connect sleeps its whole delay before it reports
		// success, so its first Connected reading trails the control's by at
		// least that delay. The control was measured connecting 2.08s after
		// its Upsert, so the check demands a gap and not a total time.
		if gap := slowConnected.Sub(controlConnected); gap < 1500*time.Millisecond {
			return fmt.Errorf("the slow worker was first seen Connected %s after the control worker, without running its two-second delay", gap)
		}

		return nil
	},
}
