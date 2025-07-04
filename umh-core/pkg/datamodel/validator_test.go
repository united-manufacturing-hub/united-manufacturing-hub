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

package datamodel_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
)

var _ = Describe("Validator", func() {
	var (
		validator *datamodel.Validator
		ctx       context.Context
	)

	BeforeEach(func() {
		validator = datamodel.NewValidator()
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

			err := validator.ValidateDataModel(ctx, dataModel)
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

			err := validator.ValidateDataModel(ctx, dataModel)
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

			err := validator.ValidateDataModel(ctx, dataModel)
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

			err := validator.ValidateDataModel(ctx, dataModel)
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

			err := validator.ValidateDataModel(ctx, dataModel)
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

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(BeNil())
		})

		It("should fail validation for a leaf node with neither _type nor _refModel", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalid": {
						Description: "has description but no type or refModel",
					},
				},
			}

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("leaf node"))
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

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("both _type and _refModel"))
			Expect(err.Error()).To(ContainSubstring("conflicted"))
		})

		It("should fail validation for a non-leaf node with _type", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidParent": {
						Type: "timeseries-number",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf node"))
			Expect(err.Error()).To(ContainSubstring("invalidParent"))
		})

		It("should fail validation for a non-leaf node with _description", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidParent": {
						Description: "should not have description",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf node"))
			Expect(err.Error()).To(ContainSubstring("invalidParent"))
		})

		It("should fail validation for a non-leaf node with _unit", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidParent": {
						Unit: "kg",
						Subfields: map[string]config.Field{
							"child": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf node"))
			Expect(err.Error()).To(ContainSubstring("invalidParent"))
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

			err := validator.ValidateDataModel(ctx, dataModel)
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

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("level1.level2.invalidLeaf"))
		})

		It("should handle empty structure", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{},
			}

			err := validator.ValidateDataModel(ctx, dataModel)
			Expect(err).To(BeNil())
		})
	})
})
