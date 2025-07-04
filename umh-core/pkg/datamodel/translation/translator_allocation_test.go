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
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/translation"
)

// BenchmarkPathConstruction benchmarks just the path building logic
func BenchmarkPathConstruction(b *testing.B) {
	// Simulate nested path construction
	levels := []string{"level1", "level2", "level3", "level4", "field"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var currentPath string
		for j, level := range levels {
			if j == 0 {
				currentPath = level
			} else {
				currentPath = currentPath + "." + level // This is how translator builds paths
			}
		}
		_ = currentPath // Prevent optimization
	}
}

// BenchmarkPathConstructionOptimized benchmarks optimized path building
func BenchmarkPathConstructionOptimized(b *testing.B) {
	levels := []string{"level1", "level2", "level3", "level4", "field"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Pre-calculate total length to avoid string reallocations
		totalLen := len(levels[0])
		for j := 1; j < len(levels); j++ {
			totalLen += 1 + len(levels[j]) // +1 for dot
		}

		// Build path with pre-allocated buffer
		result := make([]byte, 0, totalLen)
		result = append(result, levels[0]...)
		for j := 1; j < len(levels); j++ {
			result = append(result, '.')
			result = append(result, levels[j]...)
		}
		_ = string(result)
	}
}

// BenchmarkSliceGrowth benchmarks slice allocation patterns
func BenchmarkSliceGrowth(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var paths []translation.PathInfo
		// Simulate collecting 25 paths (like complex nested scenario)
		for j := 0; j < 25; j++ {
			paths = append(paths, translation.PathInfo{
				Path:      fmt.Sprintf("field%d", j),
				ValueType: "timeseries-number",
			})
		}
		_ = paths
	}
}

// BenchmarkSliceGrowthPreallocated benchmarks pre-allocated slice growth
func BenchmarkSliceGrowthPreallocated(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Pre-allocate with reasonable capacity
		paths := make([]translation.PathInfo, 0, 32)
		for j := 0; j < 25; j++ {
			paths = append(paths, translation.PathInfo{
				Path:      fmt.Sprintf("field%d", j),
				ValueType: "timeseries-number",
			})
		}
		_ = paths
	}
}

// BenchmarkTypeGrouping benchmarks the path grouping logic
func BenchmarkTypeGrouping(b *testing.B) {
	// Create paths to group
	paths := make([]translation.PathInfo, 50)
	for i := 0; i < 50; i++ {
		var valueType string
		switch i % 3 {
		case 0:
			valueType = "timeseries-number"
		case 1:
			valueType = "timeseries-string"
		case 2:
			valueType = "timeseries-boolean"
		}
		paths[i] = translation.PathInfo{
			Path:      fmt.Sprintf("field%d", i),
			ValueType: valueType,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Manual grouping to simulate the translator's logic
		groups := make(map[string][]translation.PathInfo)
		for _, path := range paths {
			groups[path.ValueType] = append(groups[path.ValueType], path)
		}
		_ = groups
	}
}

// BenchmarkJSONMarshaling benchmarks just the JSON schema marshaling
func BenchmarkJSONMarshaling(b *testing.B) {
	// Create a representative JSON schema structure
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"virtual_path": map[string]interface{}{
				"type": "string",
				"enum": []string{"field1", "field2", "field3", "field4", "field5"},
			},
			"fields": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"value": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"timestamp_ms": map[string]interface{}{
								"type": "number",
							},
							"value": map[string]interface{}{
								"type": "number",
							},
						},
						"required":             []string{"timestamp_ms", "value"},
						"additionalProperties": false,
					},
				},
				"additionalProperties": false,
			},
		},
		"required":             []string{"virtual_path", "fields"},
		"additionalProperties": false,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			b.Fatalf("JSON marshaling failed: %v", err)
		}
	}
}

// BenchmarkReferenceKeyConstruction benchmarks reference key building
func BenchmarkReferenceKeyConstruction(b *testing.B) {
	modelName := "sensor"
	version := "v1"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		referenceKey := modelName + ":" + version
		_ = referenceKey
	}
}

// BenchmarkMapOperations benchmarks map operations used in reference tracking
func BenchmarkMapOperations(b *testing.B) {
	visitedModels := make(map[string]bool)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("model%d:v1", i%10) // Cycle through 10 models

		// Check if visited
		if visitedModels[key] {
			// Skip if circular
			continue
		}

		// Mark as visited
		visitedModels[key] = true

		// Unmark (backtrack)
		delete(visitedModels, key)
	}
}

// BenchmarkPathSorting benchmarks sorting paths for schema generation
func BenchmarkPathSorting(b *testing.B) {
	// Create unsorted paths
	basePaths := []string{
		"zebra", "apple", "banana", "cherry", "date", "elderberry",
		"fig", "grape", "honeydew", "kiwi", "lemon", "mango",
		"nested.zebra", "nested.apple", "nested.banana",
		"deep.nested.field", "deep.nested.another",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Copy paths (simulating extraction from PathInfo)
		paths := make([]string, len(basePaths))
		copy(paths, basePaths)

		// Sort paths
		sort.Strings(paths)
		_ = paths
	}
}

