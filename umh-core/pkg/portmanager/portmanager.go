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
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
)

// PortManager is an interface that defines methods for managing ports.
type PortManager interface {
	// AllocatePort allocates a port for a given instance and returns it
	// Returns an error if no ports are available
	AllocatePort(ctx context.Context, instanceName string) (uint16, error)

	// ReleasePort releases a port previously allocated to an instance
	// Returns an error if the instance doesn't have a port
	ReleasePort(instanceName string) error

	// GetPort retrieves the port for a given instance
	// Returns the port and true if found, 0 and false otherwise
	GetPort(instanceName string) (uint16, bool)

	// ReservePort attempts to reserve a specific port for an instance
	// Returns an error if the port is already in use
	ReservePort(ctx context.Context, instanceName string, port uint16) error

	// PreReconcile is called before the base FSM reconciliation to ensure ports are allocated
	// It takes a list of instance names that should have ports allocated
	// Returns an error if port allocation fails
	PreReconcile(ctx context.Context, instanceNames []string) error

	// PostReconcile is called after the base FSM reconciliation to clean up any orphaned ports
	// It releases ports for instances that no longer exist
	PostReconcile(ctx context.Context) error
}

// DefaultPortManager is a thread-safe implementation of PortManager
// that randomly selects ports from a fixed service port range (20000-32767)
// to avoid conflicts with the OS ephemeral ports.
type DefaultPortManager struct {

	// instanceToPorts maps instance names to their allocated ports
	instanceToPorts map[string]uint16

	// portToInstances maps ports to instance names for reverse lookup
	portToInstances map[uint16]string

	// allocatedPorts tracks all ports we've allocated to avoid duplicates
	allocatedPorts map[uint16]bool

	// mutex to protect concurrent access to maps
	mutex sync.RWMutex

	// ephemeral port range for random selection
	minPort uint16
	maxPort uint16
}

// Global singleton instance of DefaultPortManager.
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

// initDefaultPortManager initializes the singleton DefaultPortManager.
// It ensures the DefaultPortManager is initialized only once.
func initDefaultPortManager() *DefaultPortManager {
	defaultPortManagerOnce.Do(func() {
		defaultPortManagerMutex.Lock()
		defer defaultPortManagerMutex.Unlock()

		manager := newDefaultPortManager()
		defaultPortManagerInstance = manager
	})

	// Get the initialized instance
	defaultPortManagerMutex.RLock()
	defer defaultPortManagerMutex.RUnlock()

	return defaultPortManagerInstance
}

// NewDefaultPortManager creates a new DefaultPortManager using OS port allocation.
// If a singleton instance already exists, it returns that instance.
// Otherwise, it creates and initializes the singleton instance.
func NewDefaultPortManager() (*DefaultPortManager, error) {
	// Check if singleton already exists
	if existing := GetDefaultPortManager(); existing != nil {
		return existing, nil
	}

	// Initialize singleton if it doesn't exist
	instance := initDefaultPortManager()
	if instance == nil {
		return nil, errors.New("failed to initialize port manager")
	}

	return instance, nil
}

// getServicePortRange returns the port range to use for service allocation.
// We use a custom range (20000-32767) instead of the OS ephemeral range to avoid
// conflicts with outgoing connections that the kernel assigns ports to.
func getServicePortRange() (uint16, uint16) {
	// IMPORTANT: We intentionally use a different range than the OS ephemeral ports
	// to avoid conflicts with outgoing connections that the kernel assigns ports to.
	// Using range 20000-32767 which is:
	// - Above well-known ports (0-1023) and registered ports (1024-19999)
	// - Below the typical OS ephemeral range (32768-65535)
	// - Gives us ~12,000 ports to work with
	// This prevents the race condition where the kernel "steals" our allocated ports
	// for outgoing TCP connections between allocation and service startup.
	return 20000, 32767
}

// newDefaultPortManager is an internal function that creates a new DefaultPortManager instance
// without using the singleton pattern. This is used by initDefaultPortManager.
func newDefaultPortManager() *DefaultPortManager {
	minPort, maxPort := getServicePortRange()

	return &DefaultPortManager{
		instanceToPorts: make(map[string]uint16),
		portToInstances: make(map[uint16]string),
		allocatedPorts:  make(map[uint16]bool),
		minPort:         minPort,
		maxPort:         maxPort,
	}
}

