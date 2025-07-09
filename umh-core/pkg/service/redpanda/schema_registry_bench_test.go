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
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/redpanda"
)

// BenchmarkSchemaRegistry provides comprehensive performance benchmarks for schema registry operations
func BenchmarkSchemaRegistry(b *testing.B) {
	// Setup Redpanda container for benchmarks
	ctx := context.Background()
	container, err := redpanda.Run(ctx, "redpandadata/redpanda:latest")
	if err != nil {
		b.Fatalf("Failed to start Redpanda container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			b.Logf("Failed to terminate container: %v", err)
		}
	}()

	schemaRegistryURL, err := container.SchemaRegistryAddress(ctx)
	if err != nil {
		b.Fatalf("Failed to get schema registry URL: %v", err)
	}

	registry := NewSchemaRegistry(WithSchemaRegistryAddress(schemaRegistryURL))

	// Wait for registry to be ready
	for i := 0; i < 30; i++ {
		if err := registry.Reconcile(ctx, map[SubjectName]JSONSchemaDefinition{}); err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Perform comprehensive warmup
	performWarmup(b, registry)

	b.Run("SingleSchema", func(b *testing.B) {
		benchmarkSingleSchema(b, registry)
	})

	b.Run("MultipleSchemas", func(b *testing.B) {
		benchmarkMultipleSchemas(b, registry)
	})

	b.Run("IncrementalUpdates", func(b *testing.B) {
		benchmarkIncrementalUpdates(b, registry)
	})

	b.Run("LargeSchemaPayload", func(b *testing.B) {
		benchmarkLargeSchemaPayload(b, registry)
	})

	b.Run("MetricsAccess", func(b *testing.B) {
		benchmarkMetricsAccess(b, registry)
	})

	b.Run("ConcurrentReconciliation", func(b *testing.B) {
		benchmarkConcurrentReconciliation(b, registry)
	})

	b.Run("AddOnly", func(b *testing.B) {
		benchmarkAddOnly(b, registry)
	})

	b.Run("RemoveOnly", func(b *testing.B) {
		benchmarkRemoveOnly(b, registry)
	})

	b.Run("MixedOperations", func(b *testing.B) {
		benchmarkMixedOperations(b, registry)
	})
}

// performWarmup ensures stable performance measurements by priming the system
func performWarmup(b *testing.B, registry *SchemaRegistry) {
	b.Helper()

	ctx := context.Background()

	// Pre-generate all schemas used in warmup to avoid measuring generation overhead
	singleSchema := map[SubjectName]JSONSchemaDefinition{
		"warmup-single": JSONSchemaDefinition(`{
			"type": "object",
			"properties": {
				"warmup": {"type": "boolean"}
			}
		}`),
	}

	multipleSchemas := generateSchemas(10, "warmup-multi")

	largeSchema := map[SubjectName]JSONSchemaDefinition{
		"warmup-large": generateLargeSchema(50),
	}

	emptySchemas := map[SubjectName]JSONSchemaDefinition{}

	// Phase 1: HTTP connection warmup
	b.Logf("Warmup Phase 1: HTTP connection establishment")
	for i := 0; i < 10; i++ {
		warmupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = registry.Reconcile(warmupCtx, emptySchemas)
		cancel()
	}

	// Phase 2: Single schema operations warmup
	b.Logf("Warmup Phase 2: Single schema operations")
	for i := 0; i < 20; i++ {
		warmupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_ = reconcileUntilComplete(warmupCtx, registry, singleSchema)
		cancel()
	}

	// Phase 3: Multiple schemas warmup
	b.Logf("Warmup Phase 3: Multiple schemas operations")
	for i := 0; i < 10; i++ {
		warmupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		_ = reconcileUntilComplete(warmupCtx, registry, multipleSchemas)
		cancel()
	}

	// Phase 4: Large schema warmup
	b.Logf("Warmup Phase 4: Large schema operations")
	for i := 0; i < 5; i++ {
		warmupCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		_ = reconcileUntilComplete(warmupCtx, registry, largeSchema)
		cancel()
	}

	// Phase 5: Metrics access warmup
	b.Logf("Warmup Phase 5: Metrics access")
	for i := 0; i < 1000; i++ {
		_ = registry.GetMetrics()
	}

	// Phase 6: Clean slate for benchmarks
	b.Logf("Warmup Phase 6: Clean slate preparation")
	cleanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	_ = reconcileUntilComplete(cleanCtx, registry, emptySchemas)
	cancel()

	// Allow system to stabilize
	time.Sleep(500 * time.Millisecond)

	b.Logf("Warmup completed - system ready for benchmarks")
}

// benchmarkSingleSchema measures performance of reconciling a single schema
func benchmarkSingleSchema(b *testing.B, registry *SchemaRegistry) {
	schema := map[SubjectName]JSONSchemaDefinition{
		"benchmark-single": JSONSchemaDefinition(`{
			"type": "object",
			"properties": {
				"id": {"type": "string"},
				"timestamp": {"type": "string", "format": "date-time"},
				"value": {"type": "number"}
			},
			"required": ["id", "timestamp", "value"]
		}`),
	}

	// Benchmark-specific warmup
	warmupSingleSchema(b, registry, schema)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := registry.Reconcile(ctx, schema)
		if err != nil {
			b.Fatalf("Reconciliation failed: %v", err)
		}
		cancel()
	}
}

