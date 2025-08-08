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

package bridge

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
	baseBridgeDir = constants.S6BaseDir
)

// Manager implements the FSM management for Bridge services
type Manager struct {
	*public_fsm.BaseFSMManager[config.BridgeConfig]
}

// Snapshot extends the base ManagerSnapshot with ProtocolConverter specific information
type Snapshot struct {
	// Embed BaseManagerSnapshot to include its methods using composition
	*public_fsm.BaseManagerSnapshot
}

func NewManager(name string) *Manager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentBridgeManager, name)
	baseManager := public_fsm.NewBaseFSMManager[config.BridgeConfig](
		managerName,
		baseBridgeDir,
		// Extract the bridge config from fullConfig
		func(fullConfig config.FullConfig) ([]config.BridgeConfig, error) {
			return fullConfig.Bridge, nil
		},
		// Get name for ProtocolConverter config
		func(cfg config.BridgeConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state for ProtocolConverter config
		func(cfg config.BridgeConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create ProtocolConverter instance from config
		func(cfg config.BridgeConfig) (public_fsm.FSMInstance, error) {
			// We'll pass nil for the portManager here, and the instance will get it from the services registry during reconciliation
			return NewInstance(baseBridgeDir, cfg), nil
		},
		// Compare ProtocolConverter configs
		func(instance public_fsm.FSMInstance, cfg config.BridgeConfig) (bool, error) {
			brInstance, ok := instance.(*Instance)
			if !ok {
				return false, fmt.Errorf("instance is not a Bridge Instance")
			}
			return brInstance.configSpec.Equal(cfg.ServiceConfig), nil
		},
		// Set ProtocolConverter config
		func(instance public_fsm.FSMInstance, cfg config.BridgeConfig) error {
			brInstance, ok := instance.(*Instance)
			if !ok {
				return fmt.Errorf("instance is not a Bridge Instance")
			}
			brInstance.configSpec = cfg.ServiceConfig
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			brInstance, ok := instance.(*Instance)
			if !ok {
				return 0, fmt.Errorf("instance is not a Bridge Instance")
			}
			return brInstance.GetMinimumRequiredTime(), nil
		},
	)
	metrics.InitErrorCounter(metrics.ComponentBridgeManager, name)
	return &Manager{
		BaseFSMManager: baseManager,
	}
}

// Reconcile calls the base manager's Reconcile method
// The filesystemService parameter allows for filesystem operations during reconciliation,
// enabling the method to read configuration or state information from the filesystem.
func (m *Manager) Reconcile(ctx context.Context, snapshot public_fsm.SystemSnapshot, services serviceregistry.Provider) (error, bool) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(logger.ComponentBridgeManager, m.GetManagerName(), duration)
	}()
	return m.BaseFSMManager.Reconcile(ctx, snapshot, services)
}

// CreateSnapshot overrides the base CreateSnapshot to include Manager-specific information
func (m *Manager) CreateSnapshot() public_fsm.ManagerSnapshot {
	// Get base snapshot from parent
	baseSnapshot := m.BaseFSMManager.CreateSnapshot()

	// We need to convert the interface to the concrete type
	baseManagerSnapshot, ok := baseSnapshot.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentBridgeManager).Errorf(
			"Failed to convert base snapshot to BaseManagerSnapshot, using generic snapshot")
		return baseSnapshot
	}

	// Create ProtocolConverterManager-specific snapshot
	snap := &Snapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
	}
	return snap
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *Snapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}
