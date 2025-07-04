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

package validation_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/validation"
)

var _ = Describe("Validator", func() {
	var (
		validator *validation.Validator
		ctx       context.Context
	)

	BeforeEach(func() {
		validator = validation.NewValidator()
		ctx = context.Background()
	})

	Context("ValidateDataModel", func() {
		It("should validate a valid data model with the sample structure", func() {
			// Sample data model from the user
			dataModel := config.DataModelVersion{
				Description: "pump from vendor ABC",
				Structure: map[string]config.Field{
					"count": {
						Type: "timeseries-number",
					},
					"vibration": {
						Subfields: map[string]config.Field{
							"x-axis": {
								Type:        "timeseries-number",
								Description: "whatever",
							},
							"y-axis": {
								Type: "timeseries-number",
								Unit: "mm/s",
							},
							"z-axis": {
								Type: "timeseries-number",
							},
						},
					},
					"motor": {
						ModelRef: &config.ModelRef{
							Name:    "motor",
							Version: "v1",
						},
					},
					"acceleration": {
						Subfields: map[string]config.Field{
							"x": {
								Type: "timeseries-number",
							},
							"y": {
								Type: "timeseries-number",
							},
						},
					},
					"serialNumber": {
						Type: "timeseries-string",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
		})

		It("should validate a leaf node with _type only", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"simple": {
						Type: "timeseries-number",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
		})

		It("should validate a leaf node with _type and _description", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"described": {
						Type:        "timeseries-number",
						Description: "A described field",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
		})

		It("should validate a leaf node with _type and _unit", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"measured": {
						Type: "timeseries-number",
						Unit: "kg",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
		})

		It("should validate a leaf node with _refModel", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"referenced": {
						ModelRef: &config.ModelRef{
							Name:    "otherModel",
							Version: "v1",
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
		})

		It("should validate a non-leaf node with only subfields", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"parent": {
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
		})

		It("should fail validation for a non-leaf node with _type", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"parent": {
						Type: "timeseries-object",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

<<<<<<< HEAD
			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf node cannot have _type or _refModel"))
			Expect(err.Error()).To(ContainSubstring("parent"))
		})

		It("should fail validation for a non-leaf node with _description", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"parent": {
						Description: "This is a folder description",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf node cannot have _description"))
			Expect(err.Error()).To(ContainSubstring("parent"))
		})

		It("should fail validation for a non-leaf node with _unit", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"parent": {
						Unit: "kg",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf node cannot have _unit"))
			Expect(err.Error()).To(ContainSubstring("parent"))
		})

		It("should fail validation for a non-leaf node with multiple forbidden fields", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"parent": {
						Type:        "timeseries-object",
						Description: "This is a folder description",
						Unit:        "kg",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf node cannot have _type or _refModel"))
			Expect(err.Error()).To(ContainSubstring("non-leaf node cannot have _description"))
			Expect(err.Error()).To(ContainSubstring("non-leaf node cannot have _unit"))
			Expect(err.Error()).To(ContainSubstring("parent"))
||||||| parent of 32c4f779 (save)
			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(BeNil())
=======
			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf nodes (folders) cannot have _type"))
			Expect(err.Error()).To(ContainSubstring("parent"))
		})

		It("should fail validation for a non-leaf node with _description", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"parent": {
						Description: "This is a folder description",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf nodes (folders) cannot have _description"))
			Expect(err.Error()).To(ContainSubstring("parent"))
		})

		It("should fail validation for a non-leaf node with _unit", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"parent": {
						Unit: "kg",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf nodes (folders) cannot have _unit"))
			Expect(err.Error()).To(ContainSubstring("parent"))
		})

		It("should fail validation for a non-leaf node with multiple forbidden fields", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"parent": {
						Type:        "timeseries-object",
						Description: "This is a folder description",
						Unit:        "kg",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf nodes (folders) cannot have _type"))
			Expect(err.Error()).To(ContainSubstring("non-leaf nodes (folders) cannot have _description"))
			Expect(err.Error()).To(ContainSubstring("non-leaf nodes (folders) cannot have _unit"))
			Expect(err.Error()).To(ContainSubstring("parent"))
>>>>>>> 32c4f779 (save)
		})

		It("should fail validation for a leaf node with neither _type nor _refModel", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalid": {
						Description: "has description but no type or refModel",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("leaf node must have either _type or _refModel"))
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})

		It("should fail validation for a leaf node with both _type and _refModel", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"conflicted": {
						Type: "timeseries-number",
						ModelRef: &config.ModelRef{
							Name:    "otherModel",
							Version: "v1",
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field cannot have both _type and _refModel"))
			Expect(err.Error()).To(ContainSubstring("conflicted"))
		})

		It("should fail validation for a field with both subfields and _refModel", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidParent": {
						ModelRef: &config.ModelRef{
							Name:    "otherModel",
							Version: "v1",
						},
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf node cannot have _type or _refModel"))
			Expect(err.Error()).To(ContainSubstring("invalidParent"))
		})

		It("should fail validation for invalid _refModel format - no name", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidRef": {
						ModelRef: &config.ModelRef{
							Version: "v1",
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_refModel must have a model name specified"))
		})

		It("should fail validation for invalid _refModel format - no version", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidRef": {
						ModelRef: &config.ModelRef{
							Name: "model",
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified"))
		})

		It("should fail validation for invalid _refModel format - empty version", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidRef": {
						ModelRef: &config.ModelRef{
							Name:    "model",
							Version: "",
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified"))
		})

		It("should fail validation for invalid version format", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidRef": {
<<<<<<< HEAD
						ModelRef: &config.ModelRef{
							Name:    "model",
							Version: "invalidversion",
||||||| parent of 32c4f779 (save)
						ModelRef: "model:invalidversion",
					},
				},
			}

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("version 'invalidversion' does not match pattern"))
		})

		It("should fail validation for submodel nodes with additional fields", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidSubmodel": {
						ModelRef:    "otherModel:v1",
						Description: "should not have description",
					},
				},
			}

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("subModel nodes should ONLY contain _refModel"))
		})

		It("should validate nested structures recursively", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"level1": {
						Subfields: map[string]config.Field{
							"level2": {
								Subfields: map[string]config.Field{
									"level3": {
										Type: "timeseries-number",
									},
								},
							},
=======
						ModelRef: "model:invalidversion",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("version 'invalidversion' does not match pattern"))
		})

		It("should fail validation for submodel nodes with additional fields", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidSubmodel": {
						ModelRef:    "otherModel:v1",
						Description: "should not have description",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("subModel nodes should ONLY contain _refModel"))
		})

		It("should validate nested structures recursively", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"level1": {
						Subfields: map[string]config.Field{
							"level2": {
								Subfields: map[string]config.Field{
									"level3": {
										Type: "timeseries-number",
									},
								},
							},
>>>>>>> 32c4f779 (save)
						},
					},
				},
			}

