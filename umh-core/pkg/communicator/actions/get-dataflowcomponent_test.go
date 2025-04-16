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
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsmmanager "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	fsmdfcomp "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	svcdfcomp "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
)

func init() {
	// Register the test
	_ = GinkgoT()
}

// MockGetDataFlowComponentAction is a test implementation that embeds the real action
// and adds methods for testing but keeps unexported fields
type MockGetDataFlowComponentAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager
	systemSnapshot  *fsm.SystemSnapshot
	payload         models.GetDataflowcomponentRequestSchemaJson
	actionLogger    *zap.SugaredLogger
}

// NewMockGetDataFlowComponentAction creates a new mock for testing
func NewMockGetDataFlowComponentAction(
	userEmail string,
	actionUUID uuid.UUID,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	configManager config.ConfigManager,
	systemSnapshot *fsm.SystemSnapshot,
	logger *zap.SugaredLogger,
) *MockGetDataFlowComponentAction {
	return &MockGetDataFlowComponentAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		systemSnapshot:  systemSnapshot,
		actionLogger:    logger,
	}
}

// Parse implements the Parse method
func (m *MockGetDataFlowComponentAction) Parse(payload interface{}) error {
	// Check that payload is a map with versionUUIDs
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid payload format")
	}

	uuids, exists := payloadMap["versionUUIDs"]
	if !exists {
		return fmt.Errorf("missing required field versionUUIDs")
	}

	// Make sure versionUUIDs is an array
	_, ok = uuids.([]interface{})
	if !ok {
		return fmt.Errorf("versionUUIDs must be an array")
	}

	// Parse with the real implementation
	parsedPayload, err := actions.ParseActionPayload[models.GetDataflowcomponentRequestSchemaJson](payload)
	if err != nil {
		return err
	}

	m.payload = parsedPayload
	return nil
}

// Validate implements the Validate method
func (m *MockGetDataFlowComponentAction) Validate() error {
	// GetDataFlowComponentAction has empty validate
	return nil
}

// Execute implements the Execute method
func (m *MockGetDataFlowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	// Send executing message
	actions.SendActionReply(m.instanceUUID, m.userEmail, m.actionUUID, models.ActionExecuting, "getting the dataflowcomponent", m.outboundChannel, models.GetDataFlowComponent)

	dataFlowComponents := []config.DataFlowComponentConfig{}
	// Get the DataFlowComponent
	m.actionLogger.Debugf("Getting the DataFlowComponent")

	if dataflowcomponentManager, exists := m.systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
		m.actionLogger.Debugf("Dataflowcomponent manager found, getting the dataflowcomponent")
		instances := dataflowcomponentManager.GetInstances()
		for _, instance := range instances {
			dfc, err := buildDataFlowComponentDataFromSnapshot(instance, m.actionLogger)
			if err != nil {
				m.actionLogger.Warnf("Failed to build dataflowcomponent data: %v", err)
				continue // Skip this instance if we can't build its data
			}
			currentUUID := dataflowcomponentconfig.GenerateUUIDFromName(instance.ID).String()
			if m.containsVersionUUID(currentUUID) {
				m.actionLogger.Debugf("Adding %s to the response", instance.ID)
				dataFlowComponents = append(dataFlowComponents, dfc)
			}
		}
	}

	// build the response
	m.actionLogger.Info("Building the response")
	response := buildResponse(dataFlowComponents, m.actionLogger)

	m.actionLogger.Info("Response built, returning")
	return response, nil, nil
}

// Helper method to check if UUID is in versionUUIDs
func (m *MockGetDataFlowComponentAction) containsVersionUUID(uuid string) bool {
	for _, versionUUID := range m.payload.VersionUUIDs {
		if versionUUID == uuid {
			return true
		}
	}
	return false
}

// Helper function to build dataflow component data from snapshot (copied from the action)
func buildDataFlowComponentDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (config.DataFlowComponentConfig, error) {
	dfcData := config.DataFlowComponentConfig{}

	log.Info("Building dataflowcomponent data from snapshot", zap.String("instanceID", instance.ID))

	if instance.LastObservedState != nil {
		// Try to cast to the right type
		observedState, ok := instance.LastObservedState.(*fsmdfcomp.DataflowComponentObservedStateSnapshot)
		if !ok {
			log.Warn("Failed to cast observed state", zap.String("instanceID", instance.ID))
			return config.DataFlowComponentConfig{}, nil
		}
		dfcData.DataFlowComponentConfig = observedState.Config
		dfcData.FSMInstanceConfig.Name = instance.ID
		dfcData.FSMInstanceConfig.DesiredFSMState = instance.DesiredState

	} else {
		log.Warn("No observed state found for dataflowcomponent", zap.String("instanceID", instance.ID))
		return config.DataFlowComponentConfig{}, nil
	}

	return dfcData, nil
}

