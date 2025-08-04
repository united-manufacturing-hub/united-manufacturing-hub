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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func init() {
	// Use corev1 encoder for best performance and compatibility
	encoding.ChooseEncoder(encoding.EncodingCorev1)
}

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
			Eventually(func() bool {
				pushedMessages := mockServer.DrainPushedMessages()

				for _, msg := range pushedMessages {
					// Only check messages for our test email
					if msg.Email != "e2e-test@example.com" {
						continue
					}

					// Decode the base64 encoded message content
					content, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
					if err != nil {
						e2eLogger.Errorf("Failed to decode message content: %v", err)
						continue
					}

					if content.MessageType == models.Status {
						// Try to parse the status message payload
						if statusMsg := parseStatusMessageFromPayload(content.Payload); statusMsg != nil {
							e2eLogger.Info("Received status message:")
							e2eLogger.Infof("  Core: %s (message: %s)",
								safeGetHealthState(statusMsg.Core.Health), safeGetHealthMessage(statusMsg.Core.Health))
							e2eLogger.Infof("  Agent: %s (message: %s)",
								safeGetHealthState(statusMsg.Core.Agent.Health), safeGetHealthMessage(statusMsg.Core.Agent.Health))
							e2eLogger.Infof("  Redpanda: %s (message: %s)",
								safeGetHealthState(statusMsg.Core.Redpanda.Health), safeGetHealthMessage(statusMsg.Core.Redpanda.Health))
							e2eLogger.Infof("  TopicBrowser: %s (message: %s)",
								safeGetHealthState(statusMsg.Core.TopicBrowser.Health), safeGetHealthMessage(statusMsg.Core.TopicBrowser.Health))
							return true
						}
					}
				}
				return false
			}, 60*time.Second, 2*time.Second).Should(BeTrue(), "Should receive at least one status message")

			By("Verifying all components are in healthy states")
			Eventually(func() bool {
				var latestStatusMessage *models.StatusMessage
				pushedMessages := mockServer.DrainPushedMessages()

				for _, msg := range pushedMessages {
					if msg.Email != "e2e-test@example.com" {
						continue
					}

					content, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
					if err != nil {
						continue
					}

					if content.MessageType == models.Status {
						if statusMsg := parseStatusMessageFromPayload(content.Payload); statusMsg != nil {
							latestStatusMessage = statusMsg
						}
					}
				}

				if latestStatusMessage == nil {
					return false
				}

				// Check Core health
				if latestStatusMessage.Core.Health == nil || !isHealthActiveOrAcceptable(latestStatusMessage.Core.Health, "active", "running") {
					e2eLogger.Infof("Core not ready: %s", safeGetHealthState(latestStatusMessage.Core.Health))
					return false
				}

				// Check Agent health
				if latestStatusMessage.Core.Agent.Health == nil || !isHealthActiveOrAcceptable(latestStatusMessage.Core.Agent.Health, "active", "running") {
					e2eLogger.Infof("Agent not ready: %s", safeGetHealthState(latestStatusMessage.Core.Agent.Health))
					return false
				}

				// Check Redpanda health (active or idle)
				if latestStatusMessage.Core.Redpanda.Health == nil || !isHealthActiveOrAcceptable(latestStatusMessage.Core.Redpanda.Health, "active", "idle", "running") {
					e2eLogger.Infof("Redpanda not ready: %s", safeGetHealthState(latestStatusMessage.Core.Redpanda.Health))
					return false
				}

				// Check TopicBrowser health (active or idle)
				if latestStatusMessage.Core.TopicBrowser.Health == nil || !isHealthActiveOrAcceptable(latestStatusMessage.Core.TopicBrowser.Health, "active", "idle", "running") {
					e2eLogger.Infof("TopicBrowser not ready: %s", safeGetHealthState(latestStatusMessage.Core.TopicBrowser.Health))
					return false
				}

				e2eLogger.Info("All components are healthy!")
				return true
			}, 10*time.Second, 1*time.Second).Should(BeTrue(), "All components should eventually be in healthy states")
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

	// Encode the content using the encoding package
	encodedContent, err := encoding.EncodeMessageFromUserToUMHInstance(messageContent)
	if err != nil {
		panic(fmt.Sprintf("Failed to encode subscription message: %v", err))
	}

	// Create the UMH message
	return models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "e2e-test@example.com",
		Content:      encodedContent,
		InstanceUUID: uuid.New(),
	}
}

// startPeriodicSubscription sends subscription messages periodically to maintain the subscription
func startPeriodicSubscription(ctx context.Context, mockServer *MockAPIServer, interval time.Duration) {
	// Send initial subscription immediately
	e2eLogger.Info("Sending initial subscription message")
	subscribeMessage := createSubscriptionMessage()
	mockServer.AddMessageToPullQueue(subscribeMessage)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e2eLogger.Info("Stopping periodic subscription sender")
			return
		case <-ticker.C:
			// Send resubscription message
			e2eLogger.Info("Sending periodic resubscription message")
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

	// Encode the content using the encoding package
	encodedContent, err := encoding.EncodeMessageFromUserToUMHInstance(messageContent)
	if err != nil {
		panic(fmt.Sprintf("Failed to encode resubscription message: %v", err))
	}

	// Create the UMH message
	return models.UMHMessage{
		Metadata: &models.MessageMetadata{
			TraceID: uuid.New(),
		},
		Email:        "e2e-test@example.com",
		Content:      encodedContent,
		InstanceUUID: uuid.New(),
	}
}
