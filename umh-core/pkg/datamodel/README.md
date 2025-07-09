# Data Model Validator

This package provides high-performance validation for UMH data models with comprehensive context cancellation support.

## Performance

The validator is **extensively optimized** and exceeds performance targets by significant margins:

- **Simple schemas**: 9.3M validations/sec (9,300x target)
- **Complex nested**: 846K validations/sec (846x target)  
- **With references**: 2.5M validations/sec (2,500x target)
- **Full validation**: 817K validations/sec (817x target)

Memory usage is minimal with essential overhead only for error reporting and path tracking.

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

**Performance:** 846K-9.3M validations/sec depending on complexity.

### `ValidateWithReferences(ctx, dataModel, allDataModels)`

Validates a data model and all its references recursively.

**Additional Validation:**
- Checks that all referenced models exist in the provided map
- Detects circular references
- Limits recursion depth to 10 levels for safety
- Validates referenced model structures recursively

**Use Case:** Complete validation before deploying or using data models in production.

**Performance:** 817K-2.5M validations/sec depending on reference complexity.

## Context Support

Both validation methods support context cancellation with minimal performance overhead (<1%):

- **Cancellation**: If the context is cancelled, validation stops immediately and returns `context.Canceled`
- **Timeouts**: Use `context.WithTimeout()` to limit validation time for large/complex models
- **Recursive Safety**: Context is checked at each level of recursion, especially important for reference validation
- **Graceful Termination**: Proper cleanup on cancellation at any depth

## Usage

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

- ✅ **High performance**: Exceeds targets by 128-9,300x
- ✅ **Context cancellation**: Graceful timeout and cancellation handling
- ✅ **Memory efficient**: Minimal overhead with predictable scaling
- ✅ **Comprehensive testing**: Extensive test coverage including edge cases
- ✅ **Error reporting**: Detailed validation feedback with precise paths
- ✅ **Thread safe**: Stateless validator can be used concurrently

See `PERFORMANCE.md` for detailed benchmark results and optimization details. 