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

// Helper function for get datamodel action
func createGetDataModelPayload(name string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
	}
}

var _ = Describe("GetDataModelAction", func() {
	var (
		action             *actions.GetDataModelAction
		mockConfigManager  *config.MockConfigManager
		outboundChannel    chan *models.UMHMessage
		userEmail          string
		actionUUID         uuid.UUID
		instanceUUID       uuid.UUID
		existingDataModel  config.DataModelsConfig
		existingFullConfig config.FullConfig
	)

	BeforeEach(func() {
		mockConfigManager = config.NewMockConfigManager()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()

		// Create an existing data model with multiple versions
		existingDataModel = config.DataModelsConfig{
			Name: "test-model",
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"field1": {
							PayloadShape: "timeseries-string",
						},
					},
				},
				"v2": {
					Structure: map[string]config.Field{
						"field1": {
							PayloadShape: "timeseries-string",
						},
						"field2": {
							PayloadShape: "timeseries-number",
						},
					},
				},
			},
		}

		existingFullConfig = config.FullConfig{
			DataModels: []config.DataModelsConfig{existingDataModel},
		}

		mockConfigManager = mockConfigManager.WithConfig(existingFullConfig)

		action = actions.NewGetDataModelAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfigManager)
	})

	Describe("NewGetDataModelAction", func() {
		It("should create a new action with correct parameters", func() {
			Expect(action).ToNot(BeNil())
		})
	})

	Describe("Parse", func() {
		Context("with valid payload", func() {
			It("should parse successfully", func() {
				payload := createGetDataModelPayload("test-model")
				err := action.Parse(payload)
				Expect(err).ToNot(HaveOccurred())

				parsedPayload := action.GetParsedPayload()
				Expect(parsedPayload.Name).To(Equal("test-model"))
			})
		})

		Context("with invalid payload", func() {
			It("should return error for non-map payload", func() {
				err := action.Parse("invalid")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not assert ActionPayload"))
			})

			It("should return error for empty payload", func() {
				err := action.Parse(map[string]interface{}{})
				Expect(err).ToNot(HaveOccurred()) // Parse doesn't validate, only extracts

				// But validation should fail
				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("missing required field Name"))
			})
		})
	})

	Describe("Validate", func() {
		Context("with valid payload", func() {
			BeforeEach(func() {
				payload := createGetDataModelPayload("test-model")
				err := action.Parse(payload)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should validate successfully", func() {
				err := action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with missing name", func() {
			BeforeEach(func() {
				payload := createGetDataModelPayload("")
				err := action.Parse(payload)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return validation error", func() {
				err := action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("missing required field Name"))
			})
		})
	})

	Describe("Execute", func() {
		Context("with existing data model", func() {
			BeforeEach(func() {
				payload := createGetDataModelPayload("test-model")
				err := action.Parse(payload)
				Expect(err).ToNot(HaveOccurred())
				err = action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should retrieve data model successfully", func() {
				result, metadata, err := action.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(metadata).To(BeNil())

				response, ok := result.(models.GetDataModelResponse)
				Expect(ok).To(BeTrue())
				Expect(response.Name).To(Equal("test-model"))
				Expect(response.Versions).To(HaveLen(2))

				// Check version 1
				v1, exists := response.Versions["v1"]
				Expect(exists).To(BeTrue())
				Expect(v1.EncodedStructure).ToNot(BeEmpty())

				// Decode and verify structure for v1
				decodedV1, err := base64.StdEncoding.DecodeString(v1.EncodedStructure)
				Expect(err).ToNot(HaveOccurred())
				var structureV1 map[string]models.Field
				err = yaml.Unmarshal(decodedV1, &structureV1)
				Expect(err).ToNot(HaveOccurred())
				Expect(structureV1).To(HaveKey("field1"))
				Expect(structureV1["field1"].PayloadShape).To(Equal("timeseries-string"))

				// Check version 2
				v2, exists := response.Versions["v2"]
				Expect(exists).To(BeTrue())
				Expect(v2.EncodedStructure).ToNot(BeEmpty())

				// Decode and verify structure for v2
				decodedV2, err := base64.StdEncoding.DecodeString(v2.EncodedStructure)
				Expect(err).ToNot(HaveOccurred())
				var structureV2 map[string]models.Field
				err = yaml.Unmarshal(decodedV2, &structureV2)
				Expect(err).ToNot(HaveOccurred())
				Expect(structureV2).To(HaveKey("field1"))
				Expect(structureV2).To(HaveKey("field2"))
			})

			It("should send appropriate action replies", func() {
				_, _, err := action.Execute()
				Expect(err).ToNot(HaveOccurred())

				// Should have received at least 2 messages (ActionConfirmed and ActionExecuting)
				Eventually(outboundChannel).Should(Receive())
				Eventually(outboundChannel).Should(Receive())
			})
		})

		Context("with non-existing data model", func() {
			BeforeEach(func() {
				payload := createGetDataModelPayload("non-existing-model")
				err := action.Parse(payload)
				Expect(err).ToNot(HaveOccurred())
				err = action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return error for non-existing data model", func() {
				_, _, err := action.Execute()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Data model with name \"non-existing-model\" not found"))

				// Should have received failure message
				Eventually(outboundChannel).Should(Receive())
				Eventually(outboundChannel).Should(Receive())
				Eventually(outboundChannel).Should(Receive()) // Failure message
			})
		})

		Context("with config manager error", func() {
			BeforeEach(func() {
				mockConfigManager = config.NewMockConfigManager().WithConfigError(errors.New("config error"))
				action = actions.NewGetDataModelAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfigManager)
				payload := createGetDataModelPayload("test-model")
				err := action.Parse(payload)
				Expect(err).ToNot(HaveOccurred())
				err = action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return error when config manager fails", func() {
				_, _, err := action.Execute()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Failed to get configuration"))

				// Should have received failure message
				Eventually(outboundChannel).Should(Receive())
				Eventually(outboundChannel).Should(Receive())
				Eventually(outboundChannel).Should(Receive()) // Failure message
			})
		})

		Context("with complex nested structure", func() {
			BeforeEach(func() {
				// Create a data model with nested subfields
				complexDataModel := config.DataModelsConfig{
					Name: "complex-model",
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"level1": {
									Subfields: map[string]config.Field{
										"level2": {
											PayloadShape: "timeseries-number",
											Subfields: map[string]config.Field{
												"level3": {
													PayloadShape: "timeseries-string",
												},
											},
										},
									},
								},
								"simple": {
									PayloadShape: "timeseries-string",
								},
							},
						},
					},
				}

				complexConfig := config.FullConfig{
					DataModels: []config.DataModelsConfig{complexDataModel},
				}
				mockConfigManager = mockConfigManager.WithConfig(complexConfig)
				action = actions.NewGetDataModelAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfigManager)

				payload := createGetDataModelPayload("complex-model")
				err := action.Parse(payload)
				Expect(err).ToNot(HaveOccurred())
				err = action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle nested structures correctly", func() {
				result, _, err := action.Execute()
				Expect(err).ToNot(HaveOccurred())

				response, ok := result.(models.GetDataModelResponse)
				Expect(ok).To(BeTrue())

				v1, exists := response.Versions["v1"]
				Expect(exists).To(BeTrue())

				// Decode and verify nested structure
				decodedV1, err := base64.StdEncoding.DecodeString(v1.EncodedStructure)
				Expect(err).ToNot(HaveOccurred())
				var structure map[string]models.Field
				err = yaml.Unmarshal(decodedV1, &structure)
				Expect(err).ToNot(HaveOccurred())

				Expect(structure).To(HaveKey("level1"))
				Expect(structure).To(HaveKey("simple"))
				Expect(structure["level1"].Subfields).To(HaveKey("level2"))
				Expect(structure["level1"].Subfields["level2"].Subfields).To(HaveKey("level3"))
			})
		})
	})
})
