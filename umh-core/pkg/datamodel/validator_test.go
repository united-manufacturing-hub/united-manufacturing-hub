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
				Structure: map[string]config.Field{
					"count": {
						PayloadShape: "timeseries-number",
					},
					"vibration": {
						Subfields: map[string]config.Field{
							"x-axis": {
								PayloadShape: "timeseries-number",
							},
							"y-axis": {
								PayloadShape: "timeseries-number",
							},
							"z-axis": {
								PayloadShape: "timeseries-number",
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
								PayloadShape: "timeseries-number",
							},
							"y": {
								PayloadShape: "timeseries-number",
							},
						},
					},
					"serialNumber": {
						PayloadShape: "timeseries-string",
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
						PayloadShape: "timeseries-number",
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
								PayloadShape: "timeseries-number",
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
						PayloadShape: "timeseries-object",
						Subfields: map[string]config.Field{
							"child": {
								PayloadShape: "timeseries-number",
							},
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-leaf nodes (folders) cannot have _payloadshape"))
			Expect(err.Error()).To(ContainSubstring("parent"))
		})

		It("should fail validation for a leaf node with neither _type nor _refModel", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalid": {},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("leaf nodes must contain _payloadshape"))
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})

		It("should fail validation for a leaf node with both _type and _refModel", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"conflicted": {
						PayloadShape: "timeseries-number",
						ModelRef: &config.ModelRef{
							Name:    "otherModel",
							Version: "v1",
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field cannot have both _payloadshape and _refModel"))
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
								PayloadShape: "timeseries-number",
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
						ModelRef: &config.ModelRef{
							Name:    "model",
							Version: "invalidversion",
						},
					},
				},
			}

			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("version 'invalidversion' does not match pattern"))
		})

		It("should validate nested structures recursively", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"level1": {
						Subfields: map[string]config.Field{
							"level2": {
								Subfields: map[string]config.Field{
									"level3": {
										PayloadShape: "timeseries-number",
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
									"invalidLeaf": {},
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
						PayloadShape: "timeseries-number",
					},
				},
			}

			err := validator.ValidateStructureOnly(cancelledCtx, dataModel)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})

	})
})
