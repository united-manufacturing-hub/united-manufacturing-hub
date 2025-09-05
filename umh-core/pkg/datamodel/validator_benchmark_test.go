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

// BenchmarkValidateStructureOnly_Simple benchmarks simple data model validation.
func BenchmarkValidateStructureOnly_Simple(b *testing.B) {
	validator := datamodel.NewValidator()
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

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateStructureOnly_Complex benchmarks complex nested data model validation.
func BenchmarkValidateStructureOnly_Complex(b *testing.B) {
	validator := datamodel.NewValidator()
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

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateStructureOnly_WithReferences benchmarks validation with references.
func BenchmarkValidateStructureOnly_WithReferences(b *testing.B) {
	validator := datamodel.NewValidator()
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

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateWithReferences benchmarks full reference validation.
func BenchmarkValidateWithReferences(b *testing.B) {
	validator := datamodel.NewValidator()
	ctx := context.Background()

	// Create a set of reference models
	allDataModels := map[string]config.DataModelsConfig{
		"motor": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"speed":       {PayloadShape: "timeseries-number"},
						"temperature": {PayloadShape: "timeseries-number"},
						"current":     {PayloadShape: "timeseries-number"},
					},
				},
			},
		},
		"sensor": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"value":     {PayloadShape: "timeseries-number"},
						"unit":      {PayloadShape: "timeseries-string"},
						"timestamp": {PayloadShape: "timeseries-string"},
					},
				},
			},
		},
		"controller": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"setpoint": {PayloadShape: "timeseries-number"},
						"output":   {PayloadShape: "timeseries-number"},
						"mode":     {PayloadShape: "timeseries-string"},
					},
				},
			},
		},
	}

	// Main data model that references others
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

	// Add payload shapes for the test
	payloadShapes := map[string]config.PayloadShape{
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

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		err := validator.ValidateWithReferences(ctx, dataModel, allDataModels, payloadShapes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateWithReferences_Deep benchmarks deep reference chains.
func BenchmarkValidateWithReferences_Deep(b *testing.B) {
	validator := datamodel.NewValidator()
	ctx := context.Background()

	// Create a deep chain of references (5 levels deep)
	allDataModels := map[string]config.DataModelsConfig{
		"level1": {
			Versions: map[string]config.DataModelVersion{
				"v1": {
					Structure: map[string]config.Field{
						"data": {PayloadShape: "timeseries-number"},
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
						"data": {PayloadShape: "timeseries-number"},
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
						"data": {PayloadShape: "timeseries-number"},
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
						"data": {PayloadShape: "timeseries-number"},
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
						"data":  {PayloadShape: "timeseries-number"},
						"final": {PayloadShape: "timeseries-string"},
					},
				},
			},
		},
	}

	// Main data model that starts the chain
	dataModel := config.DataModelVersion{
		Structure: map[string]config.Field{
			"root": {
				ModelRef: &config.ModelRef{
					Name:    "level1",
					Version: "v1",
				},
			},
		},
	}

	// Add payload shapes for the test
	payloadShapes := map[string]config.PayloadShape{
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

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		err := validator.ValidateWithReferences(ctx, dataModel, allDataModels, payloadShapes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateStructureOnly_Large benchmarks validation of very large schemas.
func BenchmarkValidateStructureOnly_Large(b *testing.B) {
	validator := datamodel.NewValidator()
	ctx := context.Background()

	// Generate a large data model with many fields
	structure := make(map[string]config.Field)

	// Add 100 simple fields
	for i := range 100 {
		structure[fmt.Sprintf("field_%d", i)] = config.Field{
			PayloadShape: "timeseries-number",
		}
	}

	// Add 10 complex nested structures
	for i := range 10 {
		subfields := make(map[string]config.Field)
		for j := range 10 {
			subfields[fmt.Sprintf("subfield_%d", j)] = config.Field{
				PayloadShape: "timeseries-number",
			}
		}

		structure[fmt.Sprintf("nested_%d", i)] = config.Field{
			Subfields: subfields,
		}
	}

	dataModel := config.DataModelVersion{
		Structure: structure,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidatorCreation benchmarks validator creation overhead.
func BenchmarkValidatorCreation(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		validator := datamodel.NewValidator()
		_ = validator // Prevent optimization
	}
}