// benchmarkMultipleSchemas measures performance with varying numbers of schemas
func benchmarkMultipleSchemas(b *testing.B, registry *SchemaRegistry) {
	schemaCounts := []int{5, 10, 25, 50, 100}

	for _, count := range schemaCounts {
		b.Run(fmt.Sprintf("Schemas_%d", count), func(b *testing.B) {
			// Pre-generate schemas outside benchmark timing
			schemas := generateSchemas(count, "multi-bench")

			// Benchmark-specific warmup
			warmupMultipleSchemas(b, registry, schemas)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

				// Measure time to complete reconciliation
				start := time.Now()
				err := reconcileUntilComplete(ctx, registry, schemas)
				duration := time.Since(start)

				if err != nil {
					b.Fatalf("Reconciliation failed for %d schemas: %v", count, err)
				}

				// Report custom metrics
				b.ReportMetric(float64(duration.Nanoseconds()), "ns/reconciliation")
				b.ReportMetric(float64(count), "schemas/reconciliation")
				b.ReportMetric(float64(duration.Nanoseconds())/float64(count), "ns/schema")

				cancel()
			}
		})
	}
}

// benchmarkIncrementalUpdates measures performance of incremental schema updates
func benchmarkIncrementalUpdates(b *testing.B, registry *SchemaRegistry) {
	baseSchemas := generateSchemas(20, "incremental-base")

	// Setup base schemas
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := reconcileUntilComplete(ctx, registry, baseSchemas)
	cancel()
	if err != nil {
		b.Fatalf("Failed to setup base schemas: %v", err)
	}

	// Pre-generate all schemas we'll need for incremental updates
	const maxIterations = 10000 // More than enough for any reasonable benchmark
	preGeneratedSchemas := make([]map[SubjectName]JSONSchemaDefinition, maxIterations)

	for i := 0; i < maxIterations; i++ {
		// Create incremental update: keep 15, remove 5, add 10
		updatedSchemas := make(map[SubjectName]JSONSchemaDefinition)

		// Keep first 15 from base
		j := 0
		for subject, schema := range baseSchemas {
			if j >= 15 {
				break
			}
			updatedSchemas[subject] = schema
			j++
		}

		// Add 10 new schemas (pre-generated)
		newSchemas := generateSchemas(10, fmt.Sprintf("incremental-new-%d", i))
		for subject, schema := range newSchemas {
			updatedSchemas[subject] = schema
		}

		preGeneratedSchemas[i] = updatedSchemas
	}

	// Benchmark-specific warmup using pre-generated schemas
	warmupIncrementalUpdatesWithPreGenerated(b, registry, preGeneratedSchemas[:3])

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use pre-generated schemas
		updatedSchemas := preGeneratedSchemas[i%maxIterations]

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		err := reconcileUntilComplete(ctx, registry, updatedSchemas)
		duration := time.Since(start)

		if err != nil {
			b.Fatalf("Incremental update failed: %v", err)
		}

		b.ReportMetric(float64(duration.Nanoseconds()), "ns/incremental-update")
		cancel()
	}
}

