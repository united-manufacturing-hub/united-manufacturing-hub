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

package connection

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

const (
	baseConnectionDir = constants.S6BaseDir
)

// ConnectionManager implements the FSM management for Connection services
type ConnectionManager struct {
	*public_fsm.BaseFSMManager[config.ConnectionConfig]
}

// ConnectionSnapshot extends the base ManagerSnapshot with Connection specific information
type ConnectionSnapshot struct {
	// Embed BaseManagerSnapshot to include its methods using composition
	*public_fsm.BaseManagerSnapshot
}

func NewConnectionManager(name string) *ConnectionManager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentConnectionManager, name)
	baseManager := public_fsm.NewBaseFSMManager[config.ConnectionConfig](
		managerName,
		baseConnectionDir,
		// Extract the dataflow config from fullConfig
		func(fullConfig config.FullConfig) ([]config.ConnectionConfig, error) {
			return fullConfig.Internal.Connection, nil
		},
		// Get name for Connection config
		func(cfg config.ConnectionConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state for Connection config
		func(cfg config.ConnectionConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create Connection instance from config
		func(cfg config.ConnectionConfig) (public_fsm.FSMInstance, error) {
			return NewConnectionInstance(baseConnectionDir, cfg), nil
		},
		// Compare Connection configs
		func(instance public_fsm.FSMInstance, cfg config.ConnectionConfig) (bool, error) {
			connectionInstance, ok := instance.(*ConnectionInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a ConnectionInstance")
			}
			return connectionInstance.config.Equal(cfg.ConnectionServiceConfig), nil
		},
		// Set Connection config
		func(instance public_fsm.FSMInstance, cfg config.ConnectionConfig) error {
			connectionInstance, ok := instance.(*ConnectionInstance)
			if !ok {
				return fmt.Errorf("instance is not a ConnectionInstance")
			}
			connectionInstance.config = cfg.ConnectionServiceConfig
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			connectionInstance, ok := instance.(*ConnectionInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a ConnectionInstance")
			}
			return connectionInstance.GetMinimumRequiredTime(), nil
		},
	)
	metrics.InitErrorCounter(metrics.ComponentConnectionManager, name)
	return &ConnectionManager{
		BaseFSMManager: baseManager,
	}
}

// CreateSnapshot overrides the base CreateSnapshot to include ConnectionManager-specific information
func (m *ConnectionManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	// Get base snapshot from parent
	baseSnapshot := m.BaseFSMManager.CreateSnapshot()

	// We need to convert the interface to the concrete type
	baseManagerSnapshot, ok := baseSnapshot.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentConnectionManager).Errorf(
			"Failed to convert base snapshot to BaseManagerSnapshot, using generic snapshot")
		return baseSnapshot
	}

	// Create ConnectionManager-specific snapshot
	snap := &ConnectionSnapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
	}
	return snap
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *ConnectionSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}
