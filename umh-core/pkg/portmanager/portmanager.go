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

// Package portmanager provides functionality to allocate, reserve and manage ports for services
package portmanager

import (
	"context"
	"fmt"
	"sync"
)

// PortManager is an interface that defines methods for managing ports
type PortManager interface {
	// AllocatePort allocates a port for a given instance and returns it
	// Returns an error if no ports are available
	AllocatePort(instanceName string) (int, error)

	// ReleasePort releases a port previously allocated to an instance
	// Returns an error if the instance doesn't have a port
	ReleasePort(instanceName string) error

	// GetPort retrieves the port for a given instance
	// Returns the port and true if found, 0 and false otherwise
	GetPort(instanceName string) (int, bool)

	// ReservePort attempts to reserve a specific port for an instance
	// Returns an error if the port is already in use
	ReservePort(instanceName string, port int) error

	// PreReconcile is called before the base FSM reconciliation to ensure ports are allocated
	// It takes a list of instance names that should have ports allocated
	// Returns an error if port allocation fails
	PreReconcile(ctx context.Context, instanceNames []string) error

	// PostReconcile is called after the base FSM reconciliation to clean up any orphaned ports
	// It releases ports for instances that no longer exist
	PostReconcile(ctx context.Context) error
}

// DefaultPortManager is a thread-safe implementation of PortManager
// that keeps track of ports in a simple in-memory store
type DefaultPortManager struct {
	// mutex to protect concurrent access to maps
	mutex sync.RWMutex

	// instanceToPorts maps instance names to their allocated ports
	instanceToPorts map[string]int

	// portToInstances maps ports to instance names
	portToInstances map[int]string

	// configuration
	minPort  int
	maxPort  int
	nextPort int
}

// Global singleton instance of DefaultPortManager
var (
	defaultPortManagerInstance *DefaultPortManager
	defaultPortManagerOnce     sync.Once
	defaultPortManagerMutex    sync.RWMutex
)

// GetDefaultPortManager returns the singleton instance of DefaultPortManager.
// If the instance hasn't been initialized yet, it returns nil.
func GetDefaultPortManager() *DefaultPortManager {
	defaultPortManagerMutex.RLock()
	defer defaultPortManagerMutex.RUnlock()
	return defaultPortManagerInstance
}

// InitDefaultPortManager initializes the singleton DefaultPortManager with the given port range.
// It ensures the DefaultPortManager is initialized only once.
// Returns error if initialization fails or if it was already initialized with different parameters.
func InitDefaultPortManager(minPort, maxPort int) (*DefaultPortManager, error) {
	var initErr error

	defaultPortManagerOnce.Do(func() {
		defaultPortManagerMutex.Lock()
		defer defaultPortManagerMutex.Unlock()

		manager, err := newDefaultPortManager(minPort, maxPort)
		if err != nil {
			initErr = err
			return
		}
		defaultPortManagerInstance = manager
	})

	if initErr != nil {
		return nil, initErr
	}

	// Check if already initialized with different parameters
	defaultPortManagerMutex.RLock()
	defer defaultPortManagerMutex.RUnlock()
	inst := defaultPortManagerInstance
	if inst == nil {
		return nil, fmt.Errorf("port manager failed to initialize previously; call InitDefaultPortManager again with valid parameters")
	}

	if inst.minPort != minPort || inst.maxPort != maxPort {
		return defaultPortManagerInstance, fmt.Errorf(
			"port manager already initialized with different range (%d-%d)",
			inst.minPort, inst.maxPort,
		)
	}

	return inst, nil
}

// NewDefaultPortManager creates a new DefaultPortManager with the given port range.
// If a singleton instance already exists, it returns that instance.
// Otherwise, it creates and initializes the singleton instance.
func NewDefaultPortManager(minPort, maxPort int) (*DefaultPortManager, error) {
	// Check if singleton already exists
	if existing := GetDefaultPortManager(); existing != nil {
		// Return the existing instance along with a warning if parameters don't match
		if existing.minPort != minPort || existing.maxPort != maxPort {
			return existing, fmt.Errorf(
				"warning: using existing port manager with different range (%d-%d) than requested (%d-%d)",
				existing.minPort, existing.maxPort, minPort, maxPort,
			)
		}
		return existing, nil
	}

	// Initialize singleton if it doesn't exist
	return InitDefaultPortManager(minPort, maxPort)
}

// newDefaultPortManager is an internal function that creates a new DefaultPortManager instance
// without using the singleton pattern. This is used by InitDefaultPortManager.
func newDefaultPortManager(minPort, maxPort int) (*DefaultPortManager, error) {
	if minPort <= 0 || maxPort <= 0 {
		return nil, fmt.Errorf("port range must be positive")
	}
	if minPort >= maxPort {
		return nil, fmt.Errorf("minPort must be less than maxPort")
	}
	if minPort < 1024 {
		return nil, fmt.Errorf("minPort must be at least 1024 (non-privileged)")
	}
	if maxPort > 65535 {
		return nil, fmt.Errorf("maxPort must be at most 65535")
	}

	return &DefaultPortManager{
		instanceToPorts: make(map[string]int),
		portToInstances: make(map[int]string),
		minPort:         minPort,
		maxPort:         maxPort,
		nextPort:        minPort,
	}, nil
}

