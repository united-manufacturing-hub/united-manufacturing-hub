// Package factory provides a registry-based worker factory for dynamic FSM v2 worker creation.
// This enables hierarchical composition where parent workers can create child workers by type name.
package factory

import (
	"errors"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

var (
	// registry maps worker type names to factory functions.
	registry = make(map[string]func(fsmv2.Identity) fsmv2.Worker)
	// registryMu protects concurrent access to the registry.
	registryMu sync.RWMutex
)

// RegisterWorkerType adds a worker type to the global registry.
// This is typically called during package initialization (init functions) or startup.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// However, duplicate registrations will return an error.
//
// ERROR CONDITIONS:
//   - Returns error if workerType is empty
//   - Returns error if workerType is already registered
//
// Example usage:
//
//	func init() {
//	    err := factory.RegisterWorkerType("mqtt_client", func(id fsmv2.Identity) fsmv2.Worker {
//	        return &MQTTWorker{identity: id}
//	    })
//	    if err != nil {
//	        panic(err)
//	    }
//	}
func RegisterWorkerType(workerType string, factoryFunc func(fsmv2.Identity) fsmv2.Worker) error {
	if workerType == "" {
		return errors.New("worker type cannot be empty")
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[workerType]; exists {
		return errors.New("worker type already registered: " + workerType)
	}

	registry[workerType] = factoryFunc

	return nil
}

// NewWorker creates a worker instance by type name.
// This is called by supervisors during hierarchical child instantiation.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// The registry is protected by a read-write mutex.
//
// ERROR CONDITIONS:
//   - Returns error if workerType is empty
//   - Returns error if workerType is not registered
//
// Example usage in supervisor:
//
//	// When processing ChildSpec during reconciliation
//	worker, err := factory.NewWorker(spec.WorkerType, identity)
//	if err != nil {
//	    return fmt.Errorf("failed to create child worker: %w", err)
//	}
func NewWorker(workerType string, identity fsmv2.Identity) (fsmv2.Worker, error) {
	if workerType == "" {
		return nil, errors.New("worker type cannot be empty")
	}

	registryMu.RLock()

	factoryFunc, exists := registry[workerType]

	registryMu.RUnlock()

	if !exists {
		return nil, errors.New("unknown worker type: " + workerType)
	}

	return factoryFunc(identity), nil
}

// ResetRegistry clears all registered worker types.
// This is primarily used for testing to ensure clean state between tests.
//
// THREAD SAFETY:
// This function is thread-safe but should only be called during test setup,
// not in production code.
//
// Example usage in tests:
//
//	func TestMyFeature(t *testing.T) {
//	    factory.ResetRegistry()  // Clean state
//	    // ... register test worker types
//	    // ... run tests
//	}
func ResetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry = make(map[string]func(fsmv2.Identity) fsmv2.Worker)
}

// ListRegisteredTypes returns all registered worker type names.
// Thread-safe - returns a copy of registered type names, not references to internal registry.
// Useful for debugging, introspection, and validation.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// The returned slice is a copy, so modifying it does not affect the registry.
//
// Return value:
// A slice of registered worker type names. Returns an empty (non-nil) slice if no types are registered.
// The order of types in the slice is not guaranteed.
//
// Example usage:
//
//	types := factory.ListRegisteredTypes()
//	if !contains(types, "mqtt_client") {
//	    return fmt.Errorf("mqtt_client worker type not registered")
//	}
func ListRegisteredTypes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	types := make([]string, 0, len(registry))
	for workerType := range registry {
		types = append(types, workerType)
	}

	return types
}
