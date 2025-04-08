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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("DeployDataflowComponent", func() {
	// Variables used across tests
	var (
		action          *actions.DeployDataflowComponentAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
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
		action = actions.NewDeployDataflowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)
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
		})

		It("should return error for missing name", func() {
			// Payload with missing required name field
			payload := map[string]interface{}{
				"meta": map[string]interface{}{
					"type": "custom",
				},
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
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Name"))
		})

		It("should return error for missing meta type", func() {
			// Payload with missing meta.type field
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{},
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
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Meta.Type"))
		})

		It("should return error for unsupported component type", func() {
			// Payload with unsupported meta.type
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "unsupported-type",
				},
				"payload": map[string]interface{}{},
			}

			// Call Parse method
			err := action.Parse(payload)
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
				"payload": map[string]interface{}{
					// Missing customDataFlowComponent
				},
			}

			// Call Parse method
			err := action.Parse(payload)
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
				"payload": map[string]interface{}{
					"customDataFlowComponent": map[string]interface{}{
						// Missing inputs
						"outputs": map[string]interface{}{
							"type": "yaml",
							"data": "output: something\nformat: json",
						},
					},
				},
			}

			// Call Parse method
			err := action.Parse(payload)
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

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field pipeline.processors"))
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
	})

	Describe("Execute", func() {
		It("should add dataflow component to configuration successfully", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
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
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// We should have messages in the channel (Confirmed, then Success)
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

			// Verify AtomicAddDataflowcomponent was called
			Expect(mockConfig.AddDataflowcomponentCalled).To(BeTrue())

			Expect(mockConfig.Config.DataFlow).To(HaveLen(1))
			Expect(mockConfig.Config.DataFlow[0].Name).To(Equal("test-component"))
			Expect(mockConfig.Config.DataFlow[0].DesiredFSMState).To(Equal("running"))
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
			Expect(err.Error()).To(ContainSubstring("Failed to add dataflowcomponent"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// We should have failure messages in the channel
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

			// For failure tests, we don't check exact content since it may be encoded
			// We check that we received the expected number of messages,
			// which confirms the error path was taken correctly
		})
	})
})

// Note: The MockConfigManager used in this test should implement the following methods:
// - GetConfig
// - AtomicAddDataflowcomponent
// - ResetCalls
// - WithAddDataflowcomponentError (to simulate failures)
// - AddDataflowcomponentCalled (to track if the method was called)
