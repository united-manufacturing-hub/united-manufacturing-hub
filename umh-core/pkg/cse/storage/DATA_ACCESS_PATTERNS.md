# CSE Data Access Patterns - Architecture Investigation

**Investigation Date**: 2025-11-10
**Context**: UMH-CORE-ENG-3806 - Analyzing CSE architecture for data access pattern support

## Executive Summary

**Current State**: CSE schema registry tracks collection metadata but does NOT automatically create tables or expose schema to frontend.

**Gap Analysis**: Current API lacks field-level metadata for data access patterns (Instant/Lazy/Partial/Explicit).

**Recommendation**: YES - Current API can support data access patterns through CollectionMetadata extensions WITHOUT breaking changes.

---

## 1. Current CSE Architecture

### Schema Registry → Frontend Sync: HOW IT WORKS TODAY

#### Current State (What Exists)

**Registry Location**: `/pkg/cse/storage/registry.go`

The Schema Registry is an **in-memory metadata store** that tracks CSE-aware collections:

```go
type CollectionMetadata struct {
    Name          string   // e.g., "container_identity"
    WorkerType    string   // e.g., "container", "relay"
    Role          string   // "identity", "desired", "observed"
    SchemaVersion string   // e.g., "v1", "v2", "v3"
    CSEFields     []string // _sync_id, _version, _created_at, etc.
    IndexedFields []string // Fields with database indexes
    RelatedTo     []string // Related collection names
}
```

**Key Finding**: Registry provides versioning API:
- `RegisterVersion(workerType, role, version)` - Sets schema version for collections
- `GetVersion(collection)` - Returns version for single collection
- `GetAllVersions()` - Returns map[collectionName]version
- `RegisterFeature(feature, supported)` - Capability negotiation
- `GetFeatures()` - Returns supported features map

#### Current State (What's Missing)

**CRITICAL GAP**: No mechanism exists to expose registry metadata to frontend!

**Evidence**:
1. GraphQL schema (`/pkg/communicator/graphql/schema.graphqls`) only exposes:
   - Topic browser queries (UNS data access)
   - NO schema metadata queries
   - NO collection metadata queries
   - NO versioning queries

2. No HTTP endpoints for schema introspection found in:
   - `/pkg/communicator/graphql/resolver.go` - Only topic browser resolvers
   - `/pkg/communicator/graphql/server.go` - Only GraphQL server setup
   - No `/api/schema` or `/api/cse/metadata` endpoints detected

3. Frontend must currently rely on:
   - Hardcoded schema knowledge
   - Version strings in API responses (not introspectable)
   - No runtime schema discovery

**Implication**: Frontend CANNOT dynamically learn about schema changes or new collections.

---

### Table Creation: HOW IT WORKS TODAY

#### Current State (Manual Collection Registration)

**Evidence from `/pkg/persistence/memory/memory.go`**:

```go
// CreateCollection creates a new empty collection with the given name.
func (s *InMemoryStore) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
    // Schema parameter is IGNORED by in-memory store
    // Simply creates empty map for collection
    s.collections[name] = make(map[string]persistence.Document)
}
```

**Key Findings**:

1. **Manual Registration Required**:
   - Collections created explicitly via `store.CreateCollection(ctx, name, schema)`
   - Schema parameter exists but IGNORED by current memory implementation
   - No auto-generation from CollectionMetadata

2. **No SQLite Implementation Found**:
   - Search for SQLite table creation yielded no results
   - Only in-memory implementation exists (`/pkg/persistence/memory/`)
   - No SQL "CREATE TABLE" statements in codebase (outside vendor/)

3. **TriangularStore Creates Collections Manually**:
   - `SaveIdentity()`, `SaveDesired()`, `SaveObserved()` methods
   - Lookup collection metadata: `registry.GetTriangularCollections(workerType)`
   - Insert documents directly: `store.Insert(ctx, collectionName, doc)`
   - **ASSUMES collections already exist** (created at startup)

#### Schema-Driven Table Creation (Future Path)

