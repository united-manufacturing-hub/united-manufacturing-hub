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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// CommunicatorScenarioV2 runs one communicator child against a mock relay
// server, with a test channel provider in the dependency map.
var CommunicatorScenarioV2 = ScenarioV2{
	Name:        "communicator",
	Description: "Communicator worker: authenticates against a mock relay server and pushes a queued message",

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
		// compile. One provider serves both keys. The communicator reads
		// communicator.ChannelProviderKey. The transport child it spawns reads
		// transport.ChannelProviderKey, because a child's map includes its
		// parent's.
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

		providerRaw, ok := config.LookupDependency(env.Dependencies, communicator.ChannelProviderKey)
		if !ok {
			return errors.New("the communicator scenario's dependency map holds no channel provider under communicator.ChannelProviderKey")
		}

		provider, ok := providerRaw.(*TransportTestChannelProvider)
		if !ok {
			return errors.New("the channel provider under communicator.ChannelProviderKey is not the scenario's TransportTestChannelProvider")
		}

		ref := dynamicchildren.Ref{WorkerType: "communicator", Name: "communicator-1"}

		env.Step("create communicator with valid auth against the mock relay server")

		if err := env.Client.Upsert(ref, map[string]any{
			"state":        "running",
			"relayURL":     server.URL(),
			"instanceUUID": "test-instance-uuid",
			"authToken":    "test-auth-token",
			"timeout":      "5s",
		}); err != nil {
			return err
		}

		if err := env.WaitFor(ctx, "communicator authenticates and reaches Syncing",
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
			}); err != nil {
			return err
		}

		env.Step("queue one outbound message")

		provider.QueueOutbound(&types.UMHMessage{InstanceUUID: "test-instance", Content: "status-update"})

		return env.WaitFor(ctx, "the server received the queued message",
			func(ctx context.Context) (bool, string, error) {
				pushed := len(server.GetPushedMessages())
				return pushed == 1, fmt.Sprintf("pushed=%d", pushed), nil
			})
	},
}
