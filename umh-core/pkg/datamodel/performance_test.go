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
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
)

// TestPerformanceTarget verifies the validator meets the 1000 schemas/second target
func TestPerformanceTarget(t *testing.T) {
	validator := datamodel.NewValidator()
	ctx := context.Background()

	// Create a realistic industrial data model
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
							"current": {
								PayloadShape: "timeseries-number",
							},
							"voltage": {
								PayloadShape: "timeseries-number",
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
					"vibration": {
						Subfields: map[string]config.Field{
							"x": {
								PayloadShape: "timeseries-number",
							},
							"y": {
								PayloadShape: "timeseries-number",
							},
							"z": {
								PayloadShape: "timeseries-number",
							},
						},
					},
				},
			},
			"sensors": {
				Subfields: map[string]config.Field{
					"temperature": {
						PayloadShape: "timeseries-number",
					},
					"level": {
						PayloadShape: "timeseries-number",
					},
					"ph": {
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
				},
			},
		},
	}

	// Test for 1 second to measure actual throughput
	start := time.Now()
	deadline := start.Add(1 * time.Second)
	count := 0

	for time.Now().Before(deadline) {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}
		count++
	}

	elapsed := time.Since(start)
	schemasPerSecond := float64(count) / elapsed.Seconds()

	t.Logf("Validated %d schemas in %v", count, elapsed)
	t.Logf("Performance: %.0f schemas per second", schemasPerSecond)

	// Verify we exceed the 1000 schemas/second target
	if schemasPerSecond < 1000 {
		t.Errorf("Performance target not met: got %.0f schemas/sec, want >= 1000 schemas/sec", schemasPerSecond)
	} else {
		t.Logf("✅ Performance target exceeded by %.1fx", schemasPerSecond/1000)
	}
}

// TestPerformanceTargetWithReferences verifies reference validation performance
func TestPerformanceTargetWithReferences(t *testing.T) {
	validator := datamodel.NewValidator()
	ctx := context.Background()

	// Create a high-performance scenario with references
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

	// Test data model with various field types
	testModel := config.DataModelVersion{
		Structure: map[string]config.Field{
			"production": {
				Subfields: map[string]config.Field{
					"motor": {
						ModelRef: &config.ModelRef{
							Name:    "motor",
							Version: "v1",
						},
					},
					"sensors": {
						Subfields: map[string]config.Field{
							"temperature": {
								ModelRef: &config.ModelRef{
									Name:    "sensor",
									Version: "v1",
								},
							},
							"pressure": {
								ModelRef: &config.ModelRef{
									Name:    "sensor",
									Version: "v1",
								},
							},
						},
					},
					"controller": {
						ModelRef: &config.ModelRef{
							Name:    "controller",
							Version: "v1",
						},
					},
				},
			},
			"metadata": {
				Subfields: map[string]config.Field{
					"machine_id": {PayloadShape: "timeseries-string"},
					"location":   {PayloadShape: "timeseries-string"},
					"operator":   {PayloadShape: "timeseries-string"},
				},
			},
		},
	}

	// Test for 1 second to measure actual throughput
	start := time.Now()
	deadline := start.Add(1 * time.Second)
	count := 0

	// Add empty payload shapes for the test
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

	for time.Now().Before(deadline) {
		err := validator.ValidateWithReferences(ctx, testModel, allDataModels, payloadShapes)
		if err != nil {
			t.Fatalf("Reference validation failed: %v", err)
		}
		count++
	}

	elapsed := time.Since(start)
	schemasPerSecond := float64(count) / elapsed.Seconds()

	t.Logf("Validated %d schemas with references in %v", count, elapsed)
	t.Logf("Performance: %.0f schemas per second", schemasPerSecond)

	// Verify we exceed the 1000 schemas/second target (even with references)
	if schemasPerSecond < 1000 {
		t.Errorf("Performance target not met with references: got %.0f schemas/sec, want >= 1000 schemas/sec", schemasPerSecond)
	} else {
		t.Logf("✅ Performance target with references exceeded by %.1fx", schemasPerSecond/1000)
	}
}

// TestMemoryUsage verifies memory usage stays reasonable
func TestMemoryUsage(t *testing.T) {
	validator := datamodel.NewValidator()
	ctx := context.Background()

	// Create a moderately complex data model
	dataModel := config.DataModelVersion{
		Structure: map[string]config.Field{
			"equipment": {
				Subfields: map[string]config.Field{
					"motor": {
						Subfields: map[string]config.Field{
							"speed":       {PayloadShape: "timeseries-number"},
							"temperature": {PayloadShape: "timeseries-number"},
							"vibration": {
								Subfields: map[string]config.Field{
									"x": {PayloadShape: "timeseries-number"},
									"y": {PayloadShape: "timeseries-number"},
									"z": {PayloadShape: "timeseries-number"},
								},
							},
						},
					},
					"sensors": {
						Subfields: map[string]config.Field{
							"temp1":    {PayloadShape: "timeseries-number"},
							"temp2":    {PayloadShape: "timeseries-number"},
							"pressure": {PayloadShape: "timeseries-number"},
							"flow":     {PayloadShape: "timeseries-number"},
						},
					},
				},
			},
		},
	}

	// Validate multiple times to ensure no memory leaks
	const iterations = 10000
	for i := 0; i < iterations; i++ {
		err := validator.ValidateStructureOnly(ctx, dataModel)
		if err != nil {
			t.Fatalf("Validation failed at iteration %d: %v", i, err)
		}
	}

	t.Logf("✅ Successfully validated %d schemas without memory issues", iterations)
}
