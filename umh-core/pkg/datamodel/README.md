# UMH Data Model Package

A high-performance Go package for validating and translating UMH data models with comprehensive context cancellation support.

## 🎯 Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

func main() {
    ctx := context.Background()
    
    // Create a data model
    dataModel := config.DataModelVersion{
        Description: "Industrial pump model",
        Structure: map[string]config.Field{
            "pump": {
                Subfields: map[string]config.Field{
                    "speed": {Type: "timeseries-number", Unit: "rpm"},
                    "status": {Type: "timeseries-string"},
                },
            },
        },
    }
    
    // Validate the model
    err := datamodel.ValidateStructureOnly(ctx, dataModel)
    if err != nil {
        fmt.Printf("Validation failed: %v\n", err)
        return
    }
    
    // Translate to JSON schemas  
    schemas, err := datamodel.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", nil)
    if err != nil {
        fmt.Printf("Translation failed: %v\n", err)
        return
    }
    
    // Display results
    for _, schema := range schemas {
        fmt.Printf("Schema %s:\n%s\n\n", schema.Name, schema.Schema)
    }
}
```

## 📦 Package Organization

```go
import (
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/validation"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/translation"
)

// Direct access to validation
validator := validation.NewValidator()
err := validator.ValidateStructureOnly(ctx, dataModel)

// Direct access to translation
translator := translation.NewTranslator()
schemas, err := translator.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", allModels)
```

## 🔧 Input Normalization

The translator automatically handles common user mistakes:

```go
// These all produce the same result:
schemas1, _ := datamodel.TranslateToJSONSchema(ctx, dataModel, "pump", "v1", allModels)
schemas2, _ := datamodel.TranslateToJSONSchema(ctx, dataModel, "_pump", "1", allModels)   // Auto-corrected
schemas3, _ := datamodel.TranslateToJSONSchema(ctx, dataModel, "___pump", "v1", allModels) // Auto-corrected

