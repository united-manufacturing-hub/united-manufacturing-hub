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

package examples_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

func TestCommunicatorScenario(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Communicator Scenario Suite")
}

var _ = Describe("Communicator Scenario", func() {
	var ctx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	})

	AfterEach(func() {
		cancel()
		// CRITICAL: Clean up global channel provider to prevent test pollution
		communicator.ClearChannelProvider()
	})

	Describe("Using FSMv2 worker via ApplicationSupervisor", func() {
		It("authenticates and starts running", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: 2 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for scenario completion before checking result fields
			Expect(result.AuthCallCount).To(BeNumerically(">=", 1))
		})

		It("uses default auth token when not provided", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: 1 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for scenario completion before checking result fields
			Expect(result.AuthCallCount).To(BeNumerically(">=", 1))
		})

		It("handles custom auth token", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration:  1 * time.Second,
				AuthToken: "custom-token",
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for scenario completion before checking result fields
			Expect(result.AuthCallCount).To(BeNumerically(">=", 1))
		})

		It("handles custom logger", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: 1 * time.Second,
				Logger:   zap.NewNop().Sugar(),
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for completion
		})

		It("handles custom tick interval", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration:     1 * time.Second,
				TickInterval: 50 * time.Millisecond,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for completion
		})

		It("pushes outbound messages via channel provider", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: 2 * time.Second,
				InitialOutboundMessages: []*transport.UMHMessage{{
					InstanceUUID: "test-instance",
					Content:      "status-update",
				}},
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for scenario completion before checking result fields
			// The worker reads from outbound channel and pushes to HTTP
			// Mock server receives the pushed messages
			Expect(result.PushedMessages).To(HaveLen(1))
		})

		It("handles both pull and push in same run", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: 2 * time.Second,
				InitialPullMessages: []*transport.UMHMessage{{
					InstanceUUID: "test-instance",
					Content:      "inbound-message",
				}},
				InitialOutboundMessages: []*transport.UMHMessage{{
					InstanceUUID: "test-instance",
					Content:      "outbound-message",
				}},
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for scenario completion before checking result fields
			// Outbound messages should be pushed
			Expect(result.PushedMessages).To(HaveLen(1))
			// Note: Received messages require channel provider which is set up
			// when InitialOutboundMessages is provided
		})
	})

	Describe("Error conditions", func() {
		It("returns error for negative duration", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: -1 * time.Second,
			})
			Expect(result.Error).To(HaveOccurred())
			Expect(result.Error.Error()).To(ContainSubstring("invalid duration"))
		})

		It("returns error when context already cancelled", func() {
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			result := examples.RunCommunicatorScenario(cancelledCtx, examples.CommunicatorRunConfig{
				Duration: 1 * time.Second,
			})
			Expect(result.Error).To(HaveOccurred())
			Expect(result.Error.Error()).To(ContainSubstring("context already cancelled"))
		})

		It("handles injected mock server", func() {
			mockServer := testutil.NewMockRelayServer()
			defer mockServer.Close()

			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration:   1 * time.Second,
				MockServer: mockServer,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for scenario completion before checking result fields
			// Should run successfully with injected mock server
			Expect(result.AuthCallCount).To(BeNumerically(">=", 1))
		})
	})

	Describe("Edge cases", func() {
		It("handles very short duration", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: 100 * time.Millisecond,
			})
			// Should complete without panic
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for completion
		})

		It("handles zero duration (runs until context cancelled)", func() {
			shortCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()

			result := examples.RunCommunicatorScenario(shortCtx, examples.CommunicatorRunConfig{
				Duration: 0, // Run forever
			})
			// Should complete when context times out
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for completion
		})
	})

	Describe("Result structure", func() {
		It("returns all expected fields", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: 1 * time.Second,
			})

			Expect(result.Done).NotTo(BeNil())
			Expect(result.Shutdown).NotTo(BeNil()) // Shutdown function should be set
			<-result.Done                          // Wait for completion before checking result fields
			Expect(result.PushedMessages).NotTo(BeNil())
			Expect(result.ConsecutiveErrors).To(BeNumerically(">=", 0))
			Expect(result.AuthCallCount).To(BeNumerically(">=", 0))
		})

		It("closes Done channel when complete", func() {
			result := examples.RunCommunicatorScenario(ctx, examples.CommunicatorRunConfig{
				Duration: 500 * time.Millisecond,
			})
			// Allow extra time for graceful shutdown (duration + shutdown overhead)
			Eventually(result.Done, 5*time.Second).Should(BeClosed())
		})
	})

	// Table-driven tests for configuration variations
	DescribeTable("handles various configurations correctly",
		func(cfg examples.CommunicatorRunConfig, expectSuccess bool) {
			result := examples.RunCommunicatorScenario(ctx, cfg)

			if expectSuccess {
				Expect(result.Error).NotTo(HaveOccurred())
				<-result.Done // Wait for scenario completion before checking result fields
				Expect(result.AuthCallCount).To(BeNumerically(">=", 1))
			}
		},
		Entry("minimal config",
			examples.CommunicatorRunConfig{Duration: 1 * time.Second},
			true),
		Entry("with custom auth token",
			examples.CommunicatorRunConfig{Duration: 1 * time.Second, AuthToken: "custom"},
			true),
		Entry("with logger",
			examples.CommunicatorRunConfig{Duration: 1 * time.Second, Logger: zap.NewNop().Sugar()},
			true),
		Entry("with custom tick interval",
			examples.CommunicatorRunConfig{Duration: 1 * time.Second, TickInterval: 50 * time.Millisecond},
			true),
	)
})

