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
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/validation"
)

var _ = Describe("Performance", func() {
	var (
		validator *validation.Validator
		ctx       context.Context
	)

	BeforeEach(func() {
		validator = validation.NewValidator()
		ctx = context.Background()
	})

	It("should meet the 1000 schemas/second performance target", func() {
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
			Expect(err).To(BeNil())
			count++
		}

		elapsed := time.Since(start)
		schemasPerSecond := float64(count) / elapsed.Seconds()

		GinkgoWriter.Printf("Validated %d schemas in %v\n", count, elapsed)
		GinkgoWriter.Printf("Performance: %.0f schemas per second\n", schemasPerSecond)

		// Verify we exceed the 1000 schemas/second target
		Expect(schemasPerSecond).To(BeNumerically(">=", 1000),
			"Performance target not met: got %.0f schemas/sec, want >= 1000 schemas/sec", schemasPerSecond)

		GinkgoWriter.Printf("✅ Performance target exceeded by %.1fx\n", schemasPerSecond/1000)
	})

	It("should meet performance target with references", func() {
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
			Expect(err).To(BeNil())
			count++
		}

		elapsed := time.Since(start)
		schemasPerSecond := float64(count) / elapsed.Seconds()

		GinkgoWriter.Printf("Validated %d schemas with references in %v\n", count, elapsed)
		GinkgoWriter.Printf("Performance: %.0f schemas per second\n", schemasPerSecond)

		// Verify we exceed the 1000 schemas/second target
		Expect(schemasPerSecond).To(BeNumerically(">=", 1000),
			"Performance target not met: got %.0f schemas/sec, want >= 1000 schemas/sec", schemasPerSecond)

		GinkgoWriter.Printf("✅ Performance target exceeded by %.1fx\n", schemasPerSecond/1000)
	})

	It("should have reasonable memory usage", func() {
		// Create a complex data model
		dataModel := config.DataModelVersion{
			Description: "Complex data model for memory testing",
			Structure: map[string]config.Field{
				"system": {
					Subfields: map[string]config.Field{
						"cpu": {
							Subfields: map[string]config.Field{
								"usage":       {Type: "timeseries-number", Unit: "%"},
								"temperature": {Type: "timeseries-number", Unit: "°C"},
								"frequency":   {Type: "timeseries-number", Unit: "MHz"},
							},
						},
						"memory": {
							Subfields: map[string]config.Field{
								"total": {Type: "timeseries-number", Unit: "GB"},
								"used":  {Type: "timeseries-number", Unit: "GB"},
								"free":  {Type: "timeseries-number", Unit: "GB"},
							},
						},
						"disk": {
							Subfields: map[string]config.Field{
								"read":  {Type: "timeseries-number", Unit: "MB/s"},
								"write": {Type: "timeseries-number", Unit: "MB/s"},
								"usage": {Type: "timeseries-number", Unit: "%"},
							},
						},
					},
				},
			},
		}

		// Measure memory usage
		var memBefore, memAfter runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&memBefore)

		// Perform multiple validations
		for i := 0; i < 1000; i++ {
			err := validator.ValidateStructureOnly(ctx, dataModel)
			Expect(err).To(BeNil())
		}

		runtime.GC()
		runtime.ReadMemStats(&memAfter)

		// Calculate memory usage
		memUsed := memAfter.Alloc - memBefore.Alloc
		memUsedKB := float64(memUsed) / 1024

		GinkgoWriter.Printf("Memory used for 1000 validations: %.2f KB\n", memUsedKB)
		GinkgoWriter.Printf("Memory per validation: %.2f bytes\n", float64(memUsed)/1000)

		// Verify reasonable memory usage (less than 10KB for 1000 validations)
		Expect(memUsedKB).To(BeNumerically("<", 10),
			"Memory usage too high: %.2f KB for 1000 validations", memUsedKB)

		GinkgoWriter.Printf("✅ Memory usage within acceptable limits\n")
	})
})
