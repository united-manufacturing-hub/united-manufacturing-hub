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

// Package factory provides a registry-based worker factory for dynamic FSM v2 worker creation.
// This enables hierarchical composition where parent workers can create child workers by type name.
package factory

import (
	"errors"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

var (
	// registry maps worker type names to factory functions.
	// Factory functions receive Identity and Logger to create properly-configured workers.
	registry = make(map[string]func(fsmv2.Identity, *zap.SugaredLogger) fsmv2.Worker)
	// registryMu protects concurrent access to the registry.
	registryMu sync.RWMutex

	// supervisorRegistry stores supervisor factory functions keyed by worker type.
	// Factory functions take a config interface{} parameter and return an interface{} supervisor.
	// The actual types are supervisor.Config and supervisor.SupervisorInterface, but we use
	// interface{} here to avoid circular imports between factory and supervisor packages.
	supervisorRegistry = make(map[string]func(interface{}) interface{})
	// supervisorRegistryMu protects concurrent access to the supervisor registry.
	supervisorRegistryMu sync.RWMutex
)

// RegisterFactoryByType adds a worker type to the global registry using a runtime string type.
// This is used for supervisor internals that work with children polymorphically.
// For worker package initialization, use RegisterFactory[TObserved, TDesired]() instead.
//
// The factory function receives the supervisor's logger, allowing workers to use consistent
// structured logging throughout the worker hierarchy.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// However, duplicate registrations will return an error.
//
// ERROR CONDITIONS:
//   - Returns error if workerType is empty
//   - Returns error if workerType is already registered
//
// Example usage (supervisor internals):
//
//	err := factory.RegisterFactoryByType("mqtt_client", func(id fsmv2.Identity, logger *zap.SugaredLogger) fsmv2.Worker {
//	    return NewMQTTWorker(id, logger)
//	})
func RegisterFactoryByType(workerType string, factoryFunc func(fsmv2.Identity, *zap.SugaredLogger) fsmv2.Worker) error {
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

// RegisterFactory adds a worker type to the global registry using compile-time type parameters.
// This is the recommended API for worker packages registering themselves during initialization.
// The workerType is automatically derived from the TObserved type parameter.
//
// The factory function receives the supervisor's logger, allowing workers to use consistent
// structured logging throughout the worker hierarchy.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// However, duplicate registrations will return an error.
//
// ERROR CONDITIONS:
//   - Returns error if the derived workerType is empty
//   - Returns error if the workerType is already registered
//
// Example usage:
//
//	func init() {
//	    err := factory.RegisterFactory[ContainerObservedState, ContainerDesiredState](
//	        func(id fsmv2.Identity, logger *zap.SugaredLogger) fsmv2.Worker {
//	            return NewContainerWorker(id, logger)
//	        })
//	    if err != nil {
//	        panic(err)
//	    }
//	}
func RegisterFactory[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](
	factoryFunc func(fsmv2.Identity, *zap.SugaredLogger) fsmv2.Worker,
) error {
	workerType, err := storage.DeriveWorkerType[TObserved]()
	if err != nil {
		return fmt.Errorf("failed to derive worker type: %w", err)
	}

	return RegisterFactoryByType(workerType, factoryFunc)
}

// RegisterSupervisorFactory registers a supervisor factory for a worker type.
// The supervisor factory creates properly-typed supervisors for the given worker type.
// This is called alongside RegisterFactory to enable child supervisor creation.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// However, duplicate registrations will return an error.
//
// ERROR CONDITIONS:
//   - Returns error if the supervisor factory is already registered for this worker type
//
// Example usage:
//
//	func init() {
//	    err := factory.RegisterSupervisorFactory[ContainerObservedState, ContainerDesiredState](
//	        func(cfg interface{}) interface{} {
//	            supervisorCfg := cfg.(supervisor.Config)
//	            return supervisor.New[ContainerObservedState, ContainerDesiredState](supervisorCfg)
//	        })
//	    if err != nil {
//	        panic(err)
//	    }
//	}
func RegisterSupervisorFactory[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](
	factoryFunc func(interface{}) interface{},
) error {
	workerType, err := storage.DeriveWorkerType[TObserved]()
	if err != nil {
		return fmt.Errorf("failed to derive worker type: %w", err)
	}

	supervisorRegistryMu.Lock()
	defer supervisorRegistryMu.Unlock()

	if _, exists := supervisorRegistry[workerType]; exists {
		return fmt.Errorf("supervisor factory already registered for worker type: %s", workerType)
	}

	supervisorRegistry[workerType] = factoryFunc

	return nil
}

// RegisterSupervisorFactoryByType adds a supervisor factory using a runtime string type name.
// This is used for testing and cases where the worker type is determined at runtime.
// For worker package initialization, use RegisterSupervisorFactory[TObserved, TDesired]() instead.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// However, duplicate registrations will return an error.
//
// ERROR CONDITIONS:
//   - Returns error if workerType is empty
//   - Returns error if workerType is already registered
//
// Example usage (testing):
//
//	err := factory.RegisterSupervisorFactoryByType("child", func(cfg interface{}) interface{} {
//	    supervisorCfg := cfg.(supervisor.Config)
//	    return supervisor.NewSupervisor[*TestObservedState, *TestDesiredState](supervisorCfg)
//	})
func RegisterSupervisorFactoryByType(workerType string, factoryFunc func(interface{}) interface{}) error {
	if workerType == "" {
		return errors.New("worker type cannot be empty")
	}

	supervisorRegistryMu.Lock()
	defer supervisorRegistryMu.Unlock()

	if _, exists := supervisorRegistry[workerType]; exists {
		return fmt.Errorf("supervisor factory already registered for worker type: %s", workerType)
	}

	supervisorRegistry[workerType] = factoryFunc

	return nil
}

// NewWorkerByType creates a worker instance by runtime string type name.
// This is used by supervisors during hierarchical child instantiation (runtime polymorphism).
// For compile-time type-safe worker creation, use GetFactory[TObserved, TDesired]() instead.
//
// The logger parameter is passed to the factory function, allowing workers to receive
// the supervisor's logger for consistent structured logging throughout the hierarchy.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// The registry is protected by a read-write mutex.
//
// ERROR CONDITIONS:
//   - Returns error if workerType is empty
//   - Returns error if workerType is not registered
//
// Example usage in supervisor (processing ChildSpec):
//
//	worker, err := factory.NewWorkerByType(spec.WorkerType, identity, s.logger)
//	if err != nil {
//	    return fmt.Errorf("failed to create child worker: %w", err)
//	}
func NewWorkerByType(workerType string, identity fsmv2.Identity, logger *zap.SugaredLogger) (fsmv2.Worker, error) {
	if workerType == "" {
		return nil, errors.New("worker type cannot be empty")
	}

	registryMu.RLock()

	factoryFunc, exists := registry[workerType]

	registryMu.RUnlock()

	if !exists {
		return nil, errors.New("unknown worker type: " + workerType)
	}

	return factoryFunc(identity, logger), nil
}

// NewSupervisorByType creates a supervisor for the given worker type.
// This is used by parent supervisors when creating child supervisors.
//
// The config parameter should be of type supervisor.Config, and the returned
// interface{} should be cast to supervisor.SupervisorInterface by the caller.
// We use interface{} types here to avoid circular imports between factory and supervisor packages.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
// The supervisor registry is protected by a read-write mutex.
//
// ERROR CONDITIONS:
//   - Returns error if workerType is empty
//   - Returns error if no supervisor factory is registered for the worker type
//
// Example usage in parent supervisor:
//
//	rawSupervisor, err := factory.NewSupervisorByType(spec.WorkerType, supervisorConfig)
//	if err != nil {
//	    return fmt.Errorf("failed to create child supervisor: %w", err)
//	}
//	childSupervisor := rawSupervisor.(supervisor.SupervisorInterface)
func NewSupervisorByType(workerType string, config interface{}) (interface{}, error) {
	if workerType == "" {
		return nil, errors.New("worker type cannot be empty")
	}

	supervisorRegistryMu.RLock()

	factoryFunc, exists := supervisorRegistry[workerType]

	supervisorRegistryMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no supervisor factory registered for worker type: %s", workerType)
	}

	return factoryFunc(config), nil
}

// ResetRegistry clears all registered worker and supervisor factories.
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

	registry = make(map[string]func(fsmv2.Identity, *zap.SugaredLogger) fsmv2.Worker)

	registryMu.Unlock()

	supervisorRegistryMu.Lock()

	supervisorRegistry = make(map[string]func(interface{}) interface{})

	supervisorRegistryMu.Unlock()
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

// GetFactory retrieves a worker factory by compile-time type parameters.
// This is the recommended API for code that knows worker types at compile time.
// The workerType is automatically derived from the TObserved type parameter.
//
// THREAD SAFETY:
// This function is thread-safe and can be called concurrently from multiple goroutines.
//
// Return value:
// Returns the factory function and true if the worker type is registered.
// Returns nil and false if the worker type is not registered.
//
// Example usage:
//
//	factory, ok, err := factory.GetFactory[ContainerObservedState, ContainerDesiredState]()
//	if err != nil {
//	    return fmt.Errorf("failed to derive worker type: %w", err)
//	}
//	if !ok {
//	    return fmt.Errorf("container worker type not registered")
//	}
//	worker := factory(identity, logger)
func GetFactory[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState]() (func(fsmv2.Identity, *zap.SugaredLogger) fsmv2.Worker, bool, error) {
	workerType, err := storage.DeriveWorkerType[TObserved]()
	if err != nil {
		return nil, false, fmt.Errorf("failed to derive worker type: %w", err)
	}

	registryMu.RLock()
	defer registryMu.RUnlock()

	factoryFunc, exists := registry[workerType]

	return factoryFunc, exists, nil
}