**Design Decision Location**: `/pkg/persistence/store.go:177-197`

```go
// CreateCollection creates a new collection with an optional schema.
//
// DESIGN DECISION: Schema parameter is optional (can be nil)
// WHY: Support both schema-less backends (MongoDB) and schema-required backends (SQL).
// For SQL databases, schema defines table structure (columns, types, constraints).
```

**Implications**:
- Schema is OPTIONAL by design (nil-safe)
- Backend-specific implementations determine table structure
- For SQLite: Could use `persistence.Schema` to generate CREATE TABLE
- For MongoDB: Schema might be validation rules or ignored entirely

**Gap**: No code currently uses Schema parameter for table generation!

---

## 2. Data Access Pattern Support Analysis

### User-Described Patterns

| Pattern | Description | Sync Behavior | Access Method |
|---------|-------------|---------------|---------------|
| **Instant** | Main table columns | Always synced, always cached | Direct field access |
| **Lazy** | 1:1 child tables | Loaded on `.include()`, then cached | `document.include("relatedField")` |
| **Partial** | 1:many child tables | Paginated with cursor/window | `document.comments.page(cursor, limit)` |
| **Explicit** | `*_explicit` suffix tables | Manual request only, never auto-synced | `document.fetchExplicit("heavyData")` |

### Current API Analysis

#### CollectionMetadata - CAN IT SUPPORT PATTERNS?

**Current Fields**:
```go
type CollectionMetadata struct {
    Name          string   // ✅ Identifies collection
    WorkerType    string   // ✅ Groups related collections
    Role          string   // ✅ Triangular model role
    SchemaVersion string   // ✅ Versioning
    CSEFields     []string // ✅ System metadata fields
    IndexedFields []string // ✅ Query performance hints
    RelatedTo     []string // ✅ Collection relationships
}
```

**Missing Fields for Data Access Patterns**:
- ❌ Field-level access patterns (which fields are lazy/partial/explicit)
- ❌ Relationship types (1:1, 1:many)
- ❌ Loading strategies per field
- ❌ Sync policies per field (always/on-demand/never)

**Verdict**: CollectionMetadata tracks **collections**, not **fields** → Cannot support field-level patterns!

---

### Reflect.Type Metadata - CAN IT SUPPORT PATTERNS?

**Current Use**: NOT USED in CSE storage layer

**Evidence**:
- `type_registry.go` file exists but contains placeholder/experimental code
- `TYPED_REGISTRY_TDD_PLAN.md` shows Type Registry is planned, not implemented
- No `reflect.Type` metadata tracked in current registry

**Verdict**: Type metadata infrastructure exists conceptually but NOT implemented!

---

### Gap Analysis: What's Missing?

#### 1. Field-Level Metadata

**Problem**: CollectionMetadata tracks collections, not individual fields.

**Need**: Ability to specify per-field access patterns:
```go
// HYPOTHETICAL - Not implemented!
type FieldMetadata struct {
    Name         string
    Type         string        // "string", "int", "relation"
    AccessPattern string       // "instant", "lazy", "partial", "explicit"
    Relationship  *Relationship // For 1:1 or 1:many relations
    SyncPolicy    string       // "always", "on_demand", "never"
}

type CollectionMetadata struct {
    // ... existing fields ...
    Fields []FieldMetadata // NEW - Field-level metadata
}
```

#### 2. Relationship Metadata

**Problem**: `RelatedTo []string` tracks collection names, not relationship types.

**Need**: Distinguish 1:1 (lazy) from 1:many (partial) relations:
```go
// HYPOTHETICAL
type Relationship struct {
    Collection string // Target collection
    Type       string // "one_to_one", "one_to_many"
    ForeignKey string // Join field
}
```

#### 3. Sync Policy Configuration

**Problem**: No way to specify "never sync" for explicit fields or "sync on first access" for lazy fields.

**Need**: Per-field sync policies:
```go
// HYPOTHETICAL
type SyncPolicy struct {
    Strategy string // "instant", "lazy", "explicit"
    CacheTTL int    // Cache expiration (seconds)
}
```

