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
		validator *datamodel.Validator
		ctx       context.Context
	)

	BeforeEach(func() {
		validator = datamodel.NewValidator()
		ctx = context.Background()
	})

	Context("ValidateDataModelWithReferences", func() {
		It("should validate a data model with valid references", func() {
			// Create a motor model
			motorModel := config.DataModelVersion{
				Description: "Motor data model",
				Structure: map[string]config.Field{
					"rpm": {
						Type: "timeseries-number",
						Unit: "rpm",
					},
					"temperature": {
						Type: "timeseries-number",
						Unit: "celsius",
					},
				},
			}

			// Create a pump model that references the motor
			pumpModel := config.DataModelVersion{
				Description: "Pump data model",
				Structure: map[string]config.Field{
					"flowRate": {
						Type: "timeseries-number",
						Unit: "l/min",
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
					Name:        "motor",
					Description: "Motor data model",
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
				"pump": {
					Name:        "pump",
					Description: "Pump data model",
					Versions: map[string]config.DataModelVersion{
						"v1": pumpModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, pumpModel, allDataModels)
			Expect(err).To(BeNil())
		})

		It("should fail when referencing a non-existent model", func() {
			testModel := config.DataModelVersion{
				Description: "Test model with non-existent reference",
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
					Name:        "pump",
					Description: "Pump data model",
					Versions: map[string]config.DataModelVersion{
						"v1": testModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, testModel, allDataModels)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced model 'nonexistent' does not exist"))
		})

		It("should fail when referencing a non-existent version", func() {
			motorModel := config.DataModelVersion{
				Description: "Motor data model",
				Structure: map[string]config.Field{
					"rpm": {
						Type: "timeseries-number",
					},
				},
			}

			testModel := config.DataModelVersion{
				Description: "Test model with non-existent version",
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
					Name:        "motor",
					Description: "Motor data model",
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
				"pump": {
					Name:        "pump",
					Description: "Pump data model",
					Versions: map[string]config.DataModelVersion{
						"v1": testModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, testModel, allDataModels)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced model 'motor' does not have version 'v2'"))
		})

		It("should detect circular references", func() {
			// Create model A that references model B
			modelA := config.DataModelVersion{
				Description: "Model A",
				Structure: map[string]config.Field{
					"fieldA": {
						Type: "timeseries-number",
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
				Description: "Model B",
				Structure: map[string]config.Field{
					"fieldB": {
						Type: "timeseries-string",
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
					Name:        "modelA",
					Description: "Model A",
					Versions: map[string]config.DataModelVersion{
						"v1": modelA,
					},
				},
				"modelB": {
					Name:        "modelB",
					Description: "Model B",
					Versions: map[string]config.DataModelVersion{
						"v1": modelB,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, modelA, allDataModels)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circular reference detected"))
		})

		It("should handle deep nesting without false circular reference detection", func() {
			// Create a base sensor model
			sensorModel := config.DataModelVersion{
				Description: "Sensor data model",
				Structure: map[string]config.Field{
					"value": {
						Type: "timeseries-number",
					},
					"timestamp": {
						Type: "timeseries-string",
					},
				},
			}

			// Create a temperature sensor that references the base sensor
			tempSensorModel := config.DataModelVersion{
				Description: "Temperature sensor",
				Structure: map[string]config.Field{
					"sensor": {
						ModelRef: &config.ModelRef{
							Name:    "sensor",
							Version: "v1",
						},
					},
					"unit": {
						Type: "timeseries-string",
					},
				},
			}

			// Create a motor model that references the temperature sensor
			motorModel := config.DataModelVersion{
				Description: "Motor with temperature sensor",
				Structure: map[string]config.Field{
					"rpm": {
						Type: "timeseries-number",
					},
					"temperature": {
						ModelRef: &config.ModelRef{
							Name:    "tempSensor",
							Version: "v1",
						},
					},
				},
			}

			// Create a pump model that references the motor
			pumpModel := config.DataModelVersion{
				Description: "Pump with motor",
				Structure: map[string]config.Field{
					"flowRate": {
						Type: "timeseries-number",
					},
					"motor": {
						ModelRef: &config.ModelRef{
							Name:    "motor",
							Version: "v1",
						},
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"sensor": {
					Name:        "sensor",
					Description: "Base sensor",
					Versions: map[string]config.DataModelVersion{
						"v1": sensorModel,
					},
				},
				"tempSensor": {
					Name:        "tempSensor",
					Description: "Temperature sensor",
					Versions: map[string]config.DataModelVersion{
						"v1": tempSensorModel,
					},
				},
				"motor": {
					Name:        "motor",
					Description: "Motor",
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
				"pump": {
					Name:        "pump",
					Description: "Pump",
					Versions: map[string]config.DataModelVersion{
						"v1": pumpModel,
					},
				},
			}

			err := validator.ValidateWithReferences(ctx, pumpModel, allDataModels)
			Expect(err).To(BeNil())
		})

		It("should handle context cancellation gracefully", func() {
			// Create a complex model to ensure validation takes some time
			complexModel := config.DataModelVersion{
				Description: "Complex model",
				Structure: map[string]config.Field{
					"level1": {
						Subfields: map[string]config.Field{
							"level2": {
								Subfields: map[string]config.Field{
									"level3": {
										ModelRef: &config.ModelRef{
											Name:    "sensor",
											Version: "v1",
										},
									},
								},
							},
						},
					},
				},
			}

			sensorModel := config.DataModelVersion{
				Description: "Sensor",
				Structure: map[string]config.Field{
					"value": {
						Type: "timeseries-number",
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"sensor": {
					Name:        "sensor",
					Description: "Sensor",
					Versions: map[string]config.DataModelVersion{
						"v1": sensorModel,
					},
				},
				"complex": {
					Name:        "complex",
					Description: "Complex model",
					Versions: map[string]config.DataModelVersion{
						"v1": complexModel,
					},
				},
			}

			// Create a cancelled context
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			err := validator.ValidateWithReferences(cancelledCtx, complexModel, allDataModels)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})

		It("should respect maximum recursion depth", func() {
			// Create a chain of models that reference each other in a linear fashion
			// This creates a deep chain without cycles
			models := make(map[string]config.DataModelsConfig)

			// Create the deepest model (base case)
			models["model10"] = config.DataModelsConfig{
				Name:        "model10",
				Description: "Model 10",
				Versions: map[string]config.DataModelVersion{
					"v1": {
						Description: "Model 10 version 1",
						Structure: map[string]config.Field{
							"value": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			// Create chain of models, each referencing the next
			for i := 9; i >= 0; i-- {
				modelName := fmt.Sprintf("model%d", i)
				nextModelName := fmt.Sprintf("model%d", i+1)

				models[modelName] = config.DataModelsConfig{
					Name:        modelName,
					Description: fmt.Sprintf("Model %d", i),
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Description: fmt.Sprintf("Model %d version 1", i),
							Structure: map[string]config.Field{
								"next": {
									ModelRef: &config.ModelRef{
										Name:    nextModelName,
										Version: "v1",
									},
								},
							},
						},
					},
				}
			}

			// This should work (10 levels deep)
			err := validator.ValidateWithReferences(ctx, models["model0"].Versions["v1"], models)
			Expect(err).To(BeNil())

			// Now create an even deeper chain (11+ levels) to test the limit
			models["model11"] = config.DataModelsConfig{
				Name:        "model11",
				Description: "Model 11",
				Versions: map[string]config.DataModelVersion{
					"v1": {
						Description: "Model 11 version 1",
						Structure: map[string]config.Field{
							"value": {
								Type: "timeseries-number",
							},
						},
					},
				},
			}

			// Update model10 to reference model11
			models["model10"].Versions["v1"] = config.DataModelVersion{
				Description: "Model 10 version 1",
				Structure: map[string]config.Field{
					"next": {
						ModelRef: &config.ModelRef{
							Name:    "model11",
							Version: "v1",
						},
					},
				},
			}

			// This should fail due to depth limit
			err = validator.ValidateWithReferences(ctx, models["model0"].Versions["v1"], models)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maximum reference depth exceeded"))
		})
	})
})
