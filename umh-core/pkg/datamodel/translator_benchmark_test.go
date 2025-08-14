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
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
)

// BenchmarkTranslateDataModel_Simple benchmarks simple data model translation.
func BenchmarkTranslateDataModel_Simple(b *testing.B) {
	translator := datamodel.NewTranslator()
	ctx := context.Background()

	// Simple data model with just a few fields
	dataModel := config.DataModelVersion{
		Structure: map[string]config.Field{
			"temperature": {
				PayloadShape: "timeseries-number",
			},
			"pressure": {
				PayloadShape: "timeseries-number",
			},
			"status": {
				PayloadShape: "timeseries-string",
			},
		},
	}

	payloadShapes := map[string]config.PayloadShape{}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		result, err := translator.TranslateDataModel(
			ctx,
			"_pump_data",
			"v1",
			dataModel,
			payloadShapes,
			nil,
		)
		if err != nil {
			b.Fatal(err)
		}

		if result == nil {
			b.Fatal("result is nil")
		}
	}
}

// BenchmarkTranslateDataModel_Complex benchmarks complex nested data model translation.
func BenchmarkTranslateDataModel_Complex(b *testing.B) {
	translator := datamodel.NewTranslator()
	ctx := context.Background()

	// Complex data model with nested structures
	dataModel := config.DataModelVersion{
		Structure: map[string]config.Field{
			"pump": {
				Subfields: map[string]config.Field{
					"motor": {
						Subfields: map[string]config.Field{
							"speed": {
								PayloadShape: "timeseries-number",
							},
							"temperature": {
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
						},
					},
					"flow": {
						PayloadShape: "timeseries-number",
					},
					"pressure": {
						Subfields: map[string]config.Field{
							"inlet": {
								PayloadShape: "timeseries-number",
							},
							"outlet": {
								PayloadShape: "timeseries-number",
							},
						},
					},
				},
			},
			"sensors": {
				Subfields: map[string]config.Field{
					"temperature": {
						Subfields: map[string]config.Field{
							"ambient": {
								PayloadShape: "timeseries-number",
							},
							"fluid": {
								PayloadShape: "timeseries-number",
							},
						},
					},
					"level": {
						PayloadShape: "timeseries-number",
					},
				},
			},
			"metadata": {
				Subfields: map[string]config.Field{
					"serialNumber": {
						PayloadShape: "timeseries-string",
					},
					"manufacturer": {
						PayloadShape: "timeseries-string",
					},
					"model": {
						PayloadShape: "timeseries-string",
					},
					"installationDate": {
						PayloadShape: "timeseries-string",
					},
				},
			},
		},
	}

	payloadShapes := map[string]config.PayloadShape{}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		result, err := translator.TranslateDataModel(
			ctx,
			"_complex_pump",
			"v1",
			dataModel,
			payloadShapes,
			nil,
		)
		if err != nil {
			b.Fatal(err)
		}

		if result == nil {
			b.Fatal("result is nil")
		}
	}
}

// BenchmarkTranslateDataModel_WithReferences benchmarks translation with model references.
func BenchmarkTranslateDataModel_WithReferences(b *testing.B) {
	translator := datamodel.NewTranslator()
	ctx := context.Background()

	// Data model with references
	dataModel := config.DataModelVersion{
		Structure: map[string]config.Field{
			"motor": {
				ModelRef: &config.ModelRef{
					Name:    "motor",
					Version: "v1",
				},
			},
			"sensor": {
				ModelRef: &config.ModelRef{
					Name:    "sensor",
					Version: "v1",
				},
			},
			"pump": {
				Subfields: map[string]config.Field{
					"flow": {
						PayloadShape: "timeseries-number",
					},
					"controller": {
						ModelRef: &config.ModelRef{
							Name:    "controller",
							Version: "v1",
						},
					},
				},
			},
		},
	}

	// Model references with referenced models
	modelReferences := map[string]config.DataModelsConfig{
		"motor": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"speed": {
							PayloadShape: "timeseries-number",
						},
						"temperature": {
							PayloadShape: "timeseries-number",
						},
					},
				},
			},
		},
		"sensor": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"value": {
							PayloadShape: "timeseries-number",
						},
						"timestamp": {
							PayloadShape: "timeseries-string",
						},
					},
				},
			},
		},
		"controller": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"setpoint": {
							PayloadShape: "timeseries-number",
						},
						"output": {
							PayloadShape: "timeseries-number",
						},
					},
				},
			},
		},
	}

	payloadShapes := map[string]config.PayloadShape{}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		result, err := translator.TranslateDataModel(
			ctx,
			"_pump_with_refs",
			"v1",
			dataModel,
			payloadShapes,
			modelReferences,
		)
		if err != nil {
			b.Fatal(err)
		}

		if result == nil {
			b.Fatal("result is nil")
		}
	}
}

