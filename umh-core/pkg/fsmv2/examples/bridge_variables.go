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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	example_bridge "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplebridge"
	example_flow "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleflow"
)

// BridgeVariablesScenario checks that a bridge's own values render into its
// flow templates. Those values are its config fields, its variables, the agent
// location and the config values the application hands down. It checks that
// each bridge's flows stay its own. It also checks that a config mistake holds
// back only the worker or the flow it is a mistake in. A broken write template
// keeps write's last good text while read keeps rendering. A bridge whose
// connection host or variable name is a mistake is never created. A bridge
// whose read template cannot parse runs, and its read flow is never created.
var BridgeVariablesScenario = Scenario{
	Name:        "bridge-variables",
	Description: "A bridge's values render into its flows; two bridges with one shared template render apart; a config mistake holds back only the flow it is a mistake in",

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

		b1Values := func() map[string]any {
			return map[string]any{
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
			}
		}

		if err := env.Client.Upsert(b1Ref, b1Values()); err != nil {
			return fmt.Errorf("upsert bridge b1: %w", err)
		}

		env.Step("wait for b1's flows to render every value")

		// The literal texts the templates render, written by hand: a failing
		// render fails the wait, and the texts are never computed from the
		// same inputs in code.
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

		// waitFlow waits until the flow child named name is Running and its
		// desired config holds exactly the rendered text want.
		waitFlow := func(name string, want string) error {
			ref := dynamicchildren.Ref{WorkerType: example_flow.WorkerTypeName, Name: name}

			return env.WaitFor(ctx, name+"'s flow is Running and its desired config holds the exact rendered text",
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[example_flow.ExampleflowStatus](ctx, env.Client, ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the flow has not published an observation yet", nil
						}

						return false, "", err
					}

					cfg, err := fsmv2client.GetDesired[example_flow.ExampleflowConfig](ctx, env.Client, ref)
					if err != nil {
						return false, "", err
					}

					done := obs.State == "Running" && string(cfg.Flow) == want

					return done, fmt.Sprintf("state=%s flow=%q", obs.State, cfg.Flow), nil
				})
		}

		if err := waitFlow("b1-read", b1ReadWant); err != nil {
			return fmt.Errorf("wait for b1's read flow: %w", err)
		}

		if err := waitFlow("b1-write", b1WriteWant); err != nil {
			return fmt.Errorf("wait for b1's write flow: %w", err)
		}

		env.Step("upsert bridge b2 with b1's read template and b2's own values")

		b2Ref := dynamicchildren.Ref{WorkerType: example_bridge.WorkerTypeName, Name: "b2"}

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
			"read": b1Read,
		}); err != nil {
			return fmt.Errorf("upsert bridge b2: %w", err)
		}

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

		if err := waitFlow("b2-read", b2ReadWant); err != nil {
			return fmt.Errorf("wait for b2's read flow: %w", err)
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

		if err := waitFlow("b3-read", "acme.hall.unknown.cell"); err != nil {
			return fmt.Errorf("wait for b3's read flow: %w", err)
		}

		env.Step("S7: break b1's write template with a misspelled variable name")

		brokenWrite := b1Values()
		brokenWrite["write"] = "{{ .variables.SLAVEID }}\n"

		if err := env.Client.Upsert(b1Ref, brokenWrite); err != nil {
			return fmt.Errorf("upsert bridge b1 with a broken write template: %w", err)
		}

		if err := env.WaitForWarning(ctx, "config_error", "SLAVEID"); err != nil {
			return fmt.Errorf("wait for the SLAVEID config error: %w", err)
		}

		// The broken spec holds back only write: it keeps its last good
		// rendered text, and read keeps rendering.
		if err := waitFlow("b1-write", b1WriteWant); err != nil {
			return fmt.Errorf("write keeps its last good rendered text: %w", err)
		}

		if err := waitFlow("b1-read", b1ReadWant); err != nil {
			return fmt.Errorf("read keeps rendering while write is broken: %w", err)
		}

		env.Step("S8: restore b1's write template with a suffix")

		fixedWrite := b1Values()
		fixedWrite["write"] = b1Write + " fixed\n"

		if err := env.Client.Upsert(b1Ref, fixedWrite); err != nil {
			return fmt.Errorf("upsert bridge b1 with a restored write template: %w", err)
		}

		if err := waitFlow("b1-write", b1WriteWant+" fixed\n"); err != nil {
			return fmt.Errorf("wait for write's new rendered text: %w", err)
		}

		// absentForNPolls checks, ten times with a pause between, that every
		// given child is absent (the client holds no observation for it)
		// while b1's read and write flows stay Running. The caller waits for
		// the positive events first: the config_error warning.
		absentForNPolls := func(refs ...dynamicchildren.Ref) error {
			for range 10 {
				for _, ref := range refs {
					if _, err := fsmv2client.Get[example_flow.ExampleflowStatus](ctx, env.Client, ref); err == nil {
						return fmt.Errorf("the child %s exists but must not", ref.Name)
					} else if !errors.Is(err, fsmv2client.ErrNotObserved) {
						return err
					}
				}

				for _, name := range []string{"b1-read", "b1-write"} {
					ref := dynamicchildren.Ref{WorkerType: example_flow.WorkerTypeName, Name: name}

					obs, err := fsmv2client.Get[example_flow.ExampleflowStatus](ctx, env.Client, ref)
					if err != nil {
						return err
					}

					if obs.State != "Running" {
						return fmt.Errorf("%s left Running: state=%s", name, obs.State)
					}
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
			}

			return nil
		}

		env.Step("S9: a bridge whose connection host holds a placeholder is never created")

		b4Ref := dynamicchildren.Ref{WorkerType: example_bridge.WorkerTypeName, Name: "b4"}

		if err := env.Client.Upsert(b4Ref, map[string]any{
			"state": "running",
			"name":  "b4",
			"connection": map[string]any{
				"host": "{{ .x }}",
				"port": 502,
			},
			"read": "{{ .IP }}",
		}); err != nil {
			return fmt.Errorf("upsert bridge b4: %w", err)
		}

		if err := env.WaitForWarning(ctx, "config_error", "connection.host"); err != nil {
			return fmt.Errorf("wait for the connection.host config error: %w", err)
		}

		if err := absentForNPolls(b4Ref); err != nil {
			return fmt.Errorf("poll for b4's absence while b1's flows stay Running: %w", err)
		}

		env.Step("S10: a bridge whose variable claims the location name is never created")

		b5Ref := dynamicchildren.Ref{WorkerType: example_bridge.WorkerTypeName, Name: "b5"}

		if err := env.Client.Upsert(b5Ref, map[string]any{
			"state": "running",
			"name":  "b5",
			"variables": map[string]any{
				"location": "own",
			},
			"read": "{{ .location }}",
		}); err != nil {
			return fmt.Errorf("upsert bridge b5: %w", err)
		}

		if err := env.WaitForWarning(ctx, "config_error", `"location" is both`); err != nil {
			return fmt.Errorf("wait for the location clash config error: %w", err)
		}

		if err := absentForNPolls(b5Ref); err != nil {
			return fmt.Errorf("poll for b5's absence while b1's flows stay Running: %w", err)
		}

		env.Step("S14: a bridge whose read template cannot parse runs, and its read flow is never created")

		b6Ref := dynamicchildren.Ref{WorkerType: example_bridge.WorkerTypeName, Name: "b6"}

		if err := env.Client.Upsert(b6Ref, map[string]any{
			"state": "running",
			"name":  "b6",
			"write": "{{ .name }}",
			"read":  "{{ .location.0 }}",
		}); err != nil {
			return fmt.Errorf("upsert bridge b6: %w", err)
		}

		if err := env.WaitForWarning(ctx, "config_error", `unexpected ".0" in operand`); err != nil {
			return fmt.Errorf("wait for the location.0 parse error: %w", err)
		}

		b6ReadRef := dynamicchildren.Ref{WorkerType: example_flow.WorkerTypeName, Name: "b6-read"}

		if err := absentForNPolls(b6ReadRef); err != nil {
			return fmt.Errorf("poll for b6-read's absence while b6 itself runs: %w", err)
		}

		b6Obs, err := fsmv2client.Get[example_bridge.ExamplebridgeStatus](ctx, env.Client, b6Ref)
		if err != nil {
			return fmt.Errorf("read b6: %w", err)
		}

		if b6Obs.State != "Running" {
			return fmt.Errorf("b6 left Running: state=%s", b6Obs.State)
		}

		env.Step("S11: only the changed flow changes")

		// read uses the variables, the agent location and the config
		// historian; write uses only the connection port.
		s11Read := `{{ .variables.SLAVE_ID }} {{ .location_path }} {{ .config.historian.timescale.host }}
`
		s11Write := "{{ .connection.port }}\n"

		// The client exposes a child spec's names only through the bridge's
		// own desired config: the names the flows render from are a deep
		// copy of the bridge's. A probe render of the changed name against
		// the bridge's stored names proves the new names arrived.
		bridgeNames := func(probe string, want string) error {
			return env.WaitFor(ctx, "b1's stored desired names render "+probe,
				func(ctx context.Context) (bool, string, error) {
					cfg, err := fsmv2client.GetDesired[example_bridge.ExamplebridgeConfig](ctx, env.Client, b1Ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNoDesiredState) {
							return false, "the bridge has no desired state yet", nil
						}

						return false, "", err
					}

					rendered, err := config.Template(probe).Render("probe", cfg.Names)
					if err != nil {
						return false, "", err
					}

					return rendered == want, fmt.Sprintf("probe=%q", rendered), nil
				})
		}

		upsertB1 := func(variables, agent, historian, location map[string]any) error {
			// The password stays with the historian through every change: b2's
			// read template renders it, so dropping it would break b2.
			timescale := map[string]any{"password": "${DB_PASS}"}
			for k, v := range historian {
				timescale[k] = v
			}

			cfgValues := map[string]any{
				"agent": agent,
				"historian": map[string]any{
					"timescale": timescale,
				},
			}

			if err := env.Client.SetConfigVariables(cfgValues); err != nil {
				return fmt.Errorf("set the config variables: %w", err)
			}

			return env.Client.Upsert(b1Ref, map[string]any{
				"state": "running",
				"name":  "b1",
				"connection": map[string]any{
					"host": "10.0.0.5",
					"port": 502,
				},
				"location":  location,
				"variables": variables,
				"read":      s11Read,
				"write":     s11Write,
			})
		}

		baseVariables := map[string]any{"SLAVE_ID": 7}
		baseAgent := map[string]any{"location": map[string]any{"0": "acme", "1": ""}}
		baseHistorian := map[string]any{"host": "db.local"}
		baseLocation := map[string]any{"1": "hall", "3": "cell"}

		if err := upsertB1(baseVariables, baseAgent, baseHistorian, baseLocation); err != nil {
			return fmt.Errorf("upsert bridge b1 for the changed-input cases: %w", err)
		}

		const s11WriteWant = "502\n"

		if err := waitFlow("b1-read", "7 acme.hall.unknown.cell db.local\n"); err != nil {
			return fmt.Errorf("read renders the changed-input base: %w", err)
		}

		if err := waitFlow("b1-write", s11WriteWant); err != nil {
			return fmt.Errorf("write renders the changed-input base: %w", err)
		}

		// changeInput changes one input, waits for read's new text and for
		// the new names to reach the bridge's stored desired config, and
		// then asserts write's rendered text is unchanged.
		changeInput := func(label string, variables, agent, historian, location map[string]any,
			readWant, probe, probeWant string,
		) error {
			env.Step("S11: change " + label)

			if err := upsertB1(variables, agent, historian, location); err != nil {
				return fmt.Errorf("upsert bridge b1 with %s: %w", label, err)
			}

			if err := waitFlow("b1-read", readWant); err != nil {
				return fmt.Errorf("read renders %s: %w", label, err)
			}

			if err := bridgeNames(probe, probeWant); err != nil {
				return fmt.Errorf("the new names reach the bridge: %w", err)
			}

			return waitFlow("b1-write", s11WriteWant)
		}

		if err := changeInput("the SLAVE_ID variable",
			map[string]any{"SLAVE_ID": 9}, baseAgent, baseHistorian, baseLocation,
			"9 acme.hall.unknown.cell db.local\n",
			`{{ .variables.SLAVE_ID }}`, "9",
		); err != nil {
			return err
		}

		if err := changeInput("the agent location",
			map[string]any{"SLAVE_ID": 9},
			map[string]any{"location": map[string]any{"0": "omega", "1": ""}},
			baseHistorian, baseLocation,
			"9 omega.hall.unknown.cell db.local\n",
			`{{ .location_path }}`, "omega.hall.unknown.cell",
		); err != nil {
			return err
		}

		if err := changeInput("the bridge's own location",
			map[string]any{"SLAVE_ID": 9},
			map[string]any{"location": map[string]any{"0": "omega", "1": ""}},
			baseHistorian,
			map[string]any{"1": "hall", "3": "line"},
			"9 omega.hall.unknown.line db.local\n",
			`{{ index .location "3" }}`, "line",
		); err != nil {
			return err
		}

		if err := changeInput("the historian host",
			map[string]any{"SLAVE_ID": 9},
			map[string]any{"location": map[string]any{"0": "omega", "1": ""}},
			map[string]any{"host": "db.remote"},
			map[string]any{"1": "hall", "3": "line"},
			"9 omega.hall.unknown.line db.remote\n",
			`{{ .config.historian.timescale.host }}`, "db.remote",
		); err != nil {
			return err
		}

		return nil
	},

	// The config_error warnings of the cases that hold a worker back are
	// expected; no Sentry error or warning comes from any of them.
	ExpectedWarnings: []string{"config_error"},
}