---

## 3. API Extensions Needed (Stubs WITHOUT Implementation)

### Phase 1: Extend CollectionMetadata (Backward Compatible)

**File**: `/pkg/cse/storage/registry.go`

**Add NEW fields to CollectionMetadata**:
```go
type CollectionMetadata struct {
    Name          string
    WorkerType    string
    Role          string
    SchemaVersion string
    CSEFields     []string
    IndexedFields []string
    RelatedTo     []string

    // NEW - Data Access Pattern Support (stubs)
    Fields        []FieldMetadata    // Field-level metadata
    GoType        reflect.Type       // Type info for codegen
    TypeVersion   string             // Type schema version (semantic versioning)
}

// NEW - Field-level metadata
type FieldMetadata struct {
    Name          string        // Field name (Go struct field or JSON key)
    JSONName      string        // JSON serialization name
    GoType        string        // Go type (string, int64, *CustomType)
    AccessPattern AccessPattern // How this field is loaded
    Relationship  *Relationship // For relational fields (1:1, 1:many)
    SyncPolicy    SyncPolicy    // When/how to sync this field
    Tags          map[string]string // Struct tags (json, db, cse)
}

// NEW - Access pattern enum
type AccessPattern string
const (
    AccessInstant  AccessPattern = "instant"  // Main table, always synced
    AccessLazy     AccessPattern = "lazy"     // 1:1 child table, load on .include()
    AccessPartial  AccessPattern = "partial"  // 1:many child table, paginated
    AccessExplicit AccessPattern = "explicit" // Manual request only, never auto-sync
)

// NEW - Relationship metadata
type Relationship struct {
    TargetCollection string        // Related collection name
    Type             RelationType  // 1:1 or 1:many
    ForeignKey       string        // Join field
    Inverse          string        // Inverse relation name (for bidirectional)
}

type RelationType string
const (
    RelationOneToOne  RelationType = "one_to_one"
    RelationOneToMany RelationType = "one_to_many"
)

// NEW - Sync policy
type SyncPolicy struct {
    Strategy string // "instant", "lazy", "explicit"
    CacheTTL int    // Cache expiration (0 = forever)
}
```

**Why This Works**:
- ✅ Backward compatible (existing code ignores new fields)
- ✅ No breaking changes to existing methods
- ✅ Enables gradual migration (populate fields over time)
- ✅ Stubs provide extension points for future implementation

---

### Phase 2: Add Type Registry Integration (Future)

**File**: `/pkg/cse/storage/type_registry.go` (already exists, needs implementation)

**Add methods to register Go types**:
```go
// RegisterType associates a Go type with a collection
// This enables reflect-based codegen for TypeScript types
func (r *Registry) RegisterType(collection string, typ reflect.Type) error {
    // Stub implementation
    meta, err := r.Get(collection)
    if err != nil {
        return err
    }
    meta.GoType = typ
    return nil
}

// GetTypeMetadata returns type information for codegen
func (r *Registry) GetTypeMetadata(collection string) (reflect.Type, error) {
    meta, err := r.Get(collection)
    if err != nil {
        return nil, err
    }
    if meta.GoType == nil {
        return nil, fmt.Errorf("no type registered for collection %q", collection)
    }
    return meta.GoType, nil
}
```

**Why This Matters**:
- Frontend codegen needs Go type info (struct fields, tags, types)
- `reflect.Type` preserves struct field metadata (json tags, validation tags)
- Enables automatic TypeScript type generation from Go structs

---

### Phase 3: GraphQL Schema Extension (Frontend Access)

**File**: `/pkg/communicator/graphql/schema.graphqls`

