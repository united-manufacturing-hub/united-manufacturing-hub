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

// InheritanceScenario sets variables through env.Client.SetVariables and
// checks what an exampleparent and both of its children render from them.
// It reads the rendered values from each worker's desired state. It fails on
// each of these bugs:
//   - The registry has no place for variables, so no child receives IP and PORT.
//   - The parent's supervisor overwrites Global with an empty map on every
//     tick, so the parent's label does not follow a change to SITE.
//   - A child's spec sets IP, and the child's value replaces the parent's
//     value.
var InheritanceScenario = Scenario{
	Name: "inheritance",
	Description: "The application's variables reach an exampleparent and its two children. " +
		"The parent renders the Global variable SITE into its label, and the label follows a change to SITE. " +
		"Each child's spec sets its own IP, and the parent's IP still wins.",

	// The supervisor logs this once per child, because each child's spec
	// sets IP and the parent already holds IP.
	ExpectedWarnings: []string{"child_variable_conflict"},

	Run: func(ctx context.Context, env Env) error {
		user := map[string]any{
			"IP":   "192.168.1.100",
			"PORT": 502,
		}

		env.Step("set the User variables IP and PORT and the Global variable SITE=plant-a for every application child")

		// The variables are set before the upsert, so no child is ever built
		// without IP. A child without IP would fail its template in strict
		// mode, and the logged error would fail the run.
		env.Client.SetVariables(config.VariableBundle{
			User:   user,
			Global: map[string]any{"SITE": "plant-a"},
		})

		env.Step("create the parent with two children. The parent's label renders SITE. " +
			"Each child's spec sets IP to 10.0.0.1, and each child's config renders IP and PORT into its address")

		parentRef := dynamicchildren.Ref{WorkerType: "exampleparent", Name: "inheritance-parent"}

		if err := env.Client.Upsert(parentRef, map[string]any{
			"state":           "running",
			"children_count":  2,
			"child_config":    "address: \"{{ .IP }}:{{ .PORT }}\"\ndevice: \"{{ .DEVICE_ID }}\"",
			"child_variables": map[string]any{"IP": "10.0.0.1"},
			"label":           "{{ .global.SITE }}",
		}); err != nil {
			return fmt.Errorf("upsert parent: %w", err)
		}

		waitForLabel := func(want string) error {
			return env.WaitFor(ctx, fmt.Sprintf("the parent's desired config holds the label %s", want),
				func(ctx context.Context) (bool, string, error) {
					cfg, err := fsmv2client.GetDesired[example_parent.ExampleparentConfig](ctx, env.Client, parentRef)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNoDesiredState) {
							return false, "the parent has no desired state yet", nil
						}

						return false, "", err
					}

					return cfg.Label == want, fmt.Sprintf("label=%q", cfg.Label), nil
				})
		}

		if err := waitForLabel("plant-a"); err != nil {
			return err
		}

		env.Step("change SITE to plant-b. The parent's first desired state is built before its first tick, " +
			"so only this change shows whether each tick passes the Global variables to the parent")

		env.Client.SetVariables(config.VariableBundle{
			User:   user,
			Global: map[string]any{"SITE": "plant-b"},
		})

		if err := waitForLabel("plant-b"); err != nil {
			return err
		}

		for i := range 2 {
			name := fmt.Sprintf("child-%d", i)

			wantAddress := "192.168.1.100:502"
			wantDevice := fmt.Sprintf("device-%d", i)

			childRef := dynamicchildren.Ref{WorkerType: "examplechild", Name: name}

			if err := env.WaitFor(ctx,
				fmt.Sprintf("the child %s is Connected, and its desired config holds the address %s and the device %s. "+
					"The address fails if the child's IP 10.0.0.1 replaces the parent's IP", name, wantAddress, wantDevice),
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_child.ExamplechildStatus](ctx, env.Client, childRef)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the child has not published an observation yet", nil
						}

						return false, "", err
					}

					cfg, err := fsmv2client.GetDesired[example_child.ExamplechildConfig](ctx, env.Client, childRef)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNoDesiredState) {
							return false, "the child has no desired state yet", nil
						}

						return false, "", err
					}

					done := obs.State == "Connected" &&
						cfg.Address == wantAddress &&
						cfg.Device == wantDevice

					return done, fmt.Sprintf("state=%s address=%s device=%s", obs.State, cfg.Address, cfg.Device), nil
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