// BenchmarkTranslateDataModel_MultiplePayloadShapes benchmarks translation with different payload shapes.
func BenchmarkTranslateDataModel_MultiplePayloadShapes(b *testing.B) {
	translator := datamodel.NewTranslator()
	ctx := context.Background()

	// Data model with various payload shapes using built-in types
	dataModel := config.DataModelVersion{
		Structure: map[string]config.Field{
			"measurements": {
				Subfields: map[string]config.Field{
					"temperature": {
						PayloadShape: "timeseries-number",
					},
					"humidity": {
						PayloadShape: "timeseries-number",
					},
					"pressure": {
						PayloadShape: "timeseries-number",
					},
					"flow_rate": {
						PayloadShape: "timeseries-number",
					},
				},
			},
			"diagnostics": {
				Subfields: map[string]config.Field{
					"status": {
						PayloadShape: "timeseries-string",
					},
					"alarm_text": {
						PayloadShape: "timeseries-string",
					},
					"error_code": {
						PayloadShape: "timeseries-string",
					},
					"operator_id": {
						PayloadShape: "timeseries-string",
					},
				},
			},
		},
	}

	payloadShapes := map[string]config.PayloadShape{}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		result, err := translator.TranslateDataModel(
			ctx,
			"_multi_shape",
			"v1",
			dataModel,
			payloadShapes,
			nil,
		)
		if err != nil {
			b.Fatal(err)
		}

		if result == nil {
			b.Fatal("result is nil")
		}
	}
}

// BenchmarkTranslateDataModel_Large benchmarks large data model translation.
func BenchmarkTranslateDataModel_Large(b *testing.B) {
	translator := datamodel.NewTranslator()
	ctx := context.Background()

	// Large data model with many fields (50+ fields across multiple levels)
	structure := make(map[string]config.Field)

	// Create 10 major subsystems
	for subsystemIndex := range 10 {
		subsystemFields := make(map[string]config.Field)

		// Each subsystem has 5-8 components
		for componentIndex := range 5 + subsystemIndex%4 {
			componentFields := make(map[string]config.Field)

			// Each component has 3-6 measurements
			for measurementIndex := range 3 + componentIndex%4 {
				if measurementIndex%2 == 0 {
					componentFields[fmt.Sprintf("measurement_%d", measurementIndex)] = config.Field{
						PayloadShape: "timeseries-number",
					}
				} else {
					componentFields[fmt.Sprintf("status_%d", measurementIndex)] = config.Field{
						PayloadShape: "timeseries-string",
					}
				}
			}

			subsystemFields[fmt.Sprintf("component_%d", componentIndex)] = config.Field{
				Subfields: componentFields,
			}
		}

		structure[fmt.Sprintf("subsystem_%d", subsystemIndex)] = config.Field{
			Subfields: subsystemFields,
		}
	}

	dataModel := config.DataModelVersion{
		Structure: structure,
	}

	payloadShapes := map[string]config.PayloadShape{}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		result, err := translator.TranslateDataModel(
			ctx,
			"_large_system",
			"v1",
			dataModel,
			payloadShapes,
			nil,
		)
		if err != nil {
			b.Fatal(err)
		}

		if result == nil {
			b.Fatal("result is nil")
		}
	}
}

// BenchmarkTranslatorCreation benchmarks translator instance creation.
func BenchmarkTranslatorCreation(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		translator := datamodel.NewTranslator()
		if translator == nil {
			b.Fatal("translator is nil")
		}
	}
}
