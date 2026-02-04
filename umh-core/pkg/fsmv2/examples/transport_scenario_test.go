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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	transportWorker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

var _ = Describe("Transport Scenario", func() {
	var ctx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	})

	AfterEach(func() {
		cancel()
		// CRITICAL: Clean up global channel provider to prevent test pollution
		transportWorker.ClearChannelProvider()
	})

	Describe("Using FSMv2 TransportWorker via ApplicationSupervisor", func() {
		// ACTIVE TEST: This test should pass - authenticates and enters Starting state
		It("authenticates and enters Starting state", func() {
			result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
				Duration: 2 * time.Second,
			})
			Expect(result.Error).NotTo(HaveOccurred())
			<-result.Done // Wait for scenario completion before checking result fields
			Expect(result.AuthCallCount).To(BeNumerically(">=", 1))
		})

		// ============================================================================
		// FUTURE TESTS: Commented out for future tickets
		// These tests define the complete scenario structure upfront.
		// Uncomment and implement when the corresponding tickets are worked on.
		// ============================================================================

		// ENG-4262: PushWorker implementation
		// It("creates PushWorker child", func() {
		// 	result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
		// 		Duration: 2 * time.Second,
		// 	})
		// 	Expect(result.Error).NotTo(HaveOccurred())
		// 	<-result.Done
		// 	// Verify PushWorker child was created
		// 	// TODO: Add assertion for PushWorker child existence
		// })

		// ENG-4263: PullWorker implementation
		// It("creates PullWorker child", func() {
		// 	result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
		// 		Duration: 2 * time.Second,
		// 	})
		// 	Expect(result.Error).NotTo(HaveOccurred())
		// 	<-result.Done
		// 	// Verify PullWorker child was created
		// 	// TODO: Add assertion for PullWorker child existence
		// })

		// ENG-4262: PushWorker pushes messages
		// It("pushes messages every 100ms", func() {
		// 	result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
		// 		Duration: 2 * time.Second,
		// 		InitialOutboundMessages: []*transport.UMHMessage{{
		// 			InstanceUUID: "test-instance",
		// 			Content:      "status-update",
		// 		}},
		// 	})
		// 	Expect(result.Error).NotTo(HaveOccurred())
		// 	<-result.Done
		// 	// Verify messages were pushed at expected interval
		// 	Expect(result.PushedMessages).To(HaveLen(1))
		// })

		// ENG-4263: PullWorker pulls messages continuously
		// It("pulls messages continuously", func() {
		// 	result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
		// 		Duration: 2 * time.Second,
		// 		InitialPullMessages: []*transport.UMHMessage{{
		// 			InstanceUUID: "test-instance",
		// 			Content:      "inbound-message",
		// 		}},
		// 	})
		// 	Expect(result.Error).NotTo(HaveOccurred())
		// 	<-result.Done
		// 	// Verify messages were pulled and delivered to inbound channel
		// 	Expect(result.ReceivedMessages).To(HaveLen(1))
		// })

		// ENG-4263: PullWorker handles backpressure
		// It("handles backpressure", func() {
		// 	// Fill the inbound channel to create backpressure
		// 	result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
		// 		Duration: 2 * time.Second,
		// 	})
		// 	Expect(result.Error).NotTo(HaveOccurred())
		// 	<-result.Done
		// 	// Verify backpressure was handled gracefully (no panics, no data loss)
		// 	// TODO: Add specific backpressure assertions
		// })

		// ENG-4264: Instance stays online when Pull times out
		// It("instance stays online when Pull times out", func() {
		// 	result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
		// 		Duration: 5 * time.Second,
		// 		// No pull messages - will timeout
		// 	})
		// 	Expect(result.Error).NotTo(HaveOccurred())
		// 	<-result.Done
		// 	// Verify instance remained online despite pull timeouts
		// 	// Auth should have been called, worker should still be running
		// 	Expect(result.AuthCallCount).To(BeNumerically(">=", 1))
		// })
	})

	Describe("Error conditions", func() {
		It("returns error for negative duration", func() {
			result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
				Duration: -1 * time.Second,
			})
			Expect(result.Error).To(HaveOccurred())
			Expect(result.Error.Error()).To(ContainSubstring("invalid duration"))
		})

		It("returns error when context already cancelled", func() {
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			result := examples.RunTransportScenario(cancelledCtx, examples.TransportRunConfig{
				Duration: 1 * time.Second,
			})
			Expect(result.Error).To(HaveOccurred())
			Expect(result.Error.Error()).To(ContainSubstring("context already cancelled"))
		})
	})

	Describe("Result structure", func() {
		It("returns all expected fields", func() {
			result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
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
			result := examples.RunTransportScenario(ctx, examples.TransportRunConfig{
				Duration: 500 * time.Millisecond,
			})
			// Allow extra time for cascading graceful shutdown:
			// - Child supervisor graceful timeout: 5s
			// - Parent supervisor graceful timeout: 5s
			// - Processing overhead
			// Total: 15s should be sufficient
			Eventually(result.Done, 15*time.Second).Should(BeClosed())
		})
	})
})

var _ = Describe("TransportTestChannelProvider", func() {
	It("provides channels for test scenarios", func() {
		provider := examples.NewTransportTestChannelProvider(10)
		inbound, outbound := provider.GetChannels("test-worker")

		Expect(inbound).NotTo(BeNil())
		Expect(outbound).NotTo(BeNil())
	})

	It("queues and drains messages correctly", func() {
		provider := examples.NewTransportTestChannelProvider(10)

		// Queue a message via outbound
		testMsg := &transport.UMHMessage{Content: "test-message"}
		provider.QueueOutbound(testMsg)

		// Get channels and read from outbound
		_, outbound := provider.GetChannels("test-worker")
		receivedMsg := <-outbound
		Expect(receivedMsg.Content).To(Equal("test-message"))
	})

	It("drains inbound messages", func() {
		provider := examples.NewTransportTestChannelProvider(10)
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
		provider := examples.NewTransportTestChannelProvider(10)
		messages := provider.DrainInbound()
		Expect(messages).To(BeEmpty())
	})
})
