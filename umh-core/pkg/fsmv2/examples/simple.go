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
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	example_parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
)

// SimpleScenarioV2 runs one exampleparent with two children and waits for
// the whole hierarchy to come up: the parent reaches Running, both its
// children reach Connected, and the parent's framework health count reports
// both children healthy.
var SimpleScenarioV2 = ScenarioV2{
	Name:        "simple",
	Description: "Single parent worker with 2 dynamically-created child workers",

	Run: func(ctx context.Context, env Env) error {
		parentRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "parent-1"}

		env.Step("create the parent with two children")

		if err := env.Client.Upsert(parentRef, map[string]any{
			"state":          "running",
			"children_count": 2,
		}); err != nil {
			return fmt.Errorf("upsert parent: %w", err)
		}

		if err := env.WaitFor(ctx, "the parent reaches Running",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the parent has not published an observation yet", nil
					}

					return false, "", err
				}

				return obs.State == "Running", "state=" + obs.State, nil
			}); err != nil {
			return err
		}

		for _, name := range []string{"child-0", "child-1"} {
			childRef := dynamicchildren.Ref{WorkerType: "examplechild", Name: name}

			if err := env.WaitFor(ctx, "the child "+name+" reaches Connected",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_child.ExamplechildStatus](ctx, env.Client, childRef)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the child has not published an observation yet", nil
						}

						return false, "", err
					}

					return obs.State == "Connected", "state=" + obs.State, nil
				}); err != nil {
				return err
			}
		}

		// ChildrenHealthy is a framework field the supervisor fills in, so
		// reading it after both children are Connected checks that the
		// parent observed their health and not just their existence.
		return env.WaitFor(ctx, "the parent reports both children healthy",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					return false, "", fmt.Errorf("read the parent again: %w", err)
				}

				return obs.ChildrenHealthy == 2, fmt.Sprintf("state=%s healthy=%d", obs.State, obs.ChildrenHealthy), nil
			})
	},
}
