package communicator

import (
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

func TestNewCommunicatorRegistry(t *testing.T) {
	registry := NewCommunicatorRegistry()

	if registry == nil {
		t.Fatal("NewCommunicatorRegistry() returned nil")
	}

	identity, desired, observed, err := registry.GetTriangularCollections("communicator")
	if err != nil {
		t.Fatalf("Failed to get triangular collections: %v", err)
	}

	if identity.Name != "communicator_identity" {
		t.Errorf("Expected identity collection name 'communicator_identity', got '%s'", identity.Name)
	}
	if identity.Role != storage.RoleIdentity {
		t.Errorf("Expected identity role '%s', got '%s'", storage.RoleIdentity, identity.Role)
	}
	if identity.WorkerType != "communicator" {
		t.Errorf("Expected identity worker type 'communicator', got '%s'", identity.WorkerType)
	}

	if desired.Name != "communicator_desired" {
		t.Errorf("Expected desired collection name 'communicator_desired', got '%s'", desired.Name)
	}
	if desired.Role != storage.RoleDesired {
		t.Errorf("Expected desired role '%s', got '%s'", storage.RoleDesired, desired.Role)
	}
	if desired.WorkerType != "communicator" {
		t.Errorf("Expected desired worker type 'communicator', got '%s'", desired.WorkerType)
	}

	if observed.Name != "communicator_observed" {
		t.Errorf("Expected observed collection name 'communicator_observed', got '%s'", observed.Name)
	}
	if observed.Role != storage.RoleObserved {
		t.Errorf("Expected observed role '%s', got '%s'", storage.RoleObserved, observed.Role)
	}
	if observed.WorkerType != "communicator" {
		t.Errorf("Expected observed worker type 'communicator', got '%s'", observed.WorkerType)
	}
}
