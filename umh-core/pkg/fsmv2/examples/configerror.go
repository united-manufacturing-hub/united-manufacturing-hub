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
	example_failing "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing"
	example_parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
)

// ConfigErrorScenario runs a valid parent beside workers whose config the
// supervisor rejects when it derives their desired state.
var ConfigErrorScenario = Scenario{
	Name:        "configerror",
	Description: "Workers with a bad config are rejected and never start; beside them a valid parent reaches Running and an empty-config parent stays in TryingToStart",

	// The underlying error is a yaml type error with no sentinel value, so
	// ExpectedErrorCauses cannot name it.
	ExpectedErrors: []string{
		"worker_add_derive_desired_failed",
		"child_supervisor_add_worker_failed",
	},

	ExpectedWarnings: []string{"reducer_error"},

	Run: func(ctx context.Context, env Env) error {
		validRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "config-valid"}
		mismatchRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "config-type-mismatch"}
		emptyRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "config-empty"}
		defaultsRef := dynamicchildren.Ref{WorkerType: "examplefailing", Name: "config-bad-max-failures"}

		env.Step("create a valid parent, an empty-config parent, and two workers whose config has a string where a number belongs; the two are rejected with an error on every tick")

		if err := env.Client.Upsert(validRef, map[string]any{
			"state":          "running",
			"children_count": 2,
		}); err != nil {
			return fmt.Errorf("upsert valid parent: %w", err)
		}

		if err := env.Client.Upsert(mismatchRef, map[string]any{
			"state":          "running",
			"children_count": "not-a-number",
		}); err != nil {
			return fmt.Errorf("upsert type-mismatch parent: %w", err)
		}

		if err := env.Client.Upsert(emptyRef, map[string]any{
			"state": "running",
		}); err != nil {
			return fmt.Errorf("upsert empty-config parent: %w", err)
		}

		if err := env.Client.Upsert(defaultsRef, map[string]any{
			"state":        "running",
			"max_failures": "three",
		}); err != nil {
			return fmt.Errorf("upsert bad-max-failures worker: %w", err)
		}

		if err := env.WaitFor(ctx, "the valid parent reaches Running",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, validRef)
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

			if err := env.WaitFor(ctx, "the valid parent's child "+name+" reaches Connected",
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

		// With zero children, TryingToStartState's ChildrenHealthy > 0 condition never holds.
		if err := env.WaitFor(ctx, "the empty-config parent is in TryingToStart",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, emptyRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the parent has not published an observation yet", nil
					}

					return false, "", err
				}

				return obs.State == "TryingToStart", "state=" + obs.State, nil
			}); err != nil {
			return err
		}

		// This check runs after the valid parent reached Running, so an
		// invalid-config worker has had as long to appear as a valid one.
		const unobservedPolls = 20

		polls := 0

		return env.WaitFor(ctx, fmt.Sprintf("the two invalid-config workers stay unobserved across %d polls", unobservedPolls),
			func(ctx context.Context) (bool, string, error) {
				if _, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, mismatchRef); err == nil {
					return false, "", errors.New("the type-mismatch parent was observed, although its children_count is not a number")
				} else if !errors.Is(err, fsmv2client.ErrNotObserved) {
					return false, "", err
				}

				if _, err := fsmv2client.Get[example_failing.ExamplefailingStatus](ctx, env.Client, defaultsRef); err == nil {
					return false, "", errors.New("the bad-max-failures worker was observed, although its max_failures is not a number")
				} else if !errors.Is(err, fsmv2client.ErrNotObserved) {
					return false, "", err
				}

				polls++

				return polls >= unobservedPolls, fmt.Sprintf("unobserved polls=%d", polls), nil
			})
	},
}
