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
	"maps"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// EditInstance tests verify the behavior of the EditInstanceAction.
// This test suite ensures that the action correctly handles location updates,
// validates input, and properly reports errors in various scenarios.
var _ = Describe("EditInstance", func() {
	// Variables used across tests
	var (
		action          *actions.EditInstanceAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		snapshotManager *fsm.SnapshotManager
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking

		// Create initial config with existing location data
		initialConfig := config.FullConfig{
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
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)
		snapshotManager = fsm.NewSnapshotManager()
		action = actions.NewEditInstanceAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, snapshotManager)
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
			// Valid payload with complete location information using generic format
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"0": "Test Enterprise",
					"1": "Test Site",
					"2": "Test Area",
					"3": "Test Line",
					"4": "Test WorkCell",
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Check if the location was properly parsed
			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Location).NotTo(BeNil())
			Expect(parsedPayload.Location[0]).To(Equal("Test Enterprise"))
			Expect(parsedPayload.Location[1]).To(Equal("Test Site"))
			Expect(parsedPayload.Location[2]).To(Equal("Test Area"))
			Expect(parsedPayload.Location[3]).To(Equal("Test Line"))
			Expect(parsedPayload.Location[4]).To(Equal("Test WorkCell"))
		})

		It("should parse but fail validation for missing location data", func() {
			// Payload without location information - parsing succeeds but validation fails
			payload := map[string]interface{}{}

			// Parse should succeed
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// But validation should fail
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("location is required"))
		})

		It("should return error for invalid location format", func() {
			// Invalid payload with location as a string instead of a map
			payload := map[string]interface{}{
				"location": "Invalid Location",
			}

			// Parse should fail
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
		})

		It("should parse successfully but fail validation for missing enterprise", func() {
			// Payload with location missing the required enterprise field (level 0)
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"1": "Test Site",
				},
			}

			// Parse should succeed
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// But validation should fail
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("enterprise (level 0) is required"))
		})

		It("should return error for invalid location key", func() {
			// Payload with invalid location key (non-integer)
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"enterprise": "Test Enterprise", // Should be "0" instead
				},
			}

			// Parse should fail
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
		})

		It("should parse successfully but fail validation for empty enterprise", func() {
			// Payload with empty location value for enterprise
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"0": "",
				},
			}

			// Parse should succeed
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// But validation should fail
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("enterprise (level 0) is required"))
		})

		It("should return error for non-string location value", func() {
			// Payload with non-string location value
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"0": 123,
				},
			}

			// Parse should fail
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
		})
	})

	Describe("Validate", func() {
		It("should validate with valid location data", func() {
			// Valid payload
			payload := models.EditInstanceLocationModel{
				Location: map[int]string{
					0: "Test Enterprise",
				},
			}

			action.SetParsedPayload(payload)

			err := action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for location without enterprise", func() {
			// Create a payload without enterprise (level 0)
			payload := models.EditInstanceLocationModel{
				Location: map[int]string{
					1: "Test Site",
				},
			}

			action.SetParsedPayload(payload)

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("enterprise (level 0) is required"))
		})

		It("should return error for location with empty enterprise", func() {
			// Create a payload with empty enterprise (level 0)
			payload := models.EditInstanceLocationModel{
				Location: map[int]string{
					0: "",
				},
			}

			action.SetParsedPayload(payload)

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("enterprise (level 0) is required"))
		})

		It("should return error for empty location map", func() {
			// Create a payload with empty location map
			payload := models.EditInstanceLocationModel{
				Location: map[int]string{},
			}

			action.SetParsedPayload(payload)

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("location is required"))
		})
	})

	Describe("Execute", func() {
		It("should fail validation for missing location", func() {
			// Payload with missing location - parsing succeeds but validation fails
			payload := map[string]interface{}{}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("location is required"))
		})

		It("should update location successfully", func() {
			// Valid payload
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"0": "New Enterprise",
					"1": "New Site",
					"2": "New Area",
				},
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Reset tracking
			mockConfig.ResetCalls()

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Successfully updated"))
			Expect(metadata).To(BeNil())

			// Verify correct message sequence - note that success is not sent by Execute
			var messages []*models.UMHMessage
			for range 2 {
				select {
				case msg := <-outboundChannel:
					messages = append(messages, msg)
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out waiting for message")
				}
			}
			Expect(messages).To(HaveLen(2))

			// Verify GetConfig was called
			Expect(mockConfig.IsGetConfigCalled()).To(BeTrue())

			// Check that config was updated correctly
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			updatedConfig, _ := mockConfig.GetConfig(ctx, 0)
			Expect(updatedConfig.Agent.Location[0]).To(Equal("New Enterprise"))
			Expect(updatedConfig.Agent.Location[1]).To(Equal("New Site"))
			Expect(updatedConfig.Agent.Location[2]).To(Equal("New Area"))
		})

		It("should handle GetConfig failure", func() {
			// Set up mock to fail on GetConfig
			mockConfig.WithConfigError(errors.New("mock GetConfig failure"))

			// Valid payload
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"0": "New Enterprise",
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

			// Verify all expected messages: Confirmed + Executing + Failure
			var messages []*models.UMHMessage
			for range 3 {
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
			// First configure mock to return success on GetConfig
			mockConfig.WithConfigError(nil)

			// Then create a custom mock to fail on WriteConfig
			customMock := &writeFailingMockConfigManager{
				mockConfigManager: mockConfig,
			}

			// Create new action with our custom mock
			action = actions.NewEditInstanceAction(userEmail, actionUUID, instanceUUID, outboundChannel, customMock, snapshotManager)

			// Valid payload
			payload := map[string]interface{}{
				"location": map[string]interface{}{
					"0": "New Enterprise",
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

			// Verify all expected messages: Confirmed + Executing + Failure
			var messages []*models.UMHMessage
			for range 3 {
				select {
				case msg := <-outboundChannel:
					messages = append(messages, msg)
				case <-time.After(100 * time.Millisecond):
					Fail("Timed out waiting for message")
				}
			}
			Expect(messages).To(HaveLen(3))
		})
	})
})