// Helper function to build response (copied from the action)
func buildResponse(dataFlowComponents []config.DataFlowComponentConfig, log *zap.SugaredLogger) models.GetDataflowcomponentResponse {
	response := models.GetDataflowcomponentResponse{}
	for _, component := range dataFlowComponents {
		// build the payload
		dfc_payload := models.CommonDataFlowComponentCDFCPropertiesPayload{}
		tagValue := "not-used"
		dfc_payload.CDFCProperties.BenthosImageTag = &models.CommonDataFlowComponentBenthosImageTagConfig{
			Tag: &tagValue,
		}
		dfc_payload.CDFCProperties.IgnoreErrors = nil
		//fill the inputs, outputs, pipeline and rawYAML
		// Convert the BenthosConfig input to CommonDataFlowComponentInputConfig
		inputData, err := yaml.Marshal(component.DataFlowComponentConfig.BenthosConfig.Input)
		if err != nil {
			log.Warnf("Failed to marshal input data: %v", err)
		}
		dfc_payload.CDFCProperties.Inputs = models.CommonDataFlowComponentInputConfig{
			Data: string(inputData),
			Type: "benthos", // Default type for benthos inputs
		}

		// Convert the BenthosConfig output to CommonDataFlowComponentOutputConfig
		outputData, err := yaml.Marshal(component.DataFlowComponentConfig.BenthosConfig.Output)
		if err != nil {
			log.Warnf("Failed to marshal output data: %v", err)
		}
		dfc_payload.CDFCProperties.Outputs = models.CommonDataFlowComponentOutputConfig{
			Data: string(outputData),
			Type: "benthos", // Default type for benthos outputs
		}

		// Convert the BenthosConfig pipeline to CommonDataFlowComponentPipelineConfig
		processors := models.CommonDataFlowComponentPipelineConfigProcessors{}

		// Extract processors from the pipeline if they exist
		if pipeline, ok := component.DataFlowComponentConfig.BenthosConfig.Pipeline["processors"].([]interface{}); ok {
			for i, proc := range pipeline {
				procData, err := yaml.Marshal(proc)
				if err != nil {
					log.Warnf("Failed to marshal processor data: %v", err)
					continue
				}
				// Use index as processor name if not specified
				procName := GetProcessorName(i)
				processors[procName] = struct {
					Data string `json:"data" yaml:"data" mapstructure:"data"`
					Type string `json:"type" yaml:"type" mapstructure:"type"`
				}{
					Data: string(procData),
					Type: "bloblang", // Default type for benthos processors
				}
			}
		}

		// Set threads value if present in the pipeline
		var threads *int
		if threadsVal, ok := component.DataFlowComponentConfig.BenthosConfig.Pipeline["threads"]; ok {
			if t, ok := threadsVal.(int); ok {
				threads = &t
			}
		}

		dfc_payload.CDFCProperties.Pipeline = models.CommonDataFlowComponentPipelineConfig{
			Processors: processors,
			Threads:    threads,
		}

		// Create RawYAML from the cache_resources, rate_limit_resources, and buffer
		rawYAMLMap := map[string]interface{}{}

		// Add cache resources if present
		if len(component.DataFlowComponentConfig.BenthosConfig.CacheResources) > 0 {
			rawYAMLMap["cache_resources"] = component.DataFlowComponentConfig.BenthosConfig.CacheResources
		}

		// Add rate limit resources if present
		if len(component.DataFlowComponentConfig.BenthosConfig.RateLimitResources) > 0 {
			rawYAMLMap["rate_limit_resources"] = component.DataFlowComponentConfig.BenthosConfig.RateLimitResources
		}

		// Add buffer if present
		if len(component.DataFlowComponentConfig.BenthosConfig.Buffer) > 0 {
			rawYAMLMap["buffer"] = component.DataFlowComponentConfig.BenthosConfig.Buffer
		}

		// Only create rawYAML if we have any data
		if len(rawYAMLMap) > 0 {
			rawYAMLData, err := yaml.Marshal(rawYAMLMap)
			if err != nil {
				log.Warnf("Failed to marshal rawYAML data: %v", err)
			} else {
				dfc_payload.CDFCProperties.RawYAML = &models.CommonDataFlowComponentRawYamlConfig{
					Data: string(rawYAMLData),
				}
			}
		}

		response[dataflowcomponentconfig.GenerateUUIDFromName(component.FSMInstanceConfig.Name).String()] = models.GetDataflowcomponentResponseContent{
			CreationTime: 0,
			Creator:      "",
			Meta: models.CommonDataFlowComponentMeta{
				Type: "custom",
			},
			Name:      component.FSMInstanceConfig.Name,
			ParentDFC: nil,
			Payload:   dfc_payload,
		}
	}
	return response
}