// AllocatePort allocates the next available port for a given instance
func (pm *DefaultPortManager) AllocatePort(instanceName string) (int, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if instance already has a port
	if port, exists := pm.instanceToPorts[instanceName]; exists {
		return port, nil
	}

	// Find an available port
	startingPort := pm.nextPort
	if startingPort < pm.minPort {
		startingPort = pm.minPort
	}
	if startingPort > pm.maxPort {
		startingPort = pm.maxPort
	}

	port := startingPort

	for {
		// Check if this port is available
		if _, exists := pm.portToInstances[port]; !exists {
			// Found an available port, allocate it
			pm.instanceToPorts[instanceName] = port
			pm.portToInstances[port] = instanceName

			// Update next port for the next allocation
			pm.nextPort = port + 1
			if pm.nextPort > pm.maxPort {
				pm.nextPort = pm.minPort
			}

			return port, nil
		}

		// Try the next port
		port++
		if port > pm.maxPort {
			port = pm.minPort
		}

		// If we've checked all ports, none are available
		if port == startingPort {
			return 0, fmt.Errorf("no available ports in range %d-%d", pm.minPort, pm.maxPort)
		}
	}
}

// ReleasePort releases a port previously allocated to an instance
func (pm *DefaultPortManager) ReleasePort(instanceName string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	port, exists := pm.instanceToPorts[instanceName]
	if !exists {
		return fmt.Errorf("instance %s has no allocated port", instanceName)
	}

	// Remove the instance-to-port mapping
	delete(pm.instanceToPorts, instanceName)

	// Remove the port-to-instance mapping
	delete(pm.portToInstances, port)

	return nil
}

// GetPort retrieves the port for a given instance
func (pm *DefaultPortManager) GetPort(instanceName string) (int, bool) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	port, exists := pm.instanceToPorts[instanceName]
	return port, exists
}

// ReservePort attempts to reserve a specific port for an instance
func (pm *DefaultPortManager) ReservePort(instanceName string, port int) error {
	if port <= 0 {
		return fmt.Errorf("invalid port: %d (must be positive)", port)
	}
	if port < pm.minPort || port > pm.maxPort {
		return fmt.Errorf("port %d is outside the allowed range (%d-%d)", port, pm.minPort, pm.maxPort)
	}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if port is already in use
	if existingInstance, exists := pm.portToInstances[port]; exists {
		if existingInstance != instanceName {
			return fmt.Errorf("port %d is already in use by instance %s", port, existingInstance)
		}
		// Port is already reserved for this instance, nothing to do
		return nil
	}

	// Check if instance already has a different port
	if existingPort, exists := pm.instanceToPorts[instanceName]; exists {
		if existingPort != port {
			return fmt.Errorf("instance %s already has port %d allocated", instanceName, existingPort)
		}
		// Port is already reserved for this instance, nothing to do
		return nil
	}

	// Reserve the port
	pm.instanceToPorts[instanceName] = port
	pm.portToInstances[port] = instanceName

	return nil
}

// PreReconcile implements the PreReconcile method for DefaultPortManager
func (pm *DefaultPortManager) PreReconcile(ctx context.Context, instanceNames []string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Track any errors during allocation
	var errs []error

	// Try to allocate ports for all instances that don't have one
	for _, name := range instanceNames {
		// Skip if instance already has a port
		if _, exists := pm.instanceToPorts[name]; exists {
			continue
		}

		// Try to allocate a port
		port := pm.nextPort
		if port < pm.minPort {
			port = pm.minPort
		}
		if port > pm.maxPort {
			port = pm.maxPort
		}

		startingPort := port
		allocated := false

		// Try to find an available port
		for {
			if _, exists := pm.portToInstances[port]; !exists {
				// Found an available port, allocate it
				pm.instanceToPorts[name] = port
				pm.portToInstances[port] = name

				// Update next port for the next allocation
				pm.nextPort = port + 1
				if pm.nextPort > pm.maxPort {
					pm.nextPort = pm.minPort
				}

				allocated = true
				break
			}

			// Try the next port
			port++
			if port > pm.maxPort {
				port = pm.minPort
			}

			// If we've checked all ports, none are available
			if port == startingPort {
				errs = append(errs, fmt.Errorf("no available ports for instance %s", name))
				break
			}
		}

		if !allocated {
			errs = append(errs, fmt.Errorf("failed to allocate port for instance %s", name))
		}
	}

	if len(errs) > 0 {
		// Combine all errors into a single error message
		errMsg := "port allocation failed:"
		for _, err := range errs {
			errMsg += "\n  - " + err.Error()
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// PostReconcile implements the PostReconcile method for DefaultPortManager
func (pm *DefaultPortManager) PostReconcile(ctx context.Context) error {
	// No cleanup needed for DefaultPortManager as ports are released explicitly
	// when instances are removed via ReleasePort
	return nil
}
