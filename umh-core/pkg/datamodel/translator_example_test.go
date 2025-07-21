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

var _ = Describe("Translator Examples", func() {

	Context("Complete Example from ENG-3100", func() {
		It("should produce the exact JSON schema format expected by benthos-umh", func() {
			translator := datamodel.NewTranslator()
			ctx := context.Background()

			// Use empty payload shapes since defaults are auto-injected by the translator
			payloadShapes := map[string]config.PayloadShape{}

			// Define the data model structure as it would appear in YAML
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
						},
					},
				},
			}

			// Translate to JSON schemas
			result, err := translator.TranslateDataModel(
				ctx,
				"_pump_data",
				"v1",
				dataModel,
				payloadShapes,
				nil, // No model references
			)

			Expect(err).To(BeNil())

			// Get the generated schema
			subjectName := "_pump_data_v1-timeseries-number"
			schema, exists := result.Schemas[subjectName]
			Expect(exists).To(BeTrue())

			// Print the result for demonstration
			jsonBytes, err := result.GetSchemaAsJSON(subjectName)
			Expect(err).To(BeNil())

			GinkgoWriter.Printf("\n=== Translation Example ===\n")
			GinkgoWriter.Printf("Input YAML Structure:\n")
			GinkgoWriter.Printf("  count:\n")
			GinkgoWriter.Printf("    _payloadshape: timeseries-number\n")
			GinkgoWriter.Printf("  vibration:\n")
			GinkgoWriter.Printf("    x-axis:\n")
			GinkgoWriter.Printf("      _payloadshape: timeseries-number\n\n")

			GinkgoWriter.Printf("Generated Schema Registry Subject: %s\n\n", subjectName)
			GinkgoWriter.Printf("Generated JSON Schema:\n%s\n\n", string(jsonBytes))

			// Verify the exact structure matches expectations (check individual fields to avoid map ordering issues)
			Expect(schema["type"]).To(Equal("object"))
			Expect(schema["additionalProperties"]).To(Equal(false))
			Expect(schema["required"]).To(ConsistOf("virtual_path", "fields"))

			properties, ok := schema["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			// Check virtual_path property
			virtualPath, ok := properties["virtual_path"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(virtualPath["type"]).To(Equal("string"))
			Expect(virtualPath["enum"]).To(ConsistOf("count", "vibration.x-axis"))

			// Check fields property
			fields, ok := properties["fields"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(fields["type"]).To(Equal("object"))
			Expect(fields["additionalProperties"]).To(Equal(false))
			Expect(fields["required"]).To(ConsistOf("timestamp_ms", "value"))

			fieldsProps, ok := fields["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			timestampMs, ok := fieldsProps["timestamp_ms"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(timestampMs["type"]).To(Equal("number"))

			value, ok := fieldsProps["value"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(value["type"]).To(Equal("number"))

			// Verify this schema would validate the example UNS message:
			// {"timestamp_ms": 1719859200000, "value": 0.5}
			// with virtual_path: "vibration.x-axis"
			GinkgoWriter.Printf("This schema validates UNS messages like:\n")
			GinkgoWriter.Printf("{\n")
			GinkgoWriter.Printf("  \"virtual_path\": \"vibration.x-axis\",\n")
			GinkgoWriter.Printf("  \"fields\": {\n")
			GinkgoWriter.Printf("    \"timestamp_ms\": 1719859200000,\n")
			GinkgoWriter.Printf("    \"value\": 0.5\n")
			GinkgoWriter.Printf("  }\n")
			GinkgoWriter.Printf("}\n\n")
		})

		It("should handle model references correctly", func() {
			translator := datamodel.NewTranslator()
			ctx := context.Background()

			// Use empty payload shapes since defaults are auto-injected by the translator
			payloadShapes := map[string]config.PayloadShape{}

			// Motor model definition
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

			// Main model with reference (new format from ENG-3100)
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

			subjectName := "_pump_data_v1-timeseries-number"
			jsonBytes, err := result.GetSchemaAsJSON(subjectName)
			Expect(err).To(BeNil())

			GinkgoWriter.Printf("=== Model Reference Example ===\n")
			GinkgoWriter.Printf("YAML with _refModel (new format):\n")
			GinkgoWriter.Printf("  motor:\n")
			GinkgoWriter.Printf("    _refModel:\n")
			GinkgoWriter.Printf("      name: motor\n")
			GinkgoWriter.Printf("      version: v1\n")
			GinkgoWriter.Printf("  count:\n")
			GinkgoWriter.Printf("    _payloadshape: timeseries-number\n\n")

			GinkgoWriter.Printf("Generated Schema (flattened virtual paths):\n%s\n\n", string(jsonBytes))

			// Should include all flattened paths
			expectedPaths := []string{"count", "motor.speed", "motor.temperature"}
			Expect(result.PayloadShapeUsage["timeseries-number"]).To(ConsistOf(expectedPaths))

			GinkgoWriter.Printf("Virtual paths extracted: %v\n\n", result.PayloadShapeUsage["timeseries-number"])
		})
	})
})
