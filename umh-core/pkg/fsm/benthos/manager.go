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

package benthos

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
)

const (
	baseBenthosDir = constants.S6BaseDir
)

// BenthosManager implements FSM management for Benthos services.
type BenthosManager struct {
	*public_fsm.BaseFSMManager[config.BenthosConfig]

	// portManager allocates unique ports for Benthos metrics endpoints
	portManager portmanager.PortManager
}

// BenthosManagerSnapshot extends the base ManagerSnapshot with Benthos-specific information
type BenthosManagerSnapshot struct {
	// Embed the BaseManagerSnapshot to inherit its methods
	*public_fsm.BaseManagerSnapshot
	// Add Benthos-specific fields
	PortAllocations map[string]int // Maps instance name to port
}

func NewBenthosManager(name string) *BenthosManager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentBenthosManager, name)

	baseManager := public_fsm.NewBaseFSMManager[config.BenthosConfig](
		managerName,
		baseBenthosDir,
		// Extract Benthos configs from full config
		func(fullConfig config.FullConfig) ([]config.BenthosConfig, error) {
			return fullConfig.Internal.Benthos, nil
		},
		// Get name from Benthos config
		func(cfg config.BenthosConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state from Benthos config
		func(cfg config.BenthosConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create Benthos instance from config
		func(cfg config.BenthosConfig) (public_fsm.FSMInstance, error) {
			return NewBenthosInstance(cfg), nil
		},
		// Compare Benthos configs
		func(instance public_fsm.FSMInstance, cfg config.BenthosConfig) (bool, error) {
			benthosInstance, ok := instance.(*BenthosInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a BenthosInstance")
			}
			return benthosInstance.config.Equal(cfg.BenthosServiceConfig), nil
		},
		// Set Benthos config
		func(instance public_fsm.FSMInstance, cfg config.BenthosConfig) error {
			benthosInstance, ok := instance.(*BenthosInstance)
			if !ok {
				return fmt.Errorf("instance is not a BenthosInstance")
			}
			benthosInstance.config = cfg.BenthosServiceConfig
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			benthosInstance, ok := instance.(*BenthosInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a BenthosInstance")
			}
			return benthosInstance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)
	metrics.InitErrorCounter(metrics.ComponentBenthosManager, name)

	// Initialize port manager with default range (9000-9999)
	var portManager portmanager.PortManager
	defaultPortManager, err := portmanager.NewDefaultPortManager(9000, 9999)
	if err != nil {
		// Log error but continue with a mock port manager as fallback
		logger.For(managerName).Errorf("Failed to initialize port manager: %v. Using mock port manager instead.", err)
		portManager = portmanager.NewMockPortManager()
	} else {
		portManager = defaultPortManager
	}

	return &BenthosManager{
		BaseFSMManager: baseManager,
		portManager:    portManager,
	}
}

// GetPortManager returns the port manager used by this Benthos manager
func (m *BenthosManager) GetPortManager() portmanager.PortManager {
	return m.portManager
}

// WithPortManager sets a custom port manager and returns the manager
func (m *BenthosManager) WithPortManager(portManager portmanager.PortManager) *BenthosManager {
	m.portManager = portManager
	return m
}

// AllocatePortForInstance allocates a port for a service instance if needed
func (m *BenthosManager) AllocatePortForInstance(instance public_fsm.FSMInstance) error {
	benthosInstance, ok := instance.(*BenthosInstance)
	if !ok {
		return fmt.Errorf("instance is not a BenthosInstance")
	}

	// If port is already set, nothing to do
	if benthosInstance.config.MetricsPort != 0 {
		// Try to reserve this port just to be safe
		err := m.portManager.ReservePort(benthosInstance.baseFSMInstance.GetID(), benthosInstance.config.MetricsPort)
		if err != nil {
			// Log but continue - this is best effort
			logger.For(benthosInstance.baseFSMInstance.GetID()).Warnf("Failed to reserve port %d: %v",
				benthosInstance.config.MetricsPort, err)
		}
		return nil
	}

	// Allocate a new port
	port, err := m.portManager.AllocatePort(benthosInstance.baseFSMInstance.GetID())
	if err != nil {
		return fmt.Errorf("failed to allocate port for instance %s: %w",
			benthosInstance.baseFSMInstance.GetID(), err)
	}

	// Update the instance config
	benthosInstance.config.MetricsPort = port
	return nil
}

// ReleasePortForInstance releases the port allocated to an instance
func (m *BenthosManager) ReleasePortForInstance(instanceName string) error {
	// Release the port
	return m.portManager.ReleasePort(instanceName)
}

// HandleInstanceRemoved releases the port when an instance is removed
func (m *BenthosManager) HandleInstanceRemoved(instanceName string) {
	// Release the port
	err := m.ReleasePortForInstance(instanceName)
	if err != nil {
		logger.For(logger.ComponentBenthosManager).Warnf("Failed to release port for instance %s: %v",
			instanceName, err)
	}
}

// Reconcile overrides the base manager's Reconcile method to add port management
func (m *BenthosManager) Reconcile(ctx context.Context, cfg config.FullConfig, tick uint64) (error, bool) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(logger.ComponentBenthosManager, m.BaseFSMManager.GetManagerName(), duration)
	}()
	// Phase 1: Port Management Pre-reconciliation
	benthosConfigs := cfg.Internal.Benthos
	instanceNames := make([]string, len(benthosConfigs))
	for i, cfg := range benthosConfigs {
		instanceNames[i] = cfg.Name
	}

	if err := m.portManager.PreReconcile(ctx, instanceNames); err != nil {
		return fmt.Errorf("port pre-allocation failed: %w", err), false
	}

	// Save the instance count before reconciliation to detect removals
	countBefore := len(m.BaseFSMManager.GetInstances())

	// Create a new config with allocated ports
	cfgWithPorts := cfg.Clone() // Ensure you have a proper Clone method in config.FullConfig
	for i, bc := range cfgWithPorts.Internal.Benthos {
		if port, exists := m.portManager.GetPort(bc.Name); exists {
			// Update the BenthosServiceConfig with the allocated port
			cfgWithPorts.Internal.Benthos[i].BenthosServiceConfig.MetricsPort = port
		}
	}

	// Phase 2: Base FSM Reconciliation with port-aware config
	err, reconciled := m.BaseFSMManager.Reconcile(ctx, cfgWithPorts, tick)

	// Check if instances were removed as part of reconciliation (e.g., due to permanent errors)
	countAfter := len(m.BaseFSMManager.GetInstances())
	instancesWereRemoved := countBefore > countAfter

	if err != nil {
		return fmt.Errorf("base reconciliation failed: %w", err), reconciled || instancesWereRemoved
	}

	// Phase 3: Port Management Post-reconciliation
	if err := m.portManager.PostReconcile(ctx); err != nil {
		return fmt.Errorf("port post-reconciliation failed: %w", err), reconciled || instancesWereRemoved
	}

	return nil, reconciled || instancesWereRemoved
}

// CreateSnapshot overrides the base CreateSnapshot to include Benthos-specific information
func (m *BenthosManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	// Get base snapshot from parent
	baseSnapshot := m.BaseFSMManager.CreateSnapshot()

	// We need to convert the interface to the concrete type
	baseManagerSnapshot, ok := baseSnapshot.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentBenthosManager).Errorf(
			"Failed to convert base snapshot to BaseManagerSnapshot, using generic snapshot")
		return baseSnapshot
	}

	// Create Benthos-specific snapshot
	benthosSnapshot := &BenthosManagerSnapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
		PortAllocations:     make(map[string]int),
	}

	// Add port allocations
	for name := range baseManagerSnapshot.GetInstances() {
		if port, exists := m.portManager.GetPort(name); exists {
			benthosSnapshot.PortAllocations[name] = port
		}
	}

	return benthosSnapshot
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *BenthosManagerSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}

// PortManager is now implemented in the pkg/portmanager package
