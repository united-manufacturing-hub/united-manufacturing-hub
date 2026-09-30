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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	transportWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// CommunicatorScenarioV2 runs one communicator child against a mock relay
// server, with a test channel provider in the dependency map.
//
// The communicator spawns a transport child, which authenticates. Until that
// child is healthy the communicator counts zero healthy children, so it moves
// Syncing -> Recovering with "healthy=0, unhealthy=0" right after it starts.
// It returns to Syncing about a second later, once the transport child has
// authenticated.
var CommunicatorScenarioV2 = ScenarioV2{
	Name:        "communicator",
	Description: "Communicator worker: reaches Syncing once its transport child authenticates against a mock relay server",

	Dependencies: func() (map[string]any, func(), error) {
		server := testutil.NewMockRelayServer()

		serverURL := server.URL()
		if serverURL == "" {
			server.Close()
			return nil, nil, errors.New("mock relay server started but its URL is empty")
		}

		provider := NewTransportTestChannelProvider(100)

		deps := map[string]any{}

		// Declared as each key's interface: a *TransportTestChannelProvider
		// argument does not match the key's type, so SetDependency would not
		// compile. The transport child the communicator spawns reads
		// transport.ChannelProviderKey, because a child's map includes its parent's.
		var cp communicator.ChannelProvider = provider
		config.SetDependency(deps, communicator.ChannelProviderKey, cp)

		var tp transportWorker.ChannelProvider = provider
		config.SetDependency(deps, transportWorker.ChannelProviderKey, tp)

		config.SetDependency(deps, relayServerKey, server)

		return deps, func() { server.Close() }, nil
	},

	Run: func(ctx context.Context, env Env) error {
		server, ok := config.LookupDependency(env.Dependencies, relayServerKey)
		if !ok {
			return errors.New("the communicator scenario's dependency map holds no relay server under relayServerKey")
		}

		ref := dynamicchildren.Ref{WorkerType: "communicator", Name: "communicator-1"}

		env.Step("create communicator against the mock relay server; it passes through Recovering until its transport child authenticates")

		if err := env.Client.Upsert(ref, map[string]any{
			"state":        "running",
			"relayURL":     server.URL(),
			"instanceUUID": "test-instance-uuid",
			"authToken":    "test-auth-token",
			"timeout":      "5s",
		}); err != nil {
			return err
		}

		return env.WaitFor(ctx, "the communicator reaches Syncing after its transport child authenticates",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[communicator.CommunicatorStatus](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the communicator child has not published an observation yet", nil
					}

					return false, "", err
				}

				authCalls := server.AuthCallCount()
				done := obs.State == "Syncing" && authCalls >= 1
				return done, fmt.Sprintf("state=%s authCalls=%d", obs.State, authCalls), nil
			})
	},
}
