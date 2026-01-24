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
// Hierarchical composition where parent workers can create child workers by type name.
package factory

import (
	"errors"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var (
	registry   = make(map[string]func(deps.Identity, *zap.SugaredLogger, deps.StateReader, map[string]any) fsmv2.Worker)
	registryMu sync.RWMutex

	// Uses interface{} to avoid circular imports between factory and supervisor packages.
	supervisorRegistry   = make(map[string]func(interface{}) interface{})
	supervisorRegistryMu sync.RWMutex
)

// RegisterFactoryByType adds a worker type to the global registry using a runtime string type.
// For worker package initialization, use RegisterFactory[TObserved, TDesired]() instead.
func RegisterFactoryByType(workerType string, factoryFunc func(deps.Identity, *zap.SugaredLogger, deps.StateReader, map[string]any) fsmv2.Worker) error {
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
// The workerType is derived from the TObserved type parameter.
func RegisterFactory[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](
	factoryFunc func(deps.Identity, *zap.SugaredLogger, deps.StateReader, map[string]any) fsmv2.Worker,
) error {
	workerType, err := storage.DeriveWorkerType[TObserved]()
	if err != nil {
		return fmt.Errorf("failed to derive worker type: %w", err)
	}

	return RegisterFactoryByType(workerType, factoryFunc)
}

// RegisterSupervisorFactory registers a supervisor factory for a worker type.
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
// For worker package initialization, use RegisterSupervisorFactory[TObserved, TDesired]() instead.
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
// For compile-time type-safe creation, use GetFactory[TObserved, TDesired]() instead.
func NewWorkerByType(workerType string, identity deps.Identity, logger *zap.SugaredLogger, stateReader deps.StateReader, deps map[string]any) (fsmv2.Worker, error) {
	if workerType == "" {
		return nil, errors.New("worker type cannot be empty")
	}

	registryMu.RLock()

	factoryFunc, exists := registry[workerType]

	registryMu.RUnlock()

	if !exists {
		return nil, errors.New("unknown worker type: " + workerType)
	}

	return factoryFunc(identity, logger, stateReader, deps), nil
}

// NewSupervisorByType creates a supervisor for the given worker type.
// Uses interface{} to avoid circular imports; caller casts to supervisor.SupervisorInterface.
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

// ResetRegistry clears all registered worker and supervisor factories. For testing only.
func ResetRegistry() {
	registryMu.Lock()

	registry = make(map[string]func(deps.Identity, *zap.SugaredLogger, deps.StateReader, map[string]any) fsmv2.Worker)

	registryMu.Unlock()

	supervisorRegistryMu.Lock()

	supervisorRegistry = make(map[string]func(interface{}) interface{})

	supervisorRegistryMu.Unlock()
}

// ListRegisteredTypes returns all registered worker type names.
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
func GetFactory[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState]() (func(deps.Identity, *zap.SugaredLogger, deps.StateReader, map[string]any) fsmv2.Worker, bool, error) {
	workerType, err := storage.DeriveWorkerType[TObserved]()
	if err != nil {
		return nil, false, fmt.Errorf("failed to derive worker type: %w", err)
	}

	registryMu.RLock()
	defer registryMu.RUnlock()

	factoryFunc, exists := registry[workerType]

	return factoryFunc, exists, nil
}

// RegisterWorkerAndSupervisorFactoryByType registers both worker and supervisor factories.
// Not atomic: rolls back worker registration if supervisor registration fails.
func RegisterWorkerAndSupervisorFactoryByType(
	workerType string,
	workerFactory func(deps.Identity, *zap.SugaredLogger, deps.StateReader, map[string]any) fsmv2.Worker,
	supervisorFactory func(interface{}) interface{},
) error {
	if workerType == "" {
		return errors.New("worker type cannot be empty")
	}

	if err := RegisterFactoryByType(workerType, workerFactory); err != nil {
		return fmt.Errorf("failed to register worker factory: %w", err)
	}

	if err := RegisterSupervisorFactoryByType(workerType, supervisorFactory); err != nil {
		registryMu.Lock()
		delete(registry, workerType)
		registryMu.Unlock()

		return fmt.Errorf("failed to register supervisor factory: %w", err)
	}

	return nil
}

// RegisterWorkerType registers both worker and supervisor factories using compile-time type parameters.
// Preferred API: provides type safety and automatic worker type derivation.
func RegisterWorkerType[TObserved fsmv2.ObservedState, TDesired fsmv2.DesiredState](
	workerFactory func(deps.Identity, *zap.SugaredLogger, deps.StateReader, map[string]any) fsmv2.Worker,
	supervisorFactory func(interface{}) interface{},
) error {
	workerType, err := storage.DeriveWorkerType[TObserved]()
	if err != nil {
		return fmt.Errorf("failed to derive worker type: %w", err)
	}

	return RegisterWorkerAndSupervisorFactoryByType(workerType, workerFactory, supervisorFactory)
}

// ValidateRegistryConsistency checks for mismatches between worker and supervisor registries.
// Returns worker types without supervisors, and supervisor types without workers.
func ValidateRegistryConsistency() (workerOnly []string, supervisorOnly []string) {
	registryMu.RLock()

	workerTypes := make(map[string]bool, len(registry))

	for workerType := range registry {
		workerTypes[workerType] = true
	}

	registryMu.RUnlock()

	supervisorRegistryMu.RLock()

	supervisorTypes := make(map[string]bool, len(supervisorRegistry))

	for supervisorType := range supervisorRegistry {
		supervisorTypes[supervisorType] = true
	}

	supervisorRegistryMu.RUnlock()

	for workerType := range workerTypes {
		if !supervisorTypes[workerType] {
			workerOnly = append(workerOnly, workerType)
		}
	}

	for supervisorType := range supervisorTypes {
		if !workerTypes[supervisorType] {
			supervisorOnly = append(supervisorOnly, supervisorType)
		}
	}

	return workerOnly, supervisorOnly
}

// ListSupervisorTypes returns all registered supervisor type names.
func ListSupervisorTypes() []string {
	supervisorRegistryMu.RLock()
	defer supervisorRegistryMu.RUnlock()

	types := make([]string, 0, len(supervisorRegistry))
	for supervisorType := range supervisorRegistry {
		types = append(types, supervisorType)
	}

	return types
}
