# Data Model Package

This package provides high-performance validation and translation for UMH data models with comprehensive context cancellation support.

## Performance

### Validator Performance

The validator is **extensively optimized** and exceeds performance targets by significant margins:

- **Simple schemas**: 7.7M validations/sec (7,700x target)
- **Complex nested**: 833K validations/sec (833x target)  
- **With references**: 2.68M validations/sec (2,680x target)
- **Full validation**: 794K validations/sec (794x target)

Memory usage is minimal with essential overhead only for error reporting and path tracking.

### Translator Performance

The translator provides **high-performance** data model to JSON Schema conversion:

- **Simple models**: 400K translations/sec (2.5µs per translation)
- **Complex nested**: 153K translations/sec (6.5µs per translation)
- **With references**: 197K translations/sec (5.1µs per translation)  
- **Multiple payload shapes**: 266K translations/sec (3.8µs per translation)
- **Large models**: 13K translations/sec (76µs per translation)

The translator combines validation and JSON Schema generation in a single pass for maximum efficiency.

## APIs

### `ValidateStructureOnly(ctx, dataModel)`

Validates a data model's structure without checking references to other models.

**Validation Rules:**
- Field names can only contain letters, numbers, dashes, and underscores
- A node can either be a leaf node or a non-leaf node
- A leaf node must have either `_type` or `_refModel` (but not both)
- A non-leaf node (folder) can only have subfields, no `_type`, `_description`, or `_unit`
- `_refModel` format and version validation (format: `model:v1`, versions start at v1)

**Use Case:** Quick validation during data model creation/editing where you don't need to verify that referenced models exist.

**Performance:** 833K-7.7M validations/sec depending on complexity.

### `ValidateWithReferences(ctx, dataModel, allDataModels)`

Validates a data model and all its references recursively.

**Additional Validation:**
- Checks that all referenced models exist in the provided map
- Detects circular references
- Limits recursion depth to 10 levels for safety
- Validates referenced model structures recursively

**Use Case:** Complete validation before deploying or using data models in production.

**Performance:** 794K-2.68M validations/sec depending on reference complexity.

### `TranslateDataModel(ctx, contractName, version, dataModel, payloadShapes, modelReferences)`

Translates a UMH data model into JSON Schema format compatible with Schema Registry.

**Translation Process:**
- Validates the data model structure before translation
- Auto-injects standard payload shapes (timeseries-number, timeseries-string) if not provided
- Converts nested field structures to JSON Schema objects
- Handles model references using JSON Schema $ref constructs
- Groups fields by payload shape for efficient schema organization
- Generates Schema Registry-compatible subject names

**Circular Reference Protection:**
- Detects and prevents infinite loops (A→B→A scenarios)
- Limits resolution depth to 10 levels for safety
- Provides clear error messages with reference paths

**Output Format:**
- Map of Schema Registry subjects to JSON Schema objects
- Payload shape usage tracking for optimization
- Compact JSON output ready for Schema Registry

**Use Case:** Converting validated data models to JSON schemas for benthos-umh UNS output plugin and Schema Registry integration.

**Performance:** 13K-400K translations/sec depending on model complexity.

## Context Support

All validation and translation methods support context cancellation with minimal performance overhead (<1%):

- **Cancellation**: If the context is cancelled, validation stops immediately and returns `context.Canceled`
- **Timeouts**: Use `context.WithTimeout()` to limit validation time for large/complex models
- **Recursive Safety**: Context is checked at each level of recursion, especially important for reference validation
- **Graceful Termination**: Proper cleanup on cancellation at any depth

## Usage

### Validator

```go
validator := datamodel.NewValidator()

// Structure-only validation with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
err := validator.ValidateStructureOnly(ctx, dataModel)

// Full validation with cancellation support
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
err := validator.ValidateWithReferences(ctx, dataModel, allDataModels)
```

### Translator

```go
translator := datamodel.NewTranslator()

// Translate data model to JSON schemas
ctx := context.Background()
result, err := translator.TranslateDataModel(
    ctx,
    "_pump_data",  // contract name
    "v1",          // version
    dataModel,     // UMH data model
    payloadShapes, // custom payload shapes (optional)
    modelRefs,     // model references (optional)
)

// Access generated schemas
for subject, schema := range result.Schemas {
    fmt.Printf("Subject: %s\n", subject)
    schemaJSON, _ := json.Marshal(schema)
    fmt.Printf("Schema: %s\n", schemaJSON)
}

// Check payload shape usage
for shape, paths := range result.PayloadShapeUsage {
    fmt.Printf("Shape %s used by: %v\n", shape, paths)
}
```

## Error Handling

Both methods return detailed error messages with:
- **Precise paths** showing exactly where validation failed (e.g., `"pump.motor.sensor.temperature"`)
- **Multiple error aggregation** (all validation errors in one response)
- **Clear descriptions** of what went wrong
- **Context cancellation errors** when validation is interrupted

## Memory Efficiency

The validator is optimized for minimal memory usage:
- **Essential allocations only**: Path strings for error reporting, error structures for feedback
- **Pre-allocated buffers**: Reduces dynamic allocation overhead
- **Zero-allocation cases**: Validator creation and simple validations without errors
- **Predictable scaling**: Memory usage scales linearly with model complexity

## Production Readiness

### Validator
- ✅ **High performance**: Exceeds targets by 120-7,700x
- ✅ **Context cancellation**: Graceful timeout and cancellation handling
- ✅ **Memory efficient**: Minimal overhead with predictable scaling
- ✅ **Comprehensive testing**: Extensive test coverage including edge cases
- ✅ **Error reporting**: Detailed validation feedback with precise paths
- ✅ **Thread safe**: Stateless validator can be used concurrently

### Translator
- ✅ **High performance**: 13K-400K translations/sec depending on complexity
- ✅ **Context cancellation**: Graceful timeout and cancellation handling
- ✅ **Circular reference protection**: Prevents infinite loops with depth limits
- ✅ **Schema Registry compatibility**: Direct integration with benthos-umh UNS output
- ✅ **Auto-injection**: Automatic default payload shape handling
- ✅ **Comprehensive testing**: 43 test cases including edge cases and circular references
- ✅ **Thread safe**: Stateless translator can be used concurrently
- ✅ **Memory efficient**: Single-pass validation + translation with minimal allocations

See `PERFORMANCE.md` for detailed benchmark results and optimization details. 