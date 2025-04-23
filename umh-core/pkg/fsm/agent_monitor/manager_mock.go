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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
)

// NewAgentManagerWithMockedService creates an AgentManager that uses a mock agent_monitor.Service
func NewAgentManagerWithMockedService(name string, mockSvc agent_monitor.MockService) *AgentManager {
	managerName := fmt.Sprintf("%s_mock_%s", logger.AgentManagerComponentName, name)

	baseMgr := public_fsm.NewBaseFSMManager[config.AgentMonitorConfig](
		managerName,
		"/dev/null",
		// For the mock, we'll just pretend to parse from FullConfig
		func(fc config.FullConfig) ([]config.AgentMonitorConfig, error) {
			// In a real test, you'd define test data
			return []config.AgentMonitorConfig{
				{
					Name:            "test-agent",
					DesiredFSMState: OperationalStateActive,
				},
			}, nil
		},
		func(fc config.AgentMonitorConfig) (string, error) {
			return logger.AgentInstanceComponentName, nil
		},
		func(fc config.AgentMonitorConfig) (string, error) {
			return fc.DesiredFSMState, nil
		},
		func(fc config.AgentMonitorConfig) (public_fsm.FSMInstance, error) {
			inst := NewAgentInstanceWithService(fc, &mockSvc)
			return inst, nil
		},
		func(instance public_fsm.FSMInstance, fc config.AgentMonitorConfig) (bool, error) {
			ai, ok := instance.(*AgentInstance)
			if !ok {
				return false, fmt.Errorf("instance not an AgentInstance")
			}
			return ai.config.DesiredFSMState == fc.DesiredFSMState, nil
		},
		func(instance public_fsm.FSMInstance, fc config.AgentMonitorConfig) error {
			ai, ok := instance.(*AgentInstance)
			if !ok {
				return fmt.Errorf("instance not an AgentInstance")
			}
			ai.config = fc
			return ai.SetDesiredFSMState(fc.DesiredFSMState)
		},
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			ai, ok := instance.(*AgentInstance)
			if !ok {
				return 0, fmt.Errorf("instance not an AgentInstance")
			}
			return ai.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	logger.For(managerName).Info("Created AgentManager with mocked service.")
	return &AgentManager{
		BaseFSMManager: baseMgr,
	}
}
