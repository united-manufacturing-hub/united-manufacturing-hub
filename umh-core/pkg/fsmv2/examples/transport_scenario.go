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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	transportWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// TransportTestChannelProvider implements transport.ChannelProvider for test scenarios.
type TransportTestChannelProvider struct {
	inbound  chan *types.UMHMessage
	outbound chan *types.UMHMessage
}

// NewTransportTestChannelProvider creates a test channel provider with buffered channels.
func NewTransportTestChannelProvider(bufferSize int) *TransportTestChannelProvider {
	return &TransportTestChannelProvider{
		inbound:  make(chan *types.UMHMessage, bufferSize),
		outbound: make(chan *types.UMHMessage, bufferSize),
	}
}

// GetChannels returns the inbound (pulled from HTTP) and outbound (to push) channels.
func (p *TransportTestChannelProvider) GetChannels(_ string) (
	chan<- *types.UMHMessage,
	<-chan *types.UMHMessage,
) {
	return p.inbound, p.outbound
}

// GetInboundStats returns the capacity and current length of the inbound channel.
func (p *TransportTestChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return cap(p.inbound), len(p.inbound)
}

// GetInboundChan returns the inbound channel for reading received messages from the worker.
func (p *TransportTestChannelProvider) GetInboundChan() <-chan *types.UMHMessage {
	return p.inbound
}

// QueueOutbound queues a message for the worker to push.
func (p *TransportTestChannelProvider) QueueOutbound(msg *types.UMHMessage) {
	p.outbound <- msg
}

// DrainInbound reads all available messages from the inbound channel (non-blocking).
func (p *TransportTestChannelProvider) DrainInbound() []*types.UMHMessage {
	var messages []*types.UMHMessage

drainLoop:
	for {
		select {
		case msg, ok := <-p.inbound:
			if !ok {
				break drainLoop
			}

			messages = append(messages, msg)
		default:
			break drainLoop
		}
	}

	return messages
}

// relayServerKey carries the mock relay server to Run; no worker reads it.
var relayServerKey = config.NewDependencyKey[*testutil.MockRelayServer]("examples.relay_server")

// TransportScenarioV2 runs a transport child against a mock relay server.
var TransportScenarioV2 = ScenarioV2{
	Name:        "transport",
	Description: "Transport worker: authenticates against a mock relay server and pushes queued messages",

	Dependencies: func() (map[string]any, func(), error) {
		server := testutil.NewMockRelayServer()

		serverURL := server.URL()
		if serverURL == "" {
			server.Close()
			return nil, nil, errors.New("mock relay server started but its URL is empty")
		}

		provider := NewTransportTestChannelProvider(100)

		deps := map[string]any{}

		// Typed as the interface, or SetDependency cannot infer its type parameter.
		var p transportWorker.ChannelProvider = provider
		config.SetDependency(deps, transportWorker.ChannelProviderKey, p)
		config.SetDependency(deps, relayServerKey, server)

		return deps, func() { server.Close() }, nil
	},

	Run: func(ctx context.Context, env Env) error {
		server, ok := config.LookupDependency(env.Dependencies, relayServerKey)
		if !ok {
			return errors.New("the transport scenario's dependency map holds no relay server under relayServerKey")
		}

		providerRaw, ok := config.LookupDependency(env.Dependencies, transportWorker.ChannelProviderKey)
		if !ok {
			return errors.New("the transport scenario's dependency map holds no channel provider under transportWorker.ChannelProviderKey")
		}

		provider, ok := providerRaw.(*TransportTestChannelProvider)
		if !ok {
			return errors.New("the channel provider under transportWorker.ChannelProviderKey is not the scenario's TransportTestChannelProvider")
		}

		ref := dynamicchildren.Ref{WorkerType: "transport", Name: "transport-1"}

		env.Step("create transport-1 against the mock relay server, which accepts any token; it starts push and pull children and takes the instance UUID the relay server returns")

		if err := env.Client.Upsert(ref, map[string]any{
			"state":        "running",
			"relayURL":     server.URL(),
			"instanceUUID": "test-instance-uuid",
			"authToken":    "test-auth-token",
			"timeout":      "5s",
		}); err != nil {
			return err
		}

		if err := env.WaitFor(ctx, "transport authenticates and reaches Running",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[snapshot.TransportStatus](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the transport child has not published an observation yet", nil
					}

					return false, "", err
				}

				authCalls := server.AuthCallCount()
				done := obs.State == "Running" && authCalls >= 1
				return done, fmt.Sprintf("state=%s authCalls=%d", obs.State, authCalls), nil
			}); err != nil {
			return err
		}

		env.Step("queue two outbound messages")

		provider.QueueOutbound(&types.UMHMessage{InstanceUUID: "test-instance", Content: "msg1"})
		provider.QueueOutbound(&types.UMHMessage{InstanceUUID: "test-instance", Content: "msg2"})

		return env.WaitFor(ctx, "the server received both queued messages",
			func(ctx context.Context) (bool, string, error) {
				pushed := len(server.GetPushedMessages())
				return pushed == 2, fmt.Sprintf("pushed=%d", pushed), nil
			})
	},
}
