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

package translation_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/translation"
)

// BenchmarkTranslatorSimple benchmarks translation of simple schemas
func BenchmarkTranslatorSimple(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Simple schema with 3 fields of different types
	dataModel := config.DataModelVersion{
		Description: "Simple sensor",
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

	allModels := make(map[string]config.DataModelsConfig)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, dataModel, "sensor", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}

// BenchmarkTranslatorComplexNested benchmarks translation of complex nested structures
func BenchmarkTranslatorComplexNested(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Complex nested structure similar to validator's complex test
	dataModel := config.DataModelVersion{
		Description: "Complex pump system",
		Structure: map[string]config.Field{
			"pump": {
				Subfields: map[string]config.Field{
					"motor": {
						Subfields: map[string]config.Field{
							"speed": {
								Type:        "timeseries-number",
								Description: "Motor speed",
								Unit:        "rpm",
							},
							"current": {
								Type:        "timeseries-number",
								Description: "Motor current",
								Unit:        "A",
							},
							"voltage": {
								Type:        "timeseries-number",
								Description: "Motor voltage",
								Unit:        "V",
							},
							"temperature": {
								Type:        "timeseries-number",
								Description: "Motor temperature",
								Unit:        "°C",
							},
						},
					},
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
							"z-axis": {
								Type:        "timeseries-number",
								Description: "Z-axis vibration",
								Unit:        "mm/s",
							},
						},
					},
					"pressure": {
						Subfields: map[string]config.Field{
							"inlet": {
								Type:        "timeseries-number",
								Description: "Inlet pressure",
								Unit:        "bar",
							},
							"outlet": {
								Type:        "timeseries-number",
								Description: "Outlet pressure",
								Unit:        "bar",
							},
						},
					},
					"flow": {
						Subfields: map[string]config.Field{
							"rate": {
								Type:        "timeseries-number",
								Description: "Flow rate",
								Unit:        "L/min",
							},
							"volume": {
								Type:        "timeseries-number",
								Description: "Total volume",
								Unit:        "L",
							},
						},
					},
				},
			},
			"system": {
				Subfields: map[string]config.Field{
					"status": {
						Type:        "timeseries-string",
						Description: "System status",
					},
					"mode": {
						Type:        "timeseries-string",
						Description: "Operating mode",
					},
					"alarms": {
						Subfields: map[string]config.Field{
							"active": {
								Type:        "timeseries-boolean",
								Description: "Any active alarms",
							},
							"count": {
								Type:        "timeseries-number",
								Description: "Number of active alarms",
							},
						},
					},
				},
			},
			"metadata": {
				Subfields: map[string]config.Field{
					"serialNumber": {
						Type:        "timeseries-string",
						Description: "Serial number",
					},
					"model": {
						Type:        "timeseries-string",
						Description: "Model identifier",
					},
					"version": {
						Type:        "timeseries-string",
						Description: "Firmware version",
					},
				},
			},
		},
	}

	allModels := make(map[string]config.DataModelsConfig)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}

