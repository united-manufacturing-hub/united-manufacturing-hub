# CSE Storage API Reference

## Current API Surface (Before Registry Elimination)

### Legacy APIs (workerType parameter required)

These APIs require runtime registry lookup:

**SaveObserved(ctx, workerType, id, observed) (changed bool, err error)**
- File: `triangular.go:510`
- Looks up collection name via registry.GetTriangularCollections(workerType)
- Returns true if data changed (delta checking)
- Auto-injects CSE metadata (_sync_id, _version, timestamps)

**LoadObserved(ctx, workerType, id) (interface{}, error)**
- File: `triangular.go:570`
- Returns Document (map[string]interface{})
- Caller must type assert result

**SaveDesired(ctx, workerType, id, desired) error**
- File: `triangular.go:266`
- Similar to SaveObserved but for desired state

**LoadDesired(ctx, workerType, id) (interface{}, error)**
- File: `triangular.go:320`
- Returns Document

### Generic APIs (type parameter, no workerType)

These APIs use reflection for collection name:

**SaveObservedTyped[T](ts, ctx, id, observed T) (bool, error)**
- File: `triangular.go:1119`
- Type parameter T determines workerType via reflection
- Currently still calls legacy SaveObserved internally
- Goal: Make fully independent

**LoadObservedTyped[T](ts, ctx, id) (T, error)**
- File: `triangular.go:1064`
- Returns strongly-typed result
- Currently calls legacy LoadObserved internally

### Call Sites

**Supervisor**
- `/pkg/fsmv2/supervisor/supervisor.go:605` - SaveObserved for snapshot creation
- Auto-registers collections in NewSupervisor() lines 398-462

**Collector**
- `/pkg/fsmv2/supervisor/collection/collector.go:221` - SaveObserved for observed state

## Migration Path

Phase 5 will convert all call sites from legacy APIs to generic APIs.

## CSE Metadata Fields

All roles use these fields (hardcoded in Phase 2):

**Identity:**
- _sync_id (int64)
- _version (int64, always 1)
- _created_at (timestamp)

**Desired:**
- _sync_id (int64)
- _version (int64, increments on update)
- _created_at (timestamp)
- _updated_at (timestamp)

**Observed:**
- _sync_id (int64)
- _version (int64, increments on update)
- _created_at (timestamp)
- _updated_at (timestamp)
