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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	transportWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
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

// QueueOutbound queues a message for the worker to push, blocking if the channel
// is full.
func (p *TransportTestChannelProvider) QueueOutbound(msg *types.UMHMessage) {
	p.outbound <- msg
}

// TryQueueOutbound queues a message and reports whether there was room, without
// blocking. This is what the production subscriber does
// (pkg/communicator/pkg/subscriber/subscribers.go), so a scenario that models the
// once-per-second status producer must use this and count the false results
// rather than block on QueueOutbound.
func (p *TransportTestChannelProvider) TryQueueOutbound(msg *types.UMHMessage) bool {
	select {
	case p.outbound <- msg:
		return true
	default:
		return false
	}
}

// OutboundLen returns the number of messages waiting on the outbound channel.
func (p *TransportTestChannelProvider) OutboundLen() int {
	return len(p.outbound)
}

// OutboundCap returns the capacity of the outbound channel.
func (p *TransportTestChannelProvider) OutboundCap() int {
	return cap(p.outbound)
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

// TransportRunConfig configures a transport scenario run with a mock relay server.
type TransportRunConfig struct {
	Logger                  deps.FSMLogger            // If nil, creates a no-op logger
	MockServer              *testutil.MockRelayServer // If nil, creates and manages internally; caller closes if provided
	AuthToken               string                    // Defaults to "test-auth-token"
	InitialPullMessages     []*types.UMHMessage       // Messages queued for transport to pull
	InitialOutboundMessages []*types.UMHMessage       // Messages queued for worker to push
	Duration                time.Duration             // 0 = run until context cancelled; negative = error
	TickInterval            time.Duration             // Defaults to 100ms
	// HTTPTimeout caps each individual HTTP request. Defaults to 5s. Production
	// uses 10s. Lower it to make a stalled endpoint fail sooner than the 30s
	// per-action budget, raise it to let one request outlive that budget.
	HTTPTimeout time.Duration
	// ChannelProvider replaces the internally created one. Supply it when the test
	// needs to keep writing to the outbound channel while the scenario runs -- the
	// internal provider is not returned, so it cannot be reached mid-run.
	ChannelProvider *TransportTestChannelProvider
}

// TransportRunResult contains observable results after scenario completion (populated after Done closes).
type TransportRunResult struct {
	Error             error               // Non-nil if scenario setup failed
	Done              <-chan struct{}     // Closes when scenario completes
	Shutdown          func()              // Triggers graceful shutdown
	ReceivedMessages  []*types.UMHMessage // Messages pulled from HTTP (nil for HTTP-only tests)
	PushedMessages    []*types.UMHMessage // Messages pushed to HTTP
	ConsecutiveErrors int                 // Final consecutive error count from mock server
	AuthCallCount     int                 // Auth endpoint calls (>1 indicates re-auth)
	// ShutdownClean carries the supervisor's drain outcome out to callers, so a
	// CustomRunner wrapping this can populate RunResult.ShutdownClean. Without it
	// every wrapped run exits non-zero however well it went. Valid after Done.
	ShutdownClean bool
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

	channelProvider := cfg.ChannelProvider
	if channelProvider == nil {
		channelProvider = NewTransportTestChannelProvider(bufferSize)
	}

	transportWorker.SetChannelProvider(channelProvider)

	for _, msg := range cfg.InitialOutboundMessages {
		channelProvider.QueueOutbound(msg)
	}

	authToken := cfg.AuthToken
	if authToken == "" {
		authToken = "test-auth-token"
	}

	httpTimeout := cfg.HTTPTimeout
	if httpTimeout <= 0 {
		httpTimeout = 5 * time.Second
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
        timeout: "%s"
`, serverURL, authToken, httpTimeout)

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
		result.ShutdownClean = runResult.ShutdownClean

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