// writeFailingMockConfigManager is a custom mock that specifically tests the
// failure case of writing config changes back to storage.
//
// It wraps the standard MockConfigManager but forces writeConfig calls to fail,
// allowing tests to verify proper error handling when persistence operations fail.
type writeFailingMockConfigManager struct {
	mockConfigManager *config.MockConfigManager
}

// GetFileSystemService is never called in the mock but only here to implement the ConfigManager interface.
func (w *writeFailingMockConfigManager) GetFileSystemService() filesystem.Service {
	return nil
}

// GetConfig passes through to the underlying mock implementation.
func (w *writeFailingMockConfigManager) GetConfig(ctx context.Context, tick uint64) (config.FullConfig, error) {
	return w.mockConfigManager.GetConfig(ctx, tick)
}

// writeConfig always returns an error to simulate write failures.
func (w *writeFailingMockConfigManager) writeConfig(ctx context.Context, config config.FullConfig) error {
	return errors.New("mock WriteConfig failure")
}

// AtomicSetLocation implements the location update operation but forces the write to fail.
func (w *writeFailingMockConfigManager) AtomicSetLocation(ctx context.Context, location map[int]string) error {
	// Get the current config
	config, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// Update location using the generic format
	config.Agent.Location = make(map[int]string)
	maps.Copy(config.Agent.Location, location)

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, config); err != nil {
		return err
	}

	return nil
}

// AtomicAddDataflowcomponent implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicAddDataflowcomponent(ctx context.Context, dfc config.DataFlowComponentConfig) error {
	// Get the current config
	config, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not append anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, config); err != nil {
		return err
	}

	return nil
}

// AtomicDeleteDataflowcomponent implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicDeleteDataflowcomponent(ctx context.Context, componentUUID uuid.UUID) error {
	// Get the current config
	config, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not delete anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, config); err != nil {
		return err
	}

	return nil
}

