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

// TestChannelProvider provides channels for test scenarios.
// It implements communicator.ChannelProvider for injecting test channels.
type TestChannelProvider struct {
	inbound  chan *transport.UMHMessage // Worker writes here (received messages)
	outbound chan *transport.UMHMessage // Worker reads from here (messages to push)
}

// NewTestChannelProvider creates a new test channel provider with buffered channels.
func NewTestChannelProvider(bufferSize int) *TestChannelProvider {
	return &TestChannelProvider{
		inbound:  make(chan *transport.UMHMessage, bufferSize),
		outbound: make(chan *transport.UMHMessage, bufferSize),
	}
}

// GetChannels returns the channels for the specified worker.
// The inbound channel receives messages pulled from HTTP.
// The outbound channel provides messages to push to HTTP.
func (p *TestChannelProvider) GetChannels(_ string) (
	chan<- *transport.UMHMessage,
	<-chan *transport.UMHMessage,
) {
	return p.inbound, p.outbound
}

// GetInboundChan returns the inbound channel for reading received messages.
func (p *TestChannelProvider) GetInboundChan() <-chan *transport.UMHMessage {
	return p.inbound
}

// QueueOutbound adds a message to the outbound channel for the worker to push.
func (p *TestChannelProvider) QueueOutbound(msg *transport.UMHMessage) {
	p.outbound <- msg
}

// DrainInbound reads all available messages from the inbound channel.
// Non-blocking: returns immediately with whatever messages are available.
func (p *TestChannelProvider) DrainInbound() []*transport.UMHMessage {
	var messages []*transport.UMHMessage

drainLoop:
	for {
		select {
		case msg := <-p.inbound:
			messages = append(messages, msg)
		default:
			break drainLoop
		}
	}

	return messages
}

// CommunicatorRunConfig configures a communicator scenario run.
//
// This struct provides configuration for running the FSMv2 communicator worker
// via ApplicationSupervisor with a mock relay server.
//
// # Basic Usage
//
// For CLI usage, you need minimal configuration:
//
//	result := RunCommunicatorScenario(ctx, CommunicatorRunConfig{
//	    Duration: 10 * time.Second,
//	})
//
// # Test Usage with Message Injection
//
// To test message handling, pre-populate the mock server:
//
//	result := RunCommunicatorScenario(ctx, CommunicatorRunConfig{
//	    Duration: 2 * time.Second,
//	    InitialPullMessages: []*transport.UMHMessage{{
//	        InstanceUUID: "test-instance",
//	        Content:      "action-from-backend",
//	    }},
//	})
type CommunicatorRunConfig struct {
	// Duration specifies how long to run the scenario.
	// Use 0 to run until the context is cancelled.
	// Negative values are invalid and return an error.
	Duration time.Duration

	// TickInterval is the supervisor tick interval (default: 100ms if zero).
	TickInterval time.Duration

	// AuthToken authenticates with the mock relay server.
	// If empty, defaults to "test-auth-token".
	AuthToken string

	// Logger outputs scenario logs. If nil, the function creates a development logger.
	Logger *zap.SugaredLogger

	// InitialPullMessages contains messages to queue on the mock server before the scenario starts.
	// The communicator receives these messages during pull operations.
	InitialPullMessages []*transport.UMHMessage

	// InitialOutboundMessages contains messages to queue for pushing.
	// These are provided via the channel provider for the worker to push to HTTP.
	InitialOutboundMessages []*transport.UMHMessage

	// MockServer lets you inject a pre-configured mock server for error testing.
	// If nil, the function creates and manages a new mock server internally.
	// When you provide a MockServer, you are responsible for calling Close().
	MockServer *testutil.MockRelayServer
}

// CommunicatorRunResult contains observable results after scenario completion.
//
// This struct captures everything you need to verify communicator behavior
// using the FSMv2 communicator worker via ApplicationSupervisor.
//
// # Asserting on Results
//
//	result := RunCommunicatorScenario(ctx, cfg)
//	Expect(result.Error).NotTo(HaveOccurred())
//	Expect(result.AuthCallCount).To(BeNumerically(">=", 1), "should authenticate at least once")
//	Expect(result.PushedMessages).To(HaveLen(expectedPushCount))
type CommunicatorRunResult struct {
	// Done channel closes when the scenario completes (matches RunResult pattern).
	Done <-chan struct{}

	// Shutdown triggers graceful shutdown of the scenario.
	// Call this to stop the scenario gracefully (e.g., on Ctrl+C).
	Shutdown func()

	// ReceivedMessages contains messages received via the channel provider.
	// These are messages the communicator pulled from HTTP and forwarded to the inbound channel.
	// Note: Only available when using channel provider; may be nil for HTTP-only tests.
	// Note: Only populated after Done closes.
	ReceivedMessages []*transport.UMHMessage

	// PushedMessages contains all messages the mock server received from the communicator.
	// These are messages the worker pushed to HTTP.
	// Note: Only populated after Done closes.
	PushedMessages []*transport.UMHMessage

	// ConsecutiveErrors is the final consecutive error count when the scenario stopped.
	// Note: This is obtained from mock server error tracking, not FSM state.
	// Note: Only populated after Done closes.
	ConsecutiveErrors int

	// AuthCallCount indicates how many times the communicator called the auth endpoint.
	// Normally 1 for a healthy run. Higher values indicate re-authentication occurred.
	// Note: Only populated after Done closes.
	AuthCallCount int

	// Error is non-nil if the scenario failed to run (setup failure, etc.).
	Error error
}

