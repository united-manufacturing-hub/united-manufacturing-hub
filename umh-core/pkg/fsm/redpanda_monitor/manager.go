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

package redpanda_monitor

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// RedpandaMonitorManager is the FSM manager for the redpanda monitor instance
type RedpandaMonitorManager struct {
	*public_fsm.BaseFSMManager[config.RedpandaMonitorConfig]
}

// RedpandaMonitorManagerSnapshot extends the base manager snapshot to hold any redpanda monitor-specific info
type RedpandaMonitorManagerSnapshot struct {
	*public_fsm.BaseManagerSnapshot
}

// Ensure it satisfies fsm.ObservedStateSnapshot
func (b *RedpandaMonitorManagerSnapshot) IsObservedStateSnapshot() {}

// NewRedpandaMonitorManager constructs a manager.
func NewRedpandaMonitorManager() *RedpandaMonitorManager {
	managerName := fmt.Sprintf("%s_%s", logger.ComponentRedpandaMonitorManager, constants.RedpandaMonitorServiceName)

	baseMgr := public_fsm.NewBaseFSMManager[config.RedpandaMonitorConfig](
		managerName,
		"/dev/null", // no actual S6 base dir needed for a pure monitor
		// Extract config.FullConfig slice from FullConfig
		func(fc config.FullConfig) ([]config.RedpandaMonitorConfig, error) {
			// There can only be one redpanda monitor config
			if fc.Internal.RedpandaMonitor == nil {
				return nil, nil
			}
			return []config.RedpandaMonitorConfig{*fc.Internal.RedpandaMonitor}, nil
		},
		// Get name from config
		func(fc config.RedpandaMonitorConfig) (string, error) {
			return fc.Name, nil
		},
		// Desired state from config
		func(fc config.RedpandaMonitorConfig) (string, error) {
			return fc.DesiredFSMState, nil
		},
		// Create instance
		func(fc config.RedpandaMonitorConfig) (public_fsm.FSMInstance, error) {
			inst := NewRedpandaMonitorInstance(fc)
			return inst, nil
		},
		// Compare config => if same, no recreation needed
		func(instance public_fsm.FSMInstance, fc config.RedpandaMonitorConfig) (bool, error) {
			bi, ok := instance.(*RedpandaMonitorInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a RedpandaMonitorInstance")
			}
			// If same config => return true, else false
			// Minimal check:
			return bi.config.DesiredFSMState == fc.DesiredFSMState, nil
		},
		// Set config if only small changes
		func(instance public_fsm.FSMInstance, fc config.RedpandaMonitorConfig) error {
			bi, ok := instance.(*RedpandaMonitorInstance)
			if !ok {
				return fmt.Errorf("instance is not a RedpandaMonitorInstance")
			}
			bi.config = fc
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			bi, ok := instance.(*RedpandaMonitorInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a RedpandaMonitorInstance")
			}
			return bi.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)
	metrics.InitErrorCounter(logger.ComponentRedpandaMonitorManager, constants.RedpandaMonitorServiceName)

	return &RedpandaMonitorManager{
		BaseFSMManager: baseMgr,
	}
}

// CreateSnapshot overrides the base to add agent-specific fields if desired
func (m *RedpandaMonitorManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	baseSnap := m.BaseFSMManager.CreateSnapshot()
	baseSnapshot, ok := baseSnap.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentRedpandaMonitorManager).Errorf("Could not cast manager snapshot to BaseManagerSnapshot.")
		return baseSnap
	}
	snap := &RedpandaMonitorManagerSnapshot{
		BaseManagerSnapshot: baseSnapshot,
	}
	return snap
}
