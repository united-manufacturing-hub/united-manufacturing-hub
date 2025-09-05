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

// Helper function for edit datamodel action that needs base64 encoding.
func structToEncodedMapForEdit(v interface{}) map[string]interface{} {
	payload := v.(models.EditDataModelPayload)

	// Marshal the DataModelVersion to YAML
	yamlData, err := yaml.Marshal(payload.Structure)
	Expect(err).ToNot(HaveOccurred())

	// Base64 encode the YAML
	encodedStructure := base64.StdEncoding.EncodeToString(yamlData)

	return map[string]interface{}{
		"encodedStructure": encodedStructure,
		"description":      payload.Description,
		"name":             payload.Name,
	}
}

var _ = Describe("EditDataModelAction", func() {
	var (
		action          *actions.EditDataModelAction
		mockConfigMgr   *config.MockConfigManager
		outboundChannel chan *models.UMHMessage
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
	)

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		mockConfigMgr = config.NewMockConfigManager()
		outboundChannel = make(chan *models.UMHMessage, 10)

		action = actions.NewEditDataModelAction(
			userEmail,
			actionUUID,
			instanceUUID,
			outboundChannel,
			mockConfigMgr,
		)
	})

	AfterEach(func() {
		close(outboundChannel)
	})

	Describe("NewEditDataModelAction", func() {
		It("should create a new action with correct fields", func() {
			Expect(action).ToNot(BeNil())
			// Note: getUserEmail() and getUuid() are private methods, so we can't test them directly
			// We test their functionality indirectly through Execute() which uses them
		})
	})

	Describe("Parse", func() {
		Context("with valid payload", func() {
			It("should parse successfully", func() {
				payload := models.EditDataModelPayload{
					Name:        "test-model",
					Description: "Updated test data model",
					Structure: map[string]models.Field{
						"field1": {
							PayloadShape: "timeseries-string",
						},
						"newField": {
							PayloadShape: "timeseries-number",
						},
					},
				}

				err := action.Parse(structToEncodedMapForEdit(payload))

				Expect(err).ToNot(HaveOccurred())
				parsedPayload := action.GetParsedPayload()
				Expect(parsedPayload.Name).To(Equal("test-model"))
				Expect(parsedPayload.Description).To(Equal("Updated test data model"))
				Expect(parsedPayload.Structure).To(HaveLen(2))
			})
		})

		Context("with invalid payload type", func() {
			It("should return error", func() {
				invalidPayload := "not a valid payload"

				err := action.Parse(invalidPayload)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
			})
		})

		Context("with nil payload", func() {
			It("should return error", func() {
				err := action.Parse(nil)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
			})
		})
	})

	Describe("Validate", func() {
		Context("with valid payload", func() {
			BeforeEach(func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"field1": {
							PayloadShape: "timeseries-string",
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				// Set up mock config to provide empty data models and payload shapes for validation
				mockConfig := config.FullConfig{
					DataModels: []config.DataModelsConfig{},
					PayloadShapes: map[string]config.PayloadShape{
						"timeseries-string": {
							Description: "Time series string data",
							Fields: map[string]config.PayloadField{
								"value": {Type: "string"},
							},
						},
					},
				}
				mockConfigMgr.WithConfig(mockConfig)
			})

			It("should validate successfully", func() {
				err := action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with missing name", func() {
			BeforeEach(func() {
				payload := models.EditDataModelPayload{
					Structure: map[string]models.Field{
						"field1": {
							PayloadShape: "timeseries-string",
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return validation error", func() {
				err := action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("missing required field Name"))
			})
		})

		Context("with empty structure", func() {
			BeforeEach(func() {
				payload := models.EditDataModelPayload{
					Name:      "test-model",
					Structure: map[string]models.Field{},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return validation error", func() {
				err := action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("missing required field Structure"))
			})
		})

		Context("with nil structure", func() {
			BeforeEach(func() {
				payload := models.EditDataModelPayload{
					Name:      "test-model",
					Structure: nil,
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return validation error", func() {
				err := action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("missing required field Structure"))
			})
		})

		Context("with valid data model structure", func() {
			BeforeEach(func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"leaf_field": {
							PayloadShape: "timeseries-string",
						},
						"folder_field": {
							Subfields: map[string]models.Field{
								"nested_leaf": {
									PayloadShape: "timeseries-number",
								},
							},
						},
						"submodel_field": {
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				// Set up mock config with the referenced data model and payload shapes
				mockConfig := config.FullConfig{
					DataModels: []config.DataModelsConfig{
						{
							Name: "external-model",
							Versions: map[string]config.DataModelVersion{
								"v1": {
									Structure: map[string]config.Field{
										"external_field": {
											PayloadShape: "timeseries-string",
										},
									},
								},
							},
						},
					},
					PayloadShapes: map[string]config.PayloadShape{
						"timeseries-string": {
							Description: "Time series string data",
							Fields: map[string]config.PayloadField{
								"value": {Type: "string"},
							},
						},
						"timeseries-number": {
							Description: "Time series number data",
							Fields: map[string]config.PayloadField{
								"value": {Type: "number"},
							},
						},
					},
				}
				mockConfigMgr.WithConfig(mockConfig)
			})

			It("should validate successfully", func() {
				err := action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with invalid _refModel format", func() {
			It("should fail with no version", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_ref": {
							ModelRef: &models.ModelRef{
								Name: "external-model-v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified"))
			})

			It("should fail with no version in complex name", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_ref": {
							ModelRef: &models.ModelRef{
								Name: "external:model:v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified"))
			})

			It("should fail with empty version", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_ref": {
							ModelRef: &models.ModelRef{
								Name: "external-model",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified"))
			})
		})

		Context("with invalid version format", func() {
			It("should fail with version not matching v\\d+", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_version": {
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "version1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not match pattern ^v\\d+$"))
			})
		})

		Context("with invalid field combinations", func() {
			It("should fail when field has both _type and _refModel", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_field": {
							PayloadShape: "timeseries-string",
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validation error at path 'invalid_field': field cannot have both _payloadshape and _refModel"))
			})

			It("should fail when field has both subfields and _refModel", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_field": {
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
							Subfields: map[string]models.Field{
								"nested": {
									PayloadShape: "timeseries-string",
								},
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("field cannot have both subfields and _refModel"))
			})
		})

		Context("with invalid leaf nodes", func() {
			It("should fail when leaf node has no _type", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_leaf": {},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validation error at path 'invalid_leaf': leaf nodes must contain _payloadshape"))
			})
		})

		Context("with invalid submodel nodes", func() {
			It("should fail when submodel node has additional fields besides _refModel", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_submodel": {
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
							PayloadShape: "timeseries-string", // This makes it invalid - cannot have both _type and _refModel
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("field cannot have both _payloadshape and _refModel"))
			})
		})

		Context("with multiple validation errors", func() {
			It("should return all validation errors", func() {
				payload := models.EditDataModelPayload{
					Name: "test-model",
					Structure: map[string]models.Field{
						"invalid_ref1": {
							ModelRef: &models.ModelRef{
								Name: "external-model-no-colon",
							},
						},
						"invalid_ref2": {
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "version1",
							},
						},
						"invalid_leaf": {},
						"invalid_combination": {
							PayloadShape: "timeseries-string",
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				errorMsg := err.Error()
				Expect(errorMsg).To(ContainSubstring("data model structure validation failed:"))
				Expect(errorMsg).To(ContainSubstring("_refModel must have a version specified"))
				Expect(errorMsg).To(ContainSubstring("does not match pattern ^v\\d+$"))
				Expect(errorMsg).To(ContainSubstring("leaf nodes must contain _payloadshape"))
				Expect(errorMsg).To(ContainSubstring("field cannot have both _payloadshape and _refModel"))
			})
		})
	})

	Describe("Execute", func() {
		Context("with successful configuration update", func() {
			BeforeEach(func() {
				payload := models.EditDataModelPayload{
					Name:        "existing-model",
					Description: "Updated version of existing model",
					Structure: map[string]models.Field{
						"updatedField": {
							PayloadShape: "timeseries-string",
						},
						"newField": {
							PayloadShape: "timeseries-number",
						},
						"nested": {
							Subfields: map[string]models.Field{
								"subfield1": {
									PayloadShape: "timeseries-boolean",
								},
							},
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				// Set up mock config with existing data model and payload shapes
				existingConfig := config.FullConfig{
					DataModels: []config.DataModelsConfig{
						{
							Name: "existing-model",
							Versions: map[string]config.DataModelVersion{
								"v1": {
									Structure: map[string]config.Field{
										"originalField": {
											PayloadShape: "timeseries-string",
										},
									},
								},
							},
						},
					},
					PayloadShapes: map[string]config.PayloadShape{
						"timeseries-string": {
							Description: "Time series string data",
							Fields: map[string]config.PayloadField{
								"value": {Type: "string"},
							},
						},
						"timeseries-number": {
							Description: "Time series number data",
							Fields: map[string]config.PayloadField{
								"value": {Type: "number"},
							},
						},
						"timeseries-boolean": {
							Description: "Time series boolean data",
							Fields: map[string]config.PayloadField{
								"value": {Type: "boolean"},
							},
						},
					},
				}
				mockConfigMgr.WithConfig(existingConfig)
			})

			It("should execute successfully", func() {
				response, metadata, err := action.Execute()

				Expect(err).ToNot(HaveOccurred())
				Expect(metadata).To(BeNil())
				Expect(response).ToNot(BeNil())

				responseMap, ok := response.(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(responseMap["name"]).To(Equal("existing-model"))
				Expect(responseMap["description"]).To(Equal("Updated version of existing model"))
				Expect(responseMap["version"]).To(BeNumerically(">", 1)) // Should be a higher version
				Expect(responseMap["structure"]).ToNot(BeNil())

				// Verify config manager was called
				Expect(mockConfigMgr.AtomicEditDataModelCalled).To(BeTrue())
			})

		})

		Context("with configuration manager error", func() {
			BeforeEach(func() {
				payload := models.EditDataModelPayload{
					Name: "failing-model",
					Structure: map[string]models.Field{
						"field1": {
							PayloadShape: "timeseries-string",
						},
					},
				}
				err := action.Parse(structToEncodedMapForEdit(payload))
				Expect(err).ToNot(HaveOccurred())

				// Configure mock to return error
				mockConfigMgr.WithAtomicEditDataModelError(errors.New("mock edit error"))
			})

			It("should return error", func() {
				response, metadata, err := action.Execute()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Failed to edit data model"))
				Expect(response).To(BeNil())
				Expect(metadata).To(BeNil())

				// Verify config manager was called
				Expect(mockConfigMgr.AtomicEditDataModelCalled).To(BeTrue())
			})
		})
	})

	Describe("GetParsedPayload", func() {
		It("should return empty payload before parsing", func() {
			payload := action.GetParsedPayload()
			Expect(payload.Name).To(BeEmpty())
			Expect(payload.Description).To(BeEmpty())
			Expect(payload.Structure).To(BeNil())
		})

		It("should return parsed payload after parsing", func() {
			originalPayload := models.EditDataModelPayload{
				Name:        "test-model",
				Description: "Test description",
				Structure: map[string]models.Field{
					"field1": {
						PayloadShape: "timeseries-string",
					},
				},
			}

			err := action.Parse(structToEncodedMapForEdit(originalPayload))
			Expect(err).ToNot(HaveOccurred())

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Name).To(Equal("test-model"))
			Expect(parsedPayload.Description).To(Equal("Test description"))
			Expect(parsedPayload.Structure).To(HaveLen(1))
		})
	})

	Describe("Integration with different field types", func() {
		It("should handle editing model with complex nested structures", func() {
			payload := models.EditDataModelPayload{
				Name: "complex-model",
				Structure: map[string]models.Field{
					"simple_string": {
						PayloadShape: "timeseries-string",
					},
					"updated_number": {
						PayloadShape: "timeseries-number",
					},
					"new_referenced_model": {
						ModelRef: &models.ModelRef{
							Name:    "another-external-model",
							Version: "v1",
						},
					},
					"updated_nested_object": {
						Subfields: map[string]models.Field{
							"updated_nested_string": {
								PayloadShape: "timeseries-string",
							},
							"new_deeply_nested": {
								Subfields: map[string]models.Field{
									"new_deep_field": {
										PayloadShape: "timeseries-array",
									},
								},
							},
						},
					},
				},
			}

			err := action.Parse(structToEncodedMapForEdit(payload))
			Expect(err).ToNot(HaveOccurred())

			// Set up mock config with existing data model and referenced model and payload shapes
			existingConfig := config.FullConfig{
				DataModels: []config.DataModelsConfig{
					{
						Name: "complex-model",
						Versions: map[string]config.DataModelVersion{
							"v1": {
								Structure: map[string]config.Field{
									"oldField": {
										PayloadShape: "timeseries-string",
									},
								},
							},
						},
					},
					{
						Name: "another-external-model",
						Versions: map[string]config.DataModelVersion{
							"v1": {
								Structure: map[string]config.Field{
									"external_field": {
										PayloadShape: "timeseries-string",
									},
								},
							},
						},
					},
				},
				PayloadShapes: map[string]config.PayloadShape{
					"timeseries-string": {
						Description: "Time series string data",
						Fields: map[string]config.PayloadField{
							"value": {Type: "string"},
						},
					},
					"timeseries-number": {
						Description: "Time series number data",
						Fields: map[string]config.PayloadField{
							"value": {Type: "number"},
						},
					},
					"timeseries-array": {
						Description: "Time series array data",
						Fields: map[string]config.PayloadField{
							"value": {Type: "array"},
						},
					},
				},
			}
			mockConfigMgr.WithConfig(existingConfig)

			err = action.Validate()
			Expect(err).ToNot(HaveOccurred())

			response, metadata, err := action.Execute()
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
			Expect(metadata).To(BeNil())

			// Verify config manager was called
			Expect(mockConfigMgr.AtomicEditDataModelCalled).To(BeTrue())
		})
	})
})
