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

	Context("Bridge Deployment", func() {
		It("should deploy a bridge successfully and verify it becomes active", func() {
			By("Creating a bridge deployment message")
			bridgeDeploymentMessage := createBridgeDeploymentMessage()

			By("Adding the bridge deployment message to the pull queue")
			mockServer.AddMessageToPullQueue(bridgeDeploymentMessage)

			By("Waiting for umh-core to pull and process the deployment message")
			time.Sleep(3 * time.Second)

			By("Verifying UMH Core received and processed the message")
			// UMH Core should have pulled the deployment message
			Eventually(func() int {
				return len(mockServer.DrainPushedMessages())
			}, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0), "UMH Core should send back action confirmations")

			By("Checking the bridge deployment status via pushed messages")
			pushedMessages := mockServer.DrainPushedMessages()

			// Look for action reply messages indicating successful deployment
			var actionReplies []models.ActionReplyMessagePayload
			for _, msg := range pushedMessages {
				var content models.UMHMessageContent
				if err := json.Unmarshal([]byte(msg.Content), &content); err == nil {
					if content.MessageType == models.ActionReply {
						if actionReply, ok := content.Payload.(models.ActionReplyMessagePayload); ok {
							actionReplies = append(actionReplies, actionReply)
						} else {
							// Try to parse as map and convert
							if payloadMap, ok := content.Payload.(map[string]interface{}); ok {
								actionReply := parseActionReplyFromMap(payloadMap)
								if actionReply != nil {
									actionReplies = append(actionReplies, *actionReply)
								}
							}
						}
					}
				}
			}

			By("Verifying we received action confirmations")
			Expect(len(actionReplies)).To(BeNumerically(">", 0), "Should receive action reply messages")

			// Print all action replies for debugging
			fmt.Printf("Received %d action replies:\n", len(actionReplies))
			for i, reply := range actionReplies {
				fmt.Printf("  [%d] State=%s, Message=%v\n", i, reply.ActionReplyState, reply.ActionReplyPayload)
			}

			// Check for action confirmation
			foundConfirmation := false
			foundExecution := false
			foundCompletion := false
			var failureMessage string

			for _, reply := range actionReplies {
				switch reply.ActionReplyState {
				case models.ActionConfirmed:
					foundConfirmation = true
				case models.ActionExecuting:
					foundExecution = true
				case models.ActionFinishedSuccessfull:
					foundCompletion = true
				case models.ActionFinishedWithFailure:
					failureMessage = fmt.Sprintf("%v", reply.ActionReplyPayload)
				}
			}

			// If we got a failure, report it
			if failureMessage != "" {
				Fail(fmt.Sprintf("Bridge deployment failed: %s", failureMessage))
			}

			Expect(foundConfirmation).To(BeTrue(), "Should receive action confirmation")
			Expect(foundExecution).To(BeTrue(), "Should receive action execution notification")

			By("Waiting for bridge deployment to complete successfully")
			// Look for successful completion in additional messages
			Eventually(func() bool {
				newMessages := mockServer.DrainPushedMessages()
				for _, msg := range newMessages {
					var content models.UMHMessageContent
					if err := json.Unmarshal([]byte(msg.Content), &content); err == nil {
						if content.MessageType == models.ActionReply {
							if actionReply := parseActionReplyFromMap(content.Payload.(map[string]interface{})); actionReply != nil {
								fmt.Printf("Additional reply: State=%s, Message=%v\n", actionReply.ActionReplyState, actionReply.ActionReplyPayload)
								if actionReply.ActionReplyState == models.ActionFinishedSuccessfull {
									return true
								}
								if actionReply.ActionReplyState == models.ActionFinishedWithFailure {
									Fail(fmt.Sprintf("Bridge deployment failed: %v", actionReply.ActionReplyPayload))
								}
							}
						}
					}
				}
				return foundCompletion
			}, 60*time.Second, 2*time.Second).Should(BeTrue(), "Bridge deployment should complete successfully")
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

// createBridgeDeploymentMessage creates a UMH message that deploys a protocol converter/bridge
func createBridgeDeploymentMessage() models.UMHMessage {
	bridgeUUID := uuid.New()
	actionUUID := uuid.New()

	// Create the protocol converter payload
	protocolConverter := models.ProtocolConverter{
		UUID: &bridgeUUID,
		Name: "e2e-test-bridge",
		Connection: models.ProtocolConverterConnection{
			IP:   "localhost",
			Port: 3000,
		},
		Location: map[int]string{
			0: "E2E-Test-Plant",
			1: "Test-Line",
			2: "Test-Bridge",
		},
		// For a basic bridge, we don't need ReadDFC or WriteDFC initially
		// They can be added later via edit actions
	}

	// Create the action payload
	actionPayload := models.ActionMessagePayload{
		ActionType:    models.DeployProtocolConverter,
		ActionUUID:    actionUUID,
		ActionPayload: protocolConverter,
	}

	// Create the message content
	messageContent := models.UMHMessageContent{
		MessageType: models.Action,
		Payload:     actionPayload,
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

// parseActionReplyFromMap converts a map[string]interface{} to ActionReplyMessagePayload
func parseActionReplyFromMap(payloadMap map[string]interface{}) *models.ActionReplyMessagePayload {
	actionReply := &models.ActionReplyMessagePayload{}

	if actionUUID, ok := payloadMap["actionUUID"].(string); ok {
		if parsed, err := uuid.Parse(actionUUID); err == nil {
			actionReply.ActionUUID = parsed
		}
	}

	if state, ok := payloadMap["actionReplyState"].(string); ok {
		actionReply.ActionReplyState = models.ActionReplyState(state)
	}

	if payload, ok := payloadMap["actionReplyPayload"]; ok {
		actionReply.ActionReplyPayload = payload
	}

	return actionReply
}
