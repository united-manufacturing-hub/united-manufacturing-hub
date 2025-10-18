package cse_test

import (
	"fmt"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

func TestNewRegistry(t *testing.T) {
	registry := cse.NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
	}

	collections := registry.List()
	if len(collections) != 0 {
		t.Errorf("New registry should be empty, got %d collections", len(collections))
	}
}

func TestRegistry_Register(t *testing.T) {
	registry := cse.NewRegistry()

	metadata := &cse.CollectionMetadata{
		Name:          "container_identity",
		WorkerType:    "container",
		Role:          cse.RoleIdentity,
		CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion},
		IndexedFields: []string{cse.FieldSyncID},
	}

	err := registry.Register(metadata)
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	if !registry.IsRegistered("container_identity") {
		t.Error("Collection not registered")
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := cse.NewRegistry()

	metadata := &cse.CollectionMetadata{
		Name:       "container_identity",
		WorkerType: "container",
		Role:       cse.RoleIdentity,
	}

	registry.Register(metadata)

	retrieved, err := registry.Get("container_identity")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if retrieved.Name != "container_identity" {
		t.Errorf("Get() returned wrong metadata: %v", retrieved)
	}
}

func TestRegistry_GetTriangularCollections(t *testing.T) {
	registry := cse.NewRegistry()

	// Register triangular collections for "container" worker type
	registry.Register(&cse.CollectionMetadata{
		Name:       "container_identity",
		WorkerType: "container",
		Role:       cse.RoleIdentity,
	})
	registry.Register(&cse.CollectionMetadata{
		Name:       "container_desired",
		WorkerType: "container",
		Role:       cse.RoleDesired,
	})
	registry.Register(&cse.CollectionMetadata{
		Name:       "container_observed",
		WorkerType: "container",
		Role:       cse.RoleObserved,
	})

	identity, desired, observed, err := registry.GetTriangularCollections("container")
	if err != nil {
		t.Fatalf("GetTriangularCollections() failed: %v", err)
	}

	if identity.Name != "container_identity" {
		t.Errorf("Wrong identity collection: %v", identity)
	}
	if desired.Name != "container_desired" {
		t.Errorf("Wrong desired collection: %v", desired)
	}
	if observed.Name != "container_observed" {
		t.Errorf("Wrong observed collection: %v", observed)
	}
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
	registry := cse.NewRegistry()

	metadata := &cse.CollectionMetadata{
		Name:       "container_identity",
		WorkerType: "container",
		Role:       cse.RoleIdentity,
	}

	err := registry.Register(metadata)
	if err != nil {
		t.Fatalf("First Register() failed: %v", err)
	}

	err = registry.Register(metadata)
	if err == nil {
		t.Error("Duplicate Register() should return error")
	}
}

func TestRegistry_List(t *testing.T) {
	registry := cse.NewRegistry()

	// Register multiple collections
	registry.Register(&cse.CollectionMetadata{
		Name:       "container_identity",
		WorkerType: "container",
		Role:       cse.RoleIdentity,
	})
	registry.Register(&cse.CollectionMetadata{
		Name:       "container_desired",
		WorkerType: "container",
		Role:       cse.RoleDesired,
	})

	collections := registry.List()
	if len(collections) != 2 {
		t.Errorf("List() should return 2 collections, got %d", len(collections))
	}
}

func TestRegistry_IsRegistered(t *testing.T) {
	registry := cse.NewRegistry()

	if registry.IsRegistered("nonexistent") {
		t.Error("IsRegistered() should return false for unregistered collection")
	}

	registry.Register(&cse.CollectionMetadata{
		Name:       "container_identity",
		WorkerType: "container",
		Role:       cse.RoleIdentity,
	})

	if !registry.IsRegistered("container_identity") {
		t.Error("IsRegistered() should return true for registered collection")
	}
}

func TestRegistry_GetNonexistent(t *testing.T) {
	registry := cse.NewRegistry()

	_, err := registry.Get("nonexistent")
	if err == nil {
		t.Error("Get() should return error for nonexistent collection")
	}
}

func TestRegistry_GetTriangularCollections_Missing(t *testing.T) {
	registry := cse.NewRegistry()

	// Only register identity, missing desired and observed
	registry.Register(&cse.CollectionMetadata{
		Name:       "container_identity",
		WorkerType: "container",
		Role:       cse.RoleIdentity,
	})

	_, _, _, err := registry.GetTriangularCollections("container")
	if err == nil {
		t.Error("GetTriangularCollections() should return error when collections are missing")
	}
}

func TestCollectionMetadata_CSEFields(t *testing.T) {
	// Test that CSE field constants are available
	expectedFields := []string{
		cse.FieldSyncID,
		cse.FieldVersion,
		cse.FieldCreatedAt,
		cse.FieldUpdatedAt,
		cse.FieldDeletedAt,
		cse.FieldDeletedBy,
	}

	for _, field := range expectedFields {
		if field == "" {
			t.Errorf("CSE field constant should not be empty")
		}
	}
}

