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
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Benchmark suite for schema registry operations against mock HTTP server
//
// This file contains comprehensive benchmarks for testing schema registry performance
// under various scenarios and loads. All benchmarks use the MockSchemaRegistry HTTP server
// to provide realistic network-based performance measurements.
//
// Benchmark Categories:
//
// 1. BenchmarkSchemaRegistry/Reconcile - Full reconciliation scenarios:
//    - EmptyToEmpty: No-op reconciliation (both registry and expected are empty)
//    - EmptyToN: Adding N schemas to empty registry
//    - NToEmpty: Removing N schemas from populated registry
//    - NToN_Same: No-op reconciliation (registry matches expected)
//    - NToN_Different: Full replacement (N schemas removed, N different schemas added)
//    - Mixed_AddRemove: Partial overlap (some schemas kept, some removed, some added)
//
// 2. BenchmarkSchemaRegistry/GetMetrics - Metrics collection performance:
//    - Tests read-only access to registry metrics
//    - Measures RWMutex read lock performance
//
// 3. BenchmarkSchemaRegistry/Concurrent - Thread safety under load:
//    - ReadHeavy: Many metrics readers, few reconciliation writers
//    - WriteHeavy: Many reconciliation writers, few metrics readers
//    - Balanced: Equal mix of readers and writers
//    - HighContention: High concurrency stress test
//
// 4. BenchmarkSchemaRegistryPhases - Individual phase performance:
//    - LookupPhase: HTTP GET /subjects performance
//    - DecodePhase: JSON parsing performance
//    - ComparePhase: Map comparison and work queue building
//    - RemoveUnknownPhase: HTTP DELETE performance
//    - AddNewPhase: HTTP POST performance
//
// Usage Examples:
//
//   # Run all benchmarks
//   go test -bench=BenchmarkSchemaRegistry ./pkg/service/redpanda/
//
//   # Run specific scenario
//   go test -bench=BenchmarkSchemaRegistry/Reconcile/EmptyToN ./pkg/service/redpanda/
//
//   # Run with custom duration and CPU profiling
//   go test -bench=BenchmarkSchemaRegistry -benchtime=5s -cpuprofile=cpu.prof ./pkg/service/redpanda/
//
//   # Run concurrent benchmarks only
//   go test -bench=BenchmarkSchemaRegistry/Concurrent ./pkg/service/redpanda/
//
//   # Run phase benchmarks for specific schema count
//   go test -bench=BenchmarkSchemaRegistryPhases/LookupPhase/Schemas_1000 ./pkg/service/redpanda/
//
// Performance Expectations:
//
// - GetMetrics: ~8-10 ns/op (pure memory read with RWMutex)
// - DecodePhase: ~1-2 μs/op for 10 schemas, ~12-15 μs/op for 100 schemas
// - LookupPhase: ~50-70 μs/op for 100 schemas (includes HTTP overhead)
// - Full Reconciliation: ~35-40 μs/op for no-op, ~60-130 μs/op for adding schemas
// - Concurrent: ~139 μs/op for read-heavy workload (10 readers, 1 writer)
//
// All benchmarks use realistic test data and the full HTTP stack to provide
// accurate performance measurements for production deployment planning.

// BenchmarkSchemaRegistry contains all schema registry benchmarks
func BenchmarkSchemaRegistry(b *testing.B) {
	b.Run("Reconcile", benchmarkReconcile)
	b.Run("GetMetrics", benchmarkGetMetrics)
	b.Run("Concurrent", benchmarkConcurrent)
}

