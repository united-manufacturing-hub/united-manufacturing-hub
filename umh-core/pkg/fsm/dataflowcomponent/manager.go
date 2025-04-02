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

package dataflowcomponent

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

const (
	baseDir = constants.S6BaseDir
)

// DataFlowComponentManager implements FSM management for DataFlowComponent services.
type DataFlowComponentManager struct {
	*public_fsm.BaseFSMManager[config.DataFlowComponentConfig]
}

// DataFlowComponentManagerSnapshot extends the base ManagerSnapshot with DataFlowComponent-specific information
type DataFlowComponentManagerSnapshot struct {
	// Embed the BaseManagerSnapshot to inherit its methods
	*public_fsm.BaseManagerSnapshot
}

// NewDataFlowComponentManager creates a new DataFlowComponentManager
func NewDataFlowComponentManager(name string) *DataFlowComponentManager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentDataFlowComponentManager, name)

	baseManager := public_fsm.NewBaseFSMManager[config.DataFlowComponentConfig](
		managerName,
		baseDir,
		// Extract DataFlowComponent configs from full config
		func(fullConfig config.FullConfig) ([]config.DataFlowComponentConfig, error) {
			return fullConfig.DataFlowComponents, nil
		},
		// Get name from DataFlowComponent config
		func(cfg config.DataFlowComponentConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state from DataFlowComponent config
		func(cfg config.DataFlowComponentConfig) (string, error) {
			return cfg.DesiredState, nil
		},
		// Create DataFlowComponent instance from config
		func(cfg config.DataFlowComponentConfig) (public_fsm.FSMInstance, error) {
			// Create a default BenthosConfigManager for now
			configManager := NewBenthosConfigManager("/data/config.yaml")
			// Create local DataFlowComponentConfig from config.DataFlowComponentConfig
			localCfg := DataFlowComponentConfig{
				Name:          cfg.Name,
				DesiredState:  cfg.DesiredState,
				VersionUUID:   cfg.VersionUUID,
				ServiceConfig: cfg.ServiceConfig,
			}
			return NewDataFlowComponent(localCfg, configManager), nil
		},
		// Compare DataFlowComponent configs
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) (bool, error) {
			dfcInstance, ok := instance.(*DataFlowComponent)
			if !ok {
				return false, fmt.Errorf("instance is not a DataFlowComponent")
			}
			return dfcInstance.Config.VersionUUID == cfg.VersionUUID, nil
		},
		// Set DataFlowComponent config
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) error {
			dfcInstance, ok := instance.(*DataFlowComponent)
			if !ok {
				return fmt.Errorf("instance is not a DataFlowComponent")
			}
			// Create local DataFlowComponentConfig from config.DataFlowComponentConfig
			localCfg := DataFlowComponentConfig{
				Name:          cfg.Name,
				DesiredState:  cfg.DesiredState,
				VersionUUID:   cfg.VersionUUID,
				ServiceConfig: cfg.ServiceConfig,
			}
			dfcInstance.Config = localCfg
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			dfcInstance, ok := instance.(*DataFlowComponent)
			if !ok {
				return 0, fmt.Errorf("instance is not a DataFlowComponent")
			}
			return dfcInstance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentDataFlowCompManager, name)

	return &DataFlowComponentManager{
		BaseFSMManager: baseManager,
	}
}

// Reconcile calls the base manager's Reconcile method and logs performance metrics
func (m *DataFlowComponentManager) Reconcile(ctx context.Context, cfg config.FullConfig, tick uint64) (error, bool) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(metrics.ComponentDataFlowCompManager, m.BaseFSMManager.GetManagerName(), duration)
	}()

	return m.BaseFSMManager.Reconcile(ctx, cfg, tick)
}

// CreateSnapshot creates a DataFlowComponentManagerSnapshot
func (m *DataFlowComponentManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	// Get base snapshot from parent
	baseSnapshot := m.BaseFSMManager.CreateSnapshot()

	// We need to convert the interface to the concrete type
	baseManagerSnapshot, ok := baseSnapshot.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentDataFlowComponentManager).Errorf(
			"Failed to convert base snapshot to BaseManagerSnapshot, using generic snapshot")
		return baseSnapshot
	}

	// Create DataFlowComponent-specific snapshot
	dfcSnapshot := &DataFlowComponentManagerSnapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
	}

	return dfcSnapshot
}

// IsObservedStateSnapshot implements the ObservedStateSnapshot interface
func (s *DataFlowComponentManagerSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}