func TestCollectionMetadata_Roles(t *testing.T) {
	// Test that role constants are available
	expectedRoles := []string{
		cse.RoleIdentity,
		cse.RoleDesired,
		cse.RoleObserved,
	}

	for _, role := range expectedRoles {
		if role == "" {
			t.Errorf("Role constant should not be empty")
		}
	}
}

func TestRegistry_Register_Validation(t *testing.T) {
	tests := []struct {
		name     string
		metadata *cse.CollectionMetadata
		wantErr  bool
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			wantErr:  true,
		},
		{
			name: "empty collection name",
			metadata: &cse.CollectionMetadata{
				Name:       "",
				WorkerType: "container",
				Role:       cse.RoleIdentity,
			},
			wantErr: true,
		},
		{
			name: "empty worker type",
			metadata: &cse.CollectionMetadata{
				Name:       "container_identity",
				WorkerType: "",
				Role:       cse.RoleIdentity,
			},
			wantErr: true,
		},
		{
			name: "invalid role",
			metadata: &cse.CollectionMetadata{
				Name:       "container_identity",
				WorkerType: "container",
				Role:       "invalid_role",
			},
			wantErr: true,
		},
		{
			name: "valid metadata",
			metadata: &cse.CollectionMetadata{
				Name:       "container_identity",
				WorkerType: "container",
				Role:       cse.RoleIdentity,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := cse.NewRegistry()
			err := registry.Register(tt.metadata)
			if (err != nil) != tt.wantErr {
				t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	registry := cse.NewRegistry()

	// Register some initial collections
	for i := 0; i < 10; i++ {
		registry.Register(&cse.CollectionMetadata{
			Name:       fmt.Sprintf("collection_%d", i),
			WorkerType: "test",
			Role:       cse.RoleIdentity,
		})
	}

	// Concurrent reads
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			collections := registry.List()
			if len(collections) < 10 {
				t.Errorf("Expected at least 10 collections, got %d", len(collections))
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestGlobalRegistry_Register(t *testing.T) {
	// Note: This test uses the global registry, which may be affected by other tests
	// For production code, prefer using separate Registry instances in tests

	metadata := &cse.CollectionMetadata{
		Name:       "global_test_identity",
		WorkerType: "global_test",
		Role:       cse.RoleIdentity,
	}

	err := cse.Register(metadata)
	if err != nil {
		t.Fatalf("Global Register() failed: %v", err)
	}

	if !cse.IsRegistered("global_test_identity") {
		t.Error("Collection not registered in global registry")
	}
}

func TestGlobalRegistry_Get(t *testing.T) {
	metadata := &cse.CollectionMetadata{
		Name:       "global_get_test",
		WorkerType: "test",
		Role:       cse.RoleIdentity,
	}

	cse.Register(metadata)

	retrieved, err := cse.Get("global_get_test")
	if err != nil {
		t.Fatalf("Global Get() failed: %v", err)
	}

	if retrieved.Name != "global_get_test" {
		t.Errorf("Global Get() returned wrong metadata: %v", retrieved)
	}
}

func TestGlobalRegistry_List(t *testing.T) {
	// Register a collection
	cse.Register(&cse.CollectionMetadata{
		Name:       "global_list_test",
		WorkerType: "test",
		Role:       cse.RoleIdentity,
	})

	collections := cse.List()
	// Should have at least the collections we registered in previous tests
	if len(collections) == 0 {
		t.Error("Global List() returned empty list")
	}
}

func TestGlobalRegistry_GetTriangularCollections(t *testing.T) {
	// Register triangular collections
	cse.Register(&cse.CollectionMetadata{
		Name:       "global_triangular_identity",
		WorkerType: "global_triangular",
		Role:       cse.RoleIdentity,
	})
	cse.Register(&cse.CollectionMetadata{
		Name:       "global_triangular_desired",
		WorkerType: "global_triangular",
		Role:       cse.RoleDesired,
	})
	cse.Register(&cse.CollectionMetadata{
		Name:       "global_triangular_observed",
		WorkerType: "global_triangular",
		Role:       cse.RoleObserved,
	})

	identity, desired, observed, err := cse.GetTriangularCollections("global_triangular")
	if err != nil {
		t.Fatalf("Global GetTriangularCollections() failed: %v", err)
	}

	if identity.Name != "global_triangular_identity" {
		t.Errorf("Wrong identity collection: %v", identity)
	}
	if desired.Name != "global_triangular_desired" {
		t.Errorf("Wrong desired collection: %v", desired)
	}
	if observed.Name != "global_triangular_observed" {
		t.Errorf("Wrong observed collection: %v", observed)
	}
}
