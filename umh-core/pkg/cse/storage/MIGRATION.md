# Storage API Migration Guide

## Overview

Legacy storage APIs (SaveObserved, LoadObserved, SaveDesired, LoadDesired) are deprecated and will be removed.

**Why migrate:**
- Type safety (compile-time checks instead of runtime assertions)
- No registry dependency (simpler, faster)
- Cleaner API (type parameter conveys intent)

**Timeline:**
- Phase 4 (current): Legacy APIs deprecated
- Phase 5: All internal code migrated
- Phase 6: Legacy APIs removed

## Migration Examples

### Example 1: SaveObserved with Document

**Before (deprecated):**
```go
observed := persistence.Document{
    "id":     workerID,
    "status": "running",
    "cpu":    45.2,
}
changed, err := ts.SaveObserved(ctx, "container", workerID, observed)
```

**After (recommended):**
```go
// Define typed struct (if not already defined)
type ContainerObservedState struct {
    ID     string  `json:"id"`
    Status string  `json:"status"`
    CPU    float64 `json:"cpu"`
}

// Use typed API
observed := ContainerObservedState{
    ID:     workerID,
    Status: "running",
    CPU:    45.2,
}
changed, err := storage.SaveObservedTyped[ContainerObservedState](ts, ctx, workerID, observed)
```

### Example 2: LoadObserved with Type Assertion

**Before (deprecated):**
```go
doc, err := ts.LoadObserved(ctx, "relay", workerID)
if err != nil {
    return err
}

// Manual type assertion (error-prone!)
statusRaw, ok := doc["status"]
if !ok {
    return errors.New("status field missing")
}
status, ok := statusRaw.(string)
if !ok {
    return errors.New("status is not string")
}
```

**After (recommended):**
```go
type RelayObservedState struct {
    ID     string `json:"id"`
    Status string `json:"status"`
}

observed, err := storage.LoadObservedTyped[RelayObservedState](ts, ctx, workerID)
if err != nil {
    return err
}

// Type-safe field access!
status := observed.Status
```

### Example 3: SaveDesired

**Before (deprecated):**
```go
desired := persistence.Document{
    "id":     workerID,
    "config": "production",
}
err := ts.SaveDesired(ctx, "communicator", workerID, desired)
```

**After (recommended):**
```go
type CommunicatorDesiredState struct {
    ID     string `json:"id"`
    Config string `json:"config"`
}

desired := CommunicatorDesiredState{
    ID:     workerID,
    Config: "production",
}
err := storage.SaveDesiredTyped[CommunicatorDesiredState](ts, ctx, workerID, desired)
```

### Example 4: LoadDesired

**Before (deprecated):**
```go
doc, err := ts.LoadDesired(ctx, "parent", workerID)
if err != nil {
    return err
}

// Manual extraction
configRaw, _ := doc["config"]
config, _ := configRaw.(string)
```

**After (recommended):**
```go
type ParentDesiredState struct {
    ID     string `json:"id"`
    Config string `json:"config"`
}

desired, err := storage.LoadDesiredTyped[ParentDesiredState](ts, ctx, workerID)
if err != nil {
    return err
}

config := desired.Config // Type-safe!
```

## Common Pitfalls

### Pitfall 1: Struct Naming Convention

**Type parameter determines workerType via reflection.**

```go
// WRONG: Struct name doesn't match convention
type MyCustomState struct { ... }
storage.SaveObservedTyped[MyCustomState](...)
// Derives workerType = "mycustom" (wrong!)

// CORRECT: Follow convention
type ContainerObservedState struct { ... }
storage.SaveObservedTyped[ContainerObservedState](...)
// Derives workerType = "container" (correct!)
```

**Naming convention:**
- `{WorkerType}ObservedState` for observed
- `{WorkerType}DesiredState` for desired
- `{WorkerType}IdentityState` for identity

### Pitfall 2: Forgetting ID Field

Generic APIs expect `id` field in struct:

```go
// WRONG: Missing ID field
type ObservedState struct {
    Status string `json:"status"`
}

// CORRECT: Include ID field
type ObservedState struct {
    ID     string `json:"id"`
    Status string `json:"status"`
}
```

### Pitfall 3: Mixed Legacy and Generic APIs

Don't mix old and new APIs for same worker:

```go
// WRONG: Mixing APIs
storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, obs)
doc, _ := ts.LoadObserved(ctx, "container", id) // Legacy API!

// CORRECT: Use generics consistently
storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, obs)
obs, _ := storage.LoadObservedTyped[ContainerObservedState](ts, ctx, id)
```

## Struct Definition Best Practices

### Reuse Existing Structs

Many worker types already have typed structs defined:

```go
// Check if already defined in worker package:
import "github.com/.../fsmv2/workers/container"

// Use existing struct
observed, err := storage.LoadObservedTyped[container.ObservedState](ts, ctx, id)
```

### Define New Structs

If no struct exists, define in worker package:

```go
// File: pkg/fsmv2/workers/myworker/models.go

package myworker

type ObservedState struct {
    ID              string    `json:"id"`
    Status          string    `json:"status"`
    LastHeartbeat   time.Time `json:"lastHeartbeat"`
}

type DesiredState struct {
    ID     string `json:"id"`
    Config string `json:"config"`
}
```

### JSON Tags Required

Always add JSON tags matching Document field names:

```go
// WRONG: No JSON tags
type ObservedState struct {
    ID     string
    Status string
}

// CORRECT: JSON tags match Document keys
type ObservedState struct {
    ID     string `json:"id"`
    Status string `json:"status"`
}
```

## Testing Migration

After migrating, verify:

```bash
# Compile check
go build ./pkg/fsmv2/workers/...

# Run worker tests
go test -v ./pkg/fsmv2/workers/myworker/...

# Run integration tests
go test -v ./pkg/fsmv2/integration/... -run TestMyWorker
```

## Questions?

- Check examples in Phase 5 of this implementation plan
- Review existing generic API usage in tests: `pkg/cse/storage/triangular_test.go`
- See worker struct definitions: `pkg/fsmv2/workers/*/models.go`
