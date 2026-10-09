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
	example_bridge "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplebridge"
	example_flow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleflow"
)

// BridgeVariablesScenario checks that a bridge's own values — its config
// fields, its variables, the agent location and the config values the
// application hands down — render into its flow templates, and that every
// bridge's flows stay its own: a second bridge with the same template renders
// its own values, and a bridge with no variables renders its location alone.
var BridgeVariablesScenario = Scenario{
	Name:        "bridge-variables",
	Description: "A bridge's connection, location, variables and handed-down config values render into its flow templates; two bridges with one shared template render apart",

	Run: func(ctx context.Context, env Env) error {
		env.Step("hand the application its config values: the agent's location and the historian's host and password")

		if err := env.Client.SetConfigVariables(map[string]any{
			"agent": map[string]any{
				"location": map[string]any{"0": "acme", "1": ""},
			},
			"historian": map[string]any{
				"timescale": map[string]any{
					"host":     "db.local",
					"password": "${DB_PASS}",
				},
			},
		}); err != nil {
			return fmt.Errorf("set the config variables: %w", err)
		}

		env.Step("upsert bridge b1 with a read and a write template that use its connection, location and variables")

		b1Ref := dynamicchildren.Ref{WorkerType: example_bridge.WorkerTypeName, Name: "b1"}

		b1Read := `{{ .name }}
{{ .connection.host }}:{{ .connection.port }}
{{ .variables.SLAVE_ID }}
{{ .location_path }}.machine-1
{{ index .location "3" }}
{{- range .variables.AddressMappings }}
{{ .name }}={{ .addr }}{{ if index . "scale" }} scale={{ .scale }}@{{ $.location_path }}{{ end }}
{{- end }}
{{ .config.historian.timescale.host }}
${PLC_PASSWORD} ${! meta("x") }
{{ .config.historian.timescale.password }}
{{ .variables.BAUD }}
{{ .variables.NODE }}
`

		b1Write := `{{ .IP }}:{{ .PORT }} {{ .SLAVE_ID }} {{ .historian.timescale.host }}
`

		if err := env.Client.Upsert(b1Ref, map[string]any{
			"state": "running",
			"name":  "b1",
			"connection": map[string]any{
				"host": "10.0.0.5",
				"port": 502,
			},
			"location": map[string]any{
				"1": "hall",
				"3": "cell",
			},
			"variables": map[string]any{
				"SLAVE_ID": 7,
				"BAUD":     1000000,
				"NODE":     `ns=3;s="DB"."X"`,
				"AddressMappings": []any{
					map[string]any{"name": "temp", "addr": 40001, "scale": 1000000},
					map[string]any{"name": "press", "addr": 40002},
				},
			},
			"read":  b1Read,
			"write": b1Write,
		}); err != nil {
			return fmt.Errorf("upsert bridge b1: %w", err)
		}

		env.Step("wait for b1's read flow to render every value")

		// The literal texts the templates render, computed by hand from the
		// values above, never from the same inputs in code: the check fails
		// exactly when the render fails.
		const b1ReadWant = `b1
10.0.0.5:502
7
acme.hall.unknown.cell.machine-1
cell
temp=40001 scale=1000000@acme.hall.unknown.cell
press=40002
db.local
${PLC_PASSWORD} ${! meta("x") }
${DB_PASS}
1000000
ns=3;s="DB"."X"
`

		const b1WriteWant = `10.0.0.5:502 7 db.local
`

		b1ReadRef := dynamicchildren.Ref{WorkerType: example_flow.WorkerTypeName, Name: "b1-read"}
		b1WriteRef := dynamicchildren.Ref{WorkerType: example_flow.WorkerTypeName, Name: "b1-write"}

		for _, flow := range []struct {
			ref  dynamicchildren.Ref
			want string
		}{
			{b1ReadRef, b1ReadWant},
			{b1WriteRef, b1WriteWant},
		} {
			flow := flow

			if err := env.WaitFor(ctx,
				fmt.Sprintf("b1's %s flow is Running and its desired config holds the exact rendered text", flow.ref.Name),
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_flow.ExampleflowStatus](ctx, env.Client, flow.ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the flow has not published an observation yet", nil
						}

						return false, "", err
					}

					cfg, err := fsmv2client.GetDesired[example_flow.ExampleflowConfig](ctx, env.Client, flow.ref)
					if err != nil {
						return false, "", err
					}

					done := obs.State == "Running" && string(cfg.Flow) == flow.want

					return done, fmt.Sprintf("state=%s flow=%q", obs.State, cfg.Flow), nil
				}); err != nil {
				return err
			}
		}

		env.Step("upsert bridge b2 with b1's read template and b2's own values")

		b2Ref := dynamicchildren.Ref{WorkerType: example_bridge.WorkerTypeName, Name: "b2"}

		b2Read := b1Read

		if err := env.Client.Upsert(b2Ref, map[string]any{
			"state": "running",
			"name":  "b2",
			"connection": map[string]any{
				"host": "10.0.0.6",
				"port": 502,
			},
			"location": map[string]any{
				"1": "line",
				"3": "cell",
			},
			"variables": map[string]any{
				"SLAVE_ID": 8,
				"BAUD":     1000000,
				"NODE":     `ns=3;s="DB2"."X"`,
				"AddressMappings": []any{
					map[string]any{"name": "temp", "addr": 40001, "scale": 1000000},
					map[string]any{"name": "press", "addr": 40002},
				},
			},
			"read": b2Read,
		}); err != nil {
			return fmt.Errorf("upsert bridge b2: %w", err)
		}

		env.Step("wait for b2's read flow to render b2's own values")

		const b2ReadWant = `b2
10.0.0.6:502
8
acme.line.unknown.cell.machine-1
cell
temp=40001 scale=1000000@acme.line.unknown.cell
press=40002
db.local
${PLC_PASSWORD} ${! meta("x") }
${DB_PASS}
1000000
ns=3;s="DB2"."X"
`

		b2ReadRef := dynamicchildren.Ref{WorkerType: example_flow.WorkerTypeName, Name: "b2-read"}

		if err := env.WaitFor(ctx,
			"b2's read flow is Running and its desired config holds b2's rendered text",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_flow.ExampleflowStatus](ctx, env.Client, b2ReadRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the flow has not published an observation yet", nil
					}

					return false, "", err
				}

				cfg, err := fsmv2client.GetDesired[example_flow.ExampleflowConfig](ctx, env.Client, b2ReadRef)
				if err != nil {
					return false, "", err
				}

				done := obs.State == "Running" && string(cfg.Flow) == b2ReadWant

				return done, fmt.Sprintf("state=%s flow=%q", obs.State, cfg.Flow), nil
			}); err != nil {
			return err
		}

		env.Step("upsert bridge b3 with no variables and a read template of its location path alone")

		b3Ref := dynamicchildren.Ref{WorkerType: example_bridge.WorkerTypeName, Name: "b3"}

		if err := env.Client.Upsert(b3Ref, map[string]any{
			"state": "running",
			"name":  "b3",
			"location": map[string]any{
				"1": "hall",
				"3": "cell",
			},
			"read": "{{ .location_path }}",
		}); err != nil {
			return fmt.Errorf("upsert bridge b3: %w", err)
		}

		env.Step("wait for b3's read flow to render its location path")

		const b3ReadWant = "acme.hall.unknown.cell"

		b3ReadRef := dynamicchildren.Ref{WorkerType: example_flow.WorkerTypeName, Name: "b3-read"}

		if err := env.WaitFor(ctx,
			"b3's read flow is Running and its desired config holds its location path",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[example_flow.ExampleflowStatus](ctx, env.Client, b3ReadRef)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the flow has not published an observation yet", nil
					}

					return false, "", err
				}

				cfg, err := fsmv2client.GetDesired[example_flow.ExampleflowConfig](ctx, env.Client, b3ReadRef)
				if err != nil {
					return false, "", err
				}

				done := obs.State == "Running" && string(cfg.Flow) == b3ReadWant

				return done, fmt.Sprintf("state=%s flow=%q", obs.State, cfg.Flow), nil
			}); err != nil {
			return err
		}

		return nil
	},
}
