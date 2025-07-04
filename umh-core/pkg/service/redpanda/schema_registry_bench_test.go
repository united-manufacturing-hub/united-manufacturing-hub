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

package redpanda

import (
	"context"
	"fmt"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// BenchmarkParseSchemaName benchmarks the schema name parsing function
func BenchmarkParseSchemaName(b *testing.B) {
	testCases := []string{
		"pump_v1_timeseries-number",
		"motor_v2_timeseries-string",
		"sensor_v10_timeseries-bool",
		"complex_name_v99_timeseries-number",
		"invalid_format",
		"pump_v1",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, testCase := range testCases {
			_, _, _, _, _ = parseSchemaName(testCase)
		}
	}
}

// BenchmarkParseSchemaNameSingle benchmarks parsing a single schema name
func BenchmarkParseSchemaNameSingle(b *testing.B) {
	schemaName := "pump_v1_timeseries-number"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _, _, _ = parseSchemaName(schemaName)
	}
}

// BenchmarkGenerateExpectedSchemaNames benchmarks schema name generation
func BenchmarkGenerateExpectedSchemaNames(b *testing.B) {
	// Create test data models
	dataModels := createBenchmarkDataModels()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = GenerateExpectedSchemaNames(dataModels)
	}
}

// BenchmarkGenerateExpectedSchemaNamesLarge benchmarks with larger dataset
func BenchmarkGenerateExpectedSchemaNamesLarge(b *testing.B) {
	// Create larger test data models
	dataModels := createLargeBenchmarkDataModels()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = GenerateExpectedSchemaNames(dataModels)
	}
}

// BenchmarkCompareDataModelsWithRegistry benchmarks the comparison function
func BenchmarkCompareDataModelsWithRegistry(b *testing.B) {
	// Set up mock service
	mockService := NewMockRedpandaService()
	registrySchemas := createBenchmarkRegistrySchemas()
	mockService.GetAllSchemasResult = registrySchemas

	dataModels := createBenchmarkDataModels()
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mapping, err := mockService.CompareDataModelsWithRegistry(ctx, dataModels)
		if err != nil {
			b.Fatal(err)
		}
		_ = mapping // Use the result to prevent compiler optimization
	}
}

// BenchmarkGetAllSchemas benchmarks schema registry fetching with mock
func BenchmarkGetAllSchemas(b *testing.B) {
	// Set up mock service
	mockService := NewMockRedpandaService()
	mockService.GetAllSchemasResult = createBenchmarkRegistrySchemas()
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		schemas, err := mockService.GetAllSchemas(ctx)
		if err != nil {
			b.Fatal(err)
		}
		_ = schemas // Use the result to prevent compiler optimization
	}
}

// Helper functions to create benchmark data

func createBenchmarkDataModels() map[string]config.DataModelsConfig {
	return map[string]config.DataModelsConfig{
		"pump": {
			Name: "pump",
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"pressure": {
							Type: "timeseries-number",
						},
						"status": {
							Type: "timeseries-string",
						},
						"active": {
							Type: "timeseries-bool",
						},
						"metadata": {
							Subfields: map[string]config.Field{
								"temperature": {
									Type: "timeseries-number",
								},
								"location": {
									Type: "timeseries-string",
								},
							},
						},
					},
				},
				"v2": {
					Structure: map[string]config.Field{
						"pressure": {
							Type: "timeseries-number",
						},
						"flow_rate": {
							Type: "timeseries-number",
						},
					},
				},
			},
		},
		"motor": {
			Name: "motor",
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"rpm": {
							Type: "timeseries-number",
						},
						"power": {
							Type: "timeseries-number",
						},
						"running": {
							Type: "timeseries-bool",
						},
					},
				},
			},
		},
		"sensor": {
			Name: "sensor",
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"value": {
							Type: "timeseries-number",
						},
						"unit": {
							Type: "timeseries-string",
						},
					},
				},
			},
		},
	}
}

func createLargeBenchmarkDataModels() map[string]config.DataModelsConfig {
	dataModels := make(map[string]config.DataModelsConfig)

	// Create 100 different data models with multiple versions
	for i := 0; i < 100; i++ {
		modelName := fmt.Sprintf("model_%d", i)
		versions := make(map[string]config.DataModelVersion)

		// Each model has 5 versions
		for v := 1; v <= 5; v++ {
			versionName := fmt.Sprintf("v%d", v)
			structure := make(map[string]config.Field)

			// Each version has 10 fields of different types
			for f := 0; f < 10; f++ {
				fieldName := fmt.Sprintf("field_%d", f)
				fieldType := "timeseries-number"
				if f%3 == 1 {
					fieldType = "timeseries-string"
				} else if f%3 == 2 {
					fieldType = "timeseries-bool"
				}

				structure[fieldName] = config.Field{Type: fieldType}
			}

			versions[versionName] = config.DataModelVersion{Structure: structure}
		}

		dataModels[modelName] = config.DataModelsConfig{
			Name:     modelName,
			Versions: versions,
		}
	}

	return dataModels
}

func createBenchmarkRegistrySchemas() []SchemaSubject {
	return []SchemaSubject{
		{
			Subject:            "pump_v1_timeseries-number",
			Name:               "pump",
			Version:            "v1",
			DataModelType:      "timeseries",
			DataType:           "number",
			ParsedSuccessfully: true,
		},
		{
			Subject:            "pump_v1_timeseries-string",
			Name:               "pump",
			Version:            "v1",
			DataModelType:      "timeseries",
			DataType:           "string",
			ParsedSuccessfully: true,
		},
		{
			Subject:            "pump_v1_timeseries-bool",
			Name:               "pump",
			Version:            "v1",
			DataModelType:      "timeseries",
			DataType:           "bool",
			ParsedSuccessfully: true,
		},
		{
			Subject:            "motor_v1_timeseries-number",
			Name:               "motor",
			Version:            "v1",
			DataModelType:      "timeseries",
			DataType:           "number",
			ParsedSuccessfully: true,
		},
		{
			Subject:            "motor_v1_timeseries-bool",
			Name:               "motor",
			Version:            "v1",
			DataModelType:      "timeseries",
			DataType:           "bool",
			ParsedSuccessfully: true,
		},
		{
			Subject:            "sensor_v1_timeseries-number",
			Name:               "sensor",
			Version:            "v1",
			DataModelType:      "timeseries",
			DataType:           "number",
			ParsedSuccessfully: true,
		},
		{
			Subject:            "sensor_v1_timeseries-string",
			Name:               "sensor",
			Version:            "v1",
			DataModelType:      "timeseries",
			DataType:           "string",
			ParsedSuccessfully: true,
		},
		{
			Subject:            "orphaned_v1_timeseries-bool",
			Name:               "orphaned",
			Version:            "v1",
			DataModelType:      "timeseries",
			DataType:           "bool",
			ParsedSuccessfully: true,
		},
	}
}