// GetProcessorName returns a processor name using index
func GetProcessorName(index int) string {
	return "processor_" + string(rune('0'+index))
}

// GetDataFlowComponent tests verify the behavior of the GetDataFlowComponentAction.
// This test suite ensures the action correctly retrieves dataflow components from the system,
// filters them by provided UUIDs, and handles various edge cases properly.
var _ = Describe("GetDataFlowComponent", func() {
	// Variables used across tests
	var (
		action          *MockGetDataFlowComponentAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		systemSnapshot  *fsm.SystemSnapshot
		logger          *zap.SugaredLogger
	)

	// Create a mock dataflow component manager
	createMockDataflowComponentManager := func(components []config.DataFlowComponentConfig) *fsm.BaseManagerSnapshot {
		snapshot := &fsm.BaseManagerSnapshot{
			Name:         constants.DataflowcomponentManagerName,
			Instances:    make(map[string]fsm.FSMInstanceSnapshot),
			ManagerTick:  1,
			SnapshotTime: time.Now(),
		}

		// Add instances for each component
		for _, component := range components {
			// Create observed state snapshot
			observedState := &fsmdfcomp.DataflowComponentObservedStateSnapshot{
				Config: component.DataFlowComponentConfig,
				ServiceInfo: svcdfcomp.ServiceInfo{
					BenthosObservedState: benthosfsmmanager.BenthosObservedState{},
					BenthosFSMState:      "running",
				},
			}

			// Create instance snapshot
			instanceSnapshot := fsm.FSMInstanceSnapshot{
				ID:                component.FSMInstanceConfig.Name,
				CurrentState:      "running",
				DesiredState:      component.FSMInstanceConfig.DesiredFSMState,
				LastObservedState: observedState,
				CreatedAt:         time.Now().Add(-1 * time.Hour),
				LastUpdatedAt:     time.Now().Add(-10 * time.Minute),
			}

			snapshot.Instances[component.FSMInstanceConfig.Name] = instanceSnapshot
		}

		return snapshot
	}

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		logger = zaptest.NewLogger(GinkgoT()).Sugar()

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

		// Create a system snapshot with no managers initially
		systemSnapshot = &fsm.SystemSnapshot{
			Managers:     make(map[string]fsm.ManagerSnapshot),
			SnapshotTime: time.Now(),
			Tick:         1,
		}
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
		It("should parse valid payload with version UUIDs", func() {
			// Valid payload with version UUIDs
			payload := map[string]any{
				"versionUUIDs": []string{
					uuid.New().String(),
					uuid.New().String(),
				},
			}

			// Create action and parse payload
			action = NewMockGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, systemSnapshot, logger)
			err := action.Parse(payload)

			// Verify
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle empty version UUIDs array", func() {
			// Payload with empty version UUIDs
			payload := map[string]any{
				"versionUUIDs": []string{},
			}

			// Create action and parse payload
			action = NewMockGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, systemSnapshot, logger)
			err := action.Parse(payload)

			// Verify
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle missing versionUUIDs field", func() {
			// Payload without versionUUIDs field
			payload := map[string]any{}

			// Create action and parse payload
			action = NewMockGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, systemSnapshot, logger)
			err := action.Parse(payload)

			// Verify that this is considered an error since versionUUIDs is required
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("versionUUIDs"))
		})
	})

	Describe("Execute", func() {
		It("should retrieve dataflow components matching the requested UUIDs", func() {
			// Create test components
			component1Name := "test-component-1"
			component2Name := "test-component-2"
			component3Name := "test-component-3"

			component1 := config.DataFlowComponentConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            component1Name,
					DesiredFSMState: "active",
				},
				DataFlowComponentConfig: dataflowcomponentconfig.DataFlowComponentConfig{
					BenthosConfig: dataflowcomponentconfig.BenthosConfig{
						Input: map[string]interface{}{
							"type":   "mqtt",
							"broker": "tcp://localhost:1883",
							"topic":  "test/topic/1",
						},
						Output: map[string]interface{}{
							"type":    "kafka",
							"brokers": []string{"localhost:9092"},
							"topic":   "test_output_1",
						},
						Pipeline: map[string]interface{}{
							"processors": []interface{}{
								map[string]interface{}{
									"type":    "mapping",
									"mapping": "root = this",
								},
							},
						},
					},
				},
			}

			component2 := config.DataFlowComponentConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            component2Name,
					DesiredFSMState: "active",
				},
				DataFlowComponentConfig: dataflowcomponentconfig.DataFlowComponentConfig{
					BenthosConfig: dataflowcomponentconfig.BenthosConfig{
						Input: map[string]interface{}{
							"type":   "mqtt",
							"broker": "tcp://localhost:1883",
							"topic":  "test/topic/2",
						},
						Output: map[string]interface{}{
							"type":    "kafka",
							"brokers": []string{"localhost:9092"},
							"topic":   "test_output_2",
						},
						Pipeline: map[string]interface{}{
							"processors": []interface{}{
								map[string]interface{}{
									"type":    "mapping",
									"mapping": "root = this",
								},
							},
							"threads": 2,
						},
						CacheResources: []map[string]interface{}{
							{
								"label":  "my_cache",
								"memory": map[string]interface{}{},
							},
						},
					},
				},
			}

			component3 := config.DataFlowComponentConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            component3Name,
					DesiredFSMState: "active",
				},
				DataFlowComponentConfig: dataflowcomponentconfig.DataFlowComponentConfig{
					BenthosConfig: dataflowcomponentconfig.BenthosConfig{
						Input: map[string]interface{}{
							"type":   "mqtt",
							"broker": "tcp://localhost:1883",
							"topic":  "test/topic/3",
						},
						Output: map[string]interface{}{
							"type":    "kafka",
							"brokers": []string{"localhost:9092"},
							"topic":   "test_output_3",
						},
						Pipeline: map[string]interface{}{
							"processors": []interface{}{
								map[string]interface{}{
									"type":    "mapping",
									"mapping": "root = this",
								},
							},
						},
						Buffer: map[string]interface{}{
							"memory": map[string]interface{}{},
						},
					},
				},
			}

			// Generate UUIDs for the components
			component1UUID := dataflowcomponentconfig.GenerateUUIDFromName(component1Name).String()
			component2UUID := dataflowcomponentconfig.GenerateUUIDFromName(component2Name).String()
			component3UUID := dataflowcomponentconfig.GenerateUUIDFromName(component3Name).String()

			// Create a mock dataflow component manager with the test components
			managerSnapshot := createMockDataflowComponentManager([]config.DataFlowComponentConfig{
				component1, component2, component3,
			})

			// Add the manager to the system snapshot
			systemSnapshot.Managers[constants.DataflowcomponentManagerName] = managerSnapshot

			// Create the action with a payload that requests only components 1 and 3
			action = NewMockGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, systemSnapshot, logger)
			err := action.Parse(map[string]interface{}{
				"versionUUIDs": []string{component1UUID, component3UUID},
			})
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Verify the response contains only components 1 and 3
			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue(), "Result should be of type models.GetDataflowcomponentResponse")
			Expect(response).To(HaveLen(2), "Response should contain exactly 2 components")

			// Verify component 1 was correctly returned
			Expect(response).To(HaveKey(component1UUID))
			Expect(response[component1UUID].Name).To(Equal(component1Name))
			Expect(response[component1UUID].Meta.Type).To(Equal("custom"))

			// Verify component 3 was correctly returned
			Expect(response).To(HaveKey(component3UUID))
			Expect(response[component3UUID].Name).To(Equal(component3Name))
			Expect(response[component3UUID].Meta.Type).To(Equal("custom"))

			// Verify component 2 was NOT returned
			Expect(response).NotTo(HaveKey(component2UUID))

			// Verify one message in the outbound channel (ActionExecuting)
			Expect(outboundChannel).To(HaveLen(1))
		})

		It("should return empty response when no components match the requested UUIDs", func() {
			// Create a test component
			component1Name := "test-component-1"
			component1 := config.DataFlowComponentConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            component1Name,
					DesiredFSMState: "active",
				},
				DataFlowComponentConfig: dataflowcomponentconfig.DataFlowComponentConfig{
					BenthosConfig: dataflowcomponentconfig.BenthosConfig{
						Input: map[string]interface{}{
							"type":   "mqtt",
							"broker": "tcp://localhost:1883",
							"topic":  "test/topic/1",
						},
						Output: map[string]interface{}{
							"type":    "kafka",
							"brokers": []string{"localhost:9092"},
							"topic":   "test_output_1",
						},
						Pipeline: map[string]interface{}{
							"processors": []interface{}{
								map[string]interface{}{
									"type":    "mapping",
									"mapping": "root = this",
								},
							},
						},
					},
				},
			}

			// Create a mock dataflow component manager with the test component
			managerSnapshot := createMockDataflowComponentManager([]config.DataFlowComponentConfig{
				component1,
			})

			// Add the manager to the system snapshot
			systemSnapshot.Managers[constants.DataflowcomponentManagerName] = managerSnapshot

			// Create the action with a payload that requests a non-existent component
			action = NewMockGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, systemSnapshot, logger)
			err := action.Parse(map[string]interface{}{
				"versionUUIDs": []string{uuid.New().String()},
			})
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Verify the response is empty
			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue(), "Result should be of type models.GetDataflowcomponentResponse")
			Expect(response).To(HaveLen(0), "Response should be empty")

			// Verify one message in the outbound channel (ActionExecuting)
			Expect(outboundChannel).To(HaveLen(1))
		})

		It("should handle case when dataflowcomponent manager doesn't exist", func() {
			// Create a system snapshot with no managers
			emptySnapshot := &fsm.SystemSnapshot{
				Managers:     make(map[string]fsm.ManagerSnapshot),
				SnapshotTime: time.Now(),
				Tick:         1,
			}

			// Create the action with a valid payload
			action = NewMockGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, emptySnapshot, logger)
			err := action.Parse(map[string]interface{}{
				"versionUUIDs": []string{uuid.New().String()},
			})
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Verify the response is empty
			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue(), "Result should be of type models.GetDataflowcomponentResponse")
			Expect(response).To(HaveLen(0), "Response should be empty")

			// Verify one message in the outbound channel (ActionExecuting)
			Expect(outboundChannel).To(HaveLen(1))
		})

		It("should handle instances without last observed state", func() {
			// Create a mock manager with an instance missing the last observed state
			managerSnapshot := &fsm.BaseManagerSnapshot{
				Name:         constants.DataflowcomponentManagerName,
				Instances:    make(map[string]fsm.FSMInstanceSnapshot),
				ManagerTick:  1,
				SnapshotTime: time.Now(),
			}

			// Add instance without last observed state
			instanceName := "test-component-no-state"
			instanceUUIDStr := dataflowcomponentconfig.GenerateUUIDFromName(instanceName).String()

			managerSnapshot.Instances[instanceName] = fsm.FSMInstanceSnapshot{
				ID:                instanceName,
				CurrentState:      "initializing",
				DesiredState:      "active",
				LastObservedState: nil, // No observed state
				CreatedAt:         time.Now().Add(-1 * time.Hour),
				LastUpdatedAt:     time.Now().Add(-10 * time.Minute),
			}

			// Add the manager to the system snapshot
			systemSnapshot.Managers[constants.DataflowcomponentManagerName] = managerSnapshot

			// Create the action with a payload that requests the component
			action = NewMockGetDataFlowComponentAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, systemSnapshot, logger)
			err := action.Parse(map[string]interface{}{
				"versionUUIDs": []string{instanceUUIDStr},
			})
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Verify the response is empty (since the component had no observed state)
			response, ok := result.(models.GetDataflowcomponentResponse)
			Expect(ok).To(BeTrue(), "Result should be of type models.GetDataflowcomponentResponse")
			Expect(response).To(HaveLen(0), "Response should be empty")

			// Verify one message in the outbound channel (ActionExecuting)
			Expect(outboundChannel).To(HaveLen(1))
		})
	})
})
