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
	"encoding/base64"
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"gopkg.in/yaml.v3"
)

// DeployStreamProcessor tests verify the behavior of the DeployStreamProcessorAction.
// This test suite ensures the action correctly parses the stream processor payload,
// validates it, and properly adds stream processors to the system.
var _ = Describe("DeployStreamProcessor", func() {
	// Variables used across tests
	var (
		action          *actions.DeployStreamProcessorAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		spName          string
		spModel         models.StreamProcessorModelRef
		spSources       models.StreamProcessorSourceMapping
		spMapping       models.StreamProcessorMapping
		spVariables     []models.StreamProcessorVariable
		spLocation      map[int]string
		encodedConfig   string
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
		spName = "test-stream-processor"
		spModel = models.StreamProcessorModelRef{
			Name:    "test-model",
			Version: "1.0.0",
		}
		spSources = models.StreamProcessorSourceMapping{
			"opcua": "umh.v1.{{ .enterprise }}.{{ .site }}.{{ .area }}.{{ .line }}.opcua",
			"mqtt":  "umh.v1.{{ .enterprise }}.{{ .site }}.{{ .area }}.{{ .line }}.mqtt",
		}
		spMapping = models.StreamProcessorMapping{
			"count":       "opcua",
			"temperature": "opcua.temperature",
			"pressure":    "mqtt.pressure",
			"motor": models.StreamProcessorMapping{
				"speed":   "motor_data.speed",
				"current": "motor_data.current",
			},
		}
		spVariables = []models.StreamProcessorVariable{
			{
				Label: "enterprise",
				Value: "test-enterprise",
			},
			{
				Label: "site",
				Value: "test-site",
			},
		}
		spLocation = map[int]string{
			0: "TestEnterprise",
			1: "TestSite",
		}

		// Create encoded config
		spConfig := models.StreamProcessorConfig{
			Sources: spSources,
			Mapping: spMapping,
		}
		configData, err := yaml.Marshal(spConfig)
		Expect(err).NotTo(HaveOccurred())
		encodedConfig = base64.StdEncoding.EncodeToString(configData)

		// Create initial config with no stream processors
		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
			},
			StreamProcessor: []config.StreamProcessorConfig{},
			Templates:       config.TemplatesConfig{},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)

		// Startup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.Tick()
		action = actions.NewDeployStreamProcessorAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

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
		It("should parse valid stream processor payload", func() {
			// Valid payload with stream processor configuration
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"templateInfo": map[string]interface{}{
					"variables": spVariables,
				},
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Name).To(Equal(spName))
			Expect(parsedPayload.Model).To(Equal(spModel))
			Expect(parsedPayload.Location).To(Equal(spLocation))
			Expect(parsedPayload.Config.Sources).To(Equal(spSources))
			Expect(parsedPayload.Config.Mapping).To(Equal(spMapping))
		})

		It("should parse payload without templateInfo", func() {
			// Payload without templateInfo (variables should be empty)
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Name).To(Equal(spName))
			Expect(parsedPayload.Model).To(Equal(spModel))
			Expect(parsedPayload.TemplateInfo).To(BeNil())
		})

		It("should return error for invalid payload format", func() {
			// Invalid payload (not a map)
			payload := "invalid-payload"

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
		})

		It("should return error for invalid base64 encoded config", func() {
			// Payload with invalid base64 encoded config
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": "invalid-base64!@#",
				"location":      spLocation,
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to decode stream processor config"))
		})

		It("should return error for invalid YAML config", func() {
			// Create invalid YAML and encode it
			invalidYAML := "invalid: yaml: content: ["
			invalidEncodedConfig := base64.StdEncoding.EncodeToString([]byte(invalidYAML))

			// Payload with invalid YAML config
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": invalidEncodedConfig,
				"location":      spLocation,
			}

			// Call Parse method
			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to unmarshal stream processor config"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation with valid stream processor configuration", func() {
			// First parse valid payload
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Then validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation with missing name", func() {
			// Payload with missing name field
			payload := map[string]interface{}{
				"name":          "",
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Name"))
		})

		It("should fail validation with missing model name", func() {
			// Payload with missing model name
			invalidModel := models.StreamProcessorModelRef{
				Name:    "",
				Version: "1.0.0",
			}
			payload := map[string]interface{}{
				"name":          spName,
				"model":         invalidModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Model.Name"))
		})

		It("should fail validation with missing model version", func() {
			// Payload with missing model version
			invalidModel := models.StreamProcessorModelRef{
				Name:    "test-model",
				Version: "",
			}
			payload := map[string]interface{}{
				"name":          spName,
				"model":         invalidModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Model.Version"))
		})

		It("should fail validation with invalid stream processor name", func() {
			// Payload with invalid name (contains special characters)
			payload := map[string]interface{}{
				"name":          "invalid name!@#",
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name can only contain letters"))
		})
	})

	Describe("Execute", func() {
		It("should deploy stream processor successfully", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"templateInfo": map[string]interface{}{
					"variables": spVariables,
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
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify AtomicAddStreamProcessor was called
			Expect(mockConfig.AtomicAddStreamProcessorCalled).To(BeTrue())

			// Verify the response contains the expected stream processor with UUID
			responseSP, ok := result.(models.StreamProcessor)
			Expect(ok).To(BeTrue(), "Result should be a StreamProcessor")

			// Verify the UUID was generated correctly
			expectedUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(spName))
			Expect(responseSP.UUID).NotTo(BeNil())
			Expect(*responseSP.UUID).To(Equal(expectedUUID))

			// Verify other fields match input
			Expect(responseSP.Name).To(Equal(spName))
			Expect(responseSP.Model).To(Equal(spModel))
			Expect(responseSP.Location).To(Equal(spLocation))
			Expect(responseSP.EncodedConfig).To(Equal(encodedConfig))

			// Verify expected configuration changes
			Expect(mockConfig.Config.StreamProcessor).To(HaveLen(1))
			addedSP := mockConfig.Config.StreamProcessor[0]
			Expect(addedSP.Name).To(Equal(spName))
			Expect(addedSP.StreamProcessorServiceConfig.Config.Model.Name).To(Equal(spModel.Name))
			Expect(addedSP.StreamProcessorServiceConfig.Config.Model.Version).To(Equal(spModel.Version))
		})

		It("should handle AtomicAddStreamProcessor failure", func() {
			// Set up mock to fail on AtomicAddStreamProcessor
			mockConfig.WithAtomicAddStreamProcessorError(errors.New("mock add stream processor failure"))

			// Parse valid payload
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to add stream processor: mock add stream processor failure"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})

		It("should handle duplicate stream processor name", func() {
			// Add a stream processor to the initial config
			existingSP := config.StreamProcessorConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name: spName, // Same name as test
				},
			}

			mockConfig.Config.StreamProcessor = []config.StreamProcessorConfig{existingSP}
			mockConfig.WithAtomicAddStreamProcessorError(errors.New("another stream processor with name \"" + spName + "\" already exists – choose a unique name"))

			// Parse valid payload
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail with duplicate name error
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("choose a unique name"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})

		It("should create stream processor config with correct template reference", func() {
			// Parse valid payload
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			_, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify the template reference is set correctly
			Expect(mockConfig.Config.StreamProcessor).To(HaveLen(1))
			addedSP := mockConfig.Config.StreamProcessor[0]
			Expect(addedSP.StreamProcessorServiceConfig.TemplateRef).To(Equal(spName))
		})

		It("should handle variables correctly", func() {
			// Parse payload with variables
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"templateInfo": map[string]interface{}{
					"variables": spVariables,
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			_, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify variables are stored correctly
			Expect(mockConfig.Config.StreamProcessor).To(HaveLen(1))
			addedSP := mockConfig.Config.StreamProcessor[0]
			Expect(addedSP.StreamProcessorServiceConfig.Variables.User).To(HaveKey("enterprise"))
			Expect(addedSP.StreamProcessorServiceConfig.Variables.User).To(HaveKey("site"))
			Expect(addedSP.StreamProcessorServiceConfig.Variables.User["enterprise"]).To(Equal("test-enterprise"))
			Expect(addedSP.StreamProcessorServiceConfig.Variables.User["site"]).To(Equal("test-site"))
		})
	})
})
