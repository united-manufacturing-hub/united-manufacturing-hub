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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
)

var _ = Describe("Translator", func() {
	var (
		translator *datamodel.Translator
		ctx        context.Context
	)

	BeforeEach(func() {
		translator = datamodel.NewTranslator()
		ctx = context.Background()
	})

	Describe("ParseTypeInfo", func() {
		It("should parse valid type strings correctly", func() {
			tests := []struct {
				input    string
				expected datamodel.TypeInfo
			}{
				{
					input: "timeseries-number",
					expected: datamodel.TypeInfo{
						Category: datamodel.TypeCategoryTimeseries,
						SubType:  "number",
						FullType: "timeseries-number",
					},
				},
				{
					input: "timeseries-string",
					expected: datamodel.TypeInfo{
						Category: datamodel.TypeCategoryTimeseries,
						SubType:  "string",
						FullType: "timeseries-string",
					},
				},
				{
					input: "relational-table",
					expected: datamodel.TypeInfo{
						Category: datamodel.TypeCategoryRelational,
						SubType:  "table",
						FullType: "relational-table",
					},
				},
			}

			for _, test := range tests {
				result := datamodel.ParseTypeInfo(test.input)
				Expect(result).To(Equal(test.expected))
			}
		})

		It("should return empty TypeInfo for invalid type strings", func() {
			tests := []string{
				"invalidtype",     // No dash
				"",                // Empty string
				"nodash",          // No dash
				"-startswithdash", // Starts with dash
				"endswith-",       // Ends with dash
			}

			for _, test := range tests {
				result := datamodel.ParseTypeInfo(test)
				Expect(result).To(Equal(datamodel.TypeInfo{}))
			}
		})
	})

	Describe("Simple translation", func() {
		It("should translate a simple data model with timeseries fields", func() {
			// Create a simple data model
			dataModel := config.DataModelVersion{
				Description: "Simple pump model",
				Structure: map[string]config.Field{
					"temperature": {
						Type:        "timeseries-number",
						Description: "Temperature reading",
						Unit:        "°C",
					},
					"status": {
						Type:        "timeseries-string",
						Description: "Status string",
					},
					"active": {
						Type:        "timeseries-boolean",
						Description: "Is active",
					},
				},
			}

			// Translate to JSON schemas
			schemas, err := translator.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).ToNot(HaveOccurred())
			Expect(schemas).To(HaveLen(3)) // number, string, boolean

			// Verify schema names
			schemaNames := make(map[string]bool)
			for _, schema := range schemas {
				schemaNames[schema.Name] = true
			}
			Expect(schemaNames).To(HaveKey("_pump-v1-number"))
			Expect(schemaNames).To(HaveKey("_pump-v1-string"))
			Expect(schemaNames).To(HaveKey("_pump-v1-boolean"))

			// Verify schema content for number type
			var numberSchema *datamodel.SchemaOutput
			for _, schema := range schemas {
				if schema.Name == "_pump-v1-number" {
					numberSchema = &schema
					break
				}
			}
			Expect(numberSchema).ToNot(BeNil())

			// Parse and verify the JSON schema structure
			var parsedSchema map[string]interface{}
			err = json.Unmarshal([]byte(numberSchema.Schema), &parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			// Verify schema structure
			Expect(parsedSchema["type"]).To(Equal("object"))
			Expect(parsedSchema["required"]).To(Equal([]interface{}{"virtual_path", "fields"}))
			Expect(parsedSchema["additionalProperties"]).To(Equal(false))

			// Verify virtual_path
			properties := parsedSchema["properties"].(map[string]interface{})
			virtualPath := properties["virtual_path"].(map[string]interface{})
			Expect(virtualPath["type"]).To(Equal("string"))
			Expect(virtualPath["enum"]).To(Equal([]interface{}{"temperature"}))

			// Verify fields.value structure
			fields := properties["fields"].(map[string]interface{})
			fieldsProps := fields["properties"].(map[string]interface{})
			value := fieldsProps["value"].(map[string]interface{})
			valueProps := value["properties"].(map[string]interface{})

			timestampMs := valueProps["timestamp_ms"].(map[string]interface{})
			Expect(timestampMs["type"]).To(Equal("number"))

			valueField := valueProps["value"].(map[string]interface{})
			Expect(valueField["type"]).To(Equal("number"))
		})
	})

	Describe("Nested structure translation", func() {
		It("should translate nested structures correctly", func() {
			dataModel := config.DataModelVersion{
				Description: "Nested pump model",
				Structure: map[string]config.Field{
					"vibration": {
						Subfields: map[string]config.Field{
							"x-axis": {
								Type:        "timeseries-number",
								Description: "X-axis vibration",
								Unit:        "mm/s",
							},
							"y-axis": {
								Type:        "timeseries-number",
								Description: "Y-axis vibration",
								Unit:        "mm/s",
							},
						},
					},
					"serialNumber": {
						Type:        "timeseries-string",
						Description: "Serial number",
					},
				},
			}

			schemas, err := translator.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).ToNot(HaveOccurred())
			Expect(schemas).To(HaveLen(2)) // number, string

			// Find the number schema
			var numberSchema *datamodel.SchemaOutput
			for _, schema := range schemas {
				if schema.Name == "_pump-v1-number" {
					numberSchema = &schema
					break
				}
			}
			Expect(numberSchema).ToNot(BeNil())

			// Parse and verify the paths
			var parsedSchema map[string]interface{}
			err = json.Unmarshal([]byte(numberSchema.Schema), &parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			properties := parsedSchema["properties"].(map[string]interface{})
			virtualPath := properties["virtual_path"].(map[string]interface{})
			enum := virtualPath["enum"].([]interface{})

			// Should contain nested paths
			Expect(enum).To(ContainElement("vibration.x-axis"))
			Expect(enum).To(ContainElement("vibration.y-axis"))
		})
	})

	Describe("Reference resolution", func() {
		It("should resolve references correctly", func() {
			// Create a motor model
			motorModel := config.DataModelVersion{
				Description: "Motor model",
				Structure: map[string]config.Field{
					"speed": {
						Type:        "timeseries-number",
						Description: "Motor speed",
						Unit:        "rpm",
					},
					"model": {
						Type:        "timeseries-string",
						Description: "Motor model",
					},
				},
			}

			// Create a pump model that references the motor
			pumpModel := config.DataModelVersion{
				Description: "Pump with motor",
				Structure: map[string]config.Field{
					"temperature": {
						Type:        "timeseries-number",
						Description: "Temperature",
						Unit:        "°C",
					},
					"motor": {
						ModelRef: "motor:v1",
					},
				},
			}

			// Create the all models map
			allModels := map[string]config.DataModelsConfig{
				"motor": {
					Name: "motor",
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
			}

			schemas, err := translator.TranslateToJSONSchema(ctx, pumpModel, "pump", "v1", allModels)
			Expect(err).ToNot(HaveOccurred())
			Expect(schemas).To(HaveLen(2)) // number, string

			// Find the number schema
			var numberSchema *datamodel.SchemaOutput
			for _, schema := range schemas {
				if schema.Name == "_pump-v1-number" {
					numberSchema = &schema
					break
				}
			}
			Expect(numberSchema).ToNot(BeNil())

			// Parse and verify the paths include referenced fields
			var parsedSchema map[string]interface{}
			err = json.Unmarshal([]byte(numberSchema.Schema), &parsedSchema)
			Expect(err).ToNot(HaveOccurred())

			properties := parsedSchema["properties"].(map[string]interface{})
			virtualPath := properties["virtual_path"].(map[string]interface{})
			enum := virtualPath["enum"].([]interface{})

			// Should contain paths from both pump and motor (with motor prefix)
			Expect(enum).To(ContainElement("temperature"))
			Expect(enum).To(ContainElement("motor.speed"))
		})
	})

	Describe("Error handling", func() {
		It("should handle missing referenced models", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"motor": {
						ModelRef: "nonexistent:v1",
					},
				},
			}

			_, err := translator.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced model 'nonexistent' does not exist"))
		})

		It("should handle missing referenced model versions", func() {
			motorModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"speed": {
						Type: "timeseries-number",
					},
				},
			}

			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"motor": {
						ModelRef: "motor:v2", // v2 doesn't exist
					},
				},
			}

			allModels := map[string]config.DataModelsConfig{
				"motor": {
					Name: "motor",
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
			}

			_, err := translator.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", allModels)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced model 'motor' version 'v2' does not exist"))
		})

		It("should handle circular references", func() {
			// Create circular reference: pump -> motor -> pump
			pumpModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"motor": {
						ModelRef: "motor:v1",
					},
				},
			}

			motorModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"pump": {
						ModelRef: "pump:v1",
					},
				},
			}

			allModels := map[string]config.DataModelsConfig{
				"pump": {
					Name: "pump",
					Versions: map[string]config.DataModelVersion{
						"v1": pumpModel,
					},
				},
				"motor": {
					Name: "motor",
					Versions: map[string]config.DataModelVersion{
						"v1": motorModel,
					},
				},
			}

			_, err := translator.TranslateToJSONSchema(ctx, pumpModel, "pump", "v1", allModels)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circular reference detected"))
		})

		It("should handle invalid _refModel format", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"motor": {
						ModelRef: "invalid-format", // Missing colon
					},
				},
			}

			_, err := translator.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid _refModel format"))
		})
	})

	Describe("Context cancellation", func() {
		It("should respect context cancellation", func() {
			// Create a context that's already cancelled
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"temperature": {
						Type: "timeseries-number",
					},
				},
			}

			_, err := translator.TranslateToJSONSchema(cancelledCtx, dataModel, "pump", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})

		It("should respect context timeout", func() {
			// Create a context with very short timeout
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
			defer cancel()

			// Wait a bit to ensure timeout
			time.Sleep(1 * time.Millisecond)

			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"temperature": {
						Type: "timeseries-number",
					},
				},
			}

			_, err := translator.TranslateToJSONSchema(timeoutCtx, dataModel, "pump", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.DeadlineExceeded))
		})
	})

	Describe("Relational translator stub", func() {
		It("should handle relational types with stub implementation", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"inventory": {
						Type:        "relational-table",
						Description: "Inventory table",
					},
				},
			}

			schemas, err := translator.TranslateToJSONSchema(ctx, dataModel, "warehouse", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).ToNot(HaveOccurred())
			Expect(schemas).To(HaveLen(1))

			schema := schemas[0]
			Expect(schema.Name).To(Equal("_warehouse-v1-relational-table"))
			Expect(schema.Schema).To(ContainSubstring("Relational schema generation not yet implemented"))
		})
	})

	Describe("Unsupported types", func() {
		It("should handle unsupported type categories", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"unknown": {
						Type:        "unsupported-type",
						Description: "Unknown type",
					},
				},
			}

			_, err := translator.TranslateToJSONSchema(ctx, dataModel, "test", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported type category: unsupported"))
		})

		It("should handle unsupported timeseries subtypes", func() {
			dataModel := config.DataModelVersion{
				Structure: map[string]config.Field{
					"unknown": {
						Type:        "timeseries-unsupported",
						Description: "Unknown timeseries type",
					},
				},
			}

			_, err := translator.TranslateToJSONSchema(ctx, dataModel, "test", "v1", make(map[string]config.DataModelsConfig))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported timeseries subtype: unsupported"))
		})
	})
})
