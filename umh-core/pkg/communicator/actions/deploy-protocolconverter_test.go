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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/configmanager"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// DeployProtocolConverter tests verify the behavior of the DeployProtocolConverterAction.
// This test suite ensures the action correctly parses the protocol converter payload,
// validates it, and properly adds protocol converters to the system.
var _ = Describe("DeployProtocolConverter", func() {
	// Variables used across tests
	var (
		action          *actions.DeployProtocolConverterAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *configmanager.MockConfigManager
		pcName          string
		pcIP            string
		pcPort          uint32
		pcLocation      map[int]string
		stateMocker     *actions.StateMocker
		messages        []*models.UMHMessage
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		pcName = "test-protocol-converter"
		pcIP = "192.168.1.100"
		pcPort = uint32(502)
		pcLocation = map[int]string{
			0: "TestEnterprise",
			1: "TestSite",
		}

		// Create initial config with no protocol converters
		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
			},
			ProtocolConverter: []config.ProtocolConverterConfig{},
			Templates:         config.TemplatesConfig{},
		}

		mockConfig = configmanager.NewMockConfigManager().WithConfig(initialConfig)

		// Startup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.Tick()
		action = actions.NewDeployProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

		go actions.ConsumeOutboundMessages(outboundChannel, &messages, true)
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
		It("should parse valid protocol converter payload", func() {
			// Valid payload with protocol converter configuration
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   pcIP,
					"port": pcPort,
				},
				"location": pcLocation,
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Name).To(Equal(pcName))
			Expect(parsedPayload.Connection.IP).To(Equal(pcIP))
			Expect(parsedPayload.Connection.Port).To(Equal(pcPort))
			Expect(parsedPayload.Location).To(Equal(pcLocation))
		})

		It("should return error for missing name", func() {
			// Payload with missing name field
			payload := map[string]interface{}{
				"connection": map[string]interface{}{
					"ip":   pcIP,
					"port": pcPort,
				},
				"location": pcLocation,
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Name"))
		})

		It("should return error for missing connection IP", func() {
			// Payload with missing connection IP field
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"port": pcPort,
				},
				"location": pcLocation,
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Connection.IP"))
		})

		It("should return error for missing connection port", func() {
			// Payload with missing connection port field
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip": pcIP,
				},
				"location": pcLocation,
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Connection.Port"))
		})

		It("should return error for invalid payload format", func() {
			// Invalid payload (not a map)
			payload := "invalid-payload"

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation with valid protocol converter configuration", func() {
			// First parse valid payload
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   pcIP,
					"port": pcPort,
				},
				"location": pcLocation,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Then validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation with missing name", func() {
			// Test validation with empty name (validation mirrors parse logic)
			payload := map[string]interface{}{
				"name": "",
				"connection": map[string]interface{}{
					"ip":   pcIP,
					"port": pcPort,
				},
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Name"))
		})
	})

	Describe("Execute", func() {
		It("should deploy protocol converter successfully", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   pcIP,
					"port": pcPort,
				},
				"location": pcLocation,
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
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicAddProtocolConverter was called
			Expect(mockConfig.AtomicAddProtocolConverterCalled).To(BeTrue())

			// Verify the response contains the expected protocol converter with UUID
			responsePC, ok := result.(models.ProtocolConverter)
			Expect(ok).To(BeTrue(), "Result should be a ProtocolConverter")

			// Verify the UUID was generated correctly
			expectedUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(pcName)
			Expect(responsePC.UUID).NotTo(BeNil())
			Expect(*responsePC.UUID).To(Equal(expectedUUID))

			// Verify other fields match input
			Expect(responsePC.Name).To(Equal(pcName))
			Expect(responsePC.Connection.IP).To(Equal(pcIP))
			Expect(responsePC.Connection.Port).To(Equal(pcPort))
			Expect(responsePC.Location).To(Equal(pcLocation))

			// Verify optional fields are nil as expected for deployment
			Expect(responsePC.ReadDFC).To(BeNil())
			Expect(responsePC.WriteDFC).To(BeNil())
			Expect(responsePC.TemplateInfo).To(BeNil())

			// Verify expected configuration changes
			Expect(mockConfig.Config.ProtocolConverter).To(HaveLen(1))
			addedPC := mockConfig.Config.ProtocolConverter[0]
			Expect(addedPC.Name).To(Equal(pcName))

		})

		It("should handle AtomicAddProtocolConverter failure", func() {
			// Set up mock to fail on AtomicAddProtocolConverter
			mockConfig.WithAtomicAddProtocolConverterError(errors.New("mock add protocol converter failure"))

			// Parse valid payload
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   pcIP,
					"port": pcPort,
				},
				"location": pcLocation,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to add protocol converter: mock add protocol converter failure"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

		})

		It("should handle duplicate protocol converter name", func() {
			// Add a protocol converter to the initial config
			existingPC := config.ProtocolConverterConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name: pcName, // Same name as test
				},
			}

			mockConfig.Config.ProtocolConverter = []config.ProtocolConverterConfig{existingPC}
			mockConfig.WithAtomicAddProtocolConverterError(errors.New("another protocol converter with name \"" + pcName + "\" already exists – choose a unique name"))

			// Parse valid payload
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   pcIP,
					"port": pcPort,
				},
				"location": pcLocation,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail with duplicate name error
			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("choose a unique name"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})
	})
})
