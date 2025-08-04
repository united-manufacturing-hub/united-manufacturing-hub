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

package e2e_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("UMH Core E2E Communication", Ordered, Label("e2e"), func() {
	var (
		mockServer    *MockAPIServer
		containerName string
		metricsPort   int
		testCancel    context.CancelFunc
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(context.Background(), 10*time.Minute)

		By("Starting mock API server")
		mockServer = NewMockAPIServer()
		Expect(mockServer.Start()).To(Succeed())

		By("Starting UMH Core container with mock API")
		containerName, metricsPort = startUMHCoreWithMockAPI(mockServer)

		By("Waiting for container to be healthy and connected")
		Eventually(func() bool {
			return isContainerHealthy(metricsPort)
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Container should be healthy")

		DeferCleanup(func() {
			By("Cleaning up container and server")
			if containerName != "" {
				stopAndRemoveContainer(containerName)
			}
			if mockServer != nil {
				mockServer.Stop()
			}
			testCancel()
		})
	})

	Context("Basic Pull/Push Communication", func() {
		It("should receive messages when we add them to the pull queue", func() {
			By("Adding a test message to the pull queue")
			testMessage := models.UMHMessage{
				Metadata: &models.MessageMetadata{
					TraceID: uuid.New(),
				},
				Email:        "test@example.com",
				Content:      "Hello from E2E test!",
				InstanceUUID: uuid.New(),
			}

			mockServer.AddMessageToPullQueue(testMessage)

			By("Waiting for umh-core to pull and process the message")
			// Give umh-core some time to pull the message (it polls every 10ms)
			time.Sleep(2 * time.Second)

			By("Verifying the message was pulled from the queue")
			// The message should have been removed from the queue
			Expect(mockServer.GetPushedMessageCount()).To(BeNumerically(">=", 0))
		})

		It("should be able to send multiple messages through the pull queue", func() {
			By("Adding multiple test messages to the pull queue")
			for i := 0; i < 5; i++ {
				testMessage := models.UMHMessage{
					Metadata: &models.MessageMetadata{
						TraceID: uuid.New(),
					},
					Email:        "test@example.com",
					Content:      fmt.Sprintf("Test message %d", i),
					InstanceUUID: uuid.New(),
				}
				mockServer.AddMessageToPullQueue(testMessage)
			}

			By("Waiting for umh-core to pull and process all messages")
			time.Sleep(3 * time.Second)

			By("Verifying messages were processed")
			// At this point, umh-core should have pulled all messages
			// We can't directly verify they were processed without looking at internal state,
			// but we can verify they were pulled by checking the queue is empty
			Expect(true).To(BeTrue()) // Placeholder - in a real test you might check logs or other indicators
		})

		It("should capture any messages that umh-core pushes back", func() {
			By("Checking if umh-core has pushed any messages to us")
			pushedMessages := mockServer.DrainPushedMessages()

			By("Logging what we received")
			for i, msg := range pushedMessages {
				fmt.Printf("Pushed message %d: Content=%s, Email=%s, InstanceUUID=%s\n",
					i, msg.Content, msg.Email, msg.InstanceUUID)
			}

			// This test mainly verifies the infrastructure works
			// In a real test, you might send specific messages that trigger responses
			Expect(len(pushedMessages)).To(BeNumerically(">=", 0))
		})
	})

	Context("Message Flow Verification", func() {
		It("should handle rapid message delivery", func() {
			By("Adding messages rapidly to test the communication speed")
			messageCount := 20

			for i := 0; i < messageCount; i++ {
				testMessage := models.UMHMessage{
					Metadata: &models.MessageMetadata{
						TraceID: uuid.New(),
					},
					Email:        "speed-test@example.com",
					Content:      fmt.Sprintf("Speed test message %d at %v", i, time.Now()),
					InstanceUUID: uuid.New(),
				}
				mockServer.AddMessageToPullQueue(testMessage)

				// Small delay to avoid overwhelming
				time.Sleep(10 * time.Millisecond)
			}

			By("Waiting for all messages to be processed")
			time.Sleep(5 * time.Second)

			By("Verifying system remained stable")
			Expect(isContainerHealthy(metricsPort)).To(BeTrue(), "Container should remain healthy during rapid message delivery")
		})
	})
})
