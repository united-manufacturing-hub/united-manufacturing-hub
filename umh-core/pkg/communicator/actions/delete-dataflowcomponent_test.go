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

package actions_test

import (
	"errors"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// DeleteDataflowComponent tests verify the behavior of the DeleteDataflowComponentAction.
// This test suite ensures the action correctly parses the UUID, validates it,
// and properly deletes components from the system.
var _ = Describe("DeleteDataflowComponent", func() {
	// Variables used across tests
	var (
		action          *actions.DeleteDataflowComponentAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		componentName   string
		componentUUID   uuid.UUID
		stateMocker     *actions.StateMocker
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		componentName = "test-component"
		componentUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(componentName)

		// Create initial config with one data flow component
		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
			},
			DataFlow: []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            componentName,
						DesiredFSMState: "running",
					},
				},
			},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)

		// Startup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.UpdateDfcState()
		mockSnapshot := stateMocker.GetState()

		action = actions.NewDeleteDataflowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, mockSnapshot)
	})

	// Cleanup after each test
	AfterEach(func() {
		// Drain the outbound channel to prevent goroutine leaks
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse valid UUID payload", func() {
			// Valid payload with UUID of component to delete
			payload := map[string]interface{}{
				"uuid": componentUUID.String(),
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetComponentUUID()).To(Equal(componentUUID))
		})

		It("should return error for missing UUID", func() {
			// Payload with missing UUID field
			payload := map[string]interface{}{}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field UUID"))
		})

		It("should return error for invalid UUID format", func() {
			// Payload with invalid UUID format
			payload := map[string]interface{}{
				"uuid": "not-a-uuid",
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid UUID format"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation with valid UUID", func() {
			// First parse valid UUID
			payload := map[string]interface{}{
				"uuid": componentUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Then validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Execute", func() {
		It("should delete existing dataflow component from configuration successfully", func() {
			// Setup - parse valid UUID first
			payload := map[string]interface{}{
				"uuid": componentUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking for this test
			mockConfig.ResetCalls()

			// Start the state mocker
			stateMocker.Start()

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Successfully deleted data flow component"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify DeleteDataflowcomponentCalled was called
			Expect(mockConfig.DeleteDataflowcomponentCalled).To(BeTrue())

			// Verify expected configuration changes
			Expect(mockConfig.Config.DataFlow).To(HaveLen(0))
		})

		It("should handle AtomicDeleteDataflowcomponent failure", func() {
			// Set up mock to fail on AtomicDeleteDataflowcomponent
			mockConfig.WithDeleteDataflowcomponentError(errors.New("mock delete dataflow component failure"))

			// Parse valid UUID
			payload := map[string]interface{}{
				"uuid": componentUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			stateMocker.Start()

			// Execute the action - should fail
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to delete dataflow component"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Expect Confirmed and Failure messages
			var messages []*models.UMHMessage
			for i := 0; i < 2; i++ {
				select {
				case msg := <-outboundChannel:
					messages = append(messages, msg)
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out waiting for message")
				}
			}
			Expect(messages).To(HaveLen(2))

			// Verify the failure message content
			decodedMessage, err := encoding.DecodeMessageFromUMHInstanceToUser(messages[1].Content)
			Expect(err).NotTo(HaveOccurred())

			// Extract the ActionReplyPayload from the decoded message
			actionReplyPayload, ok := decodedMessage.Payload.(map[string]interface{})
			Expect(ok).To(BeTrue(), "Failed to cast Payload to map[string]interface{}")

			actionReplyPayloadStr, ok := actionReplyPayload["actionReplyPayload"].(string)
			Expect(ok).To(BeTrue(), "Failed to extract actionReplyPayload as string")
			Expect(actionReplyPayloadStr).To(ContainSubstring("failed to delete dataflow component"))
		})

		It("should return error when component not found", func() {
			// Use a non-existent UUID
			nonExistentUUID := uuid.New()
			payload := map[string]interface{}{
				"uuid": nonExistentUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Set up mock to return component not found error
			mockConfig.WithDeleteDataflowcomponentError(errors.New("dataflow component with UUID not found"))

			// Start the state mocker
			stateMocker.Start()

			// Execute the action - should fail with component not found
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dataflow component with UUID not found"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})
	})
})
