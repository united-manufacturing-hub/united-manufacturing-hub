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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //nolint:ST1001
	. "github.com/onsi/gomega"    //nolint:ST1001
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// getLoggerForBridge returns a logger for e2e tests
func getLoggerForBridge() *zap.SugaredLogger {
	// Create a new logger for testing
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

// testBridgeLifecycle tests the full bridge lifecycle: create, edit, and delete
func testBridgeLifecycle(mockServer *MockAPIServer) {
	bridgeName := "e2e-test-bridge"
	bridgeIP := "localhost"
	bridgePort := uint32(8080)
	bridgeLocation := map[int]string{
		2: "test-line",
		3: "test-device",
	}

	By("Deploying the protocol converter connection")
	deployMessage := createDeployProtocolConverterMessage(bridgeName, bridgeIP, bridgePort, bridgeLocation)
	mockServer.AddMessageToPullQueue(deployMessage)

	// Wait for deployment confirmation
	var deployedUUID uuid.UUID
	Eventually(func() bool {
		pushedMessages := mockServer.DrainPushedMessages()
		for _, msg := range pushedMessages {
			if msg.Email != "e2e-test@example.com" {
				continue
			}

			content, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
			if err != nil {
				continue
			}

			if content.MessageType == models.ActionReply {
				if actionReply := parseActionReplyFromPayload(content.Payload); actionReply != nil {
					getLoggerForBridge().Infof("Received action reply: %s - %s", actionReply.ActionReplyState, actionReply.ActionReplyPayload)

					// Check if this is a successful deployment response
					if actionReply.ActionReplyState == models.ActionFinishedSuccessfull {
						if payloadMap, ok := actionReply.ActionReplyPayload.(map[string]interface{}); ok {
							if uuidStr, exists := payloadMap["uuid"]; exists {
								if uuidVal, ok := uuidStr.(string); ok {
									if parsedUUID, err := uuid.Parse(uuidVal); err == nil {
										deployedUUID = parsedUUID
										getLoggerForBridge().Infof("Bridge deployed successfully with UUID: %s", deployedUUID)
										return true
									}
								}
							}
						}
					}
				}
			}
		}
		return false
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Bridge deployment should succeed")

	By("Adding benthos generate configuration to the bridge")
	editMessage := createEditProtocolConverterMessage(deployedUUID, bridgeName, bridgeIP, bridgePort, bridgeLocation)
	mockServer.AddMessageToPullQueue(editMessage)

	// Wait for edit confirmation
	Eventually(func() bool {
		pushedMessages := mockServer.DrainPushedMessages()
		for _, msg := range pushedMessages {
			if msg.Email != "e2e-test@example.com" {
				continue
			}

			content, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
			if err != nil {
				continue
			}

			if content.MessageType == models.ActionReply {
				if actionReply := parseActionReplyFromPayload(content.Payload); actionReply != nil {
					getLoggerForBridge().Infof("Received edit action reply: %s - %s", actionReply.ActionReplyState, actionReply.ActionReplyPayload)

					// Check if this is a successful edit response
					if actionReply.ActionReplyState == models.ActionFinishedSuccessfull {
						getLoggerForBridge().Info("Bridge edit completed successfully")
						return true
					}
				}
			}
		}
		return false
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Bridge configuration should be applied successfully")

	By("Verifying bridge is active and generating data")
	// Give the bridge some time to start generating data
	time.Sleep(10 * time.Second)

	// Check that the bridge appears in status messages as active
	Eventually(func() bool {
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
					// Check if our bridge appears in the DFCs list
					for _, dfc := range statusMsg.Core.Dfcs {
						if dfc.Name != nil && *dfc.Name == bridgeName {
							getLoggerForBridge().Infof("Found bridge %s with state: %s", bridgeName, safeGetHealthState(dfc.Health))
							if isHealthActiveOrAcceptable(dfc.Health, "active", "running") {
								getLoggerForBridge().Info("Bridge is active and healthy!")
								return true
							}
						}
					}
				}
			}
		}
		return false
	}, 10*time.Second, 1*time.Second).Should(BeTrue(), "Bridge should appear as active in status messages")

	By("Deleting the bridge")
	deleteMessage := createDeleteProtocolConverterMessage(deployedUUID)
	mockServer.AddMessageToPullQueue(deleteMessage)

	// Wait for deletion confirmation
	Eventually(func() bool {
		pushedMessages := mockServer.DrainPushedMessages()
		for _, msg := range pushedMessages {
			if msg.Email != "e2e-test@example.com" {
				continue
			}

			content, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
			if err != nil {
				continue
			}

			if content.MessageType == models.ActionReply {
				if actionReply := parseActionReplyFromPayload(content.Payload); actionReply != nil {
					getLoggerForBridge().Infof("Received delete action reply: %s - %s", actionReply.ActionReplyState, actionReply.ActionReplyPayload)

					// Check if this is a successful delete response
					if actionReply.ActionReplyState == models.ActionFinishedSuccessfull {
						getLoggerForBridge().Info("Bridge deletion completed successfully")
						return true
					}
				}
			}
		}
		return false
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Bridge deletion should succeed")

	By("Verifying bridge is removed from status messages")
	// Give the system time to process the deletion
	time.Sleep(5 * time.Second)

	// Check that the bridge no longer appears in status messages
	Eventually(func() bool {
		pushedMessages := mockServer.DrainPushedMessages()
		foundBridge := false

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
					// Check if our bridge appears in the DFCs list
					for _, dfc := range statusMsg.Core.Dfcs {
						if dfc.Name != nil && *dfc.Name == bridgeName {
							foundBridge = true
							getLoggerForBridge().Warnf("Bridge %s still appears in status messages", bridgeName)
							break
						}
					}
				}
			}
		}

		// Success if bridge is NOT found
		return !foundBridge
	}, 20*time.Second, 2*time.Second).Should(BeTrue(), "Bridge should be removed from status messages")

	getLoggerForBridge().Infof("Successfully completed full lifecycle test for bridge: %s", bridgeName)
}

// parseActionReplyFromPayload converts payload to ActionReplyMessagePayload
func parseActionReplyFromPayload(payload interface{}) *models.ActionReplyMessagePayload {
	// First try direct type assertion
	if actionReply, ok := payload.(*models.ActionReplyMessagePayload); ok {
		return actionReply
	}

	if actionReply, ok := payload.(models.ActionReplyMessagePayload); ok {
		return &actionReply
	}

	// If payload is a map, try to unmarshal it
	if payloadMap, ok := payload.(map[string]interface{}); ok {
		// Look for action reply fields
		if state, hasState := payloadMap["actionReplyState"]; hasState {
			if actionUUID, hasUUID := payloadMap["actionUUID"]; hasUUID {
				if replyPayload, hasReplyPayload := payloadMap["actionReplyPayload"]; hasReplyPayload {
					actionReply := &models.ActionReplyMessagePayload{
						ActionReplyState:   models.ActionReplyState(state.(string)),
						ActionReplyPayload: replyPayload,
					}

					// Parse UUID if it's a string
					if uuidStr, ok := actionUUID.(string); ok {
						if parsedUUID, err := uuid.Parse(uuidStr); err == nil {
							actionReply.ActionUUID = parsedUUID
						}
					}

					return actionReply
				}
			}
		}
	}

	return nil
}
