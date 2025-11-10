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

// Package storage provides CSE (Control Sync Engine) conventions for FSM v2 persistence.
//
// CSE is a three-tier sync system inspired by Linear's sync engine:
//   - Frontend ↔ Relay ↔ Edge
//   - Each tier maintains sync state (_sync_id, _version)
//   - Triangular model: Identity (immutable) + Desired (user intent) + Observed (system reality)
//   - Optimistic UI with pessimistic storage
//
// The Schema Registry tracks metadata about collections that follow CSE conventions,
// including which fields are CSE metadata, which are indexed for sync queries, and
// how collections relate to each other in the triangular model.
//
// # Global Registry
//
// A package-level global registry is provided for convenience:
//
//	// Register collections at application startup
//	storage.Register(&storage.CollectionMetadata{
//	    Name:       "container_identity",
//	    WorkerType: "container",
//	    Role:       storage.RoleIdentity,
//	})
//
//	// Later: Look up metadata
//	metadata, err := storage.Get("container_identity")
//
// For advanced use cases (testing, multiple registries), create separate Registry instances:
//
//	registry := storage.NewRegistry()
//	registry.Register(metadata)
package storage

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
)

var (
	// globalRegistry is the package-level default registry.
	//
	// DESIGN DECISION: Global registry for convenience
	// WHY: Most applications only need one registry, simplifies common case
	// TRADE-OFF: Global state, but acceptable (registry is read-only after startup)
	// INSPIRED BY: http.DefaultServeMux pattern in Go standard library.
	globalRegistry = NewRegistry()
)

// Register adds a collection's metadata to the global registry.
// This is a convenience wrapper around globalRegistry.Register().
//
// For most applications, use this function instead of creating a separate Registry.
func Register(metadata *CollectionMetadata) error {
	return globalRegistry.Register(metadata)
}

// Get retrieves a collection's metadata from the global registry.
// This is a convenience wrapper around globalRegistry.Get().
func Get(collectionName string) (*CollectionMetadata, error) {
	return globalRegistry.Get(collectionName)
}

// IsRegistered checks if a collection is registered in the global registry.
// This is a convenience wrapper around globalRegistry.IsRegistered().
func IsRegistered(collectionName string) bool {
	return globalRegistry.IsRegistered(collectionName)
}

// List returns all registered collection metadata from the global registry.
// This is a convenience wrapper around globalRegistry.List().
func List() []*CollectionMetadata {
	return globalRegistry.List()
}

// GetTriangularCollections retrieves the identity, desired, and observed collections
// for a given worker type from the global registry.
// This is a convenience wrapper around globalRegistry.GetTriangularCollections().
func GetTriangularCollections(workerType string) (identity, desired, observed *CollectionMetadata, err error) {
	return globalRegistry.GetTriangularCollections(workerType)
}

// CSE metadata field constants.
//
// These fields MUST be present in every CSE-aware collection for proper sync functionality.
// The sync engine uses these fields to track versioning, ordering, and audit trail.
//
// DESIGN DECISION: Standardized field names with underscore prefix
// WHY: Prevent naming collisions with user data, signal "system field"
// TRADE-OFF: No flexibility in field naming, but consistency is critical for sync engine
// INSPIRED BY: Linear's sync engine metadata field conventions (_sync_id, _version).
const (
	// FieldSyncID is the global sync version, incremented on every change across all collections.
	// Used for efficient "give me all changes since sync_id X" queries.
	// Index this field for sync queries: WHERE _sync_id > last_synced.
	FieldSyncID = "_sync_id"

	// FieldVersion is the document version used for optimistic locking.
	// Prevents lost updates: UPDATE ... WHERE _version = expected_version.
	FieldVersion = "_version"

	// FieldCreatedAt is the creation timestamp.
	// Immutable after initial insert.
	FieldCreatedAt = "_created_at"

	// FieldUpdatedAt is the last update timestamp.
	// Updated on every modification.
	FieldUpdatedAt = "_updated_at"

	// FieldDeletedAt is the soft delete timestamp (NULL if not deleted).
	// Enables soft deletes without losing data.
	FieldDeletedAt = "_deleted_at"

	// FieldDeletedBy is the user who deleted the record (for audit trail).
	// NULL if not deleted.
	FieldDeletedBy = "_deleted_by"
)

