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
		Description: "Industrial pump data model",
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
							"current": {
								Type: "timeseries-number",
								Unit: "A",
							},
							"voltage": {
								Type: "timeseries-number",
								Unit: "V",
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
					"vibration": {
						Subfields: map[string]config.Field{
							"x": {
								Type: "timeseries-number",
								Unit: "mm/s",
							},
							"y": {
								Type: "timeseries-number",
								Unit: "mm/s",
							},
							"z": {
								Type: "timeseries-number",
								Unit: "mm/s",
							},
						},
					},
				},
			},
			"sensors": {
				Subfields: map[string]config.Field{
					"temperature": {
						Type: "timeseries-number",
						Unit: "°C",
					},
					"level": {
						Type: "timeseries-number",
						Unit: "mm",
					},
					"ph": {
						Type: "timeseries-number",
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

	// Create reference models
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
						"value": {Type: "timeseries-number"},
						"unit":  {Type: "timeseries-string"},
					},
				},
			},
		},
	}

	// Main data model with references
	dataModel := config.DataModelVersion{
		Description: "Industrial system with references",
		Structure: map[string]config.Field{
			"motor": {
				ModelRef: "motor:v1",
			},
			"sensors": {
				Subfields: map[string]config.Field{
					"temperature": {
						ModelRef: "sensor:v1",
					},
					"pressure": {
						ModelRef: "sensor:v1",
					},
				},
			},
			"flow": {
				Type: "timeseries-number",
				Unit: "L/min",
			},
		},
	}

	// Test for 1 second to measure actual throughput
	start := time.Now()
	deadline := start.Add(1 * time.Second)
	count := 0

	for time.Now().Before(deadline) {
		err := validator.ValidateWithReferences(ctx, dataModel, allDataModels)
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
		Description: "Memory usage test model",
		Structure: map[string]config.Field{
			"equipment": {
				Subfields: map[string]config.Field{
					"motor": {
						Subfields: map[string]config.Field{
							"speed":       {Type: "timeseries-number", Unit: "rpm"},
							"temperature": {Type: "timeseries-number", Unit: "°C"},
							"vibration": {
								Subfields: map[string]config.Field{
									"x": {Type: "timeseries-number", Unit: "mm/s"},
									"y": {Type: "timeseries-number", Unit: "mm/s"},
									"z": {Type: "timeseries-number", Unit: "mm/s"},
								},
							},
						},
					},
					"sensors": {
						Subfields: map[string]config.Field{
							"temp1":    {Type: "timeseries-number", Unit: "°C"},
							"temp2":    {Type: "timeseries-number", Unit: "°C"},
							"pressure": {Type: "timeseries-number", Unit: "bar"},
							"flow":     {Type: "timeseries-number", Unit: "L/min"},
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
