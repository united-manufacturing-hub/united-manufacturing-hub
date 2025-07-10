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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"gopkg.in/yaml.v3"
)

var _ = Describe("EditStreamProcessor", func() {
	// Variables used across tests
	var (
		action          *actions.EditStreamProcessorAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		stateMocker     *actions.StateMocker
		messages        []*models.UMHMessage
		spName          string
		spUUID          uuid.UUID
		spModel         models.StreamProcessorModelRef
		spSources       map[string]string
		spMapping       models.StreamProcessorMapping
		spVariables     []models.StreamProcessorVariable
		spLocation      map[int]string
		encodedConfig   string
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		spName = "test-stream-processor"
		spUUID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(spName))
		spModel = models.StreamProcessorModelRef{
			Name:    "test-model",
			Version: "1.0.0",
		}
		spSources = map[string]string{
			"opcua": "umh.v1.{{ .enterprise }}.{{ .site }}.{{ .area }}.{{ .line }}.opcua",
			"mqtt":  "umh.v1.{{ .enterprise }}.{{ .site }}.{{ .area }}.{{ .line }}.mqtt",
		}
		spMapping = models.StreamProcessorMapping{
			Source:    "opcua",
			Transform: "count",
			Subfields: map[string]models.StreamProcessorMapping{
				"temperature": {
					Source:    "opcua.temperature",
					Transform: "average",
				},
				"pressure": {
					Source:    "mqtt.pressure",
					Transform: "latest",
				},
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

		// Create initial config with existing stream processor
		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
				Location: map[int]string{
					0: "test-enterprise",
					1: "test-site",
					2: "test-area",
					3: "test-line",
				},
			},
			StreamProcessor: []config.StreamProcessorConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            spName,
						DesiredFSMState: "active",
					},
					StreamProcessorServiceConfig: streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
						Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
							Model: streamprocessorserviceconfig.ModelRef{
								Name:    "old-model",
								Version: "0.9.0",
							},
							Sources: streamprocessorserviceconfig.SourceMapping{
								"opcua": "umh.v1.old-enterprise.old-site.old-area.old-line.opcua",
							},
							Mapping: map[string]interface{}{
								"source":    "opcua",
								"transform": "sum",
							},
						},
						Variables: variables.VariableBundle{
							User: map[string]interface{}{
								"enterprise": "old-enterprise",
								"site":       "old-site",
							},
						},
						Location: map[string]string{
							"0": "test-enterprise",
							"1": "test-site",
							"2": "test-area",
							"3": "test-line",
						},
						TemplateRef: spName, // Self-referencing template (root)
					},
				},
			},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)

		// Setup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.Tick()

		action = actions.NewEditStreamProcessorAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

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
		It("should parse valid stream processor edit payload", func() {
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
				"templateInfo": map[string]interface{}{
					"variables": spVariables,
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetStreamProcessorUUID()).To(Equal(spUUID))

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Name).To(Equal(spName))
			Expect(parsedPayload.Model).To(Equal(spModel))
			Expect(parsedPayload.Location).To(Equal(spLocation))
			Expect(*parsedPayload.UUID).To(Equal(spUUID))
		})

		It("should parse payload without templateInfo", func() {
			// Payload without templateInfo (variables should be empty)
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetStreamProcessorUUID()).To(Equal(spUUID))

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Name).To(Equal(spName))
			Expect(parsedPayload.Model).To(Equal(spModel))
		})

		It("should return error for missing UUID", func() {
			// Payload without UUID
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
			}

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field UUID"))
		})

		It("should return error for invalid payload format", func() {
			// Invalid payload (not a map)
			payload := "invalid-payload"

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse stream processor payload"))
		})

		It("should return error for invalid base64 encoded config", func() {
			// Payload with invalid base64 encoded config
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": "invalid-base64!@#",
				"location":      spLocation,
				"uuid":          spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to decode stream processor config"))
		})

		It("should return error for invalid YAML config", func() {
			// Create invalid YAML and encode it
			invalidYAML := "invalid: yaml: content: ["
			invalidEncodedConfig := base64.StdEncoding.EncodeToString([]byte(invalidYAML))

			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": invalidEncodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
			}

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
				"uuid":          spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Then validate
			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail validation with invalid UUID", func() {
			// Payload with nil UUID (this should be caught in Parse, but test validation too)
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          uuid.Nil.String(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing or invalid stream processor UUID"))
		})

		It("should fail validation with missing name", func() {
			// Payload with missing name field
			payload := map[string]interface{}{
				"name":          "",
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name can only contain letters"))
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
				"uuid":          spUUID.String(),
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
				"uuid":          spUUID.String(),
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
				"uuid":          spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name can only contain letters"))
		})
	})

	Describe("Execute", func() {
		It("should edit stream processor successfully", func() {
			// Setup - parse valid payload first
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
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

			// Verify AtomicEditStreamProcessor was called
			Expect(mockConfig.AtomicEditStreamProcessorCalled).To(BeTrue())

			// Verify the response contains the expected UUID
			responseMap, ok := result.(map[string]any)
			Expect(ok).To(BeTrue(), "Result should be a map")
			Expect(responseMap).To(HaveKey("uuid"))

			// Verify the UUID was generated correctly
			expectedUUID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(spName))
			Expect(responseMap["uuid"]).To(Equal(expectedUUID))

			// Verify expected configuration changes
			Expect(mockConfig.Config.StreamProcessor).To(HaveLen(1))
			editedSP := mockConfig.Config.StreamProcessor[0]
			Expect(editedSP.Name).To(Equal(spName))
			Expect(editedSP.StreamProcessorServiceConfig.Config.Model.Name).To(Equal(spModel.Name))
			Expect(editedSP.StreamProcessorServiceConfig.Config.Model.Version).To(Equal(spModel.Version))
		})

		It("should handle AtomicEditStreamProcessor failure", func() {
			// Set up mock to fail on AtomicEditStreamProcessor
			mockConfig.WithAtomicEditStreamProcessorError(errors.New("mock edit stream processor failure"))

			// Parse valid payload
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to persist configuration changes"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})

		It("should handle stream processor not found", func() {
			// Use a different UUID that doesn't exist
			nonExistentUUID := uuid.New()
			payload := map[string]interface{}{
				"name":          "non-existent-processor",
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          nonExistentUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("stream processor with UUID"))
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})

		It("should handle variables correctly", func() {
			// Parse payload with variables
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
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
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify variables are updated correctly
			Expect(mockConfig.Config.StreamProcessor).To(HaveLen(1))
			editedSP := mockConfig.Config.StreamProcessor[0]
			Expect(editedSP.StreamProcessorServiceConfig.Variables.User).To(HaveKey("enterprise"))
			Expect(editedSP.StreamProcessorServiceConfig.Variables.User).To(HaveKey("site"))
			Expect(editedSP.StreamProcessorServiceConfig.Variables.User["enterprise"]).To(Equal("test-enterprise"))
			Expect(editedSP.StreamProcessorServiceConfig.Variables.User["site"]).To(Equal("test-site"))

			// Verify the response contains the expected UUID
			responseMap, ok := result.(map[string]any)
			Expect(ok).To(BeTrue(), "Result should be a map")
			Expect(responseMap).To(HaveKey("uuid"))
		})

		It("should handle GetConfig failure", func() {
			// Set up mock to fail on GetConfig
			mockConfig.WithConfigError(errors.New("mock get config failure"))

			// Parse valid payload
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Failed to apply configuration mutation"))
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()
		})

		It("should update sources and mapping correctly", func() {
			// Parse payload with new sources and mapping
			payload := map[string]interface{}{
				"name":          spName,
				"model":         spModel,
				"encodedConfig": encodedConfig,
				"location":      spLocation,
				"uuid":          spUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify sources and mapping are updated correctly
			Expect(mockConfig.Config.StreamProcessor).To(HaveLen(1))
			editedSP := mockConfig.Config.StreamProcessor[0]

			// Check sources
			Expect(editedSP.StreamProcessorServiceConfig.Config.Sources).To(HaveKey("opcua"))
			Expect(editedSP.StreamProcessorServiceConfig.Config.Sources).To(HaveKey("mqtt"))

			// Check mapping
			Expect(editedSP.StreamProcessorServiceConfig.Config.Mapping).To(HaveKey("source"))
			Expect(editedSP.StreamProcessorServiceConfig.Config.Mapping).To(HaveKey("transform"))
			Expect(editedSP.StreamProcessorServiceConfig.Config.Mapping["source"]).To(Equal("opcua"))
			Expect(editedSP.StreamProcessorServiceConfig.Config.Mapping["transform"]).To(Equal("count"))

			// Verify the response contains the expected UUID
			responseMap, ok := result.(map[string]any)
			Expect(ok).To(BeTrue(), "Result should be a map")
			Expect(responseMap).To(HaveKey("uuid"))
		})
	})
})