// Triangular model role constants.
//
// DESIGN DECISION: Three separate collections per worker type
// WHY: Separates immutable identity, user intent, and system reality
// TRADE-OFF: More collections to manage, but clearer separation of concerns
// INSPIRED BY: FSM v2 triangular model architecture.
const (
	// RoleIdentity represents immutable worker identity (IP, hostname, bootstrap config).
	// Never changes after creation, establishes "what is this worker?".
	RoleIdentity = "identity"

	// RoleDesired represents user intent and configuration.
	// What the user WANTS the worker to do.
	RoleDesired = "desired"

	// RoleObserved represents system reality and current state.
	// What the worker IS ACTUALLY doing.
	RoleObserved = "observed"
)

// STUB types for future data access patterns
// These types are defined now but will be fully implemented in later phases.
type AccessPattern string

const (
	AccessInstant  AccessPattern = "instant"
	AccessLazy     AccessPattern = "lazy"
	AccessPartial  AccessPattern = "partial"
	AccessExplicit AccessPattern = "explicit"
)

type RelationType string

const (
	RelationOneToOne  RelationType = "one_to_one"
	RelationOneToMany RelationType = "one_to_many"
)

type Relationship struct {
	TargetCollection string
	Type             RelationType
	ForeignKey       string
	Inverse          string
}

type SyncPolicy struct {
	Strategy string
	CacheTTL int
}

type FieldMetadata struct {
	Name          string
	JSONName      string
	GoType        string
	AccessPattern AccessPattern
	Relationship  *Relationship
	SyncPolicy    SyncPolicy
	Tags          map[string]string
}

// CollectionMetadata describes a CSE-aware collection's schema and sync configuration.
//
// Each collection in the triangular model (identity, desired, observed) has its own
// metadata describing which fields are CSE-managed, which are indexed, and how it
// relates to other collections.
//
// Example:
//
//	metadata := &CollectionMetadata{
//	    Name:          "container_identity",
//	    WorkerType:    "container",
//	    Role:          RoleIdentity,
//	    CSEFields:     []string{FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt},
//	    IndexedFields: []string{FieldSyncID},
//	    RelatedTo:     []string{"container_desired", "container_observed"},
//	}
type CollectionMetadata struct {
	// Name is the collection name (e.g., "container_identity").
	// CONVENTION: {workerType}_{role} naming pattern
	Name string

	// WorkerType is the FSM worker type (e.g., "container", "relay").
	// Used to group related collections in the triangular model.
	WorkerType string

	// Role is the triangular model role: "identity", "desired", or "observed".
	Role string

	// SchemaVersion is the schema version for frontend codegen integration.
	// Example: "v1", "v2", "v3"
	SchemaVersion string

	// CSEFields are fields managed by CSE (_sync_id, _version, etc.).
	// These fields are automatically maintained by the sync engine.
	CSEFields []string

	// IndexedFields are fields with database indexes for sync queries.
	// Typically includes _sync_id for "WHERE _sync_id > last_synced" queries.
	IndexedFields []string

	// RelatedTo is a list of related collection names.
	// Example: container_identity relates to container_desired, container_observed.
	RelatedTo []string

	// Type Registry (implemented)
	ObservedType reflect.Type
	DesiredType  reflect.Type

	// Data Access Patterns (stubs)
	Fields      []FieldMetadata
	TypeVersion string
}

// Registry tracks CSE-aware collections and their metadata.
//
// The registry is an in-memory metadata store that tracks which collections
// follow CSE conventions, what metadata fields they have, and how they relate
// to each other. It's populated at startup and provides fast lookups during
// sync operations.
//
// DESIGN DECISION: In-memory registry, not database-backed
// WHY: Schema is static (defined at startup), doesn't change at runtime
// TRADE-OFF: Must re-register on restart, but acceptable (fast startup)
// INSPIRED BY: HTTP router registration patterns (gin, echo, chi)
//
// DESIGN DECISION: Thread-safe with RWMutex
// WHY: Registry accessed from multiple FSM workers simultaneously
// TRADE-OFF: Lock overhead, but reads are fast (RLock) and writes are rare (startup only)
// INSPIRED BY: sync.Map pattern, but simpler with map + mutex
//
// Example:
//
//	registry := NewRegistry()
//
//	// Register collections at startup
//	registry.Register(&CollectionMetadata{
//	    Name:          "container_identity",
//	    WorkerType:    "container",
//	    Role:          RoleIdentity,
//	    CSEFields:     []string{FieldSyncID, FieldVersion},
//	    IndexedFields: []string{FieldSyncID},
//	})
//
//	// Later: Look up metadata before creating collection
//	metadata, err := registry.Get("container_identity")
//	if err != nil {
//	    return err
//	}
//	// Use metadata.CSEFields to add CSE columns to SQL table
type Registry struct {
	// collections maps collection name to metadata
	collections map[string]*CollectionMetadata

	// features maps feature name to support status
	features map[string]bool

	// mu protects concurrent access to collections and features maps
	mu sync.RWMutex
}

