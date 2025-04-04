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
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

const (
	baseDir = constants.S6BaseDir
)

// DataFlowComponentManager implements FSM management for DataFlowComponent services.
type DataFlowComponentManager struct {
	*public_fsm.BaseFSMManager[config.DataFlowComponentConfig]
	logger         *zap.SugaredLogger
	benthosManager *benthos.BenthosManager
	mutex          sync.Mutex
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
	managerLogger.Debugf("Creating new DataFlowComponentManager: %s", managerName)

	// Create a benthos manager that will handle the actual benthos services
	benthosManager := benthos.NewBenthosManager(name)
	managerLogger.Debugf("Created BenthosManager for DataFlowComponentManager: %s", name)

	baseManager := public_fsm.NewBaseFSMManager[config.DataFlowComponentConfig](
		managerName,
		baseDir,
		// Extract DataFlowComponent configs from full config
		func(fullConfig config.FullConfig) ([]config.DataFlowComponentConfig, error) {
			managerLogger.Debugf("Extracting DataFlowComponent configs, found %d components", len(fullConfig.DataFlowComponents))
			for i, comp := range fullConfig.DataFlowComponents {
				managerLogger.Debugf("DataFlowComponent[%d]: Name=%s, DesiredState=%s",
					i, comp.Name, comp.DesiredState)
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

			// Create local DataFlowComponentConfig from config.DataFlowComponentConfig
			managerLogger.Debugf("Creating local config for DataFlowComponent %s", cfg.Name)
			localCfg := DataFlowComponentConfig{
				Name:          cfg.Name,
				DesiredState:  cfg.DesiredState,
				ServiceConfig: cfg.ServiceConfig,
			}

			managerLogger.Infof("Successfully created DataFlowComponent instance for: %s", cfg.Name)
			// Create a new DataFlowComponent instance with the BenthosConfigManager interface impl below
			return NewDataFlowComponent(localCfg, &benthosMgrAdapter{
				manager:    benthosManager,
				logger:     managerLogger.Named("BenthosAdapter"),
				components: make(map[string]DataFlowComponentConfig),
			}), nil
		},
		// Compare DataFlowComponent configs
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) (bool, error) {
			dfcInstance, ok := instance.(*DataFlowComponent)
			if !ok {
				managerLogger.Errorf("instance is not a DataFlowComponent")
				return false, fmt.Errorf("instance is not a DataFlowComponent")
			}

			unchanged := reflect.DeepEqual(dfcInstance.Config, cfg)
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
	managerLogger.Debugf("DataFlowComponentManager created successfully: %s", managerName)

	return &DataFlowComponentManager{
		BaseFSMManager: baseManager,
		logger:         managerLogger,
		benthosManager: benthosManager,
		mutex:          sync.Mutex{},
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

	// First run the base reconcile to update our DataFlowComponents
	err, reconciled := m.BaseFSMManager.Reconcile(ctx, cfg, tick)
	if err != nil {
		m.logger.Errorf("Error during DataFlowComponentManager reconcile: %v", err)
		return err, reconciled
	}

	// Then, generate the Benthos configs from our DataFlowComponents
	benthosConfigs := m.generateBenthosConfigs()

	// Create a new config object to pass to the BenthosManager
	cfgForBenthos := cfg.Clone()
	cfgForBenthos.Benthos = benthosConfigs

	// Pass the modified config to the BenthosManager for reconciliation
	err, benthosReconciled := m.benthosManager.Reconcile(ctx, cfgForBenthos, tick)
	if err != nil {
		m.logger.Errorf("Error during BenthosManager reconcile: %v", err)
		return err, reconciled
	}

	return nil, reconciled || benthosReconciled
}

// generateBenthosConfigs creates Benthos configs from DataFlowComponent instances
func (m *DataFlowComponentManager) generateBenthosConfigs() []config.BenthosConfig {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instances := m.BaseFSMManager.GetInstances()
	benthosConfigs := make([]config.BenthosConfig, 0, len(instances))

	for _, instance := range instances {
		dfcInstance, ok := instance.(*DataFlowComponent)
		if !ok {
			m.logger.Warnf("Instance is not a DataFlowComponent, skipping: %v", instance)
			continue
		}

		// Only generate Benthos configs for active components
		if dfcInstance.GetCurrentFSMState() == OperationalStateActive ||
			dfcInstance.GetCurrentFSMState() == OperationalStateStarting ||
			dfcInstance.GetCurrentFSMState() == OperationalStateDegraded {
			benthosConfigs = append(benthosConfigs, config.BenthosConfig{
				FSMInstanceConfig: config.FSMInstanceConfig{
					Name:            dfcInstance.Config.Name,
					DesiredFSMState: dfcInstance.Config.DesiredState,
				},
				BenthosServiceConfig: dfcInstance.Config.ServiceConfig,
			})
		}
	}

	m.logger.Debugf("Generated %d Benthos configs from DataFlowComponent instances", len(benthosConfigs))
	return benthosConfigs
}

// GetBenthosManager returns the benthos manager used by this DataFlowComponentManager
func (m *DataFlowComponentManager) GetBenthosManager() *benthos.BenthosManager {
	return m.benthosManager
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

// benthosMgrAdapter adapts the BenthosManager to the BenthosConfigManager interface
type benthosMgrAdapter struct {
	manager    *benthos.BenthosManager
	logger     *zap.SugaredLogger
	mutex      sync.Mutex
	components map[string]DataFlowComponentConfig
}

// AddComponentToBenthosConfig adds a component to the benthos config
func (a *benthosMgrAdapter) AddComponentToBenthosConfig(ctx context.Context, component DataFlowComponentConfig) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.logger.Debugf("Adding component %s to Benthos config tracking", component.Name)
	a.components[component.Name] = component
	return nil
}

// RemoveComponentFromBenthosConfig removes a component from the benthos config
func (a *benthosMgrAdapter) RemoveComponentFromBenthosConfig(ctx context.Context, componentName string) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.logger.Debugf("Removing component %s from Benthos config tracking", componentName)
	delete(a.components, componentName)
	return nil
}

// UpdateComponentInBenthosConfig updates a component in the benthos config
func (a *benthosMgrAdapter) UpdateComponentInBenthosConfig(ctx context.Context, component DataFlowComponentConfig) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.logger.Debugf("Updating component %s in Benthos config tracking", component.Name)
	a.components[component.Name] = component
	return nil
}

// ComponentExistsInBenthosConfig checks if a component exists in the benthos config
func (a *benthosMgrAdapter) ComponentExistsInBenthosConfig(ctx context.Context, componentName string) (bool, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	_, exists := a.components[componentName]
	return exists, nil
}
