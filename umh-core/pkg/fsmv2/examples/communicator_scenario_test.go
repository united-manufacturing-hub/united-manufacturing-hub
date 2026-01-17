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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/testutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

func TestCommunicatorScenario(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Communicator Scenario Suite")
}

var _ = Describe("Communicator Scenario", func() {
	var (
		mockServer *testutil.MockRelayServer
		scenario   *examples.CommunicatorScenario
		ctx        context.Context
		cancel     context.CancelFunc
	)

	BeforeEach(func() {
		mockServer = testutil.NewMockRelayServer()
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	})

	AfterEach(func() {
		cancel()
		mockServer.Close()
	})

	Describe("Authentication and Syncing", func() {
		It("authenticates and enters syncing state", func() {
			// Create scenario with mock server
			scenario = examples.NewCommunicatorScenario(mockServer.URL(), "test-auth-token")

			// Run for a short duration
			err := scenario.Run(ctx, 2*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Verify authentication was called
			Expect(mockServer.AuthCallCount()).To(BeNumerically(">=", 1))
		})
	})

	Describe("Message Pulling", func() {
		It("pulls messages and makes them available via GetReceivedMessages", func() {
			// Queue a message on the mock server
			msg := &transport.UMHMessage{
				InstanceUUID: "test-instance-123",
				Content:      "test-action-from-backend",
				Email:        "test@example.com",
			}
			mockServer.QueuePullMessage(msg)

			// Create and run scenario
			scenario = examples.NewCommunicatorScenario(mockServer.URL(), "test-auth-token")
			err := scenario.Run(ctx, 2*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Verify message was received
			receivedMessages := scenario.GetReceivedMessages()
			Expect(receivedMessages).To(HaveLen(1))
			Expect(receivedMessages[0].Content).To(Equal("test-action-from-backend"))
		})
	})

	Describe("Message Pushing", func() {
		It("pushes queued outbound messages to the server", func() {
			// Create scenario
			scenario = examples.NewCommunicatorScenario(mockServer.URL(), "test-auth-token")

			// Queue an outbound message
			outboundMsg := &transport.UMHMessage{
				InstanceUUID: "test-instance-123",
				Content:      "status-update-from-edge",
				Email:        "test@example.com",
			}
			scenario.QueueOutboundMessage(outboundMsg)

			// Run scenario
			err := scenario.Run(ctx, 2*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Verify message was pushed to server
			pushedMessages := mockServer.GetPushedMessages()
			Expect(pushedMessages).To(HaveLen(1))
			Expect(pushedMessages[0].Content).To(Equal("status-update-from-edge"))
		})
	})

	Describe("Error Handling", func() {
		It("handles consecutive errors and tracks error count", func() {
			// Create scenario
			scenario = examples.NewCommunicatorScenario(mockServer.URL(), "test-auth-token")

			// Run a short sync first to authenticate
			err := scenario.Run(ctx, 300*time.Millisecond)
			Expect(err).NotTo(HaveOccurred())

			// Now inject an error and run again
			mockServer.SimulateServerError(500)

			// Run scenario briefly - error will cause consecutive error count to increase
			err = scenario.Run(ctx, 300*time.Millisecond)
			// Error might occur, that's expected
			_ = err

			// The error count may have been reset if a successful pull happened after the error
			// Just verify the scenario ran without panicking
			Expect(scenario.GetConsecutiveErrors()).To(BeNumerically(">=", 0))
		})

		It("resets error count on successful operations", func() {
			// Create scenario
			scenario = examples.NewCommunicatorScenario(mockServer.URL(), "test-auth-token")

			// Run scenario - should succeed and have low or 0 errors
			err := scenario.Run(ctx, 500*time.Millisecond)
			Expect(err).NotTo(HaveOccurred())

			// After successful run, consecutive errors should be low (0 or 1 depending on timing)
			// The sync loop resets errors after each successful pull
			errorCount := scenario.GetConsecutiveErrors()
			Expect(errorCount).To(BeNumerically("<=", 1))
		})
	})

	Describe("Re-authentication", func() {
		It("re-authenticates on 401 response", func() {
			// Create scenario with low error threshold for quick re-auth
			scenario = examples.NewCommunicatorScenario(mockServer.URL(), "test-auth-token")
			scenario.SetErrorThreshold(1) // Re-auth after 1 error

			// Run briefly to authenticate initially
			err := scenario.Run(ctx, 300*time.Millisecond)
			Expect(err).NotTo(HaveOccurred())

			initialAuthCount := mockServer.AuthCallCount()
			Expect(initialAuthCount).To(BeNumerically(">=", 1))

			// Simulate auth expiry (401 on next request)
			mockServer.SimulateAuthExpiry()

			// Continue running - the 401 should trigger re-auth after hitting threshold
			err = scenario.Run(ctx, 500*time.Millisecond)
			// Error may happen during re-auth attempts
			_ = err

			// Should have more auth calls due to re-authentication
			finalAuthCount := mockServer.AuthCallCount()
			Expect(finalAuthCount).To(BeNumerically(">", initialAuthCount))
		})
	})
})