// NewRegistry creates a new empty Schema Registry.
//
// The registry is safe for concurrent use by multiple goroutines.
// Register collections at startup before concurrent access begins.
func NewRegistry() *Registry {
	return &Registry{
		collections: make(map[string]*CollectionMetadata),
		features:    make(map[string]bool),
	}
}

// Register adds a collection's metadata to the registry.
//
// Returns an error if:
//   - The collection is already registered (prevents duplicate registration)
//   - Metadata validation fails (empty name, invalid role)
//
// This method is typically called during application startup to register all
// CSE-aware collections before FSM workers start accessing them.
//
// Example:
//
//	err := registry.Register(&CollectionMetadata{
//	    Name:          "container_identity",
//	    WorkerType:    "container",
//	    Role:          RoleIdentity,
//	    CSEFields:     []string{FieldSyncID, FieldVersion},
//	    IndexedFields: []string{FieldSyncID},
//	})
//	if err != nil {
//	    log.Fatalf("Failed to register collection: %v", err)
//	}
func (r *Registry) Register(metadata *CollectionMetadata) error {
	// Validate metadata before acquiring lock
	if err := validateMetadata(metadata); err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.collections[metadata.Name]; exists {
		return fmt.Errorf("collection %q already registered", metadata.Name)
	}

	r.collections[metadata.Name] = metadata

	return nil
}

// validateMetadata checks that collection metadata is valid.
//
// DESIGN DECISION: Fail fast with validation at registration time
// WHY: Catch configuration errors early (at startup), not during sync
// TRADE-OFF: More upfront validation code, but prevents runtime errors
// INSPIRED BY: "Parse, don't validate" principle - ensure valid state.
func validateMetadata(metadata *CollectionMetadata) error {
	if metadata == nil {
		return errors.New("metadata cannot be nil")
	}

	if metadata.Name == "" {
		return errors.New("collection name cannot be empty")
	}

	if metadata.WorkerType == "" {
		return fmt.Errorf("worker type cannot be empty for collection %q", metadata.Name)
	}

	// Validate role is one of the triangular model roles
	switch metadata.Role {
	case RoleIdentity, RoleDesired, RoleObserved:
		// Valid role
	default:
		return fmt.Errorf("invalid role %q for collection %q (must be %q, %q, or %q)",
			metadata.Role, metadata.Name, RoleIdentity, RoleDesired, RoleObserved)
	}

	return nil
}

// Get retrieves a collection's metadata by name.
//
// Returns an error if the collection is not registered.
//
// Example:
//
//	metadata, err := registry.Get("container_identity")
//	if err != nil {
//	    return fmt.Errorf("collection not found: %w", err)
//	}
//	fmt.Println("CSE fields:", metadata.CSEFields)
func (r *Registry) Get(collectionName string) (*CollectionMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata, exists := r.collections[collectionName]
	if !exists {
		return nil, fmt.Errorf("collection %q not registered", collectionName)
	}

	return metadata, nil
}

// IsRegistered checks if a collection is registered in the registry.
//
// This is a convenience method for "does this collection exist?" checks
// without requiring error handling.
//
// Example:
//
//	if registry.IsRegistered("container_identity") {
//	    // Collection is registered, safe to use
//	}
func (r *Registry) IsRegistered(collectionName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.collections[collectionName]

	return exists
}

// List returns all registered collection metadata.
//
// The returned slice is a snapshot of the registry at the time of the call.
// It's safe to iterate over the returned slice even if the registry is modified
// by other goroutines.
//
// Example:
//
//	for _, metadata := range registry.List() {
//	    fmt.Printf("Collection: %s (role: %s)\n", metadata.Name, metadata.Role)
//	}
func (r *Registry) List() []*CollectionMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*CollectionMetadata, 0, len(r.collections))
	for _, metadata := range r.collections {
		result = append(result, metadata)
	}

	return result
}

