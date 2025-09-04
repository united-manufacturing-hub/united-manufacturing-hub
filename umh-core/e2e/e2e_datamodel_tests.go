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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:ST1001
	. "github.com/onsi/gomega"    //nolint:ST1001
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

var dataModelLogger *zap.SugaredLogger

func init() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize data model logger: %v", err))
	}
	dataModelLogger = logger.Sugar()
}

// getLoggerForDataModel returns a logger for e2e data model tests
func getLoggerForDataModel() *zap.SugaredLogger {
	return dataModelLogger
}

// testDataModelLifecycle tests the full data model lifecycle (create, edit, verify, delete)
func testDataModelLifecycle(mockServer *MockAPIServer) {
	dataModelName := "e2e-test-sensor"
	dataModelDescription := "E2E test sensor data model"

	By("Creating a new data model")
	addMessage, err := createAddDataModelMessage(dataModelName, dataModelDescription)
	if err != nil {
		getLoggerForDataModel().Fatalf("Failed to create add data model message: %v", err)
	}
	mockServer.AddMessageToPullQueue(addMessage)

	// Wait for creation confirmation
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
					getLoggerForDataModel().Infof("Received action reply: %s - %s", actionReply.ActionReplyState, actionReply.ActionReplyPayload)

					// Check if this is a successful creation response
					if actionReply.ActionReplyState == models.ActionFinishedSuccessfull {
						if payloadMap, ok := actionReply.ActionReplyPayload.(map[string]interface{}); ok {
							if name, exists := payloadMap["name"]; exists {
								if nameStr, ok := name.(string); ok && nameStr == dataModelName {
									getLoggerForDataModel().Infof("Data model %s created successfully", dataModelName)
									return true
								}
							}
						}
					}
				}
			}
		}
		return false
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Data model creation should succeed")

	By("Verifying data model appears in status messages")
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
					// Check if our data model appears in the data models list
					for _, dataModel := range statusMsg.Core.DataModels {
						if dataModel.Name == dataModelName {
							getLoggerForDataModel().Infof("Found data model %s in status with version: %s", dataModelName, dataModel.LatestVersion)
							return true
						}
					}
				}
			}
		}
		return false
	}, 15*time.Second, 1*time.Second).Should(BeTrue(), "Data model should appear in status messages")

	By("Editing the data model with additional fields")
	editedDescription := dataModelDescription + " - Updated with additional fields"
	editMessage, err := createEditDataModelMessage(dataModelName, editedDescription)
	if err != nil {
		getLoggerForDataModel().Fatalf("Failed to create edit data model message: %v", err)
	}
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
					getLoggerForDataModel().Infof("Received edit action reply: %s - %s", actionReply.ActionReplyState, actionReply.ActionReplyPayload)

					// Check if this is a successful edit response
					if actionReply.ActionReplyState == models.ActionFinishedSuccessfull {
						if payloadMap, ok := actionReply.ActionReplyPayload.(map[string]interface{}); ok {
							if name, exists := payloadMap["name"]; exists {
								if nameStr, ok := name.(string); ok && nameStr == dataModelName {
									getLoggerForDataModel().Info("Data model edit completed successfully")
									return true
								}
							}
						}
					}
				}
			}
		}
		return false
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Data model edit should succeed")

	By("Verifying updated data model in status messages")
	// Wait for the update to propagate
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
					// Check if our data model appears with updated version
					for _, dataModel := range statusMsg.Core.DataModels {
						if dataModel.Name == dataModelName {
							getLoggerForDataModel().Infof("Found updated data model %s with version: %s", dataModelName, dataModel.LatestVersion)
							// The version should be v2 after edit
							return dataModel.LatestVersion == "v2"
						}
					}
				}
			}
		}
		return false
	}, 15*time.Second, 1*time.Second).Should(BeTrue(), "Updated data model should appear with version v2")

	By("Verifying data contract was automatically created")
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
					// Check if data contract was automatically created
					expectedContractName := "_" + dataModelName + "_v1"
					for _, dataContract := range statusMsg.Core.DataContracts {
						if dataContract.Name == expectedContractName {
							getLoggerForDataModel().Infof("Found automatically created data contract: %s", expectedContractName)
							return true
						}
					}
				}
			}
		}
		return false
	}, 15*time.Second, 1*time.Second).Should(BeTrue(), "Data contract should be automatically created")

	By("Cleaning up - deleting the data model")
	deleteMessage, err := createDeleteDataModelMessage(dataModelName)
	if err != nil {
		getLoggerForDataModel().Fatalf("Failed to create delete data model message: %v", err)
	}
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
					getLoggerForDataModel().Infof("Received delete action reply: %s - %s", actionReply.ActionReplyState, actionReply.ActionReplyPayload)

					// Check if this is a successful deletion response
					if actionReply.ActionReplyState == models.ActionFinishedSuccessfull {
						getLoggerForDataModel().Info("Data model deletion completed successfully")
						return true
					}
				}
			}
		}
		return false
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Data model deletion should succeed")

	getLoggerForDataModel().Infof("Successfully completed data model lifecycle test for: %s", dataModelName)
}
