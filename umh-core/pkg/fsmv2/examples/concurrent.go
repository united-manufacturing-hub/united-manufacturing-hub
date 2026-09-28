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
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
)

// ConcurrentScenarioV2 starts several helloworld workers at once; each must reach Running.
var ConcurrentScenarioV2 = ScenarioV2{
	Name:        "concurrent",
	Description: "Tests multiple independent workers running concurrently without interference",

	Run: func(ctx context.Context, env Env) error {
		refs := make([]dynamicchildren.Ref, 0, 5)

		for i := 1; i <= 5; i++ {
			refs = append(refs, dynamicchildren.Ref{
				WorkerType: "helloworld",
				Name:       fmt.Sprintf("concurrent-worker-%d", i),
			})
		}

		env.Step("create five helloworld workers at once")

		// Upsert every worker before waiting on any, so they start together.
		for _, ref := range refs {
			if err := env.Client.Upsert(ref, map[string]any{
				"state": "running",
			}); err != nil {
				return fmt.Errorf("upsert %s: %w", ref.Name, err)
			}
		}

		for _, ref := range refs {
			if err := env.WaitFor(ctx, "the worker "+ref.Name+" reaches Running",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, env.Client, ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the worker has not published an observation yet", nil
						}

						return false, "", err
					}

					return obs.State == "Running", "state=" + obs.State, nil
				}); err != nil {
				return err
			}
		}

		return nil
	},
}
