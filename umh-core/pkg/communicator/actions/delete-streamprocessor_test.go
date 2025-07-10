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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("DeleteStreamProcessor", func() {
	// Variables used across tests
	var (
		action          *actions.DeleteStreamProcessorAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		stateMocker     *actions.StateMocker
		messages        []*models.UMHMessage
		spName          string
		spUUID          uuid.UUID
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		spName = "test-stream-processor"
		spUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(spName)

		// Create initial config with existing stream processor
		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
				Location: map[int]string{
					0: "test-enterprise",
					1: "test-site",
					2: "test-area",
					3: "test-line",
				},
			},
			StreamProcessor: []config.StreamProcessorConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            spName,
						DesiredFSMState: "active",
					},
					StreamProcessorServiceConfig: streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
						Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
							Model: streamprocessorserviceconfig.ModelRef{
								Name:    "test-model",
								Version: "1.0.0",
							},
							Sources: streamprocessorserviceconfig.SourceMapping{
								"opcua": "umh.v1.test.path.opcua",
							},
							Mapping: map[string]interface{}{
								"source":    "opcua",
								"transform": "sum",
							},
						},
						Variables: variables.VariableBundle{
							User: map[string]interface{}{
								"enterprise": "test-enterprise",
								"site":       "test-site",
							},
						},
						Location: map[string]string{
							"0": "test-enterprise",
							"1": "test-site",
							"2": "test-area",
							"3": "test-line",
						},
						TemplateRef: spName, // Self-referencing template (root)
					},
				},
			},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)

		// Setup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.Tick()

		action = actions.NewDeleteStreamProcessorAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

		go actions.ConsumeOutboundMessages(outboundChannel, &messages, true)
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
		It("should parse valid stream processor delete payload", func() {
			payload := map[string]interface{}{
				"name": spName,
				"uuid": spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetStreamProcessorUUID()).To(Equal(spUUID))

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Name).To(Equal(spName))
			Expect(*parsedPayload.UUID).To(Equal(spUUID))
		})

		It("should return error for missing UUID", func() {
			// Payload without UUID
			payload := map[string]interface{}{
				"name": spName,
			}

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field UUID"))
		})

		It("should return error for invalid payload format", func() {
			// Invalid payload (not a map)
			payload := "invalid-payload"

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse stream processor payload"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation with valid stream processor configuration", func() {
			// First parse valid payload
			payload := map[string]interface{}{
				"name": spName,
				"uuid": spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Then validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation with invalid UUID", func() {
			// Payload with nil UUID
			payload := map[string]interface{}{
				"name": spName,
				"uuid": uuid.Nil.String(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing or invalid stream processor UUID"))
		})

		It("should fail validation with missing name", func() {
			// Payload with missing name field
			payload := map[string]interface{}{
				"name": "",
				"uuid": spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name cannot be empty"))
		})

		It("should fail validation with invalid stream processor name", func() {
			// Payload with invalid name (contains special characters)
			payload := map[string]interface{}{
				"name": "invalid name!@#",
				"uuid": spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name can only contain letters"))
		})
	})

	Describe("Execute", func() {
		It("should delete stream processor successfully", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name": spName,
				"uuid": spUUID.String(),
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
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicDeleteStreamProcessor was called
			Expect(mockConfig.AtomicDeleteStreamProcessorCalled).To(BeTrue())

			// Verify the response contains the expected deleted name
			responseMap, ok := result.(map[string]any)
			Expect(ok).To(BeTrue(), "Result should be a map")
			Expect(responseMap).To(HaveKey("deleted_name"))
			Expect(responseMap["deleted_name"]).To(Equal(spName))

			// Verify the stream processor was removed from config
			Expect(mockConfig.Config.StreamProcessor).To(HaveLen(0))
		})

		It("should handle AtomicDeleteStreamProcessor failure", func() {
			// Set up mock to fail on AtomicDeleteStreamProcessor
			mockConfig.WithAtomicDeleteStreamProcessorError(errors.New("mock delete stream processor failure"))

			// Parse valid payload
			payload := map[string]interface{}{
				"name": spName,
				"uuid": spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to delete stream processor"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})

		It("should handle stream processor not found", func() {
			// Use a different name that doesn't exist
			nonExistentName := "non-existent-processor"
			nonExistentUUID := actions.GenerateUUIDFromName(nonExistentName)

			// Set up mock to fail when stream processor is not found
			mockConfig.WithAtomicDeleteStreamProcessorError(errors.New("stream processor with name \"" + nonExistentName + "\" not found"))

			payload := map[string]interface{}{
				"name": nonExistentName,
				"uuid": nonExistentUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to delete stream processor"))
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})

		It("should handle deletion with health check disabled", func() {
			// Create action with nil snapshot manager (disables health checks)
			actionWithoutHealthCheck := actions.NewDeleteStreamProcessorAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

			// Parse valid payload
			payload := map[string]interface{}{
				"name": spName,
				"uuid": spUUID.String(),
			}

			err := actionWithoutHealthCheck.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking for this test
			mockConfig.ResetCalls()

			// Execute the action (no state mocker needed)
			result, metadata, err := actionWithoutHealthCheck.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Verify AtomicDeleteStreamProcessor was called
			Expect(mockConfig.AtomicDeleteStreamProcessorCalled).To(BeTrue())

			// Verify the response contains the expected deleted name
			responseMap, ok := result.(map[string]any)
			Expect(ok).To(BeTrue(), "Result should be a map")
			Expect(responseMap).To(HaveKey("deleted_name"))
			Expect(responseMap["deleted_name"]).To(Equal(spName))
		})

		It("should delete root stream processor successfully", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name": spName,
				"uuid": spUUID.String(),
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
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicDeleteStreamProcessor was called
			Expect(mockConfig.AtomicDeleteStreamProcessorCalled).To(BeTrue())

			// Verify the response contains the expected deleted name
			responseMap, ok := result.(map[string]any)
			Expect(ok).To(BeTrue(), "Result should be a map")
			Expect(responseMap).To(HaveKey("deleted_name"))
			Expect(responseMap["deleted_name"]).To(Equal(spName))

			// Verify the stream processor was removed from config
			Expect(mockConfig.Config.StreamProcessor).To(HaveLen(0))
		})

		It("should handle deletion of stream processor with dependent children", func() {
			// Add a child stream processor to the config that depends on the root
			childName := "child-stream-processor"
			childSP := config.StreamProcessorConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            childName,
					DesiredFSMState: "active",
				},
				StreamProcessorServiceConfig: streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
					Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
						Model: streamprocessorserviceconfig.ModelRef{
							Name:    "test-model",
							Version: "1.0.0",
						},
						Sources: streamprocessorserviceconfig.SourceMapping{
							"opcua": "umh.v1.test.path.opcua",
						},
						Mapping: map[string]interface{}{
							"source":    "opcua",
							"transform": "sum",
						},
					},
					Variables: variables.VariableBundle{
						User: map[string]interface{}{
							"enterprise": "child-enterprise",
							"site":       "child-site",
						},
					},
					Location: map[string]string{
						"0": "test-enterprise",
						"1": "test-site",
						"2": "test-area",
						"3": "test-line",
					},
					TemplateRef: spName, // References the root template
				},
			}

			mockConfig.Config.StreamProcessor = append(mockConfig.Config.StreamProcessor, childSP)

			// Set up mock to fail when trying to delete root with dependent children
			mockConfig.WithAtomicDeleteStreamProcessorError(errors.New("cannot delete template \"" + spName + "\": it has dependent child instances"))

			// Parse valid payload for root deletion
			payload := map[string]interface{}{
				"name": spName,
				"uuid": spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to delete stream processor"))
			Expect(err.Error()).To(ContainSubstring("dependent child instances"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})
	})
})
