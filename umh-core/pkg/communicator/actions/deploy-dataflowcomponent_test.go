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
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// DeployDataflowComponent tests verify the behavior of the DeployDataflowComponentAction.
// This test suite ensures the action correctly handles different component types,
// validates configuration, and properly deploys valid components to the system.
// It tests both the success path and various error conditions.
var _ = Describe("DeployDataflowComponent", func() {
	// Variables used across tests
	var (
		action          *actions.DeployDataflowComponentAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		stateMocker     *actions.StateMocker
		messages        []*models.UMHMessage
		mutex           sync.Mutex
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking

		// Create initial config
		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
			},
			DataFlow: []config.DataFlowComponentConfig{},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)

		// Startup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.Tick()
		mockStateManager := stateMocker.GetStateManager()

		action = actions.NewDeployDataflowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, mockStateManager)

		go actions.ConsumeOutboundMessages(outboundChannel, &messages, &mutex, true)

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
		It("should parse valid custom dataflow component payload", func() {
			// Valid payload with complete custom dataflow component information
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "active",
				"ignoreHealthCheck": false,
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
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

			// Call Parse method
			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should parse valid custom dataflow component payload with inject data", func() {
			// Valid payload with complete custom dataflow component information including inject data
			payload := map[string]interface{}{
				"name": "test-component-with-inject",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "active",
				"ignoreHealthCheck": false,
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
						},
						"rawYAML": map[string]interface{}{
							"data": "cache_resources:\n- label: my_cache\n  memory: {}\nrate_limit_resources:\n- label: limiter\n  local: {}\nbuffer:\n  memory: {}\n",
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

			// Call Parse method
			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetParsedPayload().Inject).To(Equal("cache_resources:\n- label: my_cache\n  memory: {}\nrate_limit_resources:\n- label: limiter\n  local: {}\nbuffer:\n  memory: {}\n"))

		})

		It("should return error for invalid YAML in inject data", func() {
			// Payload with invalid YAML in inject data
			payload := map[string]interface{}{
				"name": "test-component-with-bad-inject",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "active",
				"ignoreHealthCheck": false,
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
						},
						"rawYAML": map[string]interface{}{
							"data": "cache_resources: [test: {missing: bracket}", // This is truly invalid YAML syntax
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

			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())

			// Call Validate method - this should fail
			err = action.Validate(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inject.data is not valid YAML"))
		})

		It("should return error for missing name", func() {
			// Payload with missing required name field
			payload := map[string]interface{}{
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state": "active",
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
						},
					},
				},
			}

			// Call Parse method
			err := action.Parse(context.Background(), payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Name"))
		})

		It("should return error for unsupported component type", func() {
			// Payload with unsupported meta.type
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "unsupported-type",
				},
				"state":   "active",
				"payload": map[string]interface{}{},
			}

			// Call Parse method
			err := action.Parse(context.Background(), payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported component type"))
		})

		It("should return error for missing customDataFlowComponent", func() {
			// Payload with missing customDataFlowComponent in custom type
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":   "active",
				"payload": map[string]interface{}{
					// Missing customDataFlowComponent
				},
			}

			// Call Parse method
			err := action.Parse(context.Background(), payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing customDataFlowComponent in payload"))
		})

		It("should return error for missing inputs", func() {
			// Payload with missing inputs in customDataFlowComponent
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state": "active",
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						// Missing inputs
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
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

			// Call Parse method
			err := action.Parse(context.Background(), payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field inputs"))
		})

		It("should return error for missing pipeline.processors", func() {
			// Payload with missing pipeline.processors in customDataFlowComponent
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state": "active",
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
						},
						"pipeline": map[string]interface{}{
							// Missing processors
						},
					},
				},
			}

			// Call Parse method - this should now succeed with structural parsing
			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())

			// Call Validate method - this should fail with field validation
			err = action.Validate(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field pipeline.processors"))
		})

		It("should reject flattened payload structure without customDataFlowComponent", func() {
			// Payload with flattened structure (incorrect format)
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state": "active",
				"payload": map[string]interface{}{
					// Direct inputs without customDataFlowComponent wrapper
					"inputs": map[string]interface{}{
						"type": "yaml",
						"data": "input: something\nformat: json",
					},
					"outputs": map[string]interface{}{
						"type": "yaml",
						"data": "output: something\nformat: json",
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
			}

			// Call Parse method - should fail with appropriate error
			err := action.Parse(context.Background(), payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing customDataFlowComponent in payload"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation after valid parse", func() {
			// First parse valid data
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "active",
				"ignoreHealthCheck": false,
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
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

			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())

			// Then validate
			err = action.Validate(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Execute", func() {
		It("should add dataflow component to configuration successfully", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "active",
				"ignoreHealthCheck": false,
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
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

			err := action.Parse(context.Background(), payload)
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

			// Execute the action
			result, metadata, err := action.Execute(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("success"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicAddDataflowcomponent was called
			Expect(mockConfig.AddDataflowcomponentCalled).To(BeTrue())

			// Verify expected configuration changes
			Expect(mockConfig.Config.DataFlow).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].Name).To(Equal("test-component"))
			Expect(mockConfig.Config.DataFlow[0].DesiredFSMState).To(Equal("active"))

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

		It("should fail if a non-numerous index is given", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "active",
				"ignoreHealthCheck": false,
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
						},
						"pipeline": map[string]interface{}{
							"processors": map[string]interface{}{
								"0": map[string]interface{}{
									"type": "yaml",
									"data": "type: mapping\ndescription: \"First processor - position 0\"\nprocs: []",
								},
								"1proc": map[string]interface{}{
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

			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking for this test
			mockConfig.ResetCalls()

			// start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			_, metadata, err := action.Execute(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one processor with a non-numerous key was found"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

		})

		It("should handle AtomicAddDataflowcomponent failure", func() {
			// Set up mock to fail on AtomicAddDataflowcomponent
			mockConfig.WithAddDataflowcomponentError(errors.New("mock add dataflow component failure"))

			// Parse with valid payload
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "active",
				"ignoreHealthCheck": false,
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
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

			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			result, metadata, err := action.Execute(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to add dataflow component: mock add dataflow component failure"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify the failure message content
			mutex.Lock()
			decodedMessage, err := encoding.DecodeMessageFromUMHInstanceToUser(messages[1].Content)
			mutex.Unlock()
			Expect(err).NotTo(HaveOccurred())

			// Extract the ActionReplyPayload from the decoded message
			actionReplyPayload, ok := decodedMessage.Payload.(map[string]interface{})
			Expect(ok).To(BeTrue(), "failed to cast Payload to map[string]interface{}")

			actionReplyPayloadStr, ok := actionReplyPayload["actionReplyPayload"].(string)
			Expect(ok).To(BeTrue(), "failed to extract actionReplyPayload as string")
			Expect(actionReplyPayloadStr).To(ContainSubstring("adding to configuration"))
		})

		It("should process inject data with cache resources, rate limit resources, and buffer", func() {
			// Setup - parse valid payload with inject data
			payload := map[string]interface{}{
				"name": "test-component-with-inject",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "active",
				"ignoreHealthCheck": false,
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						"inputs": map[string]interface{}{
							"type": "yaml",
							"data": "input: something\nformat: json",
						},
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
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

			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking for this test
			mockConfig.ResetCalls()

			// start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("success"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicAddDataflowcomponent was called
			Expect(mockConfig.AddDataflowcomponentCalled).To(BeTrue())

			// Verify the component was added with correct configuration
			Expect(mockConfig.Config.DataFlow).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].Name).To(Equal("test-component-with-inject"))
			Expect(mockConfig.Config.DataFlow[0].DesiredFSMState).To(Equal("active"))

			// Verify inject configuration was properly processed
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.CacheResources).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.CacheResources[0]["label"]).To(Equal("my_cache"))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.RateLimitResources).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.RateLimitResources[0]["label"]).To(Equal("limiter"))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.Buffer).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.Buffer["memory"]).To(Equal(map[string]interface{}{}))
		})

		It("should deploy dataflow component with initial state 'stopped'", func() {
			// Setup - parse payload with state set to stopped
			payload := map[string]interface{}{
				"name": "stopped-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"state":             "stopped", // Setting the initial state to stopped
				"ignoreHealthCheck": true,      // We need this since we're explicitly not starting it
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

			err := action.Parse(context.Background(), payload)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking for this test
			mockConfig.ResetCalls()

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("success"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicAddDataflowcomponent was called
			Expect(mockConfig.AddDataflowcomponentCalled).To(BeTrue())

			// Verify the component was added with correct configuration and state
			Expect(mockConfig.Config.DataFlow).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].Name).To(Equal("stopped-component"))

			// This is the key assertion - verify the component is configured with stopped state
			Expect(mockConfig.Config.DataFlow[0].DesiredFSMState).To(Equal("stopped"))

			// Verify component structure
			inputConfig := mockConfig.Config.DataFlow[0].DataFlowComponentServiceConfig.BenthosConfig.Input
			Expect(inputConfig["type"]).To(Equal("http_server"))

			httpServerConfig, ok := inputConfig["http_server"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(httpServerConfig["path"]).To(Equal("/input"))
			Expect(httpServerConfig["port"]).To(Equal(int(8000)))
		})
	})
})