**Add schema introspection queries**:
```graphql
# NEW - CSE Schema Metadata Queries
type Query {
    # ... existing topic queries ...

    # Schema introspection
    collections: [CollectionSchema!]!
    collection(name: String!): CollectionSchema

    # Versioning
    schemaVersions: [SchemaVersion!]!
    features: [Feature!]!
}

type CollectionSchema {
    name: String!
    workerType: String!
    role: String!
    version: String!
    fields: [FieldSchema!]!
    relationships: [Relationship!]!
}

type FieldSchema {
    name: String!
    jsonName: String!
    type: String!
    accessPattern: AccessPattern!
    syncPolicy: String!
    tags: [Tag!]!
}

enum AccessPattern {
    INSTANT
    LAZY
    PARTIAL
    EXPLICIT
}

type Relationship {
    field: String!
    targetCollection: String!
    type: RelationType!
    foreignKey: String!
}

enum RelationType {
    ONE_TO_ONE
    ONE_TO_MANY
}

type SchemaVersion {
    collection: String!
    version: String!
}

type Feature {
    name: String!
    supported: Boolean!
}

type Tag {
    key: String!
    value: String!
}
```

**Implementation Stub** (`/pkg/communicator/graphql/resolver.go`):
```go
func (r *queryResolver) Collections(ctx context.Context) ([]*CollectionSchema, error) {
    // Stub: Return empty list for now
    // TODO: Fetch from registry.List() and convert to GraphQL schema
    return []*CollectionSchema{}, nil
}

func (r *queryResolver) SchemaVersions(ctx context.Context) ([]*SchemaVersion, error) {
    // Stub: Return empty list for now
    // TODO: Fetch from registry.GetAllVersions()
    return []*SchemaVersion{}, nil
}

func (r *queryResolver) Features(ctx context.Context) ([]*Feature, error) {
    // Stub: Return empty list for now
    // TODO: Fetch from registry.GetFeatures()
    return []*Feature{}, nil
}
```

---

## 4. Migration Path (No Breaking Changes)

### Step 1: Add Metadata Stubs (Immediate)

**Changes**:
1. Add new fields to `CollectionMetadata` struct
2. Add new types: `FieldMetadata`, `AccessPattern`, `Relationship`, `SyncPolicy`
3. All new fields have zero values → existing code unaffected
4. No existing methods modified

**Timeline**: 1 day (struct definitions only)

**Risk**: ZERO (additive changes only)

---

### Step 2: Extend GraphQL Schema (Immediate)

**Changes**:
1. Add schema introspection queries to GraphQL
2. Implement stub resolvers returning empty arrays
3. Frontend can query but gets no data yet

**Timeline**: 1 day (schema + stubs)