// GetTriangularCollections retrieves the identity, desired, and observed collections
// for a given worker type.
//
// This is a convenience method for FSM workers that need all three collections
// in the triangular model. It returns an error if any of the three collections
// are missing (incomplete triangular model).
//
// DESIGN DECISION: Triangular model grouping helper
// WHY: Common pattern for FSM (always need all 3 collections together)
// TRADE-OFF: Assumes naming convention (workerType_role), but this is enforced
// INSPIRED BY: FSM v2 triangular model architecture
//
// Example:
//
//	identity, desired, observed, err := registry.GetTriangularCollections("container")
//	if err != nil {
//	    return fmt.Errorf("incomplete triangular model: %w", err)
//	}
//
//	fmt.Println("Identity collection:", identity.Name)
//	fmt.Println("Desired collection:", desired.Name)
//	fmt.Println("Observed collection:", observed.Name)
func (r *Registry) GetTriangularCollections(workerType string) (identity, desired, observed *CollectionMetadata, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Find collections for this worker type
	var foundIdentity, foundDesired, foundObserved *CollectionMetadata

	for _, metadata := range r.collections {
		if metadata.WorkerType != workerType {
			continue
		}

		switch metadata.Role {
		case RoleIdentity:
			foundIdentity = metadata
		case RoleDesired:
			foundDesired = metadata
		case RoleObserved:
			foundObserved = metadata
		}
	}

	// Validate all three are present
	if foundIdentity == nil {
		return nil, nil, nil, fmt.Errorf("identity collection not found for worker type %q", workerType)
	}

	if foundDesired == nil {
		return nil, nil, nil, fmt.Errorf("desired collection not found for worker type %q", workerType)
	}

	if foundObserved == nil {
		return nil, nil, nil, fmt.Errorf("observed collection not found for worker type %q", workerType)
	}

	return foundIdentity, foundDesired, foundObserved, nil
}

// RegisterVersion sets the schema version for a collection.
//
// DESIGN DECISION: Version per workerType+role, not per collection name
// WHY: Multiple collections can share same workerType+role (e.g., container_identity, container_desired)
// TRADE-OFF: Must track by composite key, but enables consistent versioning across related collections
//
// THREAD SAFETY: Write lock held during entire iteration and mutation.
// Metadata fields are safe to mutate directly because we hold exclusive write lock.
//
// INSPIRED BY: GraphQL schema versioning, Semantic Versioning
//
// Example:
//
//	registry.RegisterVersion("container", "identity", "v2")
//	// Sets version for all container identity collections
func (r *Registry) RegisterVersion(workerType, role, version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty for workerType %q role %q", workerType, role)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, meta := range r.collections {
		if meta.WorkerType == workerType && meta.Role == role {
			meta.SchemaVersion = version
		}
	}

	return nil
}

// GetVersion returns the schema version for a collection.
//
// Returns empty string if no version registered.
func (r *Registry) GetVersion(collection string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if meta, exists := r.collections[collection]; exists {
		return meta.SchemaVersion
	}

	return ""
}

// GetAllVersions returns schema versions for all collections.
//
// Returns map of collection name → schema version.
// Collections without versions are omitted from result.
func (r *Registry) GetAllVersions() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions := make(map[string]string)

	for name, meta := range r.collections {
		if meta.SchemaVersion != "" {
			versions[name] = meta.SchemaVersion
		}
	}

	return versions
}

// RegisterFeature sets whether a feature is supported.
//
// DESIGN DECISION: Feature registry for capability negotiation
// WHY: Frontend needs to know what features umh-core supports
// TRADE-OFF: Extra registry state, but enables per-feature capability detection
//
// THREAD SAFETY: Write lock held during entire mutation.
//
// Example:
//
//	registry.RegisterFeature("delta_sync", true)
//	registry.RegisterFeature("e2e_encryption", true)
//	registry.RegisterFeature("triangular_model", true)
func (r *Registry) RegisterFeature(feature string, supported bool) error {
	if feature == "" {
		return errors.New("feature name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.features[feature] = supported

	return nil
}

// HasFeature checks if a feature is supported.
//
// Returns false if feature is not registered or explicitly set to false.
func (r *Registry) HasFeature(feature string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	supported, exists := r.features[feature]

	return exists && supported
}

// GetFeatures returns all registered features and their support status.
//
// Returns map of feature name → support status.
func (r *Registry) GetFeatures() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	features := make(map[string]bool, len(r.features))
	for name, supported := range r.features {
		features[name] = supported
	}

	return features
}
