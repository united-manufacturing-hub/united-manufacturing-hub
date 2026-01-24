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

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// TestChannelProvider implements communicator.ChannelProvider for test scenarios.
type TestChannelProvider struct {
	inbound  chan *transport.UMHMessage
	outbound chan *transport.UMHMessage
}

// NewTestChannelProvider creates a test channel provider with buffered channels.
func NewTestChannelProvider(bufferSize int) *TestChannelProvider {
	return &TestChannelProvider{
		inbound:  make(chan *transport.UMHMessage, bufferSize),
		outbound: make(chan *transport.UMHMessage, bufferSize),
	}
}

// GetChannels returns the inbound (pulled from HTTP) and outbound (to push) channels.
func (p *TestChannelProvider) GetChannels(_ string) (
	chan<- *transport.UMHMessage,
	<-chan *transport.UMHMessage,
) {
	return p.inbound, p.outbound
}

// GetInboundChan returns the inbound channel for reading received messages from the worker.
func (p *TestChannelProvider) GetInboundChan() <-chan *transport.UMHMessage {
	return p.inbound
}

// QueueOutbound queues a message for the worker to push.
func (p *TestChannelProvider) QueueOutbound(msg *transport.UMHMessage) {
	p.outbound <- msg
}

// DrainInbound reads all available messages from the inbound channel (non-blocking).
func (p *TestChannelProvider) DrainInbound() []*transport.UMHMessage {
	var messages []*transport.UMHMessage

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

// CommunicatorRunConfig configures a communicator scenario run with a mock relay server.
type CommunicatorRunConfig struct {
	Logger                  *zap.SugaredLogger        // If nil, creates a development logger
	MockServer              *testutil.MockRelayServer // If nil, creates and manages internally; caller closes if provided
	AuthToken               string                    // Defaults to "test-auth-token"
	InitialPullMessages     []*transport.UMHMessage   // Messages queued for communicator to pull
	InitialOutboundMessages []*transport.UMHMessage   // Messages queued for worker to push
	Duration                time.Duration             // 0 = run until context cancelled; negative = error
	TickInterval            time.Duration             // Defaults to 100ms
}

// CommunicatorRunResult contains observable results after scenario completion (populated after Done closes).
type CommunicatorRunResult struct {
	Error             error                   // Non-nil if scenario setup failed
	Done              <-chan struct{}         // Closes when scenario completes
	Shutdown          func()                  // Triggers graceful shutdown
	ReceivedMessages  []*transport.UMHMessage // Messages pulled from HTTP (nil for HTTP-only tests)
	PushedMessages    []*transport.UMHMessage // Messages pushed to HTTP
	ConsecutiveErrors int                     // Final consecutive error count from mock server
	AuthCallCount     int                     // Auth endpoint calls (>1 indicates re-auth)
}

// RunCommunicatorScenario runs the FSMv2 communicator worker via ApplicationSupervisor with a mock relay server.
func RunCommunicatorScenario(ctx context.Context, cfg CommunicatorRunConfig) *CommunicatorRunResult {
	done := make(chan struct{})

	if cfg.Duration < 0 {
		close(done)

		return &CommunicatorRunResult{
			Done:  done,
			Error: fmt.Errorf("invalid duration %v: must be non-negative", cfg.Duration),
		}
	}

	if ctx.Err() != nil {
		close(done)

		return &CommunicatorRunResult{
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

		return &CommunicatorRunResult{
			Done:  done,
			Error: errors.New("mock server started but URL is empty"),
		}
	}

	for _, msg := range cfg.InitialPullMessages {
		mockServer.QueuePullMessage(msg)
	}

	channelProvider := NewTestChannelProvider(100)
	communicator.SetChannelProvider(channelProvider)

	for _, msg := range cfg.InitialOutboundMessages {
		channelProvider.QueueOutbound(msg)
	}

	authToken := cfg.AuthToken
	if authToken == "" {
		authToken = "test-auth-token"
	}

	scenarioConfig := fmt.Sprintf(`
children:
  - name: "communicator-1"
    workerType: "communicator"
    userSpec:
      config: |
        relayURL: "%s"
        instanceUUID: "test-instance-uuid"
        authToken: "%s"
        timeout: "5s"
`, serverURL, authToken)

	testScenario := Scenario{
		Name:        "communicator-test",
		Description: "Test communicator with mock server",
		YAMLConfig:  scenarioConfig,
	}

	logger := cfg.Logger
	if logger == nil {
		devLogger, _ := zap.NewDevelopment()
		logger = devLogger.Sugar()
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
			communicator.ClearChannelProvider()
		}

		if ownsMockServer {
			mockServer.Close()
		}

		close(done)

		return &CommunicatorRunResult{
			Done:     done,
			Shutdown: func() {},
			Error:    fmt.Errorf("failed to start scenario: %w", err),
		}
	}

	result := &CommunicatorRunResult{
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
			communicator.ClearChannelProvider()
		}

		if ownsMockServer {
			mockServer.Close()
		}

		close(done)
	}()

	return result
}
