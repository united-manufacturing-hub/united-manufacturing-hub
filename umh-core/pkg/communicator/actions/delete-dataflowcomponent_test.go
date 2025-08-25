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
	"sync"

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
		messages        []*models.UMHMessage
		mu              sync.Mutex
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
		stateMocker.Tick()
		mockStateManager := stateMocker.GetStateManager()
		action = actions.NewDeleteDataflowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, mockStateManager)

		go actions.ConsumeOutboundMessages(outboundChannel, &messages, &mu, true)

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
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Successfully deleted dataflow component with UUID: " + componentUUID.String()))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify DeleteDataflowcomponentCalled was called
			Expect(mockConfig.DeleteDataflowcomponentCalled).To(BeTrue())

			// Verify expected configuration changes
			Expect(mockConfig.Config.DataFlow).To(BeEmpty())
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
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to delete dataflow component: mock delete dataflow component failure"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify the failure message content
			mu.Lock()
			decodedMessage, err := encoding.DecodeMessageFromUMHInstanceToUser(messages[1].Content)
			mu.Unlock()
			Expect(err).NotTo(HaveOccurred())

			// Extract the ActionReplyPayload from the decoded message
			actionReplyPayload, ok := decodedMessage.Payload.(map[string]interface{})
			Expect(ok).To(BeTrue(), "Failed to cast Payload to map[string]interface{}")

			actionReplyPayloadStr, ok := actionReplyPayload["actionReplyPayload"].(string)
			Expect(ok).To(BeTrue(), "Failed to extract actionReplyPayload as string")
			Expect(actionReplyPayloadStr).To(ContainSubstring("Removing dataflow component from configuration..."))
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

			// ------------------------------------------------------------------------------------------------
			// Now, we test the action execution with the state mocker
			// The action has a pointer to the config manager and the system state
			// During the execution of the action, it will modify the config via an atomic operation
			// The state mocker has access to the same config manager. Also, the system state is shared
			// between the action and the state mocker.
			// The stateMocker.Start() starts the state mocker in a separate goroutine in which it continuously
			// updates the system state according to the config.
			// ------------------------------------------------------------------------------------------------

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

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
