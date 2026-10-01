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
	"net"
	"sync/atomic"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	fsmv2nmap "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	nmapservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// mockDialer is the target port. Run sets open while the collector goroutine dials.
type mockDialer struct {
	open atomic.Bool
}

func (m *mockDialer) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	if !m.open.Load() {
		return nil, errors.New("connection refused")
	}

	remote, local := net.Pipe()

	_ = remote.Close()

	return local, nil
}

// NmapScenarioV2 opens and then closes the port one nmap worker dials. Like the
// fsmv1 nmap worker it replaces, nmap stays running when the port closes. The
// fsmv1 connection worker decides that means down
// (ConnectionInstance.IsConnectionNmapDown in pkg/fsm/connection). So the
// scenario waits for port_state, not for degraded.
var NmapScenarioV2 = ScenarioV2{
	Name:        "nmap",
	Description: "Port monitor: dials a target through a mock dialer, reports the port open then closed",

	Dependencies: func() (map[string]any, func(), error) {
		mock := &mockDialer{}

		mock.open.Store(true)

		deps := map[string]any{}

		var d fsmv2nmap.Dialer = mock

		config.SetDependency(deps, fsmv2nmap.DialerKey, d)

		return deps, nil, nil
	},

	Run: func(ctx context.Context, env Env) error {
		d, ok := config.LookupDependency(env.Dependencies, fsmv2nmap.DialerKey)
		if !ok {
			return errors.New("the nmap scenario's dependency map holds no dialer under fsmv2nmap.DialerKey")
		}

		mock, ok := d.(*mockDialer)
		if !ok {
			return errors.New("the dialer under fsmv2nmap.DialerKey is not the scenario's mockDialer")
		}

		const target = "10.0.0.1"

		ref := dynamicchildren.Ref{WorkerType: "nmap", Name: "nmap-1"}

		env.Step("create nmap aimed at " + target + ":502 with the port open")

		if err := env.Client.Upsert(ref, map[string]any{
			"state": "running",
			"nmapServiceConfig": map[string]any{
				"target": target,
				"port":   502,
			},
		}); err != nil {
			return err
		}

		waitForPortState := func(want string) error {
			return env.WaitFor(ctx, "store shows port_state "+want,
				func(ctx context.Context) (bool, string, error) {
					obs, err := fsmv2client.Get[simple.Status[fsmv2nmap.NmapStatus]](ctx, env.Client, ref)
					if err != nil {
						if errors.Is(err, fsmv2client.ErrNotObserved) {
							return false, "the worker has not published an observation yet", nil
						}

						return false, "", err
					}

					dialedUpsertedTarget := obs.Status.Result.Target == target
					done := dialedUpsertedTarget && obs.Status.Result.PortState == want

					seen := "target=" + obs.Status.Result.Target +
						" port_state=" + obs.Status.Result.PortState

					return done, seen, nil
				})
		}

		if err := waitForPortState(string(nmapservice.PortStateOpen)); err != nil {
			return err
		}

		env.Step("close the port and wait for port_state closed")
		mock.open.Store(false)

		return waitForPortState(string(nmapservice.PortStateClosed))
	},
}
