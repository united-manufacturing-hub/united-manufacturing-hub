package communicator

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
)

func NewCommunicatorRegistry() *storage.Registry {
	registry := storage.NewRegistry()

	registry.Register(&storage.CollectionMetadata{
		Name:       "communicator_identity",
		WorkerType: "communicator",
		Role:       storage.RoleIdentity,
	})

	registry.Register(&storage.CollectionMetadata{
		Name:       "communicator_desired",
		WorkerType: "communicator",
		Role:       storage.RoleDesired,
	})

	registry.Register(&storage.CollectionMetadata{
		Name:       "communicator_observed",
		WorkerType: "communicator",
		Role:       storage.RoleObserved,
	})

	return registry
}