// benchmarkReconcile tests reconciliation performance under different scenarios
func benchmarkReconcile(b *testing.B) {
	scenarios := []struct {
		name             string
		registrySchemas  int // Number of schemas already in registry
		expectedSchemas  int // Number of schemas we expect
		setupRegistry    func(*MockSchemaRegistry, int)
		generateExpected func(int) map[SubjectName]JSONSchemaDefinition
	}{
		{
			name:            "EmptyToEmpty",
			registrySchemas: 0,
			expectedSchemas: 0,
			setupRegistry: func(mock *MockSchemaRegistry, count int) {
				// No setup needed for empty registry
			},
			generateExpected: func(count int) map[SubjectName]JSONSchemaDefinition {
				return make(map[SubjectName]JSONSchemaDefinition)
			},
		},
		{
			name:            "EmptyToN",
			registrySchemas: 0,
			expectedSchemas: 10,
			setupRegistry: func(mock *MockSchemaRegistry, count int) {
				// No setup needed for empty registry
			},
			generateExpected: generateSchemas,
		},
		{
			name:            "NToEmpty",
			registrySchemas: 10,
			expectedSchemas: 0,
			setupRegistry:   setupMockSchemas,
			generateExpected: func(count int) map[SubjectName]JSONSchemaDefinition {
				return make(map[SubjectName]JSONSchemaDefinition)
			},
		},
		{
			name:            "NToN_Same",
			registrySchemas: 10,
			expectedSchemas: 10,
			setupRegistry:   setupMockSchemas,
			generateExpected: func(count int) map[SubjectName]JSONSchemaDefinition {
				// Generate same schemas as setupMockSchemas
				return generateSchemasMatching(count)
			},
		},
		{
			name:             "NToN_Different",
			registrySchemas:  10,
			expectedSchemas:  10,
			setupRegistry:    setupMockSchemas,
			generateExpected: generateSchemas,
		},
		{
			name:             "Mixed_AddRemove",
			registrySchemas:  5,
			expectedSchemas:  8,
			setupRegistry:    setupMockSchemas,
			generateExpected: generateMixedSchemas,
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			for _, schemaCount := range []int{1, 10, 100, 1000} {
				b.Run(fmt.Sprintf("Schemas_%d", schemaCount), func(b *testing.B) {
					benchmarkReconcileScenario(b, scenario, schemaCount)
				})
			}
		})
	}
}

// benchmarkReconcileScenario runs a single reconciliation benchmark scenario
func benchmarkReconcileScenario(b *testing.B, scenario struct {
	name             string
	registrySchemas  int
	expectedSchemas  int
	setupRegistry    func(*MockSchemaRegistry, int)
	generateExpected func(int) map[SubjectName]JSONSchemaDefinition
}, schemaCount int) {
	// Setup mock registry
	mockRegistry := NewMockSchemaRegistry()
	defer mockRegistry.Close()

	// Setup initial registry state
	registryCount := min(scenario.registrySchemas, schemaCount)
	scenario.setupRegistry(mockRegistry, registryCount)

	// Generate expected schemas
	expectedCount := min(scenario.expectedSchemas, schemaCount)
	expectedSchemas := scenario.generateExpected(expectedCount)

	// Create registry instance
	registry := NewSchemaRegistry()
	overrideSchemaRegistryAddress(registry, mockRegistry.URL())

	// Create context with generous timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Reset benchmark timer after setup
	b.ResetTimer()

	// Run benchmark
	for i := 0; i < b.N; i++ {
		// Reset registry state for each iteration
		registry.currentPhase = SchemaRegistryPhaseLookup
		registry.rawSubjectsData = nil
		registry.registrySubjects = nil
		registry.missingInRegistry = make(map[SubjectName]JSONSchemaDefinition)
		registry.inRegistryButUnknownLocally = make(map[SubjectName]bool)

		// Perform reconciliation
		err := registry.Reconcile(ctx, expectedSchemas)
		if err != nil {
			b.Fatalf("Reconciliation failed: %v", err)
		}
	}
}

// benchmarkGetMetrics tests metrics collection performance
func benchmarkGetMetrics(b *testing.B) {
	registry := NewSchemaRegistry()

	// Initialize with some state
	registry.totalReconciliations = 1000
	registry.successfulOperations = 950
	registry.failedOperations = 50
	registry.lastOperationTime = time.Now()
	registry.missingInRegistry = generateSchemas(100)
	registry.inRegistryButUnknownLocally = make(map[SubjectName]bool)
	for i := 0; i < 50; i++ {
		registry.inRegistryButUnknownLocally[SubjectName(fmt.Sprintf("unknown-%d", i))] = true
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metrics := registry.GetMetrics()
		_ = metrics // Prevent compiler optimization
	}
}

// benchmarkConcurrent tests concurrent access performance
func benchmarkConcurrent(b *testing.B) {
	scenarios := []struct {
		name        string
		readers     int
		writers     int
		schemaCount int
	}{
		{"ReadHeavy", 10, 1, 100},
		{"WriteHeavy", 2, 8, 100},
		{"Balanced", 5, 5, 100},
		{"HighContention", 20, 20, 50},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchmarkConcurrentScenario(b, scenario.readers, scenario.writers, scenario.schemaCount)
		})
	}
}