// BenchmarkTranslatorWithReferences benchmarks translation with model references
func BenchmarkTranslatorWithReferences(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Create a sensor model
	sensorModel := config.DataModelVersion{
		Description: "Basic sensor",
		Structure: map[string]config.Field{
			"value": {
				Type:        "timeseries-number",
				Description: "Sensor value",
			},
			"unit": {
				Type:        "timeseries-string",
				Description: "Unit of measurement",
			},
			"calibrated": {
				Type:        "timeseries-boolean",
				Description: "Is calibrated",
			},
		},
	}

	// Create a pump model that references sensors
	pumpModel := config.DataModelVersion{
		Description: "Pump with sensors",
		Structure: map[string]config.Field{
			"temperatureSensor": {
				ModelRef: "sensor:v1",
			},
			"pressureSensor": {
				ModelRef: "sensor:v1",
			},
			"flowSensor": {
				ModelRef: "sensor:v1",
			},
			"vibrationSensor": {
				ModelRef: "sensor:v1",
			},
			"status": {
				Type:        "timeseries-string",
				Description: "Pump status",
			},
		},
	}

	// Create the all models map
	allModels := map[string]config.DataModelsConfig{
		"sensor": {
			Name: "sensor",
			Versions: map[string]config.DataModelVersion{
				"v1": sensorModel,
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, pumpModel, "pump", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}

// BenchmarkTranslatorDeepChain benchmarks translation with deep reference chains
func BenchmarkTranslatorDeepChain(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Create a chain of 10 models that reference each other
	allModels := make(map[string]config.DataModelsConfig)

	// Build deep reference chain (level1 -> level2 -> ... -> level10)
	for level := 1; level <= 10; level++ {
		modelName := fmt.Sprintf("level%d", level)
		var structure map[string]config.Field

		if level == 10 {
			// Deepest level - actual fields
			structure = map[string]config.Field{
				"deepValue": {
					Type:        "timeseries-number",
					Description: "Deep nested value",
				},
				"deepStatus": {
					Type:        "timeseries-string",
					Description: "Deep nested status",
				},
				"deepFlag": {
					Type:        "timeseries-boolean",
					Description: "Deep nested flag",
				},
			}
		} else {
			// Reference the next level
			nextLevel := fmt.Sprintf("level%d", level+1)
			structure = map[string]config.Field{
				"nested": {
					ModelRef: fmt.Sprintf("%s:v1", nextLevel),
				},
				fmt.Sprintf("localValue%d", level): {
					Type:        "timeseries-number",
					Description: fmt.Sprintf("Local value at level %d", level),
				},
			}
		}

		allModels[modelName] = config.DataModelsConfig{
			Name: modelName,
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Description: fmt.Sprintf("Model at level %d", level),
					Structure:   structure,
				},
			},
		}
	}

	// Get the root model (level1)
	rootModel := allModels["level1"].Versions["v1"]

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, rootModel, "level1", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}

// BenchmarkTranslatorLargeSchema benchmarks translation of large schemas
func BenchmarkTranslatorLargeSchema(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Create a large schema with 200+ fields
	structure := make(map[string]config.Field)

	// Add 50 number fields
	for i := 0; i < 50; i++ {
		fieldName := fmt.Sprintf("numberField%d", i)
		structure[fieldName] = config.Field{
			Type:        "timeseries-number",
			Description: fmt.Sprintf("Number field %d", i),
			Unit:        "unit",
		}
	}

	// Add 50 string fields
	for i := 0; i < 50; i++ {
		fieldName := fmt.Sprintf("stringField%d", i)
		structure[fieldName] = config.Field{
			Type:        "timeseries-string",
			Description: fmt.Sprintf("String field %d", i),
		}
	}

	// Add 50 boolean fields
	for i := 0; i < 50; i++ {
		fieldName := fmt.Sprintf("booleanField%d", i)
		structure[fieldName] = config.Field{
			Type:        "timeseries-boolean",
			Description: fmt.Sprintf("Boolean field %d", i),
		}
	}

	// Add 50 nested structures with multiple fields each
	for i := 0; i < 50; i++ {
		groupName := fmt.Sprintf("group%d", i)
		structure[groupName] = config.Field{
			Subfields: map[string]config.Field{
				"value": {
					Type:        "timeseries-number",
					Description: fmt.Sprintf("Value in group %d", i),
				},
				"status": {
					Type:        "timeseries-string",
					Description: fmt.Sprintf("Status in group %d", i),
				},
				"active": {
					Type:        "timeseries-boolean",
					Description: fmt.Sprintf("Active flag in group %d", i),
				},
			},
		}
	}

	dataModel := config.DataModelVersion{
		Description: "Large schema with 200+ fields",
		Structure:   structure,
	}

	allModels := make(map[string]config.DataModelsConfig)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, dataModel, "large", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}

// BenchmarkTranslatorMixedTypes benchmarks translation with mixed type scenarios
func BenchmarkTranslatorMixedTypes(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Create a mixed scenario with references and direct fields
	sensorModel := config.DataModelVersion{
		Description: "Sensor with mixed types",
		Structure: map[string]config.Field{
			"readings": {
				Subfields: map[string]config.Field{
					"temperature": {
						Type:        "timeseries-number",
						Description: "Temperature reading",
						Unit:        "°C",
					},
					"pressure": {
						Type:        "timeseries-number",
						Description: "Pressure reading",
						Unit:        "bar",
					},
					"humidity": {
						Type:        "timeseries-number",
						Description: "Humidity reading",
						Unit:        "%",
					},
				},
			},
			"status": {
				Type:        "timeseries-string",
				Description: "Sensor status",
			},
			"metadata": {
				Subfields: map[string]config.Field{
					"id": {
						Type:        "timeseries-string",
						Description: "Sensor ID",
					},
					"location": {
						Type:        "timeseries-string",
						Description: "Physical location",
					},
					"calibrated": {
						Type:        "timeseries-boolean",
						Description: "Is calibrated",
					},
				},
			},
		},
	}

	// Factory model with multiple sensor references
	factoryModel := config.DataModelVersion{
		Description: "Factory with multiple sensors",
		Structure: map[string]config.Field{
			"zone1": {
				Subfields: map[string]config.Field{
					"sensor1": {
						ModelRef: "sensor:v1",
					},
					"sensor2": {
						ModelRef: "sensor:v1",
					},
					"zoneStatus": {
						Type:        "timeseries-string",
						Description: "Zone status",
					},
				},
			},
			"zone2": {
				Subfields: map[string]config.Field{
					"sensor3": {
						ModelRef: "sensor:v1",
					},
					"sensor4": {
						ModelRef: "sensor:v1",
					},
					"zoneStatus": {
						Type:        "timeseries-string",
						Description: "Zone status",
					},
				},
			},
			"factory": {
				Subfields: map[string]config.Field{
					"overallStatus": {
						Type:        "timeseries-string",
						Description: "Overall factory status",
					},
					"operationalMode": {
						Type:        "timeseries-string",
						Description: "Operational mode",
					},
					"emergencyStop": {
						Type:        "timeseries-boolean",
						Description: "Emergency stop activated",
					},
				},
			},
		},
	}

	allModels := map[string]config.DataModelsConfig{
		"sensor": {
			Name: "sensor",
			Versions: map[string]config.DataModelVersion{
				"v1": sensorModel,
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, factoryModel, "factory", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}

// BenchmarkTranslatorWideChain benchmarks translation with wide reference chains (breadth vs depth)
func BenchmarkTranslatorWideChain(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Create a leaf model
	leafModel := config.DataModelVersion{
		Description: "Leaf model",
		Structure: map[string]config.Field{
			"value": {
				Type:        "timeseries-number",
				Description: "Leaf value",
			},
			"status": {
				Type:        "timeseries-string",
				Description: "Leaf status",
			},
		},
	}

	// Create root model that references many leaf models
	rootStructure := make(map[string]config.Field)
	for i := 0; i < 20; i++ {
		fieldName := fmt.Sprintf("leaf%d", i)
		rootStructure[fieldName] = config.Field{
			ModelRef: "leaf:v1",
		}
	}

	rootModel := config.DataModelVersion{
		Description: "Root model with wide references",
		Structure:   rootStructure,
	}

	allModels := map[string]config.DataModelsConfig{
		"leaf": {
			Name: "leaf",
			Versions: map[string]config.DataModelVersion{
				"v1": leafModel,
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, rootModel, "root", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}
