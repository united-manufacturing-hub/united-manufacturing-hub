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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
)

var _ = Describe("Translator", func() {
	var (
		translator    *datamodel.Translator
		ctx           context.Context
		payloadShapes map[string]config.PayloadShape
	)

	BeforeEach(func() {
		translator = datamodel.NewTranslator()
		ctx = context.Background()

		// payloadShapes is now empty by default since the translator auto-injects
		// the standard timeseries-number and timeseries-string payload shapes
		payloadShapes = map[string]config.PayloadShape{}
	})

	Context("TranslateDataModel", func() {
		type translationTestCase struct {
			description      string
			contractName     string
			version          string
			dataModel        config.DataModelVersion
			expectedSchemas  int
			expectedSubjects []string
			expectedPaths    map[string][]string
		}

		DescribeTable("basic translation scenarios",
			func(tc translationTestCase) {
				result, err := translator.TranslateDataModel(
					ctx,
					tc.contractName,
					tc.version,
					tc.dataModel,
					payloadShapes,
					nil, // No model references
				)

				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Schemas).To(HaveLen(tc.expectedSchemas))

				// Check expected subjects exist
				for _, subject := range tc.expectedSubjects {
					schema, exists := result.Schemas[subject]
					Expect(exists).To(BeTrue(), "Subject %s should exist", subject)
					Expect(schema["type"]).To(Equal("object"))
				}

				// Check payload shape usage
				for shape, expectedPaths := range tc.expectedPaths {
					Expect(result.PayloadShapeUsage[shape]).To(ConsistOf(expectedPaths),
						"Payload shape %s should have paths %v", shape, expectedPaths)
				}
			},
			Entry("simple data model with nested fields", translationTestCase{
				description:  "should translate a simple data model to JSON schemas",
				contractName: "_pump_data",
				version:      "v1",
				dataModel: config.DataModelVersion{
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
							},
						},
					},
				},
				expectedSchemas:  1,
				expectedSubjects: []string{"_pump_data_v1-timeseries-number"},
				expectedPaths: map[string][]string{
					"timeseries-number": {"count", "vibration.x-axis", "vibration.y-axis"},
				},
			}),
			Entry("multiple payload shapes", translationTestCase{
				description:  "should handle multiple payload shapes",
				contractName: "_pump_data",
				version:      "v1",
				dataModel: config.DataModelVersion{
					Structure: map[string]config.Field{
						"count": {
							PayloadShape: "timeseries-number",
						},
						"serialNumber": {
							PayloadShape: "timeseries-string",
						},
					},
				},
				expectedSchemas: 2,
				expectedSubjects: []string{
					"_pump_data_v1-timeseries-number",
					"_pump_data_v1-timeseries-string",
				},
				expectedPaths: map[string][]string{
					"timeseries-number": {"count"},
					"timeseries-string": {"serialNumber"},
				},
			}),
			Entry("contract name without underscore prefix", translationTestCase{
				description:  "should generate proper schema registry subject names",
				contractName: "motor_data", // Without underscore prefix
				version:      "v2",
				dataModel: config.DataModelVersion{
					Structure: map[string]config.Field{
						"temperature": {
							PayloadShape: "timeseries-number",
						},
					},
				},
				expectedSchemas:  1,
				expectedSubjects: []string{"_motor_data_v2-timeseries-number"},
				expectedPaths: map[string][]string{
					"timeseries-number": {"temperature"},
				},
			}),
		)

		type errorTestCase struct {
			description      string
			contractName     string
			version          string
			dataModel        config.DataModelVersion
			expectedErrorMsg string
		}

		DescribeTable("error scenarios",
			func(tc errorTestCase) {
				result, err := translator.TranslateDataModel(
					ctx,
					tc.contractName,
					tc.version,
					tc.dataModel,
					payloadShapes,
					nil, // No model references
				)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring(tc.expectedErrorMsg))
			},
			Entry("invalid data model", errorTestCase{
				description:  "should validate the data model before translation",
				contractName: "_invalid_data",
				version:      "v1",
				dataModel: config.DataModelVersion{
					Structure: map[string]config.Field{
						"invalid": {}, // No PayloadShape or ModelRef
					},
				},
				expectedErrorMsg: "data model validation failed",
			}),
		)

		It("should return JSON that can be marshaled", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"count": {
						PayloadShape: "timeseries-number",
					},
				},
			}

			result, err := translator.TranslateDataModel(
				ctx,
				"_pump_data",
				"v1",
				dataModel,
				payloadShapes,
				nil, // No model references
			)

			Expect(err).To(BeNil())

			subjectName := "_pump_data_v1-timeseries-number"
			jsonBytes, err := result.GetSchemaAsJSON(subjectName)
			Expect(err).To(BeNil())
			Expect(jsonBytes).ToNot(BeEmpty())

			// Verify it's valid JSON
			var parsedSchema map[string]interface{}
			err = json.Unmarshal(jsonBytes, &parsedSchema)
			Expect(err).To(BeNil())
			Expect(parsedSchema["type"]).To(Equal("object"))
		})

		It("should respect context cancellation", func() {
			// Create a cancelled context
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"temperature": {
						PayloadShape: "timeseries-number",
					},
				},
			}

			result, err := translator.TranslateDataModel(
				cancelledCtx,
				"_pump_data",
				"v1",
				dataModel,
				payloadShapes,
				nil, // No model references
			)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})

		type defaultPayloadShapesTestCase struct {
			description         string
			payloadShapes       map[string]config.PayloadShape
			dataModel           config.DataModelVersion
			expectedSchemas     int
			verifyDefaultFields bool
			verifyCustomFields  bool
			customFieldsToCheck []string
		}

		DescribeTable("default payload shapes injection",
			func(tc defaultPayloadShapesTestCase) {
				result, err := translator.TranslateDataModel(
					ctx,
					"_pump_data",
					"v1",
					tc.dataModel,
					tc.payloadShapes,
					nil, // No model references
				)

				Expect(err).To(BeNil())
				Expect(result).ToNot(BeNil())
				Expect(result.Schemas).To(HaveLen(tc.expectedSchemas))

				if tc.verifyDefaultFields {
					// Verify both default payload shapes were used successfully
					numberSubject := "_pump_data_v1-timeseries-number"
					stringSubject := "_pump_data_v1-timeseries-string"

					numberSchema, numberExists := result.Schemas[numberSubject]
					stringSchema, stringExists := result.Schemas[stringSubject]

					Expect(numberExists).To(BeTrue())
					Expect(stringExists).To(BeTrue())

					// Verify the injected timeseries-number schema structure
					numberProps := numberSchema["properties"].(map[string]interface{})
					numberFields := numberProps["fields"].(map[string]interface{})
					numberFieldProps := numberFields["properties"].(map[string]interface{})

					timestampField := numberFieldProps["timestamp_ms"].(map[string]interface{})
					Expect(timestampField["type"]).To(Equal("number"))

					valueField := numberFieldProps["value"].(map[string]interface{})
					Expect(valueField["type"]).To(Equal("number"))

					// Verify the injected timeseries-string schema structure
					stringProps := stringSchema["properties"].(map[string]interface{})
					stringFields := stringProps["fields"].(map[string]interface{})
					stringFieldProps := stringFields["properties"].(map[string]interface{})

					timestampField = stringFieldProps["timestamp_ms"].(map[string]interface{})
					Expect(timestampField["type"]).To(Equal("number"))

					valueField = stringFieldProps["value"].(map[string]interface{})
					Expect(valueField["type"]).To(Equal("string"))

					// Verify payload shape usage
					Expect(result.PayloadShapeUsage["timeseries-number"]).To(ConsistOf("count"))
					Expect(result.PayloadShapeUsage["timeseries-string"]).To(ConsistOf("serialNumber"))
				}

				if tc.verifyCustomFields {
					// Verify the custom payload shape was used (not the default)
					subjectName := "_pump_data_v1-timeseries-number"
					schema, exists := result.Schemas[subjectName]
					Expect(exists).To(BeTrue())

					props := schema["properties"].(map[string]interface{})
					fields := props["fields"].(map[string]interface{})
					fieldProps := fields["properties"].(map[string]interface{})

					// Check for custom fields
					for _, fieldName := range tc.customFieldsToCheck {
						_, hasField := fieldProps[fieldName]
						Expect(hasField).To(BeTrue(), "Should have custom field: %s", fieldName)
					}

					// Verify default field is not present
					_, hasDefaultTimestamp := fieldProps["timestamp_ms"]
					Expect(hasDefaultTimestamp).To(BeFalse(), "Default field should not be present")
				}
			},
			Entry("empty payload shapes map - defaults injected", defaultPayloadShapesTestCase{
				description:   "should automatically inject default payload shapes when not provided",
				payloadShapes: map[string]config.PayloadShape{}, // Empty map
				dataModel: config.DataModelVersion{
					Structure: map[string]config.Field{
						"count": {
							PayloadShape: "timeseries-number",
						},
						"serialNumber": {
							PayloadShape: "timeseries-string",
						},
					},
				},
				expectedSchemas:     2,
				verifyDefaultFields: true,
			}),
			Entry("custom payload shapes - not overridden", defaultPayloadShapesTestCase{
				description: "should not override existing payload shapes with defaults",
				payloadShapes: map[string]config.PayloadShape{
					"timeseries-number": {
						Fields: map[string]config.PayloadField{
							"custom_timestamp": {Type: "number"},
							"custom_value":     {Type: "number"},
							"extra_field":      {Type: "string"},
						},
					},
				},
				dataModel: config.DataModelVersion{
					Structure: map[string]config.Field{
						"count": {
							PayloadShape: "timeseries-number",
						},
					},
				},
				expectedSchemas:     1,
				verifyCustomFields:  true,
				customFieldsToCheck: []string{"custom_timestamp", "extra_field"},
			}),
		)
	})

	Context("TranslateDataModelWithReferences", func() {
		It("should resolve model references and flatten to virtual paths", func() {
			// Create referenced model
			motorModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"speed": {
						PayloadShape: "timeseries-number",
					},
					"temperature": {
						PayloadShape: "timeseries-number",
					},
				},
			}

			// Create main data model with reference
			mainModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"motor": {
						ModelRef: &config.ModelRef{
							Name:    "motor",
							Version: "v1",
						},
					},
					"count": {
						PayloadShape: "timeseries-number",
					},
				},
			}

			allDataModels := map[string]config.DataModelsConfig{
				"motor": {
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
			}

			result, err := translator.TranslateDataModel(
				ctx,
				"_pump_data",
				"v1",
				mainModel,
				payloadShapes,
				allDataModels, // Provide model references
			)

			Expect(err).To(BeNil())
			Expect(result.Schemas).To(HaveLen(1)) // Only timeseries-number

			// Check that all virtual paths are included (flattened from reference)
			expectedPaths := []string{"count", "motor.speed", "motor.temperature"}
			Expect(result.PayloadShapeUsage["timeseries-number"]).To(ConsistOf(expectedPaths))
		})

		type circularReferenceTestCase struct {
			description      string
			models           map[string]config.DataModelsConfig
			mainModel        config.DataModelVersion
			expectedErrorMsg string
		}

		DescribeTable("circular reference detection",
			func(tc circularReferenceTestCase) {
				result, err := translator.TranslateDataModel(
					ctx,
					"_test_data",
					"v1",
					tc.mainModel,
					payloadShapes,
					tc.models,
				)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring(tc.expectedErrorMsg))
			},
			Entry("direct self-reference", circularReferenceTestCase{
				description: "should detect direct self-reference (A -> A)",
				models: map[string]config.DataModelsConfig{
					"selfRef": {
						Versions: map[string]config.DataModelVersion{
							"v1": {
								Structure: map[string]config.Field{
									"self": {
										ModelRef: &config.ModelRef{
											Name:    "selfRef",
											Version: "v1",
										},
									},
								},
							},
						},
					},
				},
				mainModel: config.DataModelVersion{
					Structure: map[string]config.Field{
						"myRef": {
							ModelRef: &config.ModelRef{
								Name:    "selfRef",
								Version: "v1",
							},
						},
					},
				},
				expectedErrorMsg: "circular reference detected: selfRef:v1",
			}),
			Entry("simple circular reference", circularReferenceTestCase{
				description: "should detect simple circular reference (A -> B -> A)",
				models: map[string]config.DataModelsConfig{
					"modelA": {
						Versions: map[string]config.DataModelVersion{
							"v1": {
								Structure: map[string]config.Field{
									"refToB": {
										ModelRef: &config.ModelRef{
											Name:    "modelB",
											Version: "v1",
										},
									},
								},
							},
						},
					},
					"modelB": {
						Versions: map[string]config.DataModelVersion{
							"v1": {
								Structure: map[string]config.Field{
									"refToA": {
										ModelRef: &config.ModelRef{
											Name:    "modelA",
											Version: "v1",
										},
									},
								},
							},
						},
					},
				},
				mainModel: config.DataModelVersion{
					Structure: map[string]config.Field{
						"start": {
							ModelRef: &config.ModelRef{
								Name:    "modelA",
								Version: "v1",
							},
						},
					},
				},
				expectedErrorMsg: "circular reference detected",
			}),
			Entry("complex circular reference", circularReferenceTestCase{
				description: "should detect complex circular reference (A -> B -> C -> A)",
				models: map[string]config.DataModelsConfig{
					"modelA": {
						Versions: map[string]config.DataModelVersion{
							"v1": {
								Structure: map[string]config.Field{
									"refToB": {
										ModelRef: &config.ModelRef{
											Name:    "modelB",
											Version: "v1",
										},
									},
								},
							},
						},
					},
					"modelB": {
						Versions: map[string]config.DataModelVersion{
							"v1": {
								Structure: map[string]config.Field{
									"refToC": {
										ModelRef: &config.ModelRef{
											Name:    "modelC",
											Version: "v1",
										},
									},
								},
							},
						},
					},
					"modelC": {
						Versions: map[string]config.DataModelVersion{
							"v1": {
								Structure: map[string]config.Field{
									"refToA": {
										ModelRef: &config.ModelRef{
											Name:    "modelA",
											Version: "v1",
										},
									},
								},
							},
						},
					},
				},
				mainModel: config.DataModelVersion{
					Structure: map[string]config.Field{
						"start": {
							ModelRef: &config.ModelRef{
								Name:    "modelA",
								Version: "v1",
							},
						},
					},
				},
				expectedErrorMsg: "circular reference detected",
			}),
		)

		It("should handle deep nested references without circular loops", func() {
			// Create a deep but valid reference chain (no loops)
			allDataModels := map[string]config.DataModelsConfig{
				"level1": {
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"level2Ref": {
									ModelRef: &config.ModelRef{
										Name:    "level2",
										Version: "v1",
									},
								},
							},
						},
					},
				},
				"level2": {
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"level3Ref": {
									ModelRef: &config.ModelRef{
										Name:    "level3",
										Version: "v1",
									},
								},
							},
						},
					},
				},
				"level3": {
					Versions: map[string]config.DataModelVersion{
						"v1": {
							Structure: map[string]config.Field{
								"finalField": {
									PayloadShape: "timeseries-number",
								},
							},
						},
					},
				},
			}

			mainModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"start": {
						ModelRef: &config.ModelRef{
							Name:    "level1",
							Version: "v1",
						},
					},
					"directField": {
						PayloadShape: "timeseries-string",
					},
				},
			}

			result, err := translator.TranslateDataModel(
				ctx,
				"_deep_test",
				"v1",
				mainModel,
				payloadShapes,
				allDataModels,
			)

			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

			// Should resolve the deep chain correctly
			numberPaths := result.PayloadShapeUsage["timeseries-number"]
			stringPaths := result.PayloadShapeUsage["timeseries-string"]

			Expect(numberPaths).To(ConsistOf("start.level2Ref.level3Ref.finalField"))
			Expect(stringPaths).To(ConsistOf("directField"))
		})

		It("should respect the depth limit for very deep references", func() {
			// Create models that would exceed the depth limit (currently 10)
			allDataModels := map[string]config.DataModelsConfig{}

			// Create a chain of 12 models to exceed the limit
			for i := 1; i <= 12; i++ {
				modelName := fmt.Sprintf("level%d", i)
				var structure map[string]config.Field

				if i == 12 {
					// Final level - just a payload shape
					structure = map[string]config.Field{
						"finalField": {
							PayloadShape: "timeseries-number",
						},
					}
				} else {
					// Reference to next level
					structure = map[string]config.Field{
						"nextRef": {
							ModelRef: &config.ModelRef{
								Name:    fmt.Sprintf("level%d", i+1),
								Version: "v1",
							},
						},
					}
				}

				allDataModels[modelName] = config.DataModelsConfig{
					Versions: map[string]config.DataModelVersion{
						"v1": {Structure: structure},
					},
				}
			}

			mainModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"start": {
						ModelRef: &config.ModelRef{
							Name:    "level1",
							Version: "v1",
						},
					},
				},
			}

			result, err := translator.TranslateDataModel(
				ctx,
				"_depth_test",
				"v1",
				mainModel,
				payloadShapes,
				allDataModels,
			)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("depth limit exceeded"))
		})
	})
})
