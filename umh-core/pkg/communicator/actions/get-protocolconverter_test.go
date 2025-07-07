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
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/configmanager"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	connectionfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	redpandasvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
)

// GetProtocolConverter tests verify the behavior of the GetProtocolConverterAction.
// This test suite ensures the action correctly handles different scenarios when retrieving
// protocol converters, including initialized and uninitialized converters.
var _ = Describe("GetProtocolConverter", func() {
	// Variables used across tests
	var (
		action          *actions.GetProtocolConverterAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *configmanager.MockConfigManager
		snapshotManager *fsm.SnapshotManager
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking

		// Create mock config - we don't need real config for this test
		mockConfig = configmanager.NewMockConfigManager()
		snapshotManager = fsm.NewSnapshotManager()

		action = actions.NewGetProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, snapshotManager)
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
		It("should parse valid request payload with UUID", func() {
			testUUID := uuid.New()
			payload := map[string]interface{}{
				"uuid": testUUID.String(),
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Check if the payload was properly parsed
			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.UUID).To(Equal(testUUID))
		})

		It("should handle invalid payload format gracefully", func() {
			// Invalid payload with missing UUID
			payload := map[string]interface{}{
				"name": "not-uuid",
			}

			// Call Parse method - should succeed but set UUID to zero value
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Validation should fail because UUID is zero value
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("uuid must be set"))
		})

		It("should handle malformed UUID gracefully", func() {
			// Invalid UUID format
			payload := map[string]interface{}{
				"uuid": "not-a-valid-uuid",
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Validate", func() {
		It("should validate with any parsed payload", func() {
			// Parse valid payload
			testUUID := uuid.New()
			payload := map[string]interface{}{
				"uuid": testUUID.String(),
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Execute", func() {
		Context("when protocol converter exists and is initialized", func() {
			It("should return protocol converter with read and write DFC", func() {
				// Create an initialized protocol converter with both read and write DFC
				testPCName := "test-protocol-converter"

				// Create observed state with populated DFC configs
				observedState := &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
						Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
							ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
								NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
									Target: "{{ .IP }}",
									Port:   "{{ .PORT }}",
								},
							},
							DataflowComponentReadServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
								BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
									Input: map[string]interface{}{
										"modbus": map[string]interface{}{
											"address": "{{ .IP }}:{{ .PORT }}",
										},
									},
								},
							},
							DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
								BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
									Input: map[string]interface{}{
										"kafka": map[string]interface{}{
											"addresses": []string{"localhost:9092"},
											"topics":    []string{"uns"},
										},
									},
								},
							},
						},
						Variables: variables.VariableBundle{
							User: map[string]interface{}{
								"IP":   "192.168.1.100",
								"PORT": "502",
							},
						},
						Location: map[string]string{
							"1": "factory",
							"2": "line-1",
						},
					},
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						ConnectionFSMState: "up",
						StatusReason:       "Protocol converter is healthy",
						ConnectionObservedState: connectionfsm.ConnectionObservedState{
							ServiceInfo: connection.ServiceInfo{
								NmapFSMState: "open",
							},
						},
						DataflowComponentReadFSMState: "active",
						DataflowComponentReadObservedState: dfcfsm.DataflowComponentObservedState{
							ServiceInfo: dataflowcomponent.ServiceInfo{
								StatusReason: "DFC is healthy",
							},
						},
						DataflowComponentWriteFSMState: "active",
						DataflowComponentWriteObservedState: dfcfsm.DataflowComponentObservedState{
							ServiceInfo: dataflowcomponent.ServiceInfo{
								StatusReason: "DFC is healthy",
							},
						},
						RedpandaFSMState: "active",
						RedpandaObservedState: redpandafsm.RedpandaObservedState{
							ServiceInfo: redpandasvc.ServiceInfo{
								RedpandaStatus: redpandasvc.RedpandaStatus{
									HealthCheck: redpandasvc.HealthCheck{
										IsReady: true,
										IsLive:  true,
									},
								},
							},
						},
					},
				}

				// Create FSM instance
				instance := &fsm.FSMInstanceSnapshot{
					ID:                testPCName,
					CurrentState:      protocolconverter.OperationalStateActive,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: observedState,
				}

				// Create protocol converter manager snapshot
				pcManagerSnapshot := &actions.MockManagerSnapshot{
					Instances: map[string]*fsm.FSMInstanceSnapshot{
						testPCName: instance,
					},
				}

				// Create system snapshot with config
				systemSnapshot := &fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: pcManagerSnapshot,
					},
					CurrentConfig: config.FullConfig{},
				}

				snapshotManager.UpdateSnapshot(systemSnapshot)

				// Parse with the test UUID - we need to mock the deterministic UUID generation
				// Since we can't easily mock the UUID generation, let's work with the actual generated UUID
				actualUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(testPCName)
				payload := map[string]interface{}{
					"uuid": actualUUID.String(),
				}
				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())

				// Execute the action
				result, metadata, err := action.Execute()
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata).To(BeNil())

				// Check the response content
				response, ok := result.(models.ProtocolConverter)
				Expect(ok).To(BeTrue(), "Result should be a ProtocolConverter")

				// Verify basic protocol converter properties
				Expect(response.UUID).NotTo(BeNil())
				Expect(*response.UUID).To(Equal(actualUUID))
				Expect(response.Name).To(Equal(testPCName))
				Expect(response.Connection.IP).To(Equal("192.168.1.100"))
				Expect(response.Connection.Port).To(Equal(uint32(502)))

				// Verify location was properly converted from string keys to int keys
				Expect(response.Location).To(HaveLen(2))
				Expect(response.Location[1]).To(Equal("factory"))
				Expect(response.Location[2]).To(Equal("line-1"))

				// Verify read DFC is populated
				Expect(response.ReadDFC).NotTo(BeNil())
				Expect(response.ReadDFC.Inputs.Type).To(Equal("modbus"))

				// Verify write DFC is populated
				Expect(response.WriteDFC).NotTo(BeNil())
				Expect(response.WriteDFC.Inputs.Type).To(Equal("kafka"))

				// Verify meta information
				Expect(response.Meta).NotTo(BeNil())
				Expect(response.Meta.ProcessingMode).To(Equal("custom")) // Determined from read DFC
				Expect(response.Meta.Protocol).To(Equal("modbus"))       // Determined from read DFC input type

			})
		})

		Context("when protocol converter exists but is not initialized", func() {
			It("should return protocol converter without DFC configs", func() {
				// Create an uninitialized protocol converter with empty DFC configs
				testPCName := "test-protocol-converter-uninitialized"

				// Create observed state with empty DFC configs
				observedState := &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ObservedProtocolConverterSpecConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
						Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
							ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
								NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
									Target: "192.168.1.101",
									Port:   "503",
								},
							},
						},
						Location: map[string]string{
							"1": "factory",
							"3": "line-2",
						},
					},
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						ConnectionFSMState: "up",
						StatusReason:       "Protocol converter not initialized",
					},
				}

				// Create FSM instance in starting state due to missing DFC
				instance := &fsm.FSMInstanceSnapshot{
					ID:                testPCName,
					CurrentState:      protocolconverter.OperationalStateStartingFailedDFCMissing,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: observedState,
				}

				// Create protocol converter manager snapshot
				pcManagerSnapshot := &actions.MockManagerSnapshot{
					Instances: map[string]*fsm.FSMInstanceSnapshot{
						testPCName: instance,
					},
				}

				// Create system snapshot
				systemSnapshot := &fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: pcManagerSnapshot,
					},
					CurrentConfig: config.FullConfig{},
				}

				snapshotManager.UpdateSnapshot(systemSnapshot)

				// Parse with the test UUID
				actualUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(testPCName)
				payload := map[string]interface{}{
					"uuid": actualUUID.String(),
				}
				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())

				// Execute the action
				result, metadata, err := action.Execute()
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata).To(BeNil())

				// Check the response content
				response, ok := result.(models.ProtocolConverter)
				Expect(ok).To(BeTrue(), "Result should be a ProtocolConverter")

				// Verify basic protocol converter properties
				Expect(response.UUID).NotTo(BeNil())
				Expect(*response.UUID).To(Equal(actualUUID))
				Expect(response.Name).To(Equal(testPCName))
				Expect(response.Connection.IP).To(Equal("192.168.1.101"))
				Expect(response.Connection.Port).To(Equal(uint32(503)))

				// Verify location
				Expect(response.Location).To(HaveLen(2))
				Expect(response.Location[1]).To(Equal("factory"))
				Expect(response.Location[3]).To(Equal("line-2"))

				// Verify DFC configs are nil for uninitialized protocol converter
				Expect(response.ReadDFC).To(BeNil())
				Expect(response.WriteDFC).To(BeNil())

				// Verify meta information reflects uninitialized state
				Expect(response.Meta).NotTo(BeNil())
				Expect(response.Meta.ProcessingMode).To(Equal("")) // No DFC present
				Expect(response.Meta.Protocol).To(Equal(""))       // Default protocol

			})
		})

		Context("when protocol converter does not exist", func() {
			It("should return error for non-existent protocol converter", func() {
				// Create empty system snapshot
				systemSnapshot := &fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
							Instances: map[string]*fsm.FSMInstanceSnapshot{},
						},
					},
				}

				snapshotManager.UpdateSnapshot(systemSnapshot)

				// Parse with non-existent UUID
				nonExistentUUID := uuid.New()
				payload := map[string]interface{}{
					"uuid": nonExistentUUID.String(),
				}
				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())

				// Execute the action
				result, metadata, err := action.Execute()
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(metadata).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("not found"))

			})
		})

		Context("when protocol converter manager does not exist", func() {
			It("should return error when no protocol converter manager exists", func() {
				// Create system snapshot without protocol converter manager
				systemSnapshot := &fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{},
				}

				snapshotManager.UpdateSnapshot(systemSnapshot)

				// Parse with any UUID
				testUUID := uuid.New()
				payload := map[string]interface{}{
					"uuid": testUUID.String(),
				}
				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())

				// Execute the action
				result, metadata, err := action.Execute()
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(metadata).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		Context("when protocol converter has invalid observed state", func() {
			It("should return error for invalid observed state type", func() {
				testPCName := "test-protocol-converter-invalid-state"

				// Create FSM instance with invalid observed state type
				instance := &fsm.FSMInstanceSnapshot{
					ID:                testPCName,
					CurrentState:      protocolconverter.OperationalStateActive,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: &actions.MockObservedState{}, // Wrong type
				}

				// Create protocol converter manager snapshot
				pcManagerSnapshot := &actions.MockManagerSnapshot{
					Instances: map[string]*fsm.FSMInstanceSnapshot{
						testPCName: instance,
					},
				}

				// Create system snapshot
				systemSnapshot := &fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: pcManagerSnapshot,
					},
				}

				snapshotManager.UpdateSnapshot(systemSnapshot)

				// Parse with the test UUID
				actualUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(testPCName)
				payload := map[string]interface{}{
					"uuid": actualUUID.String(),
				}
				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())

				// Execute the action
				result, metadata, err := action.Execute()
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(metadata).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid observed state type"))
			})

			It("should return error for nil observed state", func() {
				testPCName := "test-protocol-converter-nil-state"

				// Create FSM instance with nil observed state
				instance := &fsm.FSMInstanceSnapshot{
					ID:                testPCName,
					CurrentState:      protocolconverter.OperationalStateActive,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: nil,
				}

				// Create protocol converter manager snapshot
				pcManagerSnapshot := &actions.MockManagerSnapshot{
					Instances: map[string]*fsm.FSMInstanceSnapshot{
						testPCName: instance,
					},
				}

				// Create system snapshot
				systemSnapshot := &fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: pcManagerSnapshot,
					},
				}

				snapshotManager.UpdateSnapshot(systemSnapshot)

				// Parse with the test UUID
				actualUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(testPCName)
				payload := map[string]interface{}{
					"uuid": actualUUID.String(),
				}
				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())

				// Execute the action
				result, metadata, err := action.Execute()
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(metadata).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid observed state"))
			})
		})
	})
})