// benchmarkConcurrentScenario runs a concurrent access benchmark
func benchmarkConcurrentScenario(b *testing.B, readers, writers, schemaCount int) {
	// Setup mock registry
	mockRegistry := NewMockSchemaRegistry()
	defer mockRegistry.Close()
	setupMockSchemas(mockRegistry, schemaCount)

	// Create registry instance
	registry := NewSchemaRegistry()
	overrideSchemaRegistryAddress(registry, mockRegistry.URL())

	// Generate test schemas
	testSchemas := generateSchemas(schemaCount)

	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup

		// Start readers (GetMetrics calls)
		for r := 0; r < readers; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				metrics := registry.GetMetrics()
				_ = metrics // Prevent compiler optimization
			}()
		}

		// Start writers (Reconcile calls)
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := registry.Reconcile(ctx, testSchemas)
				if err != nil {
					b.Errorf("Reconciliation failed: %v", err)
				}
			}()
		}

		wg.Wait()
	}
}

// Helper functions for generating test data

// generateSchemas creates n unique schemas for testing
func generateSchemas(n int) map[SubjectName]JSONSchemaDefinition {
	schemas := make(map[SubjectName]JSONSchemaDefinition)
	for i := 0; i < n; i++ {
		subject := SubjectName(fmt.Sprintf("test-subject-%d", i))
		schema := JSONSchemaDefinition(fmt.Sprintf(`{
			"type": "object",
			"properties": {
				"id": {"type": "string"},
				"value_%d": {"type": "number"},
				"timestamp": {"type": "string", "format": "date-time"}
			},
			"required": ["id", "value_%d"]
		}`, i, i))
		schemas[subject] = schema
	}
	return schemas
}

// generateSchemasMatching creates schemas that match what setupMockSchemas creates
func generateSchemasMatching(n int) map[SubjectName]JSONSchemaDefinition {
	schemas := make(map[SubjectName]JSONSchemaDefinition)
	for i := 0; i < n; i++ {
		subject := SubjectName(fmt.Sprintf("mock-subject-%d", i))
		schema := JSONSchemaDefinition(fmt.Sprintf(`{
			"type": "object",
			"properties": {
				"mock_field_%d": {"type": "string"}
			}
		}`, i))
		schemas[subject] = schema
	}
	return schemas
}

// generateMixedSchemas creates a mix of overlapping and new schemas
func generateMixedSchemas(n int) map[SubjectName]JSONSchemaDefinition {
	schemas := make(map[SubjectName]JSONSchemaDefinition)

	// Keep some existing schemas (50% overlap)
	for i := 0; i < n/2; i++ {
		subject := SubjectName(fmt.Sprintf("mock-subject-%d", i))
		schema := JSONSchemaDefinition(fmt.Sprintf(`{
			"type": "object",
			"properties": {
				"mock_field_%d": {"type": "string"}
			}
		}`, i))
		schemas[subject] = schema
	}

	// Add new schemas
	for i := n / 2; i < n; i++ {
		subject := SubjectName(fmt.Sprintf("new-subject-%d", i))
		schema := JSONSchemaDefinition(fmt.Sprintf(`{
			"type": "object",
			"properties": {
				"new_field_%d": {"type": "number"}
			}
		}`, i))
		schemas[subject] = schema
	}

	return schemas
}

// setupMockSchemas populates the mock registry with n schemas
func setupMockSchemas(mock *MockSchemaRegistry, n int) {
	for i := 0; i < n; i++ {
		subject := fmt.Sprintf("mock-subject-%d", i)
		schema := fmt.Sprintf(`{
			"type": "object",
			"properties": {
				"mock_field_%d": {"type": "string"}
			}
		}`, i)
		mock.AddSchema(subject, 1, schema)
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BenchmarkSchemaRegistryPhases benchmarks individual reconciliation phases
func BenchmarkSchemaRegistryPhases(b *testing.B) {
	b.Run("LookupPhase", benchmarkLookupPhase)
	b.Run("DecodePhase", benchmarkDecodePhase)
	b.Run("ComparePhase", benchmarkComparePhase)
	b.Run("RemoveUnknownPhase", benchmarkRemoveUnknownPhase)
	b.Run("AddNewPhase", benchmarkAddNewPhase)
}

// benchmarkLookupPhase tests the lookup phase performance
func benchmarkLookupPhase(b *testing.B) {
	for _, schemaCount := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("Schemas_%d", schemaCount), func(b *testing.B) {
			mockRegistry := NewMockSchemaRegistry()
			defer mockRegistry.Close()
			setupMockSchemas(mockRegistry, schemaCount)

			registry := NewSchemaRegistry()
			overrideSchemaRegistryAddress(registry, mockRegistry.URL())
			registry.currentPhase = SchemaRegistryPhaseLookup

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				registry.rawSubjectsData = nil
				registry.registrySubjects = nil

				err, _ := registry.lookup(ctx)
				if err != nil {
					b.Fatalf("Lookup failed: %v", err)
				}
			}
		})
	}
}