// benchmarkLargeSchemaPayload measures performance with large schema definitions
func benchmarkLargeSchemaPayload(b *testing.B, registry *SchemaRegistry) {
	// Pre-generate large schema outside benchmark timing
	largeSchema := generateLargeSchema(100) // 100 properties
	schemas := map[SubjectName]JSONSchemaDefinition{
		"benchmark-large": largeSchema,
	}

	// Benchmark-specific warmup
	warmupLargeSchema(b, registry, schemas)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		start := time.Now()
		err := reconcileUntilComplete(ctx, registry, schemas)
		duration := time.Since(start)

		if err != nil {
			b.Fatalf("Large schema reconciliation failed: %v", err)
		}

		// Report payload size and time
		payloadSize := len(string(largeSchema))
		b.ReportMetric(float64(duration.Nanoseconds()), "ns/large-schema")
		b.ReportMetric(float64(payloadSize), "bytes/schema")
		b.ReportMetric(float64(duration.Nanoseconds())/float64(payloadSize), "ns/byte")

		cancel()
	}
}

// benchmarkMetricsAccess measures performance of metrics retrieval
func benchmarkMetricsAccess(b *testing.B, registry *SchemaRegistry) {
	// Benchmark-specific warmup
	for i := 0; i < 10000; i++ {
		_ = registry.GetMetrics()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics := registry.GetMetrics()
		// Ensure we're actually using the metrics to prevent optimization
		if metrics.TotalReconciliations < 0 {
			b.Fatal("Invalid metrics")
		}
	}
}

// benchmarkConcurrentReconciliation measures performance under concurrent load
func benchmarkConcurrentReconciliation(b *testing.B, registry *SchemaRegistry) {
	schema := map[SubjectName]JSONSchemaDefinition{
		"concurrent-bench": JSONSchemaDefinition(`{
			"type": "object",
			"properties": {
				"concurrent": {"type": "boolean"},
				"thread": {"type": "number"}
			}
		}`),
	}

	// Benchmark-specific warmup
	warmupConcurrentReconciliation(b, registry, schema)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := registry.Reconcile(ctx, schema)
			if err != nil {
				b.Errorf("Concurrent reconciliation failed: %v", err)
			}
			cancel()
		}
	})
}

// benchmarkAddOnly measures performance when only adding schemas (empty registry → populated)
func benchmarkAddOnly(b *testing.B, registry *SchemaRegistry) {
	// Pre-generate schemas for adding
	schemasToAdd := generateSchemas(20, "add-only")

	// Warmup: ensure we start with a clean registry
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = reconcileUntilComplete(ctx, registry, map[SubjectName]JSONSchemaDefinition{})
		cancel()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Start with empty registry
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = reconcileUntilComplete(ctx, registry, map[SubjectName]JSONSchemaDefinition{})
		cancel()

		// Measure adding schemas
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		err := reconcileUntilComplete(ctx, registry, schemasToAdd)
		duration := time.Since(start)

		if err != nil {
			b.Fatalf("Add-only reconciliation failed: %v", err)
		}

		b.ReportMetric(float64(duration.Nanoseconds()), "ns/add-only")
		b.ReportMetric(float64(len(schemasToAdd)), "schemas/added")
		b.ReportMetric(float64(duration.Nanoseconds())/float64(len(schemasToAdd)), "ns/schema-add")

		cancel()
	}
}

