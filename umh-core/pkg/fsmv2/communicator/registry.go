package communicator

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

func NewCommunicatorRegistry() *storage.Registry {
	registry := storage.NewRegistry()

	registry.Register(&storage.CollectionMetadata{
		Name:          "communicator_identity",
		WorkerType:    "communicator",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	registry.Register(&storage.CollectionMetadata{
		Name:          "communicator_desired",
		WorkerType:    "communicator",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	registry.Register(&storage.CollectionMetadata{
		Name:          "communicator_observed",
		WorkerType:    "communicator",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	return registry
}
