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

package storage_test

import (
	"fmt"
	"log"
	"sort"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

// Example_basicUsage demonstrates basic registry usage with the global registry.
func Example_basicUsage() {
	// Register a collection at application startup
	err := storage.Register(&storage.CollectionMetadata{
		Name:          "container_identity",
		WorkerType:    "container",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
		RelatedTo:     []string{"container_desired", "container_observed"},
	})
	if err != nil {
		log.Fatalf("Failed to register collection: %v", err)
	}

	// Later: Check if collection is registered
	if storage.IsRegistered("container_identity") {
		fmt.Println("Collection registered")
	}

	// Later: Retrieve metadata
	metadata, err := storage.Get("container_identity")
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
	collections := []*storage.CollectionMetadata{
		{
			Name:          "relay_identity",
			WorkerType:    "relay",
			Role:          storage.RoleIdentity,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		},
		{
			Name:          "relay_desired",
			WorkerType:    "relay",
			Role:          storage.RoleDesired,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		},
		{
			Name:          "relay_observed",
			WorkerType:    "relay",
			Role:          storage.RoleObserved,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		},
	}

	for _, metadata := range collections {
		if err := storage.Register(metadata); err != nil {
			log.Fatalf("Failed to register %s: %v", metadata.Name, err)
		}
	}

	// Retrieve all three collections at once
	identity, desired, observed, err := storage.GetTriangularCollections("relay")
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
	registry := storage.NewRegistry()

	// Register collections
	registry.Register(&storage.CollectionMetadata{
		Name:       "test_identity",
		WorkerType: "test",
		Role:       storage.RoleIdentity,
	})
	registry.Register(&storage.CollectionMetadata{
		Name:       "test_desired",
		WorkerType: "test",
		Role:       storage.RoleDesired,
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
		storage.FieldSyncID,
		storage.FieldVersion,
		storage.FieldCreatedAt,
		storage.FieldUpdatedAt,
		storage.FieldDeletedAt,
		storage.FieldDeletedBy,
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
		storage.RoleIdentity,
		storage.RoleDesired,
		storage.RoleObserved,
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