// benchmarkRemoveOnly measures performance when only removing schemas (populated registry → empty)
func benchmarkRemoveOnly(b *testing.B, registry *SchemaRegistry) {
	// Pre-generate schemas for setup
	schemasToRemove := generateSchemas(20, "remove-only")

	// Warmup: practice the remove-only pattern
	for i := 0; i < 3; i++ {
		// Setup schemas
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = reconcileUntilComplete(ctx, registry, schemasToRemove)
		cancel()

		// Remove all schemas
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		_ = reconcileUntilComplete(ctx, registry, map[SubjectName]JSONSchemaDefinition{})
		cancel()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Setup: populate registry with schemas
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := reconcileUntilComplete(ctx, registry, schemasToRemove)
		if err != nil {
			b.Fatalf("Failed to setup schemas for removal: %v", err)
		}
		cancel()

		// Measure removing all schemas
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		err = reconcileUntilComplete(ctx, registry, map[SubjectName]JSONSchemaDefinition{})
		duration := time.Since(start)

		if err != nil {
			b.Fatalf("Remove-only reconciliation failed: %v", err)
		}

		b.ReportMetric(float64(duration.Nanoseconds()), "ns/remove-only")
		b.ReportMetric(float64(len(schemasToRemove)), "schemas/removed")
		b.ReportMetric(float64(duration.Nanoseconds())/float64(len(schemasToRemove)), "ns/schema-remove")

		cancel()
	}
}

// benchmarkMixedOperations measures performance when both adding and removing schemas
func benchmarkMixedOperations(b *testing.B, registry *SchemaRegistry) {
	// Pre-generate schemas for mixed operations
	baseSchemas := generateSchemas(20, "mixed-base")

	// Pre-generate all mixed operation scenarios
	const maxIterations = 10000
	preGeneratedMixedSchemas := make([]map[SubjectName]JSONSchemaDefinition, maxIterations)

	for i := 0; i < maxIterations; i++ {
		mixedSchemas := make(map[SubjectName]JSONSchemaDefinition)

		// Keep first 10 from base (remove 10)
		j := 0
		for subject, schema := range baseSchemas {
			if j >= 10 {
				break
			}
			mixedSchemas[subject] = schema
			j++
		}

		// Add 10 new schemas
		newSchemas := generateSchemas(10, fmt.Sprintf("mixed-new-%d", i))
		for subject, schema := range newSchemas {
			mixedSchemas[subject] = schema
		}

		preGeneratedMixedSchemas[i] = mixedSchemas
	}

	// Warmup: practice mixed operations
	for i := 0; i < 3; i++ {
		// Setup base schemas
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = reconcileUntilComplete(ctx, registry, baseSchemas)
		cancel()

		// Perform mixed operation
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		_ = reconcileUntilComplete(ctx, registry, preGeneratedMixedSchemas[i])
		cancel()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Setup: start with base schemas
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := reconcileUntilComplete(ctx, registry, baseSchemas)
		if err != nil {
			b.Fatalf("Failed to setup base schemas: %v", err)
		}
		cancel()

		// Measure mixed operation (remove 10, add 10)
		mixedSchemas := preGeneratedMixedSchemas[i%maxIterations]

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		err = reconcileUntilComplete(ctx, registry, mixedSchemas)
		duration := time.Since(start)

		if err != nil {
			b.Fatalf("Mixed operations reconciliation failed: %v", err)
		}

		b.ReportMetric(float64(duration.Nanoseconds()), "ns/mixed-ops")
		b.ReportMetric(10.0, "schemas/removed")
		b.ReportMetric(10.0, "schemas/added")
		b.ReportMetric(float64(duration.Nanoseconds())/20.0, "ns/schema-mixed")

		cancel()
	}
}

// Benchmark-specific warmup functions

// warmupSingleSchema performs targeted warmup for single schema benchmarks
func warmupSingleSchema(b *testing.B, registry *SchemaRegistry, schema map[SubjectName]JSONSchemaDefinition) {
	b.Helper()
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = registry.Reconcile(ctx, schema)
		cancel()
	}
}