<<<<<<< HEAD
			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_refModel version must match pattern 'v<number>' but got 'invalidversion'"))
||||||| parent of 32c4f779 (save)
			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(BeNil())
=======
			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
>>>>>>> 32c4f779 (save)
		})

		It("should validate valid version formats", func() {
			testCases := []string{"v1", "v2", "v10", "v999"}
			for _, version := range testCases {
				dataModel := config.DataModelVersion{
					Structure: map[string]config.Field{
						"validRef": {
							ModelRef: &config.ModelRef{
								Name:    "model",
								Version: version,
							},
						},
					},
				}

				err := validator.ValidateStructureOnly(ctx, dataModel)
				Expect(err).To(BeNil())
			}
		})

		It("should fail validation for invalid field names", func() {
			invalidNames := []string{
				"field.name",    // Contains dot
				"field name",    // Contains space
				"field@name",    // Contains special character
				"field#name",    // Contains hash
				"field$name",    // Contains dollar sign
				"field%name",    // Contains percent
				"field&name",    // Contains ampersand
				"field*name",    // Contains asterisk
				"field+name",    // Contains plus
				"field=name",    // Contains equals
				"field[name]",   // Contains brackets
				"field{name}",   // Contains braces
				"field|name",    // Contains pipe
				"field\\name",   // Contains backslash
				"field/name",    // Contains forward slash
				"field?name",    // Contains question mark
				"field<name>",   // Contains angle brackets
				"field,name",    // Contains comma
				"field;name",    // Contains semicolon
				"field:name",    // Contains colon
				"field\"name\"", // Contains quotes
				"field'name'",   // Contains single quotes
				"field`name`",   // Contains backticks
				"field~name",    // Contains tilde
				"field!name",    // Contains exclamation
			}

			for _, invalidName := range invalidNames {
				dataModel := config.DataModelVersion{
					Structure: map[string]config.Field{
						invalidName: {
							Type: "timeseries-number",
						},
					},
				}

				err := validator.ValidateStructureOnly(ctx, dataModel)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("field name can only contain letters, numbers, dashes, and underscores"))
			}
		})

		It("should validate valid field names", func() {
			validNames := []string{
				"field",
				"field_name",
				"field-name",
				"field123",
				"field_123",
				"field-123",
				"field_name_123",
				"field-name-123",
				"123field",
				"123_field",
				"123-field",
				"FieldName",
				"FIELD_NAME",
				"Field_Name_123",
				"field_",
				"field-",
				"_field",
				"-field",
				"_",
				"-",
				"a",
				"A",
				"1",
			}

			for _, validName := range validNames {
				dataModel := config.DataModelVersion{
					Structure: map[string]config.Field{
						validName: {
							Type: "timeseries-number",
						},
					},
				}

				err := validator.ValidateStructureOnly(ctx, dataModel)
				Expect(err).To(BeNil())
			}
		})

		It("should fail validation for empty field names", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"": {
						Type: "timeseries-number",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field name cannot be empty"))
		})

		It("should fail validation for _unit without _type", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalid": {
						Unit: "kg",
					},
				},
			}

<<<<<<< HEAD
			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_unit can only be used with _type"))
		})

		It("should fail validation for _unit with non-number _type", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalid": {
						Type: "timeseries-string",
						Unit: "kg",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_unit can only be used with _type: timeseries-number"))
		})

		It("should validate _unit with timeseries-number _type", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"valid": {
						Type: "timeseries-number",
						Unit: "kg",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
||||||| parent of 32c4f779 (save)
			err := validator.ValidateDataModel(ctx, dataModel)
=======
			err := validator.ValidateStructureOnly(ctx, dataModel)
>>>>>>> 32c4f779 (save)
			Expect(err).To(BeNil())
		})

		It("should fail validation for invalid _type", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalid": {
						Type: "invalid-type",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid _type 'invalid-type', must be one of: timeseries-number, timeseries-string, timeseries-boolean"))
		})

		It("should validate all valid _type values", func() {
			validTypes := []string{"timeseries-number", "timeseries-string", "timeseries-boolean"}
			for _, validType := range validTypes {
				dataModel := config.DataModelVersion{
					Structure: map[string]config.Field{
						"valid": {
							Type: validType,
						},
					},
				}

				err := validator.ValidateStructureOnly(ctx, dataModel)
				Expect(err).To(BeNil())
			}
		})
	})

	Context("ValidateStructureOnly", func() {
		It("should respect context cancellation", func() {
			// Create a cancelled context
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"simple": {
						Type: "timeseries-number",
					},
				},
			}

			err := validator.ValidateStructureOnly(cancelledCtx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})
	})
})
