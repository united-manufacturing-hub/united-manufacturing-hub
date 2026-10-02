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

const concurrentWorkerCount = 5

// ConcurrentScenario creates several helloworld workers without waiting between them; each must reach Running.
var ConcurrentScenario = Scenario{
	Name:        "concurrent",
	Description: "Helloworld workers created without waiting between them; each one reaches Running",

	Run: func(ctx context.Context, env Env) error {
		refs := make([]dynamicchildren.Ref, 0, concurrentWorkerCount)

		for i := 1; i <= concurrentWorkerCount; i++ {
			refs = append(refs, dynamicchildren.Ref{
				WorkerType: "helloworld",
				Name:       fmt.Sprintf("concurrent-worker-%d", i),
			})
		}

		env.Step("create every helloworld worker without waiting between them")

		for _, ref := range refs {
			if err := env.Client.Upsert(ref, map[string]any{
				"state": "running",
			}); err != nil {
				return fmt.Errorf("upsert %s: %w", ref.Name, err)
			}
		}

		// The wait compares against concurrentWorkerCount, not len(refs), so a run that creates no workers fails.
		return env.WaitFor(ctx, fmt.Sprintf("all %d workers reach Running", concurrentWorkerCount),
			func(ctx context.Context) (bool, string, error) {
				running := 0

				for _, ref := range refs {
					obs, err := fsmv2client.Get[hello_world.HelloworldStatus](ctx, env.Client, ref)
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						continue
					}

					if err != nil {
						return false, "", err
					}

					if obs.State == "Running" {
						running++
					}
				}

				return running == concurrentWorkerCount,
					fmt.Sprintf("%d of %d workers in Running", running, concurrentWorkerCount), nil
			})
	},
}