// All generate: "_pump-v1-number", "_pump-v1-string", etc.
```

- **Model names**: Leading underscores are automatically stripped
- **Versions**: "v" prefix is added if missing (`"1"` → `"v1"`)

## ⚠️ Important Considerations

### 1. Context Cancellation
Always use context for timeout and cancellation:

```go
// With timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := datamodel.ValidateStructureOnly(ctx, dataModel)
if err == context.DeadlineExceeded {
    // Handle timeout
}
```

### 2. Reference Validation Order
- **Structure first**: Always validate structure before references
- **Complete model map**: Ensure all referenced models are in the `allDataModels` map
- **Circular references**: Automatically detected and prevented

### 3. Memory Usage
- **Thread-safe**: All components are stateless and can be used concurrently
- **Reusable**: Create validator/translator once and reuse across multiple operations
- **Minimal overhead**: Essential allocations only for error reporting

### 4. Error Handling
Both validation and translation provide detailed error messages:

```go
err := datamodel.ValidateStructureOnly(ctx, dataModel)
if err != nil {
    // Error includes precise path: "pump.motor.sensor.temperature"
    fmt.Printf("Validation failed: %v\n", err)
}
```

### 5. Performance Scaling
- **Linear scaling**: Performance scales predictably with model complexity
- **Reference overhead**: Reference validation is ~3x slower than structure-only
- **Translation cost**: Translation is ~7-30x slower than validation (expected)

## 🛡️ Production Readiness

### ✅ Validation Features
- **Field name validation**: Letters, numbers, dashes, underscores only
- **Structure validation**: Proper leaf/non-leaf node rules
- **Reference validation**: Existence checking and circular reference detection
- **Depth limiting**: Maximum 10 levels of reference nesting
- **Context support**: Graceful cancellation and timeout handling

### ✅ Translation Features
- **Multiple schema generation**: Separate schemas per value type
- **Reference resolution**: Handles complex reference chains
- **Extensible architecture**: Easy to add new type categories
- **Input normalization**: Automatic correction of common mistakes
- **Space-ready**: Timeout protection and panic recovery

### ✅ Quality Assurance
- **Comprehensive testing**: 55+ tests covering all scenarios
- **Performance validated**: Extensive benchmarking across complexity levels
- **Memory profiled**: Allocation analysis and optimization
- **Context tested**: Cancellation and timeout scenarios

## 📊 Benchmark Results

### Validation Benchmarks
```
BenchmarkValidateStructureOnly_Simple-11     	 9,300,000 ops/sec	     256 B/op	   1 allocs/op
BenchmarkValidateStructureOnly_Complex-11    	   846,000 ops/sec	     712 B/op	  20 allocs/op
BenchmarkValidateWithReferences-11            	   817,000 ops/sec	     984 B/op	  21 allocs/op
BenchmarkValidateWithReferences_Deep-11       	   858,000 ops/sec	   1,392 B/op	  22 allocs/op
BenchmarkValidateStructureOnly_Large-11       	   128,000 ops/sec	   2,656 B/op	 101 allocs/op
```

### Translation Benchmarks
```
BenchmarkTranslatorSimple-11                  	   113,000 ops/sec	   7,704 B/op	  41 allocs/op
BenchmarkTranslatorComplexNested-11           	    74,000 ops/sec	  14,469 B/op	  82 allocs/op
BenchmarkTranslatorWithReferences-11          	    83,000 ops/sec	  12,419 B/op	  81 allocs/op
BenchmarkTranslatorLargeSchema-11             	    14,500 ops/sec	  90,012 B/op	 262 allocs/op
```

## 🔍 Common Patterns

### Industrial Equipment Model
```go
dataModel := config.DataModelVersion{
    Description: "Industrial pump",
    Structure: map[string]config.Field{
        "pump": {
            Subfields: map[string]config.Field{
                "motor": {
                    Subfields: map[string]config.Field{
                        "speed": {Type: "timeseries-number", Unit: "rpm"},
                        "temperature": {Type: "timeseries-number", Unit: "°C"},
                    },
                },
                "status": {Type: "timeseries-string"},
            },
        },
    },
}
```

### Model with References
```go
pumpModel := config.DataModelVersion{
    Structure: map[string]config.Field{
        "motor": {ModelRef: "motor:v1"},
        "sensors": {
            Subfields: map[string]config.Field{
                "temperature": {ModelRef: "sensor:v1"},
                "pressure": {ModelRef: "sensor:v1"},
            },
        },
    },
}
```

### Complete Validation Pipeline
```go
func validateAndTranslate(ctx context.Context, dataModel config.DataModelVersion, modelName, version string, allModels map[string]config.DataModelsConfig) ([]datamodel.SchemaOutput, error) {
    // Step 1: Validate structure
    if err := datamodel.ValidateStructureOnly(ctx, dataModel); err != nil {
        return nil, fmt.Errorf("structure validation failed: %w", err)
    }
    
    // Step 2: Validate references
    if err := datamodel.ValidateWithReferences(ctx, dataModel, allModels); err != nil {
        return nil, fmt.Errorf("reference validation failed: %w", err)
    }
    
    // Step 3: Translate to JSON schemas
    schemas, err := datamodel.TranslateToJSONSchema(ctx, dataModel, modelName, version, allModels)
    if err != nil {
        return nil, fmt.Errorf("translation failed: %w", err)
    }
    
    return schemas, nil
}
```

## 📚 Architecture

The package is organized into focused sub-packages:

- **`validation/`** - High-performance data model validation
- **`translation/`** - JSON schema generation with type separation
- **Public facades** - Top-level convenience APIs for backward compatibility

This design provides clean separation of concerns while maintaining ease of use for common scenarios.

---

**Ready for production use** in high-throughput industrial environments with enterprise-grade reliability and performance.
