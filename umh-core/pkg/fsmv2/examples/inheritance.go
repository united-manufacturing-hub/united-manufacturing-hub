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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
	example_parent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
)

// InheritanceScenarioV2 shows variables flowing from the application down to
// the children of an exampleparent. The variables set through
// env.Client.SetVariables reach the parent. The parent passes them on to its
// children and gives each child its own DEVICE_ID. Each child's observation
// reports the address and device it rendered, so the waits prove the values
// travelled the whole way. The scenario ends when both children are Connected
// and the parent is Running with both children healthy.
var InheritanceScenarioV2 = ScenarioV2{
	Name:        "inheritance",
	Description: "The application's variables reach an exampleparent and its two children; the parent gives each child its own DEVICE_ID",

	Run: func(ctx context.Context, env Env) error {
		env.Step("set the user variables IP and PORT for every application child")

		// The variables are set before the upsert, so no child is ever built
		// without IP. A child without IP would fail its template in strict
		// mode, and the logged error would fail the run.
		env.Client.SetVariables(config.VariableBundle{
			User: map[string]any{
				"IP":   "192.168.1.100",
				"PORT": 502,
			},
		})

		env.Step("create the parent with two children; each child's config renders IP and PORT into its address")

		parentRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "inheritance-parent"}

		if err := env.Client.Upsert(parentRef, map[string]any{
			"state":          "running",
			"children_count": 2,
			"child_config":   "address: \"{{ .IP }}:{{ .PORT }}\"\ndevice: \"{{ .DEVICE_ID }}\"",
		}); err != nil {
			return fmt.Errorf("upsert parent: %w", err)
		}

		for i := range 2 {
			name := fmt.Sprintf("child-%d", i)

			wantAddress := "192.168.1.100:502"
			wantDevice := fmt.Sprintf("device-%d", i)

			childRef := dynamicchildren.Ref{WorkerType: "examplechild", Name: name}

			if err := env.WaitFor(ctx,
				fmt.Sprintf("the child %s is Connected and reports the address %s and the device %s it rendered", name, wantAddress, wantDevice),
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_child.ExamplechildStatus](ctx, env.Client, childRef)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the child has not published an observation yet", nil
						}

						return false, "", err
					}

					done := obs.State == "Connected" &&
						obs.Status.Address == wantAddress &&
						obs.Status.Device == wantDevice

					return done, fmt.Sprintf("state=%s address=%s device=%s", obs.State, obs.Status.Address, obs.Status.Device), nil
				}); err != nil {
				return err
			}
		}

		return env.WaitFor(ctx, "the parent is Running and reports both children healthy",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_parent.ExampleparentStatus](ctx, env.Client, parentRef)
				if err != nil {
					return false, "", fmt.Errorf("read the parent: %w", err)
				}

				done := obs.State == "Running" && obs.ChildrenHealthy == 2

				return done, fmt.Sprintf("state=%s healthy=%d", obs.State, obs.ChildrenHealthy), nil
			})
	},
}
