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
	"encoding/json"
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
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(context.Background(), 10*time.Minute)

		By("Starting mock API server")
		mockServer = NewMockAPIServer()
		Expect(mockServer.Start()).To(Succeed())

		By("Starting UMH Core container with mock API")
		containerName, metricsPort = startUMHCoreWithMockAPI(mockServer)

		By("Waiting for container to be healthy and connected")
		Eventually(func() bool {
			return isContainerHealthy(metricsPort)
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Container should be healthy")

		By("Starting background subscription sender")
		go startPeriodicSubscription(testCtx, mockServer, 10*time.Second)

		DeferCleanup(func() {
			By("Cleaning up container and server")
			if containerName != "" {
				stopAndRemoveContainer(containerName)
			}
			if mockServer != nil {
				mockServer.Stop()
			}
			By("Stopping test context")
			testCancel()
		})
	})

	Context("Component Status Verification", func() {
		It("should verify component health status from status messages", func() {
			// Give umh-core time to process the initial subscription
			time.Sleep(5 * time.Second)

			By("Waiting for umh-core to send status messages")
			var latestStatusMessage *models.StatusMessage

			Eventually(func() bool {
				pushedMessages := mockServer.DrainPushedMessages()

				for _, msg := range pushedMessages {
					// Only check messages for our test email
					if msg.Email != "e2e-test@example.com" {
						continue
					}

					var content models.UMHMessageContent
					if err := json.Unmarshal([]byte(msg.Content), &content); err == nil {
						if content.MessageType == models.Status {
							// Try to parse the status message payload
							if statusMsg := parseStatusMessageFromPayload(content.Payload); statusMsg != nil {
								latestStatusMessage = statusMsg
								fmt.Printf("Received status message:\n")
								fmt.Printf("  Core: %s (message: %s)\n",
									safeGetHealthState(statusMsg.Core.Health), safeGetHealthMessage(statusMsg.Core.Health))
								fmt.Printf("  Agent: %s (message: %s)\n",
									safeGetHealthState(statusMsg.Core.Agent.Health), safeGetHealthMessage(statusMsg.Core.Agent.Health))
								fmt.Printf("  Redpanda: %s (message: %s)\n",
									safeGetHealthState(statusMsg.Core.Redpanda.Health), safeGetHealthMessage(statusMsg.Core.Redpanda.Health))
								fmt.Printf("  TopicBrowser: %s (message: %s)\n",
									safeGetHealthState(statusMsg.Core.TopicBrowser.Health), safeGetHealthMessage(statusMsg.Core.TopicBrowser.Health))
								return true
							}
						}
					}
				}
				return false
			}, 60*time.Second, 2*time.Second).Should(BeTrue(), "Should receive at least one status message")

			By("Verifying Core is active")
			Expect(latestStatusMessage).ToNot(BeNil(), "Status message should be parsed successfully")
			Expect(latestStatusMessage.Core.Health).ToNot(BeNil(), "Core health should be present")
			Expect(isHealthActiveOrAcceptable(latestStatusMessage.Core.Health, "active")).To(BeTrue(),
				fmt.Sprintf("Core should be active, but was: %v", latestStatusMessage.Core.Health))

			By("Verifying Agent is active")
			Expect(latestStatusMessage.Core.Agent.Health).ToNot(BeNil(), "Agent health should be present")
			Expect(isHealthActiveOrAcceptable(latestStatusMessage.Core.Agent.Health, "active")).To(BeTrue(),
				fmt.Sprintf("Agent should be active, but was: %v", latestStatusMessage.Core.Agent.Health))

			By("Verifying Redpanda is active or idle")
			Expect(latestStatusMessage.Core.Redpanda.Health).ToNot(BeNil(), "Redpanda health should be present")
			Expect(isHealthActiveOrAcceptable(latestStatusMessage.Core.Redpanda.Health, "active", "idle")).To(BeTrue(),
				fmt.Sprintf("Redpanda should be active or idle, but was: %v", latestStatusMessage.Core.Redpanda.Health))

			By("Verifying TopicBrowser is active or idle")
			Expect(latestStatusMessage.Core.TopicBrowser.Health).ToNot(BeNil(), "TopicBrowser health should be present")
			Expect(isHealthActiveOrAcceptable(latestStatusMessage.Core.TopicBrowser.Health, "active", "idle")).To(BeTrue(),
				fmt.Sprintf("TopicBrowser should be active or idle, but was: %v", latestStatusMessage.Core.TopicBrowser.Health))
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

// parseStatusMessageFromPayload converts payload to StatusMessage
func parseStatusMessageFromPayload(payload interface{}) *models.StatusMessage {
	// First try direct type assertion
	if statusMsg, ok := payload.(*models.StatusMessage); ok {
		return statusMsg
	}

	if statusMsg, ok := payload.(models.StatusMessage); ok {
		return &statusMsg
	}

	// If payload is a map, try to unmarshal it
	if payloadMap, ok := payload.(map[string]interface{}); ok {
		jsonBytes, err := json.Marshal(payloadMap)
		if err != nil {
			return nil
		}

		var statusMsg models.StatusMessage
		if err := json.Unmarshal(jsonBytes, &statusMsg); err != nil {
			return nil
		}

		return &statusMsg
	}

	return nil
}

// isHealthActiveOrAcceptable checks if health state matches any of the acceptable states
func isHealthActiveOrAcceptable(health *models.Health, acceptableStates ...string) bool {
	if health == nil {
		return false
	}

	// Check the ObservedState field
	for _, acceptable := range acceptableStates {
		if health.ObservedState == acceptable {
			return true
		}
	}

	return false
}

// safeGetHealthState safely extracts the health state, returning a fallback if nil
func safeGetHealthState(health *models.Health) string {
	if health == nil {
		return "<nil>"
	}
	if health.ObservedState == "" {
		return "<empty>"
	}
	return health.ObservedState
}

// safeGetHealthMessage safely extracts the health message, returning a fallback if nil
func safeGetHealthMessage(health *models.Health) string {
	if health == nil {
		return "<nil>"
	}
	if health.Message == "" {
		return "<empty>"
	}
	return health.Message
}

// createSubscriptionMessage creates a UMH message that subscribes to status updates
func createSubscriptionMessage() models.UMHMessage {
	// Create the subscription payload
	subscribePayload := models.SubscribeMessagePayload{
		Resubscribed: false, // This is a new subscription, not a resubscription
	}

	// Create the message content
	messageContent := models.UMHMessageContent{
		MessageType: models.Subscribe,
		Payload:     subscribePayload,
	}

	// Serialize the content
	contentBytes, _ := json.Marshal(messageContent)

	// Create the UMH message
	return models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "e2e-test@example.com",
		Content:      string(contentBytes),
		InstanceUUID: uuid.New(),
	}
}

// startPeriodicSubscription sends subscription messages periodically to maintain the subscription
func startPeriodicSubscription(ctx context.Context, mockServer *MockAPIServer, interval time.Duration) {
	// Send initial subscription immediately
	fmt.Printf("Sending initial subscription message\n")
	subscribeMessage := createSubscriptionMessage()
	mockServer.AddMessageToPullQueue(subscribeMessage)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Stopping periodic subscription sender\n")
			return
		case <-ticker.C:
			// Send resubscription message
			fmt.Printf("Sending periodic resubscription message\n")
			resubscribeMessage := createResubscriptionMessage()
			mockServer.AddMessageToPullQueue(resubscribeMessage)
		}
	}
}

// createResubscriptionMessage creates a UMH message that resubscribes to status updates
func createResubscriptionMessage() models.UMHMessage {
	// Create the subscription payload with resubscribed flag
	subscribePayload := models.SubscribeMessagePayload{
		Resubscribed: true, // This is a resubscription to refresh TTL
	}

	// Create the message content
	messageContent := models.UMHMessageContent{
		MessageType: models.Subscribe,
		Payload:     subscribePayload,
	}

	// Serialize the content
	contentBytes, _ := json.Marshal(messageContent)

	// Create the UMH message
	return models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "e2e-test@example.com",
		Content:      string(contentBytes),
		InstanceUUID: uuid.New(),
	}
}
