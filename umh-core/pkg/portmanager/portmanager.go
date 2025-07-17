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
	"net"
	"sync"
)

// PortManager is an interface that defines methods for managing ports
type PortManager interface {
	// AllocatePort allocates a port for a given instance and returns it
	// Returns an error if no ports are available
	AllocatePort(instanceName string) (uint16, error)

	// ReleasePort releases a port previously allocated to an instance
	// Returns an error if the instance doesn't have a port
	ReleasePort(instanceName string) error

	// GetPort retrieves the port for a given instance
	// Returns the port and true if found, 0 and false otherwise
	GetPort(instanceName string) (uint16, bool)

	// ReservePort attempts to reserve a specific port for an instance
	// Returns an error if the port is already in use
	ReservePort(instanceName string, port uint16) error

	// PreReconcile is called before the base FSM reconciliation to ensure ports are allocated
	// It takes a list of instance names that should have ports allocated
	// Returns an error if port allocation fails
	PreReconcile(ctx context.Context, instanceNames []string) error

	// PostReconcile is called after the base FSM reconciliation to clean up any orphaned ports
	// It releases ports for instances that no longer exist
	PostReconcile(ctx context.Context) error
}

// DefaultPortManager is a thread-safe implementation of PortManager
// that uses OS-assigned ports
type DefaultPortManager struct {
	// mutex to protect concurrent access to maps
	mutex sync.RWMutex

	// instanceToPorts maps instance names to their allocated ports
	instanceToPorts map[string]uint16
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

// NewDefaultPortManager creates a new DefaultPortManager.
// If a singleton instance already exists, it returns that instance.
// Otherwise, it creates and initializes the singleton instance.
func NewDefaultPortManager() *DefaultPortManager {
	defaultPortManagerOnce.Do(func() {
		defaultPortManagerMutex.Lock()
		defer defaultPortManagerMutex.Unlock()
		defaultPortManagerInstance = &DefaultPortManager{
			instanceToPorts: make(map[string]uint16),
		}
	})

	defaultPortManagerMutex.RLock()
	defer defaultPortManagerMutex.RUnlock()
	return defaultPortManagerInstance
}

// AllocatePort allocates a port by asking the OS for an available one
func (pm *DefaultPortManager) AllocatePort(instanceName string) (uint16, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if instance already has a port
	if port, exists := pm.instanceToPorts[instanceName]; exists {
		return port, nil
	}

	// Let the OS assign us a port by binding to port 0
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to get port from OS: %w", err)
	}
	defer listener.Close()

	// Extract the port from the listener's address
	addr := listener.Addr().(*net.TCPAddr)
	port := uint16(addr.Port)

	// Store the allocated port
	pm.instanceToPorts[instanceName] = port

	return port, nil
}

// ReleasePort releases a port previously allocated to an instance
func (pm *DefaultPortManager) ReleasePort(instanceName string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	_, exists := pm.instanceToPorts[instanceName]
	if !exists {
		return fmt.Errorf("instance %s has no allocated port", instanceName)
	}

	// Remove the instance-to-port mapping
	delete(pm.instanceToPorts, instanceName)

	return nil
}

// GetPort retrieves the port for a given instance
func (pm *DefaultPortManager) GetPort(instanceName string) (uint16, bool) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	port, exists := pm.instanceToPorts[instanceName]
	return port, exists
}

// ReservePort attempts to reserve a specific port for an instance
func (pm *DefaultPortManager) ReservePort(instanceName string, port uint16) error {
	if port <= 0 {
		return fmt.Errorf("invalid port: %d (must be positive)", port)
	}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if instance already has a port
	if existingPort, exists := pm.instanceToPorts[instanceName]; exists {
		if existingPort != port {
			return fmt.Errorf("instance %s already has port %d allocated", instanceName, existingPort)
		}
		// Port is already reserved for this instance, nothing to do
		return nil
	}

	// Try to bind to the specific port to check if it's available
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", port, err)
	}
	listener.Close()

	// Reserve the port
	pm.instanceToPorts[instanceName] = port

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

		// Let the OS assign us a port by binding to port 0
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to allocate port for instance %s: %w", name, err))
			continue
		}

		// Extract the port from the listener's address
		addr := listener.Addr().(*net.TCPAddr)
		port := uint16(addr.Port)
		listener.Close()

		// Store the allocated port
		pm.instanceToPorts[name] = port
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

// ResetDefaultPortManager resets the singleton instance for testing purposes
func ResetDefaultPortManager() {
	defaultPortManagerMutex.Lock()
	defer defaultPortManagerMutex.Unlock()
	defaultPortManagerInstance = nil
	defaultPortManagerOnce = sync.Once{}
}
