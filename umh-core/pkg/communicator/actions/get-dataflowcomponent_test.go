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
	"reflect"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// GetDataFlowComponent tests verify the behavior of the GetDataFlowComponentAction.
// This test suite ensures the action correctly handles different scenarios when retrieving
// dataflow components, including when components exist or don't exist, and proper response formatting.
var _ = Describe("GetDataFlowComponent", func() {
	// Variables used across tests
	var (
		action          *actions.GetDataFlowComponentAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		mockSnapshot    *fsm.SystemSnapshot
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking

		// Create mock config
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
						Name:            "test-component-1",
						DesiredFSMState: "active",
					},
					DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]interface{}{
								"kafka": map[string]interface{}{
									"addresses": []string{"localhost:9092"},
									"topics":    []string{"test-topic"},
								},
							},
							Output: map[string]interface{}{
								"kafka": map[string]interface{}{
									"addresses": []string{"localhost:9092"},
									"topic":     "output-topic",
								},
							},
							Pipeline: map[string]interface{}{
								"processors": []interface{}{
									map[string]interface{}{
										"mapping": "root = this",
									},
								},
							},
							CacheResources: []map[string]interface{}{
								{
									"label":  "test-cache",
									"memory": map[string]interface{}{},
								},
							},
							RateLimitResources: []map[string]interface{}{
								{
									"label": "test-rate-limit",
									"local": map[string]interface{}{},
								},
							},
							Buffer: map[string]interface{}{
								"memory": map[string]interface{}{},
							},
						},
					},
				},
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            "test-component-2",
						DesiredFSMState: "active",
					},
					DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]interface{}{
								"file": map[string]interface{}{
									"paths": []string{"/tmp/input.txt"},
								},
							},
							Output: map[string]interface{}{
								"file": map[string]interface{}{
									"path": "/tmp/output.txt",
								},
							},
							Pipeline: map[string]interface{}{
								"processors": []interface{}{
									map[string]interface{}{
										"text": map[string]interface{}{
											"operator": "to_upper",
										},
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
		stateMocker := actions.NewStateMocker(mockConfig)
		stateMocker.UpdateState()
		mockSnapshot = stateMocker.GetState()

		action = actions.NewGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, mockSnapshot)
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
		It("should parse valid request payload with UUIDs", func() {
			// Valid payload with UUID list
			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{
					"00000000-0000-0000-0000-000000000001",
					"00000000-0000-0000-0000-000000000002",
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Check if the payload was properly parsed
			parsedPayload := action.GetParsedVersionUUIDs()
			Expect(parsedPayload.VersionUUIDs).To(HaveLen(2))
			Expect(parsedPayload.VersionUUIDs).To(ContainElement("00000000-0000-0000-0000-000000000001"))
			Expect(parsedPayload.VersionUUIDs).To(ContainElement("00000000-0000-0000-0000-000000000002"))
		})

		It("should parse an empty UUID list", func() {
			// Empty UUID list
			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Check if the payload was properly parsed
			parsedPayload := action.GetParsedVersionUUIDs()
			Expect(parsedPayload.VersionUUIDs).To(HaveLen(0))
		})

		It("should handle invalid payload format gracefully", func() {
			// Invalid payload with versionUUIDs as string instead of array
			payload := map[string]interface{}{
				"versionUUIDs": "not-an-array",
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error unmarshaling into target type"))
		})
	})

	Describe("Validate", func() {
		It("should validate with any payload", func() {
			// Parse valid payload
			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{
					"00000000-0000-0000-0000-000000000001",
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate even with empty payload", func() {
			// Parse empty payload
			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Execute", func() {
		It("should retrieve components that match the requested UUIDs", func() {
			// Parse with valid UUIDs that match test components
			testComponentID := "test-component-1"
			testComponentUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(testComponentID).String()

			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{
					testComponentUUID,
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Check the response content
			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue(), "Result should be a GetDataflowcomponentResponse")

			// Check that we have the expected component in the response
			Expect(response).To(HaveKey(testComponentUUID))
			Expect(response[testComponentUUID].Name).To(Equal(testComponentID))
			Expect(response[testComponentUUID].Meta.Type).To(Equal("custom"))

			// Verify the component details
			component := response[testComponentUUID]
			cdfcPayload, ok := component.Payload.(models.CommonDataFlowComponentCDFCPropertiesPayload)
			Expect(ok).To(BeTrue(), "Payload should be a CommonDataFlowComponentCDFCPropertiesPayload")
			Expect(cdfcPayload.CDFCProperties.Inputs.Type).To(Equal("benthos"))
			Expect(cdfcPayload.CDFCProperties.Outputs.Type).To(Equal("benthos"))
			Expect(cdfcPayload.CDFCProperties.Pipeline.Processors).NotTo(BeEmpty())

			// Verify correct message sequence in outbound channel
			var messages []*models.UMHMessage
			select {
			case msg := <-outboundChannel:
				messages = append(messages, msg)
			case <-time.After(100 * time.Millisecond):
				Fail("Timed out waiting for message")
			}
			Expect(messages).To(HaveLen(1)) // Just the Executing message
			// Note: We're not sending ActionFinishedSuccessfull from Execute
		})

		It("should return an empty response for non-existent components", func() {
			// Parse with UUID that doesn't match any component
			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{
					"11111111-1111-1111-1111-111111111111", // Non-existent UUID
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Check that the response is empty
			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue(), "Result should be a GetDataflowcomponentResponse")
			Expect(response).To(BeEmpty())

			// Verify message in outbound channel
			var messages []*models.UMHMessage
			select {
			case msg := <-outboundChannel:
				messages = append(messages, msg)
			case <-time.After(100 * time.Millisecond):
				Fail("Timed out waiting for message")
			}
			Expect(messages).To(HaveLen(1)) // Just the Executing message
		})

		It("should handle the case when no dataflowcomponent manager exists", func() {
			// Create a new system snapshot without a dataflowcomponent manager
			emptySnapshot := &fsm.SystemSnapshot{
				Managers: map[string]fsm.ManagerSnapshot{},
			}

			// Create action with empty snapshot
			action = actions.NewGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, emptySnapshot)

			// Parse valid payload
			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{
					"00000000-0000-0000-0000-000000000001",
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Check that the response is empty
			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue(), "Result should be a GetDataflowcomponentResponse")
			Expect(response).To(BeEmpty())
		})

		It("should handle components with missing observed state", func() {
			// Create a system snapshot with a component that has no observed state
			snapshotWithMissingState := actions.CreateMockSystemSnapshotWithMissingState()

			// Create action with this special snapshot
			action = actions.NewGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, snapshotWithMissingState)

			// Parse with UUID that matches the component with missing state
			testComponentID := "test-component-missing-state"
			testComponentUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(testComponentID).String()

			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{
					testComponentUUID,
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Check that the response doesn't contain the component with missing state
			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue(), "Result should be a GetDataflowcomponentResponse")
			Expect(response).NotTo(HaveKey(testComponentUUID))
		})
	})

	Describe("buildDataFlowComponentDataFromSnapshot", func() {
		var (
			testInstance fsm.FSMInstanceSnapshot
		)

		BeforeEach(func() {
			// Set up test instance
			testInstance = fsm.FSMInstanceSnapshot{
				ID:           "test-component-build",
				DesiredState: "active",
				CurrentState: "active",
				LastObservedState: &dataflowcomponent.DataflowComponentObservedStateSnapshot{
					Config: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
						BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
							Input: map[string]interface{}{
								"test": "input",
							},
							Output: map[string]interface{}{
								"test": "output",
							},
						},
					},
				},
			}
		})

		It("should correctly build DataFlowComponentConfig from snapshot", func() {
			// Access the non-exported buildDataFlowComponentDataFromSnapshot using reflection
			buildDataFunc := reflect.ValueOf(action).Elem().MethodByName("BuildDataFlowComponentDataFromSnapshot")
			Expect(buildDataFunc.IsValid()).To(BeFalse(), "The method should not be exported")

			// Since we can't directly call the unexported function, we'll test its behavior through Execute
			// Get a system snapshot with our test instance
			managerSnapshot := &actions.MockManagerSnapshot{
				Instances: map[string]*fsm.FSMInstanceSnapshot{
					"test-component-build": &testInstance,
				},
			}

			testSnapshot := &fsm.SystemSnapshot{
				Managers: map[string]fsm.ManagerSnapshot{
					constants.DataflowcomponentManagerName: managerSnapshot,
				},
			}

			// Set up action with this snapshot
			testAction := actions.NewGetDataFlowComponentAction(
				userEmail,
				actionUUID,
				instanceUUID,
				outboundChannel,
				mockConfig,
				testSnapshot,
			)

			// Parse with the UUID of our test component
			testUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName("test-component-build").String()
			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{testUUID},
			}
			err := testAction.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute and check result
			result, _, err := testAction.Execute()
			Expect(err).NotTo(HaveOccurred())

			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue())
			Expect(response).To(HaveKey(testUUID))

			// Verify the data was correctly extracted
			component := response[testUUID]
			Expect(component.Name).To(Equal("test-component-build"))

			// Check input/output data was properly extracted
			cdfcPayload, ok := component.Payload.(models.CommonDataFlowComponentCDFCPropertiesPayload)
			Expect(ok).To(BeTrue())

			// Input data should contain our test input
			Expect(cdfcPayload.CDFCProperties.Inputs.Data).To(ContainSubstring("test: input"))

			// Output data should contain our test output
			Expect(cdfcPayload.CDFCProperties.Outputs.Data).To(ContainSubstring("test: output"))
		})

		It("should return error for invalid observed state type", func() {
			// Create an instance with invalid observed state type
			invalidInstance := fsm.FSMInstanceSnapshot{
				ID:                "invalid-type-component",
				DesiredState:      "active",
				CurrentState:      "active",
				LastObservedState: &actions.MockObservedState{}, // Not a DataflowComponentObservedStateSnapshot

			}

			// Access the buildDataFlowComponentDataFromSnapshot through Execute
			managerSnapshot := &actions.MockManagerSnapshot{
				Instances: map[string]*fsm.FSMInstanceSnapshot{
					"invalid-type-component": &invalidInstance,
				},
			}

			testSnapshot := &fsm.SystemSnapshot{
				Managers: map[string]fsm.ManagerSnapshot{
					constants.DataflowcomponentManagerName: managerSnapshot,
				},
			}

			// Set up action with this snapshot
			testAction := actions.NewGetDataFlowComponentAction(
				userEmail,
				actionUUID,
				instanceUUID,
				outboundChannel,
				mockConfig,
				testSnapshot,
			)

			// Parse with the UUID of our invalid component
			testUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName("invalid-type-component").String()
			payload := map[string]interface{}{
				"versionUUIDs": []interface{}{testUUID},
			}
			err := testAction.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute and check result - should return empty map since building the component failed
			result, _, err := testAction.Execute()
			Expect(err).NotTo(HaveOccurred())

			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue())
			Expect(response).To(BeEmpty())
		})
	})
})
