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
						ModelRef: "motor:v1",
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
						ModelRef: "otherModel:v1",
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
			Expect(err.Error()).To(ContainSubstring("leaf nodes must contain _type"))
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})

		It("should fail validation for a leaf node with both _type and _refModel", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"conflicted": {
						Type:     "timeseries-number",
						ModelRef: "otherModel:v1",
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
						ModelRef: "otherModel:v1",
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
			Expect(err.Error()).To(ContainSubstring("field cannot have both subfields and _refModel"))
			Expect(err.Error()).To(ContainSubstring("invalidParent"))
		})

		It("should fail validation for invalid _refModel format - no colon", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidRef": {
						ModelRef: "invalidformat",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_refModel must contain exactly one ':'"))
		})

		It("should fail validation for invalid _refModel format - multiple colons", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidRef": {
						ModelRef: "model:v1:extra",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_refModel must contain exactly one ':'"))
		})

		It("should fail validation for invalid _refModel format - empty version", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidRef": {
						ModelRef: "model:",
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("_refModel must have a version specified after ':'"))
		})

		It("should fail validation for invalid version format", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidRef": {
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
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
		})

		It("should fail validation for invalid nested structures", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"level1": {
						Subfields: map[string]config.Field{
							"level2": {
								Subfields: map[string]config.Field{
									"invalidLeaf": {
										Description: "no type or refModel",
									},
								},
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("level1.level2.invalidLeaf"))
		})

		It("should handle empty structure", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
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
