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
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/portmanager"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

const (
	baseBenthosDir = constants.S6BaseDir
)

// BenthosManager implements FSM management for Benthos services.
type BenthosManager struct {
	*public_fsm.BaseFSMManager[config.BenthosConfig]
}

// BenthosManagerSnapshot extends the base ManagerSnapshot with Benthos-specific information.
type BenthosManagerSnapshot struct {
	// Embed the BaseManagerSnapshot to inherit its methods
	*public_fsm.BaseManagerSnapshot
	// Add Benthos-specific fields
	PortAllocations map[string]uint16 // Maps instance name to port
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
			if err := config.ValidateComponentName(cfg.Name); err != nil {
				return "", fmt.Errorf("invalid benthos service name: %w", err)
			}

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
				return false, errors.New("instance is not a BenthosInstance")
			}

			return benthosInstance.config.Equal(cfg.BenthosServiceConfig), nil
		},
		// Set Benthos config
		func(instance public_fsm.FSMInstance, cfg config.BenthosConfig) error {
			benthosInstance, ok := instance.(*BenthosInstance)
			if !ok {
				return errors.New("instance is not a BenthosInstance")
			}

			benthosInstance.config = cfg.BenthosServiceConfig

			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			benthosInstance, ok := instance.(*BenthosInstance)
			if !ok {
				return 0, errors.New("instance is not a BenthosInstance")
			}

			return benthosInstance.GetMinimumRequiredTime(), nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentBenthosManager, name)

	return &BenthosManager{
		BaseFSMManager: baseManager,
	}
}

// AllocatePortForInstance allocates a port for a service instance if needed.
func (m *BenthosManager) AllocatePortForInstance(ctx context.Context, instance public_fsm.FSMInstance, portManager portmanager.PortManager) error {
	benthosInstance, ok := instance.(*BenthosInstance)
	if !ok {
		return errors.New("instance is not a BenthosInstance")
	}

	// If port is already set, nothing to do
	if benthosInstance.config.MetricsPort != 0 {
		// Try to reserve this port just to be safe
		err := portManager.ReservePort(ctx, benthosInstance.baseFSMInstance.GetID(), benthosInstance.config.MetricsPort)
		if err != nil {
			// Log but continue - this is best effort
			logger.For(benthosInstance.baseFSMInstance.GetID()).Warnf("Failed to reserve port %d: %v",
				benthosInstance.config.MetricsPort, err)
		}

		return nil
	}

	// Allocate a new port
	port, err := portManager.AllocatePort(ctx, benthosInstance.baseFSMInstance.GetID())
	if err != nil {
		return fmt.Errorf("failed to allocate port for instance %s: %w",
			benthosInstance.baseFSMInstance.GetID(), err)
	}

	// Update the instance config
	benthosInstance.config.MetricsPort = port

	return nil
}

// ReleasePortForInstance releases the port allocated to an instance.
func (m *BenthosManager) ReleasePortForInstance(instanceName string, portManager portmanager.PortManager) error {
	// Release the port
	return portManager.ReleasePort(instanceName)
}

// HandleInstanceRemoved releases the port when an instance is removed.
func (m *BenthosManager) HandleInstanceRemoved(instanceName string, portManager portmanager.PortManager) {
	// Release the port
	err := m.ReleasePortForInstance(instanceName, portManager)
	if err != nil {
		logger.For(logger.ComponentBenthosManager).Warnf("Failed to release port for instance %s: %v",
			instanceName, err)
	}
}

// Reconcile overrides the base manager's Reconcile method to add port management.
func (m *BenthosManager) Reconcile(ctx context.Context, snapshot public_fsm.SystemSnapshot, services serviceregistry.Provider) (error, bool) {
	start := time.Now()

	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(logger.ComponentBenthosManager, m.GetManagerName(), duration)
	}()

	// Get port manager from service registry
	portManager := services.GetPortManager()
	if portManager == nil {
		return errors.New("port manager not available in service registry"), false
	}

	// Phase 1: Port Management Pre-reconciliation
	benthosConfigs := snapshot.CurrentConfig.Internal.Benthos

	instanceNames := make([]string, len(benthosConfigs))
	for i, cfg := range benthosConfigs {
		instanceNames[i] = cfg.Name
	}

	if err := portManager.PreReconcile(ctx, instanceNames); err != nil {
		return fmt.Errorf("port pre-allocation failed: %w", err), false
	}

	// Save the instance count before reconciliation to detect removals
	countBefore := len(m.GetInstances())

	// Create a new config based on the current config with allocated ports
	cfgWithPorts := snapshot.CurrentConfig.Clone()
	for i, bc := range cfgWithPorts.Internal.Benthos {
		if port, exists := portManager.GetPort(bc.Name); exists {
			// Update the BenthosServiceConfig with the allocated port
			cfgWithPorts.Internal.Benthos[i].BenthosServiceConfig.MetricsPort = port
		}
	}

	snapshotWithPorts := snapshot // <- inexpensive value copy
	snapshotWithPorts.CurrentConfig = cfgWithPorts

	// Phase 2: Base FSM Reconciliation with port-aware config
	err, reconciled := m.BaseFSMManager.Reconcile(ctx, snapshotWithPorts, services)

	// Check if instances were removed as part of reconciliation (e.g., due to permanent errors)
	countAfter := len(m.GetInstances())
	instancesWereRemoved := countBefore > countAfter

	if err != nil {
		return fmt.Errorf("base reconciliation failed: %w", err), reconciled || instancesWereRemoved
	}

	// Phase 3: Port Management Post-reconciliation
	if err := portManager.PostReconcile(ctx); err != nil {
		return fmt.Errorf("port post-reconciliation failed: %w", err), reconciled || instancesWereRemoved
	}

	return nil, reconciled || instancesWereRemoved
}

// CreateSnapshot overrides the base CreateSnapshot to include Benthos-specific information.
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

	// Create Benthos-specific snapshot with empty port allocations
	// Port allocations will be populated in the ReconcilePortAllocations method
	benthosSnapshot := &BenthosManagerSnapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
		PortAllocations:     make(map[string]uint16),
	}
	if pm := serviceregistry.GetGlobalRegistry().GetPortManager(); pm != nil {
		m.ReconcilePortAllocations(benthosSnapshot, pm)
	}

	return benthosSnapshot
}

// ReconcilePortAllocations updates the port allocations in the snapshot
// This should be called after CreateSnapshot to populate the port allocations.
func (m *BenthosManager) ReconcilePortAllocations(snapshot *BenthosManagerSnapshot, portManager portmanager.PortManager) {
	if snapshot == nil || portManager == nil {
		return
	}

	// Add port allocations
	for name := range snapshot.GetInstances() {
		if port, exists := portManager.GetPort(name); exists {
			snapshot.PortAllocations[name] = port
		}
	}
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface.
func (s *BenthosManagerSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}