// benchmarkDecodePhase tests the decode phase performance
func benchmarkDecodePhase(b *testing.B) {
	for _, schemaCount := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("Schemas_%d", schemaCount), func(b *testing.B) {
			// Pre-generate JSON data
			subjects := make([]string, schemaCount)
			for i := 0; i < schemaCount; i++ {
				subjects[i] = fmt.Sprintf("test-subject-%d", i)
			}

			// Marshal to JSON once
			jsonData, err := json.Marshal(subjects)
			if err != nil {
				b.Fatalf("Failed to marshal test data: %v", err)
			}

			registry := NewSchemaRegistry()
			registry.currentPhase = SchemaRegistryPhaseDecode

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				registry.rawSubjectsData = jsonData
				registry.registrySubjects = nil

				err, _ := registry.decode(ctx)
				if err != nil {
					b.Fatalf("Decode failed: %v", err)
				}
			}
		})
	}
}

// benchmarkComparePhase tests the compare phase performance
func benchmarkComparePhase(b *testing.B) {
	for _, schemaCount := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("Schemas_%d", schemaCount), func(b *testing.B) {
			registry := NewSchemaRegistry()
			registry.currentPhase = SchemaRegistryPhaseCompare

			// Setup registry subjects
			registry.registrySubjects = make([]SubjectName, schemaCount)
			for i := 0; i < schemaCount; i++ {
				registry.registrySubjects[i] = SubjectName(fmt.Sprintf("registry-subject-%d", i))
			}

			// Generate expected schemas (50% overlap)
			expectedSchemas := make(map[SubjectName]JSONSchemaDefinition)
			for i := 0; i < schemaCount/2; i++ {
				subject := SubjectName(fmt.Sprintf("registry-subject-%d", i))
				schema := JSONSchemaDefinition(fmt.Sprintf(`{"type": "object", "properties": {"field_%d": {"type": "string"}}}`, i))
				expectedSchemas[subject] = schema
			}
			for i := schemaCount / 2; i < schemaCount; i++ {
				subject := SubjectName(fmt.Sprintf("expected-subject-%d", i))
				schema := JSONSchemaDefinition(fmt.Sprintf(`{"type": "object", "properties": {"field_%d": {"type": "string"}}}`, i))
				expectedSchemas[subject] = schema
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				registry.missingInRegistry = make(map[SubjectName]JSONSchemaDefinition)
				registry.inRegistryButUnknownLocally = make(map[SubjectName]bool)

				err, _ := registry.compare(ctx, expectedSchemas)
				if err != nil {
					b.Fatalf("Compare failed: %v", err)
				}
			}
		})
	}
}

// benchmarkRemoveUnknownPhase tests the remove unknown phase performance
func benchmarkRemoveUnknownPhase(b *testing.B) {
	for _, schemaCount := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("Schemas_%d", schemaCount), func(b *testing.B) {
			mockRegistry := NewMockSchemaRegistry()
			defer mockRegistry.Close()
			setupMockSchemas(mockRegistry, schemaCount)

			registry := NewSchemaRegistry()
			overrideSchemaRegistryAddress(registry, mockRegistry.URL())
			registry.currentPhase = SchemaRegistryPhaseRemoveUnknown

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Setup unknown subjects to remove
				registry.inRegistryButUnknownLocally = make(map[SubjectName]bool)
				registry.inRegistryButUnknownLocally[SubjectName(fmt.Sprintf("mock-subject-%d", i%schemaCount))] = true

				err, _ := registry.removeUnknown(ctx)
				if err != nil {
					b.Fatalf("RemoveUnknown failed: %v", err)
				}
			}
		})
	}
}

// benchmarkAddNewPhase tests the add new phase performance
func benchmarkAddNewPhase(b *testing.B) {
	for _, schemaCount := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("Schemas_%d", schemaCount), func(b *testing.B) {
			mockRegistry := NewMockSchemaRegistry()
			defer mockRegistry.Close()

			registry := NewSchemaRegistry()
			overrideSchemaRegistryAddress(registry, mockRegistry.URL())
			registry.currentPhase = SchemaRegistryPhaseAddNew

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Setup missing schemas to add
				registry.missingInRegistry = make(map[SubjectName]JSONSchemaDefinition)
				subject := SubjectName(fmt.Sprintf("new-subject-%d", i))
				schema := JSONSchemaDefinition(fmt.Sprintf(`{"type": "object", "properties": {"field_%d": {"type": "string"}}}`, i))
				registry.missingInRegistry[subject] = schema

				err, _ := registry.addNew(ctx)
				if err != nil {
					b.Fatalf("AddNew failed: %v", err)
				}
			}
		})
	}
}
