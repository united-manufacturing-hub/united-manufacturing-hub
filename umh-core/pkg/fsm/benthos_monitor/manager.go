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

package benthos_monitor

import (
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// BenthosMonitorManager is the FSM manager for the benthos monitor instance.
type BenthosMonitorManager struct {
	*public_fsm.BaseFSMManager[config.BenthosMonitorConfig]
}

// BenthosMonitorManagerSnapshot extends the base manager snapshot to hold any benthos monitor-specific info.
type BenthosMonitorManagerSnapshot struct {
	*public_fsm.BaseManagerSnapshot
}

// Ensure it satisfies fsm.ObservedStateSnapshot.
func (b *BenthosMonitorManagerSnapshot) IsObservedStateSnapshot() {}

// NewBenthosMonitorManager constructs a manager.
func NewBenthosMonitorManager(name string) *BenthosMonitorManager {
	managerName := fmt.Sprintf("%s_%s", logger.ComponentBenthosMonitorManager, name)

	baseMgr := public_fsm.NewBaseFSMManager[config.BenthosMonitorConfig](
		managerName,
		"/dev/null", // no actual S6 base dir needed for a pure monitor
		// Extract config.FullConfig slice from FullConfig
		func(fc config.FullConfig) ([]config.BenthosMonitorConfig, error) {
			return fc.Internal.BenthosMonitor, nil
		},
		// Get name from config
		func(fc config.BenthosMonitorConfig) (string, error) {
			return fc.Name, nil
		},
		// Desired state from config
		func(fc config.BenthosMonitorConfig) (string, error) {
			return fc.DesiredFSMState, nil
		},
		// Create instance
		func(fc config.BenthosMonitorConfig) (public_fsm.FSMInstance, error) {
			inst := NewBenthosMonitorInstance(fc)

			return inst, nil
		},
		// Compare config => if same, no recreation needed
		func(instance public_fsm.FSMInstance, monitorConfig config.BenthosMonitorConfig) (bool, error) {
			benthosInstance, exists := instance.(*BenthosMonitorInstance)
			if !exists {
				return false, errors.New("instance is not a BenthosMonitorInstance")
			}
			// If same config => return true, else false
			// Minimal check:
			return benthosInstance.config.DesiredFSMState == monitorConfig.DesiredFSMState && benthosInstance.config.MetricsPort == monitorConfig.MetricsPort, nil
		},
		// Set config if only small changes
		func(instance public_fsm.FSMInstance, monitorConfig config.BenthosMonitorConfig) error {
			benthosInstance, exists := instance.(*BenthosMonitorInstance)
			if !exists {
				return errors.New("instance is not a BenthosMonitorInstance")
			}

			benthosInstance.config = monitorConfig

			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			benthosInstance, exists := instance.(*BenthosMonitorInstance)
			if !exists {
				return 0, errors.New("instance is not a BenthosMonitorInstance")
			}

			return benthosInstance.GetMinimumRequiredTime(), nil
		},
	)

	metrics.InitErrorCounter(logger.ComponentBenthosMonitorManager, name)

	return &BenthosMonitorManager{
		BaseFSMManager: baseMgr,
	}
}

// CreateSnapshot overrides the base to add agent-specific fields if desired.
func (m *BenthosMonitorManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	baseSnap := m.BaseFSMManager.CreateSnapshot()

	baseSnapshot, ok := baseSnap.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentBenthosMonitorManager).Errorf("Could not cast manager snapshot to BaseManagerSnapshot.")

		return baseSnap
	}

	snap := &BenthosMonitorManagerSnapshot{
		BaseManagerSnapshot: baseSnapshot,
	}

	return snap
}