// AllocatePort allocates an available port for a given instance using random selection
// from the configured service port range with collision detection and retries.
func (pm *DefaultPortManager) AllocatePort(ctx context.Context, instanceName string) (uint16, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if instance already has a port
	if port, exists := pm.instanceToPorts[instanceName]; exists {
		return port, nil
	}

	// Try up to 5 times to find an available port
	const maxRetries = 5

	lc := &net.ListenConfig{}

	for range maxRetries {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("port allocation cancelled: %w", ctx.Err())
		default:
		}

		// Generate a random port in the ephemeral range
		portRange := pm.maxPort - pm.minPort + 1
		randomOffset := rand.IntN(int(portRange))
		port := pm.minPort + uint16(randomOffset)

		// Skip if we've already allocated this port
		if pm.allocatedPorts[port] {
			continue
		}

		// Try to bind to the port to verify it's available
		addr := fmt.Sprintf(":%d", port)

		listener, err := lc.Listen(ctx, "tcp", addr)
		if err != nil {
			// Port not available, try another one
			continue
		}

		// Close the listener immediately
		_ = listener.Close() // Ignore close errors since we've verified port availability

		// Successfully allocated the port, store the mappings
		pm.instanceToPorts[instanceName] = port
		pm.portToInstances[port] = instanceName
		pm.allocatedPorts[port] = true

		return port, nil
	}

	return 0, fmt.Errorf("failed to allocate port for instance %s after %d attempts", instanceName, maxRetries)
}

// ReleasePort releases a port previously allocated to an instance.
func (pm *DefaultPortManager) ReleasePort(instanceName string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	port, exists := pm.instanceToPorts[instanceName]
	if !exists {
		return fmt.Errorf("instance %s has no allocated port", instanceName)
	}

	// Remove the mappings
	delete(pm.instanceToPorts, instanceName)
	delete(pm.portToInstances, port)
	delete(pm.allocatedPorts, port)

	return nil
}

// GetPort retrieves the port for a given instance.
func (pm *DefaultPortManager) GetPort(instanceName string) (uint16, bool) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	port, exists := pm.instanceToPorts[instanceName]

	return port, exists
}

// ReservePort attempts to reserve a specific port for an instance.
func (pm *DefaultPortManager) ReservePort(ctx context.Context, instanceName string, port uint16) error {
	// Validate against the manager's allowed range
	if port < pm.minPort || port > pm.maxPort {
		return fmt.Errorf(
			"invalid port %d: allowed range is %d-%d; choose a port in range or let the manager allocate one",
			port, pm.minPort, pm.maxPort,
		)
	}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if port is already in use by our port manager
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

	// Try to bind to the specific port to verify it's available
	addr := fmt.Sprintf(":%d", port)
	lc := &net.ListenConfig{}

	listener, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", port, err)
	}

	// Close the listener immediately to allow external apps to use the port
	if err := listener.Close(); err != nil {
		return fmt.Errorf("failed to close listener for port %d: %w", port, err)
	}

	// Successfully reserved the port
	pm.instanceToPorts[instanceName] = port
	pm.portToInstances[port] = instanceName
	pm.allocatedPorts[port] = true

	return nil
}

// PreReconcile implements the PreReconcile method for DefaultPortManager.
func (pm *DefaultPortManager) PreReconcile(ctx context.Context, instanceNames []string) error {
	// Track any errors during allocation
	var errs []error

	// Try to allocate ports for all instances that don't have one
	for _, name := range instanceNames {
		// Skip if instance already has a port
		pm.mutex.RLock()
		_, exists := pm.instanceToPorts[name]
		pm.mutex.RUnlock()

		if exists {
			continue
		}

		// Allocate a port using our standard allocation method
		// This will handle the locking internally
		_, err := pm.AllocatePort(ctx, name)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to allocate port for instance %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("port allocation failed: %w", errors.Join(errs...))
	}

	return nil
}

// PostReconcile implements the PostReconcile method for DefaultPortManager.
func (pm *DefaultPortManager) PostReconcile(ctx context.Context) error {
	// No cleanup needed for DefaultPortManager as ports are released explicitly
	// when instances are removed via ReleasePort
	return nil
}

// ResetDefaultPortManager resets the singleton instance for testing purposes.
// Do not call concurrently with New/Get/Allocate/Reserve; use only in tests
// when no goroutines are interacting with the manager.
func ResetDefaultPortManager() {
	defaultPortManagerMutex.Lock()
	defer defaultPortManagerMutex.Unlock()

	defaultPortManagerInstance = nil
	defaultPortManagerOnce = sync.Once{}
}