// RunCommunicatorScenario runs the FSMv2 communicator worker via ApplicationSupervisor.
//
// This function creates a mock relay server, configures the communicator worker
// with the mock server URL, and runs it through the standard ApplicationSupervisor
// workflow. This tests the actual FSMv2 communicator worker implementation.
//
// # What This Function Does
//
//  1. Validates configuration (duration non-negative, context not cancelled)
//  2. Creates and starts a mock relay server (or uses the injected one)
//  3. Queues any InitialPullMessages on the mock server
//  4. Sets up channel provider for outbound messages (if any)
//  5. Builds dynamic YAML config with mock server URL
//  6. Runs via ApplicationSupervisor for the specified Duration
//  7. Captures results (messages, auth calls) from mock server
//  8. Cleans up resources
//  9. Returns results for assertions
//
// # Resource Management
//
// When MockServer is nil, the function manages the server lifecycle internally.
// When you provide a MockServer, you are responsible for closing it.
//
// # CLI Usage Example
//
//	result := RunCommunicatorScenario(ctx, CommunicatorRunConfig{
//	    Duration: 10 * time.Second,
//	    Logger:   logger,
//	})
//	if result.Error != nil {
//	    log.Fatalf("Scenario failed: %v", result.Error)
//	}
//	log.Printf("Auth calls: %d, Messages pushed: %d", result.AuthCallCount, len(result.PushedMessages))
//
// # Test Usage Example
//
//	result := RunCommunicatorScenario(ctx, CommunicatorRunConfig{
//	    Duration: 2 * time.Second,
//	    InitialPullMessages: []*transport.UMHMessage{{
//	        InstanceUUID: "test",
//	        Content:      "test-action",
//	    }},
//	})
//	Expect(result.Error).NotTo(HaveOccurred())
//	Expect(result.AuthCallCount).To(BeNumerically(">=", 1))
func RunCommunicatorScenario(ctx context.Context, cfg CommunicatorRunConfig) *CommunicatorRunResult {
	done := make(chan struct{})

	// Validate configuration
	if cfg.Duration < 0 {
		close(done)

		return &CommunicatorRunResult{
			Done:  done,
			Error: fmt.Errorf("invalid duration %v: must be non-negative", cfg.Duration),
		}
	}

	// Check context before starting
	if ctx.Err() != nil {
		close(done)

		return &CommunicatorRunResult{
			Done:  done,
			Error: fmt.Errorf("context already cancelled: %w", ctx.Err()),
		}
	}

	// Create or use provided mock server
	var mockServer *testutil.MockRelayServer

	var ownsMockServer bool

	if cfg.MockServer != nil {
		mockServer = cfg.MockServer
		ownsMockServer = false
	} else {
		mockServer = testutil.NewMockRelayServer()
		ownsMockServer = true
	}

	// Validate server started
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

	// Queue initial pull messages if provided
	for _, msg := range cfg.InitialPullMessages {
		mockServer.QueuePullMessage(msg)
	}

	// Set up channel provider unconditionally (Phase 1 requirement: singleton MUST be set)
	channelProvider := NewTestChannelProvider(100)
	communicator.SetChannelProvider(channelProvider)

	// Queue outbound messages if provided
	for _, msg := range cfg.InitialOutboundMessages {
		channelProvider.QueueOutbound(msg)
	}

	// Build auth token
	authToken := cfg.AuthToken
	if authToken == "" {
		authToken = "test-auth-token"
	}

	// Build dynamic YAML config with mock server URL
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

	// Create scenario with dynamic config
	testScenario := Scenario{
		Name:        "communicator-test",
		Description: "Test communicator with mock server",
		YAMLConfig:  scenarioConfig,
	}

	// Setup logger
	logger := cfg.Logger
	if logger == nil {
		devLogger, _ := zap.NewDevelopment()
		logger = devLogger.Sugar()
	}

	// Setup tick interval
	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = 100 * time.Millisecond
	}

	// Setup store
	store := SetupStore(logger)

	// Run via ApplicationSupervisor (non-blocking - returns immediately)
	runResult, err := Run(ctx, RunConfig{
		Scenario:     testScenario,
		Duration:     0, // Don't use Run's duration handling - we do it ourselves
		TickInterval: tickInterval,
		Logger:       logger,
		Store:        store,
	})
	if err != nil {
		// Cleanup
		if channelProvider != nil {
			communicator.ClearChannelProvider()
		}

		if ownsMockServer {
			mockServer.Close()
		}

		close(done)

		return &CommunicatorRunResult{
			Done:     done,
			Shutdown: func() {}, // No-op since already failed
			Error:    fmt.Errorf("failed to start scenario: %w", err),
		}
	}

	// Create result struct - fields will be populated when Done closes
	result := &CommunicatorRunResult{
		Done:     done,
		Shutdown: runResult.Shutdown, // Delegate to internal Shutdown
		Error:    nil,
	}

	// Handle completion in background goroutine
	go func() {
		// Handle duration-based shutdown
		if cfg.Duration > 0 {
			// Wait for duration then trigger shutdown
			select {
			case <-time.After(cfg.Duration):
				runResult.Shutdown()
			case <-ctx.Done():
				runResult.Shutdown()
			case <-runResult.Done:
				// Already completed
			}
		} else {
			// Duration 0 means run until context cancelled or Shutdown() called
			select {
			case <-ctx.Done():
				runResult.Shutdown()
			case <-runResult.Done:
				// Already completed (Shutdown was called externally)
			}
		}

		// Wait for scenario to fully complete
		<-runResult.Done

		// Capture received messages from channel provider (if used)
		if channelProvider != nil {
			result.ReceivedMessages = channelProvider.DrainInbound()
		}

		// Capture results from mock server
		result.PushedMessages = mockServer.GetPushedMessages()
		result.AuthCallCount = mockServer.AuthCallCount()

		// Cleanup
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
