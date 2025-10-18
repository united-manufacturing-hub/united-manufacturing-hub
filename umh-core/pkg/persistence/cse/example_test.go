package cse_test

import (
	"fmt"
	"log"
	"sort"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

// Example_basicUsage demonstrates basic registry usage with the global registry.
func Example_basicUsage() {
	// Register a collection at application startup
	err := cse.Register(&cse.CollectionMetadata{
		Name:          "container_identity",
		WorkerType:    "container",
		Role:          cse.RoleIdentity,
		CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion, cse.FieldCreatedAt, cse.FieldUpdatedAt},
		IndexedFields: []string{cse.FieldSyncID},
		RelatedTo:     []string{"container_desired", "container_observed"},
	})
	if err != nil {
		log.Fatalf("Failed to register collection: %v", err)
	}

	// Later: Check if collection is registered
	if cse.IsRegistered("container_identity") {
		fmt.Println("Collection registered")
	}

	// Later: Retrieve metadata
	metadata, err := cse.Get("container_identity")
	if err != nil {
		log.Fatalf("Failed to get collection: %v", err)
	}

	fmt.Printf("Collection: %s\n", metadata.Name)
	fmt.Printf("Role: %s\n", metadata.Role)
	fmt.Printf("CSE Fields: %v\n", metadata.CSEFields)

	// Output:
	// Collection registered
	// Collection: container_identity
	// Role: identity
	// CSE Fields: [_sync_id _version _created_at _updated_at]
}

// Example_triangularModel demonstrates registering and retrieving a complete triangular model.
func Example_triangularModel() {
	// Register all three collections for the "relay" worker type
	collections := []*cse.CollectionMetadata{
		{
			Name:          "relay_identity",
			WorkerType:    "relay",
			Role:          cse.RoleIdentity,
			CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion, cse.FieldCreatedAt},
			IndexedFields: []string{cse.FieldSyncID},
		},
		{
			Name:          "relay_desired",
			WorkerType:    "relay",
			Role:          cse.RoleDesired,
			CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion, cse.FieldCreatedAt, cse.FieldUpdatedAt},
			IndexedFields: []string{cse.FieldSyncID},
		},
		{
			Name:          "relay_observed",
			WorkerType:    "relay",
			Role:          cse.RoleObserved,
			CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion, cse.FieldCreatedAt, cse.FieldUpdatedAt},
			IndexedFields: []string{cse.FieldSyncID},
		},
	}

	for _, metadata := range collections {
		if err := cse.Register(metadata); err != nil {
			log.Fatalf("Failed to register %s: %v", metadata.Name, err)
		}
	}

	// Retrieve all three collections at once
	identity, desired, observed, err := cse.GetTriangularCollections("relay")
	if err != nil {
		log.Fatalf("Failed to get triangular collections: %v", err)
	}

	fmt.Printf("Identity: %s\n", identity.Name)
	fmt.Printf("Desired: %s\n", desired.Name)
	fmt.Printf("Observed: %s\n", observed.Name)

	// Output:
	// Identity: relay_identity
	// Desired: relay_desired
	// Observed: relay_observed
}

// Example_separateRegistry demonstrates creating a separate registry instance for testing.
func Example_separateRegistry() {
	// Create a separate registry (useful for testing)
	registry := cse.NewRegistry()

	// Register collections
	registry.Register(&cse.CollectionMetadata{
		Name:       "test_identity",
		WorkerType: "test",
		Role:       cse.RoleIdentity,
	})
	registry.Register(&cse.CollectionMetadata{
		Name:       "test_desired",
		WorkerType: "test",
		Role:       cse.RoleDesired,
	})

	// List all registered collections
	collections := registry.List()
	fmt.Printf("Registered %d collections\n", len(collections))

	// Sort by name for consistent output in tests
	sort.Slice(collections, func(i, j int) bool {
		return collections[i].Name < collections[j].Name
	})

	for _, metadata := range collections {
		fmt.Printf("- %s (%s)\n", metadata.Name, metadata.Role)
	}

	// Output:
	// Registered 2 collections
	// - test_desired (desired)
	// - test_identity (identity)
}

// Example_cseFieldConstants demonstrates using CSE field constants.
func Example_cseFieldConstants() {
	// CSE field constants are available for building queries
	fields := []string{
		cse.FieldSyncID,
		cse.FieldVersion,
		cse.FieldCreatedAt,
		cse.FieldUpdatedAt,
		cse.FieldDeletedAt,
		cse.FieldDeletedBy,
	}

	fmt.Println("CSE metadata fields:")
	for _, field := range fields {
		fmt.Printf("- %s\n", field)
	}

	// Output:
	// CSE metadata fields:
	// - _sync_id
	// - _version
	// - _created_at
	// - _updated_at
	// - _deleted_at
	// - _deleted_by
}

// Example_roleConstants demonstrates using role constants.
func Example_roleConstants() {
	roles := []string{
		cse.RoleIdentity,
		cse.RoleDesired,
		cse.RoleObserved,
	}

	fmt.Println("Triangular model roles:")
	for _, role := range roles {
		fmt.Printf("- %s\n", role)
	}

	// Output:
	// Triangular model roles:
	// - identity
	// - desired
	// - observed
}
