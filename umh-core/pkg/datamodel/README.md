# Data Model Validator

This package provides validation for UMH data models with two main APIs that support context cancellation.

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

### `ValidateWithReferences(ctx, dataModel, allDataModels)`

Validates a data model and all its references recursively.

**Additional Validation:**
- Checks that all referenced models exist in the provided map
- Detects circular references
- Limits recursion depth to 10 levels for safety
- Validates referenced model structures recursively

**Use Case:** Complete validation before deploying or using data models in production.

## Context Support

Both validation methods support context cancellation:

- **Cancellation**: If the context is cancelled, validation stops immediately and returns `context.Canceled`
- **Timeouts**: Use `context.WithTimeout()` to limit validation time for large/complex models
- **Recursive Safety**: Context is checked at each level of recursion, especially important for reference validation

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
- Path information showing exactly where validation failed
- Multiple error aggregation (all validation errors in one response)
- Clear descriptions of what went wrong
- Context cancellation errors when validation is interrupted 