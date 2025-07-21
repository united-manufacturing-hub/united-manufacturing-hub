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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
)

var _ = Describe("Validator References", func() {
	var (
		validator     *datamodel.Validator
		ctx           context.Context
		payloadShapes map[string]config.PayloadShape
	)

	BeforeEach(func() {
		validator = datamodel.NewValidator()
		ctx = context.Background()

		// Set up common payload shapes for all tests
		payloadShapes = map[string]config.PayloadShape{
			"timeseries-number": {
				Description: "Time series number data",
				Fields: map[string]config.PayloadField{
					"value": {Type: "number"},
				},
			},
			"timeseries-string": {
				Description: "Time series string data",
				Fields: map[string]config.PayloadField{
					"value": {Type: "string"},
				},
			},
		}
	})

	Context("ValidateDataModelWithReferences", func() {
		It("should validate a data model with valid references", func() {
			// Create a motor model
			motorModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"rpm": {
						PayloadShape: "timeseries-number",
					},
					"temperature": {
						PayloadShape: "timeseries-number",
					},
				},
			}

			// Create a pump model that references the motor
			pumpModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"flowRate": {
						PayloadShape: "timeseries-number",
					},
					"motor": {
						ModelRef: &config.ModelRef{
							Name:    "motor",
							Version: "v1",
						},
					},
				},
			}

			// Create the data models map
			allDataModels := map[string]config.DataModelsConfig{
				"motor": {
					Name: "motor",
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
				"pump": {
					Name: "pump",
					Versions: map[string]config.DataModelVersion{
						"v1": pumpModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, pumpModel, allDataModels, payloadShapes)
			Expect(err).To(BeNil())
		})

		It("should fail when referencing a non-existent model", func() {
			testModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"motor": {
						ModelRef: &config.ModelRef{
							Name:    "nonexistent",
							Version: "v1",
						},
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"pump": {
					Name: "pump",
					Versions: map[string]config.DataModelVersion{
						"v1": testModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, testModel, allDataModels, payloadShapes)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced model 'nonexistent' does not exist"))
		})

		It("should fail when referencing a non-existent version", func() {
			motorModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"rpm": {
						PayloadShape: "timeseries-number",
					},
				},
			}

			testModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"motor": {
						ModelRef: &config.ModelRef{
							Name:    "motor",
							Version: "v2", // v2 doesn't exist
						},
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"motor": {
					Name: "motor",
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
				"pump": {
					Name: "pump",
					Versions: map[string]config.DataModelVersion{
						"v1": testModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, testModel, allDataModels, payloadShapes)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced model 'motor' version 'v2' does not exist"))
		})

		It("should detect circular references", func() {
			// Create model A that references model B
			modelA := config.DataModelVersion{
				Structure: map[string]config.Field{
					"fieldA": {
						PayloadShape: "timeseries-number",
					},
					"refToB": {
						ModelRef: &config.ModelRef{
							Name:    "modelB",
							Version: "v1",
						},
					},
				},
			}

			// Create model B that references model A (circular)
			modelB := config.DataModelVersion{
				Structure: map[string]config.Field{
					"fieldB": {
						PayloadShape: "timeseries-string",
					},
					"refToA": {
						ModelRef: &config.ModelRef{
							Name:    "modelA",
							Version: "v1",
						},
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"modelA": {
					Name: "modelA",
					Versions: map[string]config.DataModelVersion{
						"v1": modelA,
					},
				},
				"modelB": {
					Name: "modelB",
					Versions: map[string]config.DataModelVersion{
						"v1": modelB,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, modelA, allDataModels, payloadShapes)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circular reference detected"))
		})

		It("should detect self-referencing models", func() {
			// Create a model that references itself
			selfRefModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"field1": {
						PayloadShape: "timeseries-number",
					},
					"selfRef": {
						ModelRef: &config.ModelRef{
							Name:    "selfRef",
							Version: "v1",
						},
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"selfRef": {
					Name: "selfRef",
					Versions: map[string]config.DataModelVersion{
						"v1": selfRefModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, selfRefModel, allDataModels, payloadShapes)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circular reference detected: selfRef:v1"))
		})

		It("should handle deep reference chains up to 10 levels", func() {
			allDataModels := make(map[string]config.DataModelsConfig)

			// Create a chain of 9 models (level0 -> level1 -> ... -> level8)
			for i := 0; i < 9; i++ {
				modelName := fmt.Sprintf("level%d", i)
				var structure map[string]config.Field

				if i == 8 { // Last level - no reference
					structure = map[string]config.Field{
						"finalField": {
							PayloadShape: "timeseries-number",
						},
					}
				} else { // Reference next level
					nextLevel := fmt.Sprintf("level%d", i+1)
					structure = map[string]config.Field{
						"field": {
							PayloadShape: "timeseries-number",
						},
						"nextRef": {
							ModelRef: &config.ModelRef{
								Name:    nextLevel,
								Version: "v1",
							},
						},
					}
				}

				allDataModels[modelName] = config.DataModelsConfig{
					Name: modelName,
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: structure,
						},
					},
				}
			}

			// Validate the first model - should pass (9 levels deep)
			err := validator.ValidateWithReferences(ctx, allDataModels["level0"].Versions["v1"], allDataModels, payloadShapes)
			Expect(err).To(BeNil())
		})

		It("should reject reference chains deeper than 10 levels", func() {
			allDataModels := make(map[string]config.DataModelsConfig)

			// Create a chain of 12 models (level0 -> level1 -> ... -> level11)
			// This ensures we definitely exceed 10 levels
			for i := 0; i < 12; i++ {
				modelName := fmt.Sprintf("level%d", i)
				var structure map[string]config.Field

				if i == 11 { // Last level - no reference
					structure = map[string]config.Field{
						"finalField": {
							PayloadShape: "timeseries-number",
						},
					}
				} else { // Reference next level
					nextLevel := fmt.Sprintf("level%d", i+1)
					structure = map[string]config.Field{
						"field": {
							PayloadShape: "timeseries-number",
						},
						"nextRef": {
							ModelRef: &config.ModelRef{
								Name:    nextLevel,
								Version: "v1",
							},
						},
					}
				}

				allDataModels[modelName] = config.DataModelsConfig{
					Name: modelName,
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: structure,
						},
					},
				}
			}

			// Validate the first model - should fail (12 levels deep)
			err := validator.ValidateWithReferences(ctx, allDataModels["level0"].Versions["v1"], allDataModels, payloadShapes)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reference validation depth limit exceeded (10 levels)"))
		})

		It("should validate nested references in subfields", func() {
			// Create a sensor model
			sensorModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"value": {
						PayloadShape: "timeseries-number",
					},
					"unit": {
						PayloadShape: "timeseries-string",
					},
				},
			}

			// Create a complex model with nested references
			complexModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"metadata": {
						Subfields: map[string]config.Field{
							"temperature_sensor": {
								ModelRef: &config.ModelRef{
									Name:    "sensor",
									Version: "v1",
								},
							},
							"pressure_sensor": {
								ModelRef: &config.ModelRef{
									Name:    "sensor",
									Version: "v1",
								},
							},
						},
					},
					"readings": {
						Subfields: map[string]config.Field{
							"primary": {
								ModelRef: &config.ModelRef{
									Name:    "sensor",
									Version: "v1",
								},
							},
						},
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"sensor": {
					Name: "sensor",
					Versions: map[string]config.DataModelVersion{
						"v1": sensorModel,
					},
				},
				"complex": {
					Name: "complex",
					Versions: map[string]config.DataModelVersion{
						"v1": complexModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, complexModel, allDataModels, payloadShapes)
			Expect(err).To(BeNil())
		})

		It("should fail basic validation if the data model structure is invalid", func() {
			// Create an invalid data model (missing _type)
			invalidModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"invalidField": {
						// Missing Type and ModelRef
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"invalid": {
					Name: "invalid",
					Versions: map[string]config.DataModelVersion{
						"v1": invalidModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, invalidModel, allDataModels, payloadShapes)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("leaf nodes must contain _payloadshape"))
		})

		It("should handle complex reference scenarios", func() {
			// Create a complex scenario with multiple levels of references
			allDataModels := map[string]config.DataModelsConfig{
				"motor": {
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"speed":       {PayloadShape: "timeseries-number"},
								"temperature": {PayloadShape: "timeseries-number"},
								"sensor":      {ModelRef: &config.ModelRef{Name: "sensor", Version: "v1"}},
							},
						},
					},
				},
				"sensor": {
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"value": {PayloadShape: "timeseries-number"},
								"unit":  {PayloadShape: "timeseries-string"},
							},
						},
					},
				},
			}

			// Data model that references motor which references sensor
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"pump": {
						Subfields: map[string]config.Field{
							"motor": {ModelRef: &config.ModelRef{Name: "motor", Version: "v1"}},
							"flow":  {PayloadShape: "timeseries-number"},
						},
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, dataModel, allDataModels, payloadShapes)
			Expect(err).To(BeNil())
		})

		It("should respect context cancellation during reference validation", func() {
			// Create a cancelled context
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			allDataModels := map[string]config.DataModelsConfig{
				"motor": {
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"speed": {PayloadShape: "timeseries-number"},
							},
						},
					},
				},
			}

			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"motor": {ModelRef: &config.ModelRef{Name: "motor", Version: "v1"}},
				},
			}

			err := validator.ValidateWithReferences(cancelledCtx, dataModel, allDataModels, payloadShapes)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})
	})
})
