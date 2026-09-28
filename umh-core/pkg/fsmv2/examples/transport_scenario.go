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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
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

// relayServerKey holds the scenario's mock relay server. Only this scenario's
// Run reads it, to read the server's URL and its counters; the transport
// worker never does.
var relayServerKey = config.NewDependencyKey[*testutil.MockRelayServer]("examples.relay_server")

// TransportScenarioV2 runs one transport child against a mock relay server
// and a test channel provider held in the dependency map: it creates the
// child with valid auth, waits for it to authenticate and reach Running, then
// queues two outbound messages and waits for the server to receive both.
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

		// Declared as the interface, because SetDependency stores the value
		// under the key's type.
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

		env.Step("create transport with valid auth against the mock relay server")

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

// TransportRunConfig configures a transport scenario run with a mock relay server.
type TransportRunConfig struct {
	Logger                  deps.FSMLogger             // If nil, creates a no-op logger
	MockServer              *testutil.MockRelayServer // If nil, creates and manages internally; caller closes if provided
	AuthToken               string                    // Defaults to "test-auth-token"
	InitialPullMessages     []*types.UMHMessage   // Messages queued for transport to pull
	InitialOutboundMessages []*types.UMHMessage   // Messages queued for worker to push
	Duration                time.Duration             // 0 = run until context cancelled; negative = error
	TickInterval            time.Duration             // Defaults to 100ms
}

// TransportRunResult contains observable results after scenario completion (populated after Done closes).
type TransportRunResult struct {
	Error             error                   // Non-nil if scenario setup failed
	Done              <-chan struct{}         // Closes when scenario completes
	Shutdown          func()                  // Triggers graceful shutdown
	ReceivedMessages  []*types.UMHMessage // Messages pulled from HTTP (nil for HTTP-only tests)
	PushedMessages    []*types.UMHMessage // Messages pushed to HTTP
	ConsecutiveErrors int                     // Final consecutive error count from mock server
	AuthCallCount     int                     // Auth endpoint calls (>1 indicates re-auth)
}

// RunTransportScenario runs the FSMv2 transport worker via ApplicationSupervisor with a mock relay server.
func RunTransportScenario(ctx context.Context, cfg TransportRunConfig) *TransportRunResult {
	done := make(chan struct{})

	if cfg.Duration < 0 {
		close(done)

		return &TransportRunResult{
			Done:  done,
			Error: fmt.Errorf("invalid duration %v: must be non-negative", cfg.Duration),
		}
	}

	if ctx.Err() != nil {
		close(done)

		return &TransportRunResult{
			Done:  done,
			Error: fmt.Errorf("context already cancelled: %w", ctx.Err()),
		}
	}

	var mockServer *testutil.MockRelayServer

	var ownsMockServer bool

	if cfg.MockServer != nil {
		mockServer = cfg.MockServer
		ownsMockServer = false
	} else {
		mockServer = testutil.NewMockRelayServer()
		ownsMockServer = true
	}

	serverURL := mockServer.URL()

	if serverURL == "" {
		if ownsMockServer {
			mockServer.Close()
		}

		close(done)

		return &TransportRunResult{
			Done:  done,
			Error: errors.New("mock server started but URL is empty"),
		}
	}

	for _, msg := range cfg.InitialPullMessages {
		mockServer.QueuePullMessage(msg)
	}

	// Size buffer to accommodate seed messages (prevents QueueOutbound from blocking)
	bufferSize := 100
	if len(cfg.InitialOutboundMessages) > bufferSize {
		bufferSize = len(cfg.InitialOutboundMessages)
	}

	channelProvider := NewTransportTestChannelProvider(bufferSize)
	transportWorker.SetChannelProvider(channelProvider)

	for _, msg := range cfg.InitialOutboundMessages {
		channelProvider.QueueOutbound(msg)
	}

	authToken := cfg.AuthToken
	if authToken == "" {
		authToken = "test-auth-token"
	}

	scenarioConfig := fmt.Sprintf(`
children:
  - name: "transport-1"
    workerType: "transport"
    userSpec:
      config: |
        relayURL: "%s"
        instanceUUID: "test-instance-uuid"
        authToken: "%s"
        timeout: "5s"
`, serverURL, authToken)

	testScenario := Scenario{
		Name:        "transport-test",
		Description: "Test transport worker with mock server via ApplicationSupervisor",
		YAMLConfig:  scenarioConfig,
	}

	logger := cfg.Logger
	if logger == nil {
		logger = deps.NewNopFSMLogger()
	}

	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = 100 * time.Millisecond
	}

	store := SetupStore(logger)

	runResult, err := Run(ctx, RunConfig{
		Scenario:     testScenario,
		Duration:     0,
		TickInterval: tickInterval,
		Logger:       logger,
		Store:        store,
	})
	if err != nil {
		if channelProvider != nil {
			transportWorker.ClearChannelProvider()
		}

		if ownsMockServer {
			mockServer.Close()
		}

		close(done)

		return &TransportRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("failed to start scenario: %w", err),
		}
	}

	result := &TransportRunResult{
		Done:     done,
		Shutdown: runResult.Shutdown,
		Error:    nil,
	}

	go func() {
		if cfg.Duration > 0 {
			select {
			case <-time.After(cfg.Duration):
				runResult.Shutdown()
			case <-ctx.Done():
				runResult.Shutdown()
			case <-runResult.Done:
			}
		} else {
			select {
			case <-ctx.Done():
				runResult.Shutdown()
			case <-runResult.Done:
			}
		}

		<-runResult.Done

		if channelProvider != nil {
			result.ReceivedMessages = channelProvider.DrainInbound()
		}

		result.PushedMessages = mockServer.GetPushedMessages()
		result.AuthCallCount = mockServer.AuthCallCount()

		if channelProvider != nil {
			transportWorker.ClearChannelProvider()
		}

		if ownsMockServer {
			mockServer.Close()
		}

		close(done)
	}()

	return result
}