// BenchmarkStructCreation benchmarks creating the JSON schema struct
func BenchmarkStructCreation(b *testing.B) {
	paths := []string{"field1", "field2", "field3", "field4", "field5"}
	jsonType := "number"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// This simulates the structure creation in TimeseriesTranslator
		schema := struct {
			Type       string `json:"type"`
			Properties struct {
				VirtualPath struct {
					Type string   `json:"type"`
					Enum []string `json:"enum"`
				} `json:"virtual_path"`
				Fields struct {
					Type       string `json:"type"`
					Properties struct {
						Value struct {
							Type       string `json:"type"`
							Properties struct {
								TimestampMs struct {
									Type string `json:"type"`
								} `json:"timestamp_ms"`
								Value struct {
									Type string `json:"type"`
								} `json:"value"`
							} `json:"properties"`
							Required             []string `json:"required"`
							AdditionalProperties bool     `json:"additionalProperties"`
						} `json:"value"`
					} `json:"properties"`
					AdditionalProperties bool `json:"additionalProperties"`
				} `json:"fields"`
			} `json:"properties"`
			Required             []string `json:"required"`
			AdditionalProperties bool     `json:"additionalProperties"`
		}{
			Type: "object",
		}

		schema.Properties.VirtualPath.Type = "string"
		schema.Properties.VirtualPath.Enum = paths
		schema.Properties.Fields.Type = "object"
		schema.Properties.Fields.Properties.Value.Type = "object"
		schema.Properties.Fields.Properties.Value.Properties.TimestampMs.Type = "number"
		schema.Properties.Fields.Properties.Value.Properties.Value.Type = jsonType
		schema.Properties.Fields.Properties.Value.Required = []string{"timestamp_ms", "value"}
		schema.Properties.Fields.Properties.Value.AdditionalProperties = false
		schema.Properties.Fields.AdditionalProperties = false
		schema.Required = []string{"virtual_path", "fields"}
		schema.AdditionalProperties = false

		_ = schema
	}
}

// BenchmarkStringIndexByte benchmarks the string parsing used in reference resolution
func BenchmarkStringIndexByte(b *testing.B) {
	modelRef := "sensor:v1"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		colonIndex := fmt.Sprintf("strings.IndexByte(%s, ':')", modelRef) // This will allocate
		_ = colonIndex
	}
}

// BenchmarkStringIndexByteOptimized benchmarks optimized string parsing
func BenchmarkStringIndexByteOptimized(b *testing.B) {
	modelRef := "sensor:v1"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		colonIndex := len(modelRef) // Just to have some work, no actual indexOf needed for bench
		for j, c := range []byte(modelRef) {
			if c == ':' {
				colonIndex = j
				break
			}
		}
		_ = colonIndex
	}
}

// BenchmarkCollectPathsOnly benchmarks just the path collection without JSON generation
func BenchmarkCollectPathsOnly(b *testing.B) {
	ctx := context.Background()

	// Simple structure for collection
	structure := map[string]config.Field{
		"field1": {Type: "timeseries-number"},
		"field2": {Type: "timeseries-string"},
		"nested": {
			Subfields: map[string]config.Field{
				"field3": {Type: "timeseries-number"},
				"field4": {Type: "timeseries-boolean"},
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Manual path collection to simulate the translator's logic
		var paths []translation.PathInfo

		// Collect paths manually
		for fieldName, field := range structure {
			if field.Type != "" {
				paths = append(paths, translation.PathInfo{
					Path:      fieldName,
					ValueType: field.Type,
				})
			}
			if field.Subfields != nil {
				for subFieldName, subField := range field.Subfields {
					if subField.Type != "" {
						paths = append(paths, translation.PathInfo{
							Path:      fieldName + "." + subFieldName,
							ValueType: subField.Type,
						})
					}
				}
			}
		}

		// Context check to simulate real logic
		select {
		case <-ctx.Done():
			b.Fatalf("Context cancelled")
		default:
		}

		_ = paths
	}
}

// BenchmarkTranslateOnlyNumbers benchmarks translation with only number types
func BenchmarkTranslateOnlyNumbers(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Create structure with only number fields to isolate type-specific costs
	structure := make(map[string]config.Field)
	for i := 0; i < 10; i++ {
		fieldName := fmt.Sprintf("numberField%d", i)
		structure[fieldName] = config.Field{
			Type:        "timeseries-number",
			Description: fmt.Sprintf("Number field %d", i),
		}
	}

	dataModel := config.DataModelVersion{
		Description: "Numbers only",
		Structure:   structure,
	}

	allModels := make(map[string]config.DataModelsConfig)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, dataModel, "numbers", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}

// BenchmarkTranslateMixedTypes benchmarks the overhead of multiple type handling
func BenchmarkTranslateMixedTypes(b *testing.B) {
	translator := translation.NewTranslator()
	ctx := context.Background()

	// Create structure with mixed types to see grouping overhead
	structure := make(map[string]config.Field)
	for i := 0; i < 10; i++ {
		var fieldType string
		switch i % 3 {
		case 0:
			fieldType = "timeseries-number"
		case 1:
			fieldType = "timeseries-string"
		case 2:
			fieldType = "timeseries-boolean"
		}

		fieldName := fmt.Sprintf("field%d", i)
		structure[fieldName] = config.Field{
			Type:        fieldType,
			Description: fmt.Sprintf("Field %d", i),
		}
	}

	dataModel := config.DataModelVersion{
		Description: "Mixed types",
		Structure:   structure,
	}

	allModels := make(map[string]config.DataModelsConfig)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := translator.TranslateToJSONSchema(ctx, dataModel, "mixed", "v1", allModels)
		if err != nil {
			b.Fatalf("Translation failed: %v", err)
		}
	}
}