var _ = Describe("CommunicatorScenarioEntry registry", func() {
	It("is registered with CustomRunner that uses ApplicationSupervisor internally", func() {
		scenario, exists := examples.Registry["communicator"]
		Expect(exists).To(BeTrue())
		Expect(scenario.Name).To(Equal("communicator"))
		Expect(scenario.Description).NotTo(BeEmpty())
		// Uses CustomRunner for mock server orchestration (but still uses ApplicationSupervisor internally)
		Expect(scenario.CustomRunner).NotTo(BeNil())
		Expect(scenario.YAMLConfig).To(BeEmpty()) // Config built dynamically with mock server URL
	})

	It("description explains ApplicationSupervisor usage", func() {
		scenario := examples.Registry["communicator"]
		// Description should mention that it uses ApplicationSupervisor (not bypass)
		Expect(scenario.Description).To(ContainSubstring("ApplicationSupervisor"))
	})
})

var _ = Describe("TestChannelProvider", func() {
	It("provides channels for test scenarios", func() {
		provider := examples.NewTestChannelProvider(10)
		inbound, outbound := provider.GetChannels("test-worker")

		Expect(inbound).NotTo(BeNil())
		Expect(outbound).NotTo(BeNil())
	})

	It("queues and drains messages correctly", func() {
		provider := examples.NewTestChannelProvider(10)

		// Queue a message via outbound
		testMsg := &transport.UMHMessage{Content: "test-message"}
		provider.QueueOutbound(testMsg)

		// Get channels and read from outbound
		_, outbound := provider.GetChannels("test-worker")
		receivedMsg := <-outbound
		Expect(receivedMsg.Content).To(Equal("test-message"))
	})

	It("drains inbound messages", func() {
		provider := examples.NewTestChannelProvider(10)
		inbound, _ := provider.GetChannels("test-worker")

		// Send messages to inbound
		inbound <- &transport.UMHMessage{Content: "msg1"}
		inbound <- &transport.UMHMessage{Content: "msg2"}

		// Drain
		messages := provider.DrainInbound()
		Expect(messages).To(HaveLen(2))
		Expect(messages[0].Content).To(Equal("msg1"))
		Expect(messages[1].Content).To(Equal("msg2"))
	})

	It("returns empty slice when no messages available", func() {
		provider := examples.NewTestChannelProvider(10)
		messages := provider.DrainInbound()
		Expect(messages).To(BeEmpty())
	})
})
