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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"gopkg.in/yaml.v3"
)

// Helper function to convert a struct to map[string]interface{}
// to simulate how payloads are received from the router
func structToEncodedMap(v interface{}) map[string]interface{} {
	payload := v.(models.AddDataModelPayload)

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

var _ = Describe("AddDataModelAction", func() {
	var (
		action          *actions.AddDataModelAction
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

		action = actions.NewAddDataModelAction(
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

	Describe("NewAddDataModelAction", func() {
		It("should create a new action with correct fields", func() {
			Expect(action).ToNot(BeNil())
			// Note: getUserEmail() and getUuid() are private methods, so we can't test them directly
			// We test their functionality indirectly through Execute() which uses them
		})
	})

	Describe("Parse", func() {
		Context("with valid payload", func() {
			It("should parse successfully", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"field1": {
							Type: "timeseries-string",
						},
					},
				}

				err := action.Parse(structToEncodedMap(payload))

				Expect(err).ToNot(HaveOccurred())
				parsedPayload := action.GetParsedPayload()
				Expect(parsedPayload.Name).To(Equal("test-model"))
				Expect(parsedPayload.Description).To(Equal("Test data model"))
				Expect(parsedPayload.Structure).To(HaveLen(1))
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
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"field1": {
							Type: "timeseries-string",
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should validate successfully", func() {
				err := action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with missing name", func() {
			BeforeEach(func() {
				payload := models.AddDataModelPayload{
					Name:        "",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"field1": {
							Type: "timeseries-string",
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
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
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure:   map[string]models.Field{},
				}
				err := action.Parse(structToEncodedMap(payload))
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
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure:   nil,
				}
				err := action.Parse(structToEncodedMap(payload))
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
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"leaf_field": {
							Type: "timeseries-string",
						},
						"folder_field": {
							Subfields: map[string]models.Field{
								"nested_leaf": {
									Type: "timeseries-number",
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
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should validate successfully", func() {
				err := action.Validate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with invalid _refModel format", func() {
			It("should fail with no version", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"invalid_ref": {
							ModelRef: &models.ModelRef{
								Name: "external-model-v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified"))
			})

			It("should fail with no version in complex name", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"invalid_ref": {
							ModelRef: &models.ModelRef{
								Name: "external:model:v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified"))
			})

			It("should fail with empty version", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"invalid_ref": {
							ModelRef: &models.ModelRef{
								Name: "external-model",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified"))
			})
		})

		Context("with invalid version format", func() {
			It("should fail with version not matching v\\d+", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"invalid_version": {
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "version1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("_refModel version must match pattern 'v<number>' but got 'version1'"))
			})
		})

		Context("with invalid field combinations", func() {
			It("should fail when field has both _type and _refModel", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"invalid_field": {
							Type: "timeseries-string",
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("field cannot have both _type and _refModel"))
			})

			It("should fail when field has both subfields and _refModel", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"invalid_field": {
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
							Subfields: map[string]models.Field{
								"nested": {
									Type: "timeseries-string",
								},
							},
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("non-leaf node cannot have _type or _refModel"))
			})
		})

		Context("with invalid leaf nodes", func() {
			It("should fail when leaf node has no _type", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"invalid_leaf": {
							Description: "Missing type",
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("leaf node must have either _type or _refModel"))
			})
		})

		Context("with invalid submodel nodes", func() {
			It("should fail when submodel node has additional fields besides _refModel", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"invalid_submodel": {
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
							Description: "Should not have description",
							Unit:        "kg",
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("subModel nodes should ONLY contain _refModel"))
			})
		})

		Context("with multiple validation errors", func() {
			It("should return all validation errors", func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
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
						"invalid_leaf": {
							Description: "Missing type",
						},
						"invalid_combination": {
							Type: "timeseries-string",
							ModelRef: &models.ModelRef{
								Name:    "external-model",
								Version: "v1",
							},
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())

				err = action.Validate()
				Expect(err).To(HaveOccurred())
				errorMsg := err.Error()
				Expect(errorMsg).To(ContainSubstring("data model structure validation failed:"))
				Expect(errorMsg).To(ContainSubstring("_refModel must have a version specified"))
				Expect(errorMsg).To(ContainSubstring("_refModel version must match pattern 'v<number>' but got 'version1'"))
				Expect(errorMsg).To(ContainSubstring("leaf node must have either _type or _refModel"))
				Expect(errorMsg).To(ContainSubstring("field cannot have both _type and _refModel"))
			})
		})
	})

	Describe("Execute", func() {
		Context("with successful configuration update", func() {
			BeforeEach(func() {
				payload := models.AddDataModelPayload{
					Name:        "test-model",
					Description: "Test data model",
					Structure: map[string]models.Field{
						"field1": {
							Type: "timeseries-string",
						},
						"nested": {
							Subfields: map[string]models.Field{
								"subfield1": {
									Type: "timeseries-number",
								},
							},
						},
					},
				}
				err := action.Parse(structToEncodedMap(payload))
				Expect(err).ToNot(HaveOccurred())
			})

			It("should execute successfully", func() {
				response, metadata, err := action.Execute()

				Expect(err).ToNot(HaveOccurred())
				Expect(metadata).To(BeNil())
				Expect(response).ToNot(BeNil())

				responseMap, ok := response.(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(responseMap["name"]).To(Equal("test-model"))
				Expect(responseMap["description"]).To(Equal("Test data model"))
				Expect(responseMap["version"]).To(Equal(1))
				Expect(responseMap["structure"]).ToNot(BeNil())

				// Verify config manager was called
				Expect(mockConfigMgr.AtomicAddDataModelCalled).To(BeTrue())
			})

			It("should send correct action replies to outbound channel", func() {
				go func() {
					defer GinkgoRecover()
					_, _, _ = action.Execute()
				}()

				// Should receive multiple messages
				var messages []*models.UMHMessage
				Eventually(func() int {
					select {
					case msg := <-outboundChannel:
						messages = append(messages, msg)
					case <-time.After(100 * time.Millisecond):
						// Timeout to prevent hanging
					}
					return len(messages)
				}, "1s").Should(BeNumerically(">=", 2))

				// Verify we received some messages (exact structure depends on UMHMessage)
				Expect(len(messages)).To(BeNumerically(">", 0))
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

	})

	Describe("Integration with different field types", func() {
		It("should handle complex nested structures", func() {
			payload := models.AddDataModelPayload{
				Name:        "complex-model",
				Description: "Complex data model with nested fields",
				Structure: map[string]models.Field{
					"simple_string": {
						Type: "timeseries-string",
					},
					"simple_number": {
						Type: "timeseries-number",
					},
					"referenced_model": {
						ModelRef: &models.ModelRef{
							Name:    "external-model",
							Version: "v1",
						},
					},
					"nested_object": {
						Subfields: map[string]models.Field{
							"nested_string": {
								Type: "timeseries-string",
							},
							"deeply_nested": {
								Subfields: map[string]models.Field{
									"deep_field": {
										Type: "timeseries-boolean",
									},
								},
							},
						},
					},
				},
			}

			err := action.Parse(structToEncodedMap(payload))
			Expect(err).ToNot(HaveOccurred())

			err = action.Validate()
			Expect(err).ToNot(HaveOccurred())

			response, metadata, err := action.Execute()
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
			Expect(metadata).To(BeNil())

			// Verify config manager was called
			Expect(mockConfigMgr.AtomicAddDataModelCalled).To(BeTrue())
		})
	})
})
