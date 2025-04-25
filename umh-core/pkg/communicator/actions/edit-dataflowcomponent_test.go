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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// EditDataflowComponent tests verify the behavior of the EditDataflowComponentAction.
// This test suite ensures the action correctly handles component editing,
// validates configuration, and properly updates components in the system.
var _ = Describe("EditDataflowComponent", func() {
	// Variables used across tests
	var (
		action          *actions.EditDataflowComponentAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		componentName   string
		componentUUID   uuid.UUID
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		componentName = "test-component"
		componentUUID = dataflowcomponentconfig.GenerateUUIDFromName(componentName)

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
						DesiredFSMState: "active",
					},
					DataFlowComponentConfig: dataflowcomponentconfig.DataFlowComponentConfig{
						BenthosConfig: dataflowcomponentconfig.BenthosConfig{
							Input: map[string]interface{}{
								"type": "http_server",
								"http_server": map[string]interface{}{
									"path": "/input",
									"port": 8000,
								},
							},
							Output: map[string]interface{}{
								"type": "stdout",
							},
							Pipeline: map[string]interface{}{
								"processors": []interface{}{
									map[string]interface{}{
										"type": "mapping",
									},
								},
							},
						},
					},
				},
			},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)
		action = actions.NewEditDataflowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)
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
		It("should parse valid edit dataflow component payload", func() {
			// Valid payload with complete edit dataflow component information
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"proc1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetComponentUUID()).To(Equal(componentUUID))
		})

		It("should return error for missing UUID", func() {
			// Payload with missing UUID field
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
					},
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid UUID"))
		})

		It("should return error for invalid UUID format", func() {
			// Payload with invalid UUID format
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": "not-a-uuid",
				"meta": map[string]interface{}{
					"type": "custom",
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid UUID format"))
		})

		It("should return error for missing name", func() {
			// Payload with missing name field
			payload := map[string]interface{}{
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
					},
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Name"))
		})

		It("should return error for missing meta type", func() {
			// Payload with missing meta type
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
					},
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Meta.Type"))
		})

		It("should return error for unsupported component type", func() {
			// Payload with unsupported component type
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "unsupported-type",
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported component type"))
		})

		It("should handle parse with inject data", func() {
			// Payload with inject data
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"inject": map[string]interface{}{
							"type": "yaml",
							"data": "cache_resources:\n- label: my_cache\n  memory: {}\nrate_limit_resources:\n- label: limiter\n  local: {}\nbuffer:\n  memory: {}\n",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"proc1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetParsedPayload().Inject.Data).To(Equal("cache_resources:\n- label: my_cache\n  memory: {}\nrate_limit_resources:\n- label: limiter\n  local: {}\nbuffer:\n  memory: {}\n"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation after valid parse", func() {
			// First parse valid data
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"proc1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Then validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation with invalid YAML in processor", func() {
			// Payload with invalid YAML in processor
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"proc1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: [missing: bracket}", // Invalid YAML
								},
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Then validate - should fail
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is not valid YAML"))
		})
	})

	Describe("Execute", func() {
		It("should edit dataflow component in configuration successfully", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"proc1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking for this test
			mockConfig.ResetCalls()

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Successfully edited dataflow component: test-component-updated"))
			Expect(metadata).To(BeNil())

			// Expect only the Confirmed message in the channel
			// Success message is sent by HandleActionMessage, not by Execute
			var messages []*models.UMHMessage
			for i := 0; i < 1; i++ {
				select {
				case msg := <-outboundChannel:
					messages = append(messages, msg)
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out waiting for message")
				}
			}
			Expect(messages).To(HaveLen(1))

			// Verify AtomicEditDataflowcomponent was called
			Expect(mockConfig.EditDataflowcomponentCalled).To(BeTrue())

			// Verify expected configuration changes
			Expect(mockConfig.Config.DataFlow).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].Name).To(Equal("test-component-updated"))
			Expect(mockConfig.Config.DataFlow[0].DesiredFSMState).To(Equal("active"))

			// Verify the component was updated with the new configuration
			// Checking input configuration was updated
			inputConfig := mockConfig.Config.DataFlow[0].DataFlowComponentConfig.BenthosConfig.Input
			Expect(inputConfig["type"]).To(Equal("http_server"))

			httpServerConfig, ok := inputConfig["http_server"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(httpServerConfig["path"]).To(Equal("/updated"))
			Expect(httpServerConfig["port"]).To(Equal(int(8001)))
		})

		It("should handle edit with inject data containing cache resources", func() {
			// Setup - parse valid payload with inject data
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"inject": map[string]interface{}{
							"type": "yaml",
							"data": `cache_resources:
- label: my_cache
  memory: {}
rate_limit_resources:
- label: limiter
  local: {}
buffer:
  memory: {}`,
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"proc1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking for this test
			mockConfig.ResetCalls()

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Successfully edited dataflow component: test-component-updated"))
			Expect(metadata).To(BeNil())

			// Verify AtomicEditDataflowcomponent was called
			Expect(mockConfig.EditDataflowcomponentCalled).To(BeTrue())

			// Verify the component was updated with the new configuration including inject data
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentConfig.BenthosConfig.CacheResources).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentConfig.BenthosConfig.CacheResources[0]["label"]).To(Equal("my_cache"))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentConfig.BenthosConfig.RateLimitResources).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentConfig.BenthosConfig.RateLimitResources[0]["label"]).To(Equal("limiter"))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentConfig.BenthosConfig.Buffer).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentConfig.BenthosConfig.Buffer["memory"]).To(Equal(map[string]interface{}{}))
		})

		It("should handle AtomicEditDataflowcomponent failure", func() {
			// Set up mock to fail on AtomicEditDataflowcomponent
			mockConfig.WithEditDataflowcomponentError(errors.New("mock edit dataflow component failure"))

			// Parse with valid payload
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"proc1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to edit dataflow component: mock edit dataflow component failure"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

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
			Expect(actionReplyPayloadStr).To(ContainSubstring("Updating dataflow component 'test-component-updated' configuration.."))
		})

		It("should handle failure when component not found", func() {
			// Use a non-existent UUID
			nonExistentUUID := uuid.New()
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": nonExistentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /updated\n  port: 8001",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"proc1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Set up mock to return component not found error
			mockConfig.WithEditDataflowcomponentError(errors.New("dataflow component with UUID not found"))

			// Execute the action - should fail with component not found
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dataflow component with UUID not found"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())
		})
	})
})