// AtomicEditDataflowcomponent implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicEditDataflowcomponent(ctx context.Context, componentUUID uuid.UUID, dfc config.DataFlowComponentConfig) (config.DataFlowComponentConfig, error) {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return config.DataFlowComponentConfig{}, err
	}

	// do not edit anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return config.DataFlowComponentConfig{}, err
	}

	return config.DataFlowComponentConfig{}, nil
}

// AtomicAddProtocolConverter implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicAddProtocolConverter(ctx context.Context, pc config.ProtocolConverterConfig) error {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not add anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return err
	}

	return nil
}

// AtomicEditProtocolConverter implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicEditProtocolConverter(ctx context.Context, componentUUID uuid.UUID, pc config.ProtocolConverterConfig) (config.ProtocolConverterConfig, error) {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return config.ProtocolConverterConfig{}, err
	}

	// do not edit anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return config.ProtocolConverterConfig{}, err
	}

	return config.ProtocolConverterConfig{}, nil
}

// AtomicDeleteProtocolConverter implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicDeleteProtocolConverter(ctx context.Context, componentUUID uuid.UUID) error {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not delete anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return err
	}

	return nil
}

// GetConfigAsString implements the ConfigManager interface.
func (w *writeFailingMockConfigManager) GetConfigAsString(ctx context.Context) (string, error) {
	return w.mockConfigManager.GetConfigAsString(ctx)
}

// GetCacheModTimeWithoutUpdate returns the modification time without updating the cache.
func (w *writeFailingMockConfigManager) GetCacheModTimeWithoutUpdate() time.Time {
	return w.mockConfigManager.GetCacheModTimeWithoutUpdate()
}

// UpdateAndGetCacheModTime updates the cache and returns the modification time.
func (w *writeFailingMockConfigManager) UpdateAndGetCacheModTime(ctx context.Context) (time.Time, error) {
	return w.mockConfigManager.UpdateAndGetCacheModTime(ctx)
}

// WriteYAMLConfigFromString implements the ConfigManager interface.
func (w *writeFailingMockConfigManager) WriteYAMLConfigFromString(ctx context.Context, config string, expectedModTime string) error {
	return w.mockConfigManager.WriteYAMLConfigFromString(ctx, config, expectedModTime)
}

// AtomicAddDataModel implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicAddDataModel(ctx context.Context, name string, dmVersion config.DataModelVersion, description string) error {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not add anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return err
	}

	return nil
}

// AtomicEditDataModel implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicEditDataModel(ctx context.Context, name string, dmVersion config.DataModelVersion, description string) error {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not edit anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return err
	}

	return nil
}

// AtomicDeleteDataModel implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicDeleteDataModel(ctx context.Context, name string) error {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not delete anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return err
	}

	return nil
}

// AtomicAddDataContract implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicAddDataContract(ctx context.Context, dataContract config.DataContractsConfig) error {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not add anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return err
	}

	return nil
}

// AtomicAddStreamProcessor implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicAddStreamProcessor(ctx context.Context, sp config.StreamProcessorConfig) error {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not add anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return err
	}

	return nil
}

// AtomicEditStreamProcessor implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicEditStreamProcessor(ctx context.Context, sp config.StreamProcessorConfig) (config.StreamProcessorConfig, error) {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return config.StreamProcessorConfig{}, err
	}

	// do not edit anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return config.StreamProcessorConfig{}, err
	}

	return config.StreamProcessorConfig{}, nil
}

// AtomicDeleteStreamProcessor implements the required interface method but ensures the write fails.
func (w *writeFailingMockConfigManager) AtomicDeleteStreamProcessor(ctx context.Context, name string) error {
	// Get the current config
	configData, err := w.GetConfig(ctx, 0)
	if err != nil {
		return err
	}

	// do not delete anything

	// Write config (will fail with this mock)
	if err := w.writeConfig(ctx, configData); err != nil {
		return err
	}

	return nil
}
