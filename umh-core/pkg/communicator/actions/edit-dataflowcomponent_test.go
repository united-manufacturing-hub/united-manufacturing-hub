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
		stateMocker     *actions.StateMocker
		messages        *ThreadSafeMessages
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
		messages = NewThreadSafeMessages()

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
					DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
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

		// Startup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.Tick()
		mockManagerSnapshot := stateMocker.GetStateManager()

		action = actions.NewEditDataflowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, mockManagerSnapshot)

		go ConsumeOutboundMessagesThreadSafe(outboundChannel, messages, true)

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
				"state": "active",
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
				"state": "active",
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
				"state": "active",
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
				"state": "active",
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
				"name":  "test-component-updated",
				"uuid":  componentUUID.String(),
				"meta":  map[string]interface{}{},
				"state": "active",
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
				"state": "active",
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
				"state": "active",
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
						"rawYAML": map[string]interface{}{
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
			Expect(action.GetParsedPayload().Inject).To(Equal("cache_resources:\n- label: my_cache\n  memory: {}\nrate_limit_resources:\n- label: limiter\n  local: {}\nbuffer:\n  memory: {}\n"))
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
				"state": "active",
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
				"state": "active",
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
				"state": "active",
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
								"0": map[string]interface{}{
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

			// ------------------------------------------------------------------------------------------------
			// Now, we test the action execution with the state mocker
			// The action has a pointer to the config manager and the system state
			// During the execution of the action, it will modify the config via an atomic operation
			// The state mocker has access to the same config manager. Also, the system state is shared
			// between the action and the state mocker.
			// The stateMocker.Start() starts the state mocker in a separate goroutine in which it continuously
			// updates the system state according to the config.
			// ------------------------------------------------------------------------------------------------

			// start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(1 * time.Second)

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("success"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicEditDataflowcomponent was called
			Expect(mockConfig.EditDataflowcomponentCalled).To(BeTrue())

			// Verify expected configuration changes
			Expect(mockConfig.Config.DataFlow).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].Name).To(Equal("test-component-updated"))
			Expect(mockConfig.Config.DataFlow[0].DesiredFSMState).To(Equal("active"))

			// Verify the component was updated with the new configuration
			// Checking input configuration was updated
			inputConfig := mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.Input
			Expect(inputConfig["type"]).To(Equal("http_server"))

			httpServerConfig, ok := inputConfig["http_server"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(httpServerConfig["path"]).To(Equal("/updated"))
			Expect(httpServerConfig["port"]).To(Equal(int(8001)))
		})

		It("should preserve processor order when using numeric keys", func() {
			// Setup - parse valid payload with numerically ordered processors
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state": "active",
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
								"0": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\ndescription: \"First processor - position 0\"\nprocs: []",
								},
								"1": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\ndescription: \"Second processor - position 1\"\nprocs: []",
								},
								"2": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\ndescription: \"Third processor - position 2\"\nprocs: []",
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

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("success"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicEditDataflowcomponent was called
			Expect(mockConfig.EditDataflowcomponentCalled).To(BeTrue())

			// Verify processor order is preserved
			processorsPipeline, ok := mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.Pipeline["processors"].([]interface{})
			Expect(ok).To(BeTrue(), "Pipeline processors should be a slice of interfaces")
			Expect(processorsPipeline).To(HaveLen(3), "Should have 3 processors")

			// Verify the order is preserved by checking the descriptions added to each processor
			firstProcessor, ok := processorsPipeline[0].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Processor should be a map")
			Expect(firstProcessor["type"]).To(Equal("mapping"))
			Expect(firstProcessor["procs"]).To(Equal([]interface{}{}))
			Expect(firstProcessor).To(HaveKey("description"), "First processor should have a description")
			Expect(firstProcessor["description"]).To(Equal("First processor - position 0"))

			secondProcessor, ok := processorsPipeline[1].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Processor should be a map")
			Expect(secondProcessor["type"]).To(Equal("mapping"))
			Expect(secondProcessor["procs"]).To(Equal([]interface{}{}))
			Expect(secondProcessor).To(HaveKey("description"), "Second processor should have a description")
			Expect(secondProcessor["description"]).To(Equal("Second processor - position 1"))

			thirdProcessor, ok := processorsPipeline[2].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Processor should be a map")
			Expect(thirdProcessor["type"]).To(Equal("mapping"))
			Expect(thirdProcessor["procs"]).To(Equal([]interface{}{}))
			Expect(thirdProcessor).To(HaveKey("description"), "Third processor should have a description")
			Expect(thirdProcessor["description"]).To(Equal("Third processor - position 2"))
		})

		It("should handle edit with inject data containing cache resources", func() {
			// Setup - parse valid payload with inject data
			payload := map[string]interface{}{
				"name": "test-component-updated",
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state": "active",
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
						"rawYAML": map[string]interface{}{
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
								"0": map[string]interface{}{
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

			// start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("success"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicEditDataflowcomponent was called
			Expect(mockConfig.EditDataflowcomponentCalled).To(BeTrue())

			// Verify the component was updated with the new configuration including inject data
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.CacheResources).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.CacheResources[0]["label"]).To(Equal("my_cache"))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.RateLimitResources).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.RateLimitResources[0]["label"]).To(Equal("limiter"))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.Buffer).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.Buffer["memory"]).To(Equal(map[string]interface{}{}))
		})

		It("should successfully edit component state from active to stopped and back", func() {
			// Start with a component that has "active" as the desired state
			mockConfig.WithConfig(config.FullConfig{
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
						DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
							BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
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
							},
						},
					},
				},
			})

			// Update stateMocker with the new config
			stateMocker = actions.NewStateMocker(mockConfig)
			stateMocker.Tick()
			mockManagerSnapshot := stateMocker.GetStateManager()

			action = actions.NewEditDataflowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, mockManagerSnapshot)

			// Step 1: Change state from active to stopped
			payloadStopped := map[string]interface{}{
				"name": componentName,
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state": "stopped", // Set desired state to stopped
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /input\n  port: 8000",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			// Parse the stopped state payload
			err := action.Parse(payloadStopped)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking
			mockConfig.ResetCalls()

			// Start state mocker for the first state transition
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the first state change (active to stopped)
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("success"))
			Expect(metadata).To(BeNil())

			// Verify the state was updated to stopped
			Expect(mockConfig.Config.DataFlow[0].DesiredFSMState).To(Equal("stopped"))

			// Step 2: Change state from stopped back to active
			payloadActive := map[string]interface{}{
				"name": componentName,
				"uuid": componentUUID.String(),
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state": "active", // Set desired state back to active
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: http_server\nhttp_server:\n  path: /input\n  port: 8000",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "type: stdout",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\nprocs: []",
								},
							},
						},
					},
				},
			}

			// Reset tracking
			mockConfig.ResetCalls()

			// Create fresh action instance for the second state transition
			action = actions.NewEditDataflowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, mockManagerSnapshot)

			// Parse the active state payload
			err = action.Parse(payloadActive)
			Expect(err).NotTo(HaveOccurred())

			// Execute the second state change (stopped to active)
			result, metadata, err = action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("success"))
			Expect(metadata).To(BeNil())

			// Stop state mocker
			stateMocker.Stop()

			// Verify the state was updated back to active
			Expect(mockConfig.Config.DataFlow[0].DesiredFSMState).To(Equal("active"))
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
				"state": "active",
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
								"0": map[string]interface{}{
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
			Expect(err.Error()).To(ContainSubstring("failed to edit dataflow component: mock edit dataflow component failure"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// Verify the failure message content
			decodedMessage, err := encoding.DecodeMessageFromUMHInstanceToUser(messages.Get(1).Content)
			Expect(err).NotTo(HaveOccurred())

			// Extract the ActionReplyPayload from the decoded message
			actionReplyPayload, ok := decodedMessage.Payload.(map[string]interface{})
			Expect(ok).To(BeTrue(), "Failed to cast Payload to map[string]interface{}")

			actionReplyPayloadStr, ok := actionReplyPayload["actionReplyPayload"].(string)
			Expect(ok).To(BeTrue(), "Failed to extract actionReplyPayload as string")
			Expect(actionReplyPayloadStr).To(ContainSubstring("updating configuration"))
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
				"state": "active",
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
								"0": map[string]interface{}{
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
