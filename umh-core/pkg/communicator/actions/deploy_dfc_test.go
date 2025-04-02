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
	"fmt"
	"reflect"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// Export the AddDataflowcomponentRequestPayload type for testing
type TestAddDataflowcomponentRequestPayload struct {
	// IgnoreHealthCheck flag to ignore health check
	IgnoreHealthCheck bool `json:"ignoreHealthCheck,omitempty"`
	// Meta contains metadata about the component
	Meta struct {
		// Type of the component (e.g., "custom", "data-bridge", "protocol-converter", "stream-processor")
		Type string `json:"type"`
		// AdditionalMetadata contains additional key-value pairs
		AdditionalMetadata map[string]interface{} `json:"additionalMetadata,omitempty"`
	} `json:"meta"`
	// Name of the component
	Name string `json:"name"`
	// Payload contains the specific configuration for the component type
	Payload map[string]interface{} `json:"payload"`
}

// DFCMockConfigManager implements the config.ConfigManager interface for testing
type DFCMockConfigManager struct {
	config          config.FullConfig
	getShouldFail   bool
	writeShouldFail bool
	latestConfig    config.FullConfig
	mutex           sync.Mutex
	writeCallCount  int
	getCallCount    int
}

func NewDFCMockConfigManager() *DFCMockConfigManager {
	return &DFCMockConfigManager{
		config: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
			},
			Services: []config.S6FSMConfig{},
			Benthos:  []config.BenthosConfig{},
			Nmap:     []config.NmapConfig{},
		},
	}
}

func (m *DFCMockConfigManager) GetConfig(_ context.Context, _ uint64) (config.FullConfig, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.getCallCount++

	if m.getShouldFail {
		return config.FullConfig{}, errors.New("mock GetConfig failure")
	}
	return m.config.Clone(), nil
}

func (m *DFCMockConfigManager) WriteConfig(_ context.Context, cfg config.FullConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.writeCallCount++

	if m.writeShouldFail {
		return errors.New("mock WriteConfig failure")
	}

	m.latestConfig = cfg.Clone()
	m.config = cfg.Clone()
	return nil
}

func (m *DFCMockConfigManager) WithGetConfigFailure() *DFCMockConfigManager {
	m.getShouldFail = true
	return m
}

func (m *DFCMockConfigManager) WithWriteConfigFailure() *DFCMockConfigManager {
	m.writeShouldFail = true
	return m
}

func (m *DFCMockConfigManager) GetWriteCallCount() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.writeCallCount
}

func (m *DFCMockConfigManager) GetGetCallCount() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.getCallCount
}

func (m *DFCMockConfigManager) GetLatestConfig() config.FullConfig {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.latestConfig.Clone()
}

