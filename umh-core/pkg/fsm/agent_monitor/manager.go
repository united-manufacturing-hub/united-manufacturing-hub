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

package agent_monitor

import (
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// AgentManager is the FSM manager for the agent monitor instance.
type AgentManager struct {
	*public_fsm.BaseFSMManager[config.AgentMonitorConfig]
}

// AgentManagerSnapshot extends the base manager snapshot to hold any agent-specific info.
type AgentManagerSnapshot struct {
	*public_fsm.BaseManagerSnapshot
}

// Ensure it satisfies fsm.ObservedStateSnapshot.
func (a *AgentManagerSnapshot) IsObservedStateSnapshot() {}

// NewAgentManager constructs a manager.
func NewAgentManager(name string) *AgentManager {
	managerName := fmt.Sprintf("%s_%s", logger.AgentManagerComponentName, name)

	baseMgr := public_fsm.NewBaseFSMManager[config.AgentMonitorConfig](
		managerName,
		"/dev/null", // no actual S6 base dir needed for a pure monitor
		// Extract config.FullConfig slice from FullConfig
		func(fc config.FullConfig) ([]config.AgentMonitorConfig, error) {
			// Always return a single config
			var configs []config.AgentMonitorConfig

			configs = append(configs, config.AgentMonitorConfig{
				Name:            constants.DefaultInstanceName,
				DesiredFSMState: OperationalStateActive,
			})

			return configs, nil
		},
		// Get name from config
		func(fc config.AgentMonitorConfig) (string, error) {
			return logger.AgentInstanceComponentName, nil
		},
		// Desired state from config
		func(fc config.AgentMonitorConfig) (string, error) {
			return fc.DesiredFSMState, nil
		},
		// Create instance
		func(fc config.AgentMonitorConfig) (public_fsm.FSMInstance, error) {
			inst := NewAgentInstance(fc)

			return inst, nil
		},
		// Compare config => if same, no recreation needed
		func(instance public_fsm.FSMInstance, fc config.AgentMonitorConfig) (bool, error) {
			ai, ok := instance.(*AgentInstance)
			if !ok {
				return false, errors.New("instance is not an AgentInstance")
			}
			// If same config => return true, else false
			// Minimal check:
			return ai.config.DesiredFSMState == fc.DesiredFSMState, nil
		},
		// Set config if only small changes
		func(instance public_fsm.FSMInstance, fc config.AgentMonitorConfig) error {
			ai, ok := instance.(*AgentInstance)
			if !ok {
				return errors.New("instance is not an AgentInstance")
			}

			ai.config = fc

			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			ai, ok := instance.(*AgentInstance)
			if !ok {
				return 0, errors.New("instance is not an AgentInstance")
			}

			return ai.GetMinimumRequiredTime(), nil
		},
	)

	metrics.InitErrorCounter(logger.AgentManagerComponentName, name)

	return &AgentManager{
		BaseFSMManager: baseMgr,
	}
}

// CreateSnapshot overrides the base to add agent-specific fields if desired.
func (m *AgentManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	baseSnap := m.BaseFSMManager.CreateSnapshot()

	baseSnapshot, ok := baseSnap.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.AgentManagerComponentName).Errorf("Could not cast manager snapshot to BaseManagerSnapshot.")

		return baseSnap
	}

	snap := &AgentManagerSnapshot{
		BaseManagerSnapshot: baseSnapshot,
	}

	return snap
}
