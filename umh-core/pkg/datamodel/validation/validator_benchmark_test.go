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

package validation_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/validation"
)

// BenchmarkValidateStructureOnly_Simple benchmarks simple data model validation
func BenchmarkValidateStructureOnly_Simple(b *testing.B) {
	validator := validation.NewValidator()
	ctx := context.Background()

	// Simple data model with just a few fields
	dataModel := config.DataModelVersion{
		Description: "Simple benchmark model",
		Structure: map[string]config.Field{
			"temperature": {
				Type: "timeseries-number",
				Unit: "°C",
			},
			"pressure": {
				Type: "timeseries-number",
				Unit: "bar",
			},
			"status": {
				Type: "timeseries-string",
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateStructureOnly_Complex benchmarks complex nested data model validation
func BenchmarkValidateStructureOnly_Complex(b *testing.B) {
	validator := validation.NewValidator()
	ctx := context.Background()

	// Complex data model with nested structures
	dataModel := config.DataModelVersion{
		Description: "Complex benchmark model",
		Structure: map[string]config.Field{
			"pump": {
				Subfields: map[string]config.Field{
					"motor": {
						Subfields: map[string]config.Field{
							"speed": {
								Type: "timeseries-number",
								Unit: "rpm",
							},
							"temperature": {
								Type: "timeseries-number",
								Unit: "°C",
							},
							"vibration": {
								Subfields: map[string]config.Field{
									"x-axis": {
										Type: "timeseries-number",
										Unit: "mm/s",
									},
									"y-axis": {
										Type: "timeseries-number",
										Unit: "mm/s",
									},
									"z-axis": {
										Type: "timeseries-number",
										Unit: "mm/s",
									},
								},
							},
						},
					},
					"flow": {
						Type: "timeseries-number",
						Unit: "L/min",
					},
					"pressure": {
						Subfields: map[string]config.Field{
							"inlet": {
								Type: "timeseries-number",
								Unit: "bar",
							},
							"outlet": {
								Type: "timeseries-number",
								Unit: "bar",
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
								Type: "timeseries-number",
								Unit: "°C",
							},
							"fluid": {
								Type: "timeseries-number",
								Unit: "°C",
							},
						},
					},
					"level": {
						Type: "timeseries-number",
						Unit: "mm",
					},
				},
			},
			"metadata": {
				Subfields: map[string]config.Field{
					"serialNumber": {
						Type: "timeseries-string",
					},
					"manufacturer": {
						Type: "timeseries-string",
					},
					"model": {
						Type: "timeseries-string",
					},
					"installationDate": {
						Type: "timeseries-string",
					},
				},
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateStructureOnly_WithReferences benchmarks validation with references
func BenchmarkValidateStructureOnly_WithReferences(b *testing.B) {
	validator := validation.NewValidator()
	ctx := context.Background()

	// Data model with references
	dataModel := config.DataModelVersion{
		Description: "Model with references",
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
						Type: "timeseries-number",
						Unit: "L/min",
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

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateWithReferences benchmarks full reference validation
func BenchmarkValidateWithReferences(b *testing.B) {
	validator := validation.NewValidator()
	ctx := context.Background()

	// Create a set of reference models
	allDataModels := map[string]config.DataModelsConfig{
		"motor": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"speed":       {Type: "timeseries-number", Unit: "rpm"},
						"temperature": {Type: "timeseries-number", Unit: "°C"},
						"current":     {Type: "timeseries-number", Unit: "A"},
					},
				},
			},
		},
		"sensor": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"value":     {Type: "timeseries-number"},
						"unit":      {Type: "timeseries-string"},
						"timestamp": {Type: "timeseries-string"},
					},
				},
			},
		},
		"controller": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"setpoint": {Type: "timeseries-number"},
						"output":   {Type: "timeseries-number"},
						"mode":     {Type: "timeseries-string"},
					},
				},
			},
		},
	}

	// Main data model that references others
	dataModel := config.DataModelVersion{
		Description: "Model with references",
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
						Type: "timeseries-number",
						Unit: "L/min",
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

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := validator.ValidateWithReferences(ctx, dataModel, allDataModels)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateWithReferences_Deep benchmarks deep reference chains
func BenchmarkValidateWithReferences_Deep(b *testing.B) {
	validator := validation.NewValidator()
	ctx := context.Background()

	// Create a deep chain of references (5 levels deep)
	allDataModels := map[string]config.DataModelsConfig{
		"level1": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"data": {Type: "timeseries-number"},
						"next": {
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
						"data": {Type: "timeseries-number"},
						"next": {
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
						"data": {Type: "timeseries-number"},
						"next": {
							ModelRef: &config.ModelRef{
								Name:    "level4",
								Version: "v1",
							},
						},
					},
				},
			},
		},
		"level4": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"data": {Type: "timeseries-number"},
						"next": {
							ModelRef: &config.ModelRef{
								Name:    "level5",
								Version: "v1",
							},
						},
					},
				},
			},
		},
		"level5": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"data":  {Type: "timeseries-number"},
						"final": {Type: "timeseries-string"},
					},
				},
			},
		},
	}

	// Main data model that starts the chain
	dataModel := config.DataModelVersion{
		Description: "Deep reference chain",
		Structure: map[string]config.Field{
			"root": {
				ModelRef: &config.ModelRef{
					Name:    "level1",
					Version: "v1",
				},
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := validator.ValidateWithReferences(ctx, dataModel, allDataModels)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateStructureOnly_Large benchmarks validation of very large schemas
func BenchmarkValidateStructureOnly_Large(b *testing.B) {
	validator := validation.NewValidator()
	ctx := context.Background()

	// Generate a large data model with many fields
	structure := make(map[string]config.Field)

	// Add 100 simple fields
	for i := 0; i < 100; i++ {
		structure[fmt.Sprintf("field_%d", i)] = config.Field{
			Type: "timeseries-number",
			Unit: "unit",
		}
	}

	// Add 10 complex nested structures
	for i := 0; i < 10; i++ {
		subfields := make(map[string]config.Field)
		for j := 0; j < 10; j++ {
			subfields[fmt.Sprintf("subfield_%d", j)] = config.Field{
				Type: "timeseries-number",
			}
		}
		structure[fmt.Sprintf("nested_%d", i)] = config.Field{
			Subfields: subfields,
		}
	}

	dataModel := config.DataModelVersion{
		Description: "Large benchmark model",
		Structure:   structure,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidatorCreation benchmarks validator creation overhead
func BenchmarkValidatorCreation(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		validator := validation.NewValidator()
		_ = validator // Prevent optimization
	}
}