var _ = Describe("DeployDFCAction", func() {
	// Variables used across tests
	var (
		action          *actions.DeployDFCAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *DFCMockConfigManager
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		mockConfig = NewDFCMockConfigManager()

		// Create the action instance using the constructor and set the mock config manager
		action = actions.NewDeployDFCAction(userEmail, actionUUID, instanceUUID, outboundChannel)
		action.WithConfigManager(mockConfig)
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
		It("should parse valid custom component payload", func() {
			// Valid payload for a custom component
			payload := map[string]interface{}{
				"name": "test-custom-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"input": map[string]interface{}{
						"mqtt": map[string]interface{}{
							"urls":   []string{"tcp://localhost:1883"},
							"topics": []string{"test/topic"},
						},
					},
					"output": map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"localhost:9092"},
							"topic":     "test-topic",
						},
					},
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// We can't access the payload field directly since it's private, but we can verify the component attributes
			// through the validation that happens later
		})

		It("should parse valid data-bridge component payload", func() {
			// Valid payload for a data-bridge component
			payload := map[string]interface{}{
				"name": "test-bridge-component",
				"meta": map[string]interface{}{
					"type": "data-bridge",
				},
				"payload": map[string]interface{}{
					"bridge": map[string]interface{}{
						"inputType":        "mqtt",
						"outputType":       "kafka",
						"dataContractName": "test-contract",
					},
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Validate succeeding is evidence enough the payload was parsed correctly
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Validate", func() {
		It("should validate with valid payload", func() {
			// Parse a valid payload
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{},
			}

			// Parse the payload first
			Expect(action.Parse(payload)).To(Succeed())

			// Validate
			err := action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation with missing name", func() {
			// Parse an invalid payload (missing name)
			payload := map[string]interface{}{
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{},
			}

			Expect(action.Parse(payload)).To(Succeed())

			// Validate
			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name is required"))
		})

		It("should fail validation with missing type", func() {
			// Parse an invalid payload (missing type)
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "",
				},
				"payload": map[string]interface{}{},
			}

			Expect(action.Parse(payload)).To(Succeed())

			// Validate
			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("meta.type is required"))
		})

		It("should fail validation with invalid type", func() {
			// Parse an invalid payload (invalid type)
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "invalid-type",
				},
				"payload": map[string]interface{}{},
			}

			Expect(action.Parse(payload)).To(Succeed())

			// Validate
			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid component type"))
		})
	})

	Describe("Execute", func() {
		BeforeEach(func() {
			// Set a valid payload for all execute tests
			payload := map[string]interface{}{
				"name": "test-component",
				"meta": map[string]interface{}{
					"type": "custom",
				},
				"payload": map[string]interface{}{
					"input": map[string]interface{}{
						"mqtt": map[string]interface{}{
							"urls":   []string{"tcp://localhost:1883"},
							"topics": []string{"test/topic"},
						},
					},
					"output": map[string]interface{}{
						"kafka": map[string]interface{}{
							"addresses": []string{"localhost:9092"},
							"topic":     "test-topic",
						},
					},
				},
			}

			Expect(action.Parse(payload)).To(Succeed())
		})

		It("should successfully deploy a component", func() {
			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Successfully deployed"))
			Expect(metadata).NotTo(BeNil())
			Expect(metadata["name"]).To(Equal("test-component"))

			// Verify the config was updated
			Expect(mockConfig.GetGetCallCount()).To(Equal(1))
			Expect(mockConfig.GetWriteCallCount()).To(Equal(1))

			// Verify the Benthos configuration was added
			latestConfig := mockConfig.GetLatestConfig()
			Expect(latestConfig.Benthos).To(HaveLen(1))
			Expect(latestConfig.Benthos[0].Name).To(Equal("test-component"))
			Expect(latestConfig.Benthos[0].DesiredFSMState).To(Equal("active"))
			Expect(latestConfig.Benthos[0].BenthosServiceConfig).NotTo(BeNil())
		})

		It("should handle GetConfig failure", func() {
			// Configure mock to fail on GetConfig
			mockConfig.WithGetConfigFailure()

			// Execute the action
			_, _, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get current configuration"))

			// Verify GetConfig was called but WriteConfig was not
			Expect(mockConfig.GetGetCallCount()).To(Equal(1))
			Expect(mockConfig.GetWriteCallCount()).To(Equal(0))
		})

		It("should handle WriteConfig failure", func() {
			// Configure mock to fail on WriteConfig
			mockConfig.WithWriteConfigFailure()

			// Execute the action
			_, _, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write updated configuration"))

			// Verify both GetConfig and WriteConfig were called
			Expect(mockConfig.GetGetCallCount()).To(Equal(1))
			Expect(mockConfig.GetWriteCallCount()).To(Equal(1))
		})
	})
})

// Helper functions to access unexported fields for testing
func SetField(obj interface{}, fieldName string, value interface{}) error {
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	} else {
		return fmt.Errorf("object must be a pointer")
	}

	field := objValue.FieldByName(fieldName)
	if !field.IsValid() {
		return fmt.Errorf("field %s doesn't exist", fieldName)
	}

	if !field.CanSet() {
		return fmt.Errorf("field %s cannot be set", fieldName)
	}

	fieldValue := reflect.ValueOf(value)
	if field.Type() != fieldValue.Type() {
		return fmt.Errorf("field type mismatch: expected %v but got %v", field.Type(), fieldValue.Type())
	}

	field.Set(fieldValue)
	return nil
}

func GetField(obj interface{}, fieldName string) interface{} {
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}

	field := objValue.FieldByName(fieldName)
	if !field.IsValid() {
		return nil
	}

	return field.Interface()
}
