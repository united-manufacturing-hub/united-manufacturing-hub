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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// MockConfigManager implements the config.ConfigManager interface for testing
type MockConfigManager struct {
	config          config.FullConfig
	getShouldFail   bool
	writeShouldFail bool
	latestConfig    config.FullConfig
	mutex           sync.Mutex
	writeCallCount  int
	getCallCount    int
}

func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{
		config: config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
				Location: map[int]string{
					0: "Old Enterprise",
					1: "Old Site",
				},
			},
			Internal: config.InternalConfig{
				Services: []config.S6FSMConfig{},
				Benthos:  []config.BenthosConfig{},
				Nmap:     []config.NmapConfig{},
			},
		},
	}
}

func (m *MockConfigManager) GetConfig(_ context.Context, _ uint64) (config.FullConfig, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.getCallCount++

	if m.getShouldFail {
		return config.FullConfig{}, errors.New("mock GetConfig failure")
	}
	return m.config.Clone(), nil
}

func (m *MockConfigManager) WriteConfig(_ context.Context, cfg config.FullConfig) error {
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

func (m *MockConfigManager) WithGetConfigFailure() *MockConfigManager {
	m.getShouldFail = true
	return m
}

func (m *MockConfigManager) WithWriteConfigFailure() *MockConfigManager {
	m.writeShouldFail = true
	return m
}

func (m *MockConfigManager) GetWriteCallCount() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.writeCallCount
}

func (m *MockConfigManager) GetGetCallCount() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.getCallCount
}

var _ = Describe("EditInstance", func() {
	// Variables used across tests
	var (
		action          *actions.EditInstanceAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *MockConfigManager
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		mockConfig = NewMockConfigManager()

		// Create the action instance using the new constructor
		action = actions.NewEditInstanceAction(userEmail, actionUUID, instanceUUID, outboundChannel)
		// Inject our mock config manager
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
		It("should parse valid location data", func() {
			// Valid payload with complete location information
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"enterprise": "Test Enterprise",
					"site":       "Test Site",
					"area":       "Test Area",
					"line":       "Test Line",
					"workCell":   "Test WorkCell",
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Check if the location was properly parsed
			location := action.GetLocation()
			Expect(location).NotTo(BeNil())
		})

		It("should handle missing location data", func() {
			// Payload without location information
			payload := map[string]interface{}{}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Location should be nil
			location := action.GetLocation()
			Expect(location).To(BeNil())
		})

		It("should return error for invalid location format", func() {
			// Invalid payload with location as a string instead of a map
			payload := map[string]interface{}{
				"location": "Invalid Location",
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid location format"))
		})

		It("should return error for missing enterprise in location", func() {
			// Payload with location missing the required enterprise field
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"site": "Test Site",
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing or invalid enterprise"))
		})
	})

	Describe("Validate", func() {
		It("should validate with valid location data", func() {
			// Create a valid location and set it using the exported method
			location := &actions.EditInstanceLocation{
				Enterprise: "Test Enterprise",
			}

			action.SetLocation(location)

			// Validate
			err := action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate with nil location", func() {
			// Don't set any location (which results in nil location)
			action.SetLocation(nil)

			// Validate
			err := action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Execute", func() {
		It("should handle nil location gracefully", func() {
			// Parse with nil location
			payload := map[string]interface{}{}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("No changes were made"))
			Expect(metadata).To(BeNil())

			// We should have 2 messages in the channel (Confirmed and Executing)
			var messages []*models.UMHMessage
			for i := 0; i < 2; i++ {
				select {
				case msg := <-outboundChannel:
					messages = append(messages, msg)
				default:
					Fail("Expected more messages in the outbound channel")
				}
			}
			Expect(messages).To(HaveLen(2))

			// No calls to config manager should be made
			Expect(mockConfig.GetGetCallCount()).To(Equal(0))
			Expect(mockConfig.GetWriteCallCount()).To(Equal(0))
		})

		It("should update location successfully", func() {
			// Parse with valid location
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"enterprise": "New Enterprise",
					"site":       "New Site",
					"area":       "New Area",
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Successfully updated"))
			Expect(metadata).To(BeNil())

			// We should have 3 messages in the channel (Confirmed + Executing + Success)
			var messages []*models.UMHMessage
			for i := 0; i < 3; i++ {
				select {
				case msg := <-outboundChannel:
					messages = append(messages, msg)
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out waiting for message")
				}
			}
			Expect(messages).To(HaveLen(3))

			// Config manager should be called
			Expect(mockConfig.GetGetCallCount()).To(Equal(1))
			Expect(mockConfig.GetWriteCallCount()).To(Equal(1))

			// Check that config was updated correctly
			Expect(mockConfig.latestConfig.Agent.Location[0]).To(Equal("New Enterprise"))
			Expect(mockConfig.latestConfig.Agent.Location[1]).To(Equal("New Site"))
			Expect(mockConfig.latestConfig.Agent.Location[2]).To(Equal("New Area"))
		})

		It("should handle GetConfig failure", func() {
			// Set up mock to fail on GetConfig
			mockConfig.WithGetConfigFailure()

			// Parse with valid location
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"enterprise": "New Enterprise",
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update instance location"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// We should have 3 messages in the channel (Confirmed + Executing + Failure)
			var messages []*models.UMHMessage
			for i := 0; i < 3; i++ {
				select {
				case msg := <-outboundChannel:
					messages = append(messages, msg)
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out waiting for message")
				}
			}
			Expect(messages).To(HaveLen(3))
		})

		It("should handle WriteConfig failure", func() {
			// Set up mock to fail on WriteConfig
			mockConfig.WithWriteConfigFailure()

			// Parse with valid location
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"enterprise": "New Enterprise",
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update instance location"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// We should have 3 messages in the channel (Confirmed + Executing + Failure)
			var messages []*models.UMHMessage
			for i := 0; i < 3; i++ {
				select {
				case msg := <-outboundChannel:
					messages = append(messages, msg)
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out waiting for message")
				}
			}
			Expect(messages).To(HaveLen(3))

			// Config manager should be called
			Expect(mockConfig.GetGetCallCount()).To(Equal(1))
			Expect(mockConfig.GetWriteCallCount()).To(Equal(1))
		})
	})
})
