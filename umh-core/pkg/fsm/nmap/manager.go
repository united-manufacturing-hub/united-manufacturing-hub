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

package nmap

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
	baseNmapDir = constants.S6BaseDir
)

// NmapManager is the FSM manager for multiple nmap monitor instances
type NmapManager struct {
	*public_fsm.BaseFSMManager[config.NmapConfig]
}

// NmapManagerSnapshot extends the base manager snapshot to hold any nmap-specific info
type NmapManagerSnapshot struct {
	*public_fsm.BaseManagerSnapshot
}

// Ensure it satisfies fsm.ObservedStateSnapshot
func (n *NmapManagerSnapshot) IsObservedStateSnapshot() {}

// NewNmapManager constructs a manager.
// You might keep `sharedMonitorService` if you want one global service instance.
func NewNmapManager(name string) *NmapManager {
	managerName := fmt.Sprintf("%s_%s", logger.ComponentNmapManager, name)

	baseMgr := public_fsm.NewBaseFSMManager[config.NmapConfig](
		managerName,
		baseNmapDir,
		// Extract NmapConfig slice from FullConfig
		func(fc config.FullConfig) ([]config.NmapConfig, error) {
			return fc.Internal.Nmap, nil
		},
		// Get name from config
		func(cfg config.NmapConfig) (string, error) {
			return cfg.Name, nil
		},
		// Desired state from config
		func(cfg config.NmapConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create instance
		func(cfg config.NmapConfig) (public_fsm.FSMInstance, error) {
			inst := NewNmapInstance(cfg)
			return inst, nil
		},
		// Compare config => if same, no recreation needed
		func(instance public_fsm.FSMInstance, cfg config.NmapConfig) (bool, error) {
			ni, ok := instance.(*NmapInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a NmapInstance")
			}
			// If same config => return true, else false
			return ni.config.NmapServiceConfig.Equal(cfg.NmapServiceConfig), nil
		},
		// Set config if only small changes
		func(instance public_fsm.FSMInstance, cfg config.NmapConfig) error {
			ni, ok := instance.(*NmapInstance)
			if !ok {
				return fmt.Errorf("instance is not a NmapInstance")
			}
			ni.config = cfg
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			ni, ok := instance.(*NmapInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a NmapInstance")
			}
			return ni.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)
	metrics.InitErrorCounter(metrics.ComponentNmapManager, name)

	return &NmapManager{
		BaseFSMManager: baseMgr,
	}
}

// CreateSnapshot overrides the base to add nmap-specific fields if desired
func (m *NmapManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	baseSnap := m.BaseFSMManager.CreateSnapshot()
	baseSnapshot, ok := baseSnap.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentNmapManager).Errorf("Could not cast nmap snapshot to BaseManagerSnapshot.")
		return baseSnap
	}
	snap := &NmapManagerSnapshot{
		BaseManagerSnapshot: baseSnapshot,
	}
	return snap
}