// warmupMultipleSchemas performs targeted warmup for multiple schema benchmarks
func warmupMultipleSchemas(b *testing.B, registry *SchemaRegistry, schemas map[SubjectName]JSONSchemaDefinition) {
	b.Helper()
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = reconcileUntilComplete(ctx, registry, schemas)
		cancel()
	}
}

// warmupIncrementalUpdatesWithPreGenerated performs targeted warmup using pre-generated schemas
func warmupIncrementalUpdatesWithPreGenerated(b *testing.B, registry *SchemaRegistry, preGeneratedSchemas []map[SubjectName]JSONSchemaDefinition) {
	b.Helper()

	for i, schemas := range preGeneratedSchemas {
		if i >= 3 { // Only use first 3 for warmup
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_ = reconcileUntilComplete(ctx, registry, schemas)
		cancel()
	}
}

// warmupLargeSchema performs targeted warmup for large schema benchmarks
func warmupLargeSchema(b *testing.B, registry *SchemaRegistry, schemas map[SubjectName]JSONSchemaDefinition) {
	b.Helper()
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = reconcileUntilComplete(ctx, registry, schemas)
		cancel()
	}
}

// warmupConcurrentReconciliation performs targeted warmup for concurrent benchmarks
func warmupConcurrentReconciliation(b *testing.B, registry *SchemaRegistry, schema map[SubjectName]JSONSchemaDefinition) {
	b.Helper()

	// Sequential warmup first
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = registry.Reconcile(ctx, schema)
		cancel()
	}

	// Concurrent warmup
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 5; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_ = registry.Reconcile(ctx, schema)
				cancel()
			}
		}()
	}

	// Wait for all warmup goroutines to complete
	for i := 0; i < 5; i++ {
		<-done
	}
}

// Helper functions

// generateSchemas creates a map of schemas with specified count and prefix
func generateSchemas(count int, prefix string) map[SubjectName]JSONSchemaDefinition {
	schemas := make(map[SubjectName]JSONSchemaDefinition)
	for i := 0; i < count; i++ {
		subject := SubjectName(fmt.Sprintf("%s-%d", prefix, i))
		schema := JSONSchemaDefinition(fmt.Sprintf(`{
			"type": "object",
			"properties": {
				"id": {"type": "string"},
				"index": {"type": "number", "minimum": %d, "maximum": %d},
				"timestamp": {"type": "string", "format": "date-time"},
				"category": {"type": "string", "enum": ["A", "B", "C"]},
				"active": {"type": "boolean"},
				"metadata": {
					"type": "object",
					"properties": {
						"version": {"type": "string"},
						"source": {"type": "string"}
					}
				}
			},
			"required": ["id", "index", "timestamp"]
		}`, i, i+1000))
		schemas[subject] = schema
	}
	return schemas
}

// generateLargeSchema creates a schema with many properties
func generateLargeSchema(propertyCount int) JSONSchemaDefinition {
	properties := make(map[string]interface{})
	required := make([]string, 0, propertyCount/2)

	for i := 0; i < propertyCount; i++ {
		propName := fmt.Sprintf("property_%d", i)
		properties[propName] = map[string]interface{}{
			"type":        "string",
			"description": fmt.Sprintf("Property %d for large schema testing", i),
		}

		// Make half of them required
		if i%2 == 0 {
			required = append(required, propName)
		}
	}

	schema := map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}

	// Convert to JSON
	jsonBytes, _ := json.Marshal(schema)
	return JSONSchemaDefinition(string(jsonBytes))
}

// reconcileUntilComplete calls reconcile repeatedly until completion
func reconcileUntilComplete(ctx context.Context, registry *SchemaRegistry, schemas map[SubjectName]JSONSchemaDefinition) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := registry.Reconcile(ctx, schemas)
		if err != nil {
			return err
		}

		metrics := registry.GetMetrics()
		if metrics.SubjectsToAdd == 0 && metrics.SubjectsToRemove == 0 {
			return nil // Complete
		}

		// Small delay to prevent busy waiting
		time.Sleep(10 * time.Millisecond)
	}
}
