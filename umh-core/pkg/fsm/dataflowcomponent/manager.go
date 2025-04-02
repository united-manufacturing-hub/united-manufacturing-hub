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

	"go.uber.org/zap"

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
	logger *zap.SugaredLogger
}

// DataFlowComponentManagerSnapshot extends the base ManagerSnapshot with DataFlowComponent-specific information
type DataFlowComponentManagerSnapshot struct {
	// Embed the BaseManagerSnapshot to inherit its methods
	*public_fsm.BaseManagerSnapshot
}

// NewDataFlowComponentManager creates a new DataFlowComponentManager
func NewDataFlowComponentManager(name string) *DataFlowComponentManager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentDataFlowComponentManager, name)
	managerLogger := logger.For(managerName)
	managerLogger.Infof("Creating new DataFlowComponentManager: %s", managerName)

	baseManager := public_fsm.NewBaseFSMManager[config.DataFlowComponentConfig](
		managerName,
		baseDir,
		// Extract DataFlowComponent configs from full config
		func(fullConfig config.FullConfig) ([]config.DataFlowComponentConfig, error) {
			managerLogger.Debugf("Extracting DataFlowComponent configs, found %d components", len(fullConfig.DataFlowComponents))
			for i, comp := range fullConfig.DataFlowComponents {
				managerLogger.Debugf("DataFlowComponent[%d]: Name=%s, DesiredState=%s, VersionUUID=%s",
					i, comp.Name, comp.DesiredState, comp.VersionUUID)
			}
			return fullConfig.DataFlowComponents, nil
		},
		// Get name from DataFlowComponent config
		func(cfg config.DataFlowComponentConfig) (string, error) {
			managerLogger.Debugf("Getting name for DataFlowComponent: %s", cfg.Name)
			return cfg.Name, nil
		},
		// Get desired state from DataFlowComponent config
		func(cfg config.DataFlowComponentConfig) (string, error) {
			managerLogger.Debugf("Getting desired state for DataFlowComponent %s: %s", cfg.Name, cfg.DesiredState)
			return cfg.DesiredState, nil
		},
		// Create DataFlowComponent instance from config
		func(cfg config.DataFlowComponentConfig) (public_fsm.FSMInstance, error) {
			managerLogger.Infof("Creating DataFlowComponent instance for: %s", cfg.Name)

			// Create a default BenthosConfigManager for now
			configFilePath := "/data/config.yaml"
			managerLogger.Debugf("Creating BenthosConfigManager with path: %s", configFilePath)
			configManager := NewBenthosConfigManager(configFilePath)

			// Create local DataFlowComponentConfig from config.DataFlowComponentConfig
			managerLogger.Debugf("Creating local config for DataFlowComponent %s", cfg.Name)
			localCfg := DataFlowComponentConfig{
				Name:          cfg.Name,
				DesiredState:  cfg.DesiredState,
				VersionUUID:   cfg.VersionUUID,
				ServiceConfig: cfg.ServiceConfig,
			}

			managerLogger.Infof("Successfully created DataFlowComponent instance for: %s", cfg.Name)
			return NewDataFlowComponent(localCfg, configManager), nil
		},
		// Compare DataFlowComponent configs
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) (bool, error) {
			dfcInstance, ok := instance.(*DataFlowComponent)
			if !ok {
				managerLogger.Errorf("instance is not a DataFlowComponent")
				return false, fmt.Errorf("instance is not a DataFlowComponent")
			}

			managerLogger.Debugf("Comparing DataFlowComponent %s versions: instance=%s, config=%s",
				cfg.Name, dfcInstance.Config.VersionUUID, cfg.VersionUUID)

			unchanged := dfcInstance.Config.VersionUUID == cfg.VersionUUID
			if unchanged {
				managerLogger.Debugf("DataFlowComponent %s is unchanged", cfg.Name)
			} else {
				managerLogger.Debugf("DataFlowComponent %s has changed and needs to be updated", cfg.Name)
			}
			return unchanged, nil
		},
		// Set DataFlowComponent config
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) error {
			dfcInstance, ok := instance.(*DataFlowComponent)
			if !ok {
				managerLogger.Errorf("instance is not a DataFlowComponent")
				return fmt.Errorf("instance is not a DataFlowComponent")
			}

			managerLogger.Debugf("Updating DataFlowComponent %s configuration", cfg.Name)

			// Create local DataFlowComponentConfig from config.DataFlowComponentConfig
			localCfg := DataFlowComponentConfig{
				Name:          cfg.Name,
				DesiredState:  cfg.DesiredState,
				VersionUUID:   cfg.VersionUUID,
				ServiceConfig: cfg.ServiceConfig,
			}
			dfcInstance.Config = localCfg

			managerLogger.Debugf("Successfully updated DataFlowComponent %s configuration", cfg.Name)
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			dfcInstance, ok := instance.(*DataFlowComponent)
			if !ok {
				managerLogger.Errorf("instance is not a DataFlowComponent")
				return 0, fmt.Errorf("instance is not a DataFlowComponent")
			}
			duration := dfcInstance.GetExpectedMaxP95ExecutionTimePerInstance()
			managerLogger.Debugf("Expected max P95 execution time for %s: %v", dfcInstance.Config.Name, duration)
			return duration, nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentDataFlowCompManager, name)
	managerLogger.Infof("DataFlowComponentManager created successfully: %s", managerName)

	return &DataFlowComponentManager{
		BaseFSMManager: baseManager,
		logger:         managerLogger,
	}
}

// Reconcile calls the base manager's Reconcile method and logs performance metrics
func (m *DataFlowComponentManager) Reconcile(ctx context.Context, cfg config.FullConfig, tick uint64) (error, bool) {
	m.logger.Debugf("Starting DataFlowComponentManager reconcile, tick: %d", tick)

	// Log number of dataflow components in the config
	m.logger.Debugf("Config contains %d DataFlowComponents", len(cfg.DataFlowComponents))

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(metrics.ComponentDataFlowCompManager, m.BaseFSMManager.GetManagerName(), duration)
		m.logger.Debugf("DataFlowComponentManager reconcile completed in %v", duration)
	}()

	err, reconciled := m.BaseFSMManager.Reconcile(ctx, cfg, tick)
	if err != nil {
		m.logger.Errorf("Error during DataFlowComponentManager reconcile: %v", err)
	} else if reconciled {
		m.logger.Debugf("DataFlowComponentManager reconciled successfully")
	} else {
		m.logger.Debugf("DataFlowComponentManager reconcile had nothing to do")
	}

	return err, reconciled
}

// CreateSnapshot creates a DataFlowComponentManagerSnapshot
func (m *DataFlowComponentManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	m.logger.Debugf("Creating DataFlowComponentManager snapshot")

	// Get base snapshot from parent
	baseSnapshot := m.BaseFSMManager.CreateSnapshot()

	// We need to convert the interface to the concrete type
	baseManagerSnapshot, ok := baseSnapshot.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		m.logger.Errorf("Failed to convert base snapshot to BaseManagerSnapshot, using generic snapshot")
		return baseSnapshot
	}

	// Create DataFlowComponent-specific snapshot
	dfcSnapshot := &DataFlowComponentManagerSnapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
	}

	m.logger.Debugf("DataFlowComponentManager snapshot created successfully")
	return dfcSnapshot
}

// IsObservedStateSnapshot implements the ObservedStateSnapshot interface
func (s *DataFlowComponentManagerSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}
