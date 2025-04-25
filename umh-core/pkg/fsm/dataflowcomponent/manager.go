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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

const (
	baseDataflowComponentDir = constants.S6BaseDir
)

// DataflowComponentManager implements the FSM management for DataflowComponent services
type DataflowComponentManager struct {
	*public_fsm.BaseFSMManager[config.DataFlowComponentConfig]
}

// DataFlowComponentSnapshot extends the base ManagerSnapshot with Dataflowcomponent specific information
type DataflowComponentSnapshot struct {
	// Embed BaseManagerSnapshot to include its methods using composition
	*public_fsm.BaseManagerSnapshot
}

func NewDataflowComponentManager(name string) *DataflowComponentManager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentDataFlowComponentManager, name)
	baseManager := public_fsm.NewBaseFSMManager[config.DataFlowComponentConfig](
		managerName,
		baseDataflowComponentDir,
		// Extract the dataflow config from fullConfig
		func(fullConfig config.FullConfig) ([]config.DataFlowComponentConfig, error) {
			return fullConfig.DataFlow, nil
		},
		// Get name for Dataflowcomponent config
		func(cfg config.DataFlowComponentConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state for Dataflowcomponent config
		func(cfg config.DataFlowComponentConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create Dataflowcomponent instance from config
		func(cfg config.DataFlowComponentConfig) (public_fsm.FSMInstance, error) {
			return NewDataflowComponentInstance(baseDataflowComponentDir, cfg), nil
		},
		// Compare Dataflowcomponent configs
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) (bool, error) {
			dataflowComponentInstance, ok := instance.(*DataflowComponentInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a DataflowComponentInstance")
			}
			return dataflowComponentInstance.config.Equal(cfg.DataFlowComponentServiceConfig), nil
		},
		// Set DataflowComponent config
		func(instance public_fsm.FSMInstance, cfg config.DataFlowComponentConfig) error {
			dataflowComponentInstance, ok := instance.(*DataflowComponentInstance)
			if !ok {
				return fmt.Errorf("instance is not a DataflowComponentInstance")
			}
			dataflowComponentInstance.config = cfg.DataFlowComponentServiceConfig
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			dataflowComponentInstance, ok := instance.(*DataflowComponentInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a DataflowComponentInstance")
			}
			return dataflowComponentInstance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)
	metrics.InitErrorCounter(metrics.ComponentDataFlowCompManager, name)
	return &DataflowComponentManager{
		BaseFSMManager: baseManager,
	}
}

// Reconcile calls the base manager's Reconcile method
// The filesystemService parameter allows for filesystem operations during reconciliation,
// enabling the method to read configuration or state information from the filesystem.
func (m *DataflowComponentManager) Reconcile(ctx context.Context, snapshot public_fsm.SystemSnapshot, services serviceregistry.Provider) (error, bool) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(logger.ComponentDataFlowComponentManager, m.GetManagerName(), duration)
	}()
	return m.BaseFSMManager.Reconcile(ctx, snapshot, services)
}

// CreateSnapshot overrides the base CreateSnapshot to include DataflowComponentManager-specific information
func (m *DataflowComponentManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	// Get base snapshot from parent
	baseSnapshot := m.BaseFSMManager.CreateSnapshot()

	// We need to convert the interface to the concrete type
	baseManagerSnapshot, ok := baseSnapshot.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentDataFlowComponentManager).Errorf(
			"Failed to convert base snapshot to BaseManagerSnapshot, using generic snapshot")
		return baseSnapshot
	}

	// Create DataflowComponentManager-specific snapshot
	snap := &DataflowComponentSnapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
	}
	return snap
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *DataflowComponentSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}