**Risk**: LOW (stubs don't break existing queries)

---

### Step 3: Populate Metadata (Gradual)

**Changes**:
1. Update `Register()` calls to populate new fields
2. Start with 1-2 collections (e.g., container_identity)
3. Add field metadata manually for critical fields
4. Expand coverage over time

**Timeline**: 1 week per collection (manual annotation)

**Risk**: MEDIUM (requires careful field-by-field analysis)

---

### Step 4: Implement Type Registry (Future)

**Changes**:
1. Complete `/pkg/cse/storage/type_registry.go` implementation
2. Add `RegisterType()` for automatic field extraction via reflection
3. Auto-populate `Fields []FieldMetadata` from Go struct tags

**Timeline**: 2-3 weeks (reflection + validation logic)

**Risk**: MEDIUM (reflection can be complex, need thorough testing)

---

### Step 5: Implement Access Pattern Logic (Future)

**Changes**:
1. Add lazy loading logic to TriangularStore
2. Implement `.include()` method for lazy fields
3. Add pagination for partial fields (`.page(cursor, limit)`)
4. Add explicit fetch for explicit fields

**Timeline**: 4-6 weeks (query logic + caching)

**Risk**: HIGH (complex query patterns, cache invalidation)

---

## 5. Specific API Extension Locations

### Files to Modify (Phase 1 - Stubs Only)

#### `/pkg/cse/storage/registry.go`
**Lines 165-208**: Add new fields to `CollectionMetadata`
```go
// After line 208 (after RelatedTo []string)

// Fields describes individual field metadata for data access patterns.
// Empty slice means no field-level metadata registered yet.
Fields []FieldMetadata

// GoType stores reflect.Type for codegen integration.
// Nil means no type information registered yet.
GoType reflect.Type

// TypeVersion tracks schema version for this type (semantic versioning).
TypeVersion string
```

**After line 208**: Add new types
```go
// FieldMetadata describes a single field's access pattern and sync policy.
type FieldMetadata struct {
    Name          string
    JSONName      string
    GoType        string
    AccessPattern AccessPattern
    Relationship  *Relationship
    SyncPolicy    SyncPolicy
    Tags          map[string]string
}

// AccessPattern defines how a field is loaded and cached.
type AccessPattern string
const (
    AccessInstant  AccessPattern = "instant"
    AccessLazy     AccessPattern = "lazy"
    AccessPartial  AccessPattern = "partial"
    AccessExplicit AccessPattern = "explicit"
)

// Relationship describes a field's relation to another collection.
type Relationship struct {
    TargetCollection string
    Type             RelationType
    ForeignKey       string
    Inverse          string
}

type RelationType string
const (
    RelationOneToOne  RelationType = "one_to_one"
    RelationOneToMany RelationType = "one_to_many"
)

// SyncPolicy controls when and how a field is synchronized.
type SyncPolicy struct {
    Strategy string // "instant", "lazy", "explicit"
    CacheTTL int    // 0 = cache forever
}
```

#### `/pkg/communicator/graphql/schema.graphqls`
**After line 61**: Add schema introspection queries
```graphql
# Schema introspection (stubs for future implementation)
collections: [CollectionSchema!]!
collection(name: String!): CollectionSchema
schemaVersions: [SchemaVersion!]!
features: [Feature!]!
```

**After line 61**: Add new types
```graphql
type CollectionSchema {
    name: String!
    workerType: String!
    role: String!
    version: String!
}

type SchemaVersion {
    collection: String!
    version: String!
}

type Feature {
    name: String!
    supported: Boolean!
}
```

#### `/pkg/communicator/graphql/resolver.go`
**After line 408**: Add stub resolvers
```go
func (r *queryResolver) Collections(ctx context.Context) ([]*CollectionSchema, error) {
    // Stub: Return empty for now
    return []*CollectionSchema{}, nil
}

func (r *queryResolver) SchemaVersions(ctx context.Context) ([]*SchemaVersion, error) {
    // Stub: Return empty for now
    return []*SchemaVersion{}, nil
}

func (r *queryResolver) Features(ctx context.Context) ([]*Feature, error) {
    // Stub: Return empty for now
    return []*Feature{}, nil
}
```

---

## 6. Answers to Investigation Questions

### Q1: Current CSE Sync Mechanism (How Frontend Gets Schema)

**Answer**: Frontend DOES NOT get schema metadata today!

**Evidence**:
- GraphQL schema only exposes topic browser (UNS data), not CSE metadata
- No HTTP endpoints for schema introspection detected
- Registry has versioning API (`GetAllVersions()`) but no GraphQL binding
- Frontend must hardcode schema knowledge

**Gap**: Need to add GraphQL queries for schema introspection.

---

### Q2: Table Creation Mechanism (Manual vs Auto-Generated)

**Answer**: **MANUAL** - Collections created explicitly via `CreateCollection()`

**Evidence**:
- `InMemoryStore.CreateCollection()` creates empty map for collection
- Schema parameter exists but IGNORED (not used for table structure)
- TriangularStore assumes collections exist (created at startup)
- No SQLite implementation found (only in-memory store exists)

**Gap**: No code currently generates tables from CollectionMetadata.

---

### Q3: Can Current API Support Data Access Patterns?

**Answer**: **YES** - With extensions, NO breaking changes needed!

**Reasoning**:
1. ✅ CollectionMetadata is extensible (add new fields)
2. ✅ Backward compatible (zero values for new fields)
3. ✅ Registry API supports features (`RegisterFeature()`, `GetFeatures()`)
4. ✅ TriangularStore can be extended with `.include()`, `.page()`, `.fetchExplicit()` methods

**Missing**:
- Field-level metadata (which fields are lazy/partial/explicit)
- Relationship types (1:1 vs 1:many)
- Sync policies per field

**Solution**: Add `Fields []FieldMetadata` to CollectionMetadata (Phase 1 stubs).

---

### Q4: Specific API Extensions Needed (Fields to Add, Stubs to Create)

**Summary of Changes**:

| File | Change Type | Lines | Description |
|------|-------------|-------|-------------|
| `registry.go` | Add fields | After 208 | Add `Fields`, `GoType`, `TypeVersion` to CollectionMetadata |
| `registry.go` | Add types | After 208 | Define `FieldMetadata`, `AccessPattern`, `Relationship`, `SyncPolicy` |
| `schema.graphqls` | Add queries | After 61 | Add `collections`, `schemaVersions`, `features` queries |
| `schema.graphqls` | Add types | After 61 | Define `CollectionSchema`, `SchemaVersion`, `Feature` |
| `resolver.go` | Add stubs | After 408 | Implement stub resolvers returning empty arrays |

**All changes are ADDITIVE** - no existing code modified!

---

## 7. Conclusion

### Current State Summary

1. **Schema Registry Exists**: Tracks collection metadata (name, version, role)
2. **No Frontend Exposure**: GraphQL does not expose schema metadata
3. **Manual Table Creation**: Collections created explicitly, schema parameter ignored
4. **No Field-Level Metadata**: Cannot distinguish instant/lazy/partial/explicit fields

### Gap Analysis Summary

1. **Missing Field-Level Metadata**: Need `Fields []FieldMetadata` on CollectionMetadata
2. **Missing Relationship Types**: Need `Relationship` struct with 1:1 vs 1:many distinction
3. **Missing Sync Policies**: Need `SyncPolicy` per field
4. **Missing Frontend API**: Need GraphQL schema introspection queries

### Extensibility Assessment

**Verdict**: ✅ **YES - Current API CAN support data access patterns**

**Justification**:
- CollectionMetadata is extensible (add new fields without breaking changes)
- Registry provides versioning and feature negotiation
- GraphQL schema can be extended with introspection queries
- TriangularStore methods can be enhanced with lazy/partial/explicit loading

**Next Steps** (in priority order):
1. Add metadata stubs (1 day, zero risk)
2. Extend GraphQL schema (1 day, low risk)
3. Populate field metadata manually (1 week per collection, medium risk)
4. Implement type registry (2-3 weeks, medium risk)
5. Implement access pattern logic (4-6 weeks, high risk)

---

## Appendix: Code Evidence

### A1. Registry Already Tracks Versions

**File**: `/pkg/cse/storage/registry.go:463-526`

```go
func (r *Registry) RegisterVersion(workerType, role, version string) error
func (r *Registry) GetVersion(collection string) string
func (r *Registry) GetAllVersions() map[string]string
func (r *Registry) RegisterFeature(feature string, supported bool) error
func (r *Registry) HasFeature(feature string) bool
func (r *Registry) GetFeatures() map[string]bool
```

**Implication**: Infrastructure for versioning exists, just not exposed to frontend.

### A2. Memory Store Ignores Schema Parameter

**File**: `/pkg/persistence/memory/memory.go:142-151`

```go
func (s *InMemoryStore) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
    // Schema parameter exists but is IGNORED
    s.collections[name] = make(map[string]persistence.Document)
}
```

**Implication**: Schema parameter is a placeholder for future SQLite implementation.

### A3. TriangularStore Uses Registry for Collection Lookup

**File**: `/pkg/cse/storage/triangular.go:171-180`

```go
func (ts *TriangularStore) SaveIdentity(...) error {
    // Look up collection metadata
    identityMeta, _, _, err := ts.registry.GetTriangularCollections(workerType)
    // Insert into collection
    _, err = ts.store.Insert(ctx, identityMeta.Name, identity)
}
```

**Implication**: TriangularStore already uses registry, can be extended to use field metadata.

---

**Plan Location**: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/DATA_ACCESS_PATTERNS.md`
**Status**: Investigation complete, ready for implementation planning
