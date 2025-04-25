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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// NewBenthosMonitorManagerWithMockedService creates an BenthosMonitorManager that uses a mock benthos_monitor.Service
func NewBenthosMonitorManagerWithMockedService(name string, mockSvc benthos_monitor.MockBenthosMonitorService) *BenthosMonitorManager {
	managerName := fmt.Sprintf("%s_mock_%s", logger.ComponentBenthosMonitorManager, name)

	baseMgr := public_fsm.NewBaseFSMManager[config.BenthosMonitorConfig](
		managerName,
		"/dev/null",
		// For the mock, we'll just pretend to parse from FullConfig
		func(fc config.FullConfig) ([]config.BenthosMonitorConfig, error) {
			// In a real test, you'd define test data
			return []config.BenthosMonitorConfig{
				{
					Name:            name,
					DesiredFSMState: OperationalStateActive,
				},
			}, nil
		},
		func(fc config.BenthosMonitorConfig) (string, error) {
			return fc.Name, nil
		},
		func(fc config.BenthosMonitorConfig) (string, error) {
			return fc.DesiredFSMState, nil
		},
		func(fc config.BenthosMonitorConfig) (public_fsm.FSMInstance, error) {
			inst := NewBenthosMonitorInstanceWithService(fc, &mockSvc)
			return inst, nil
		},
		func(instance public_fsm.FSMInstance, fc config.BenthosMonitorConfig) (bool, error) {
			bi, ok := instance.(*BenthosMonitorInstance)
			if !ok {
				return false, fmt.Errorf("instance not an BenthosMonitorInstance")
			}
			return bi.config.DesiredFSMState == fc.DesiredFSMState, nil
		},
		func(instance public_fsm.FSMInstance, fc config.BenthosMonitorConfig) error {
			bi, ok := instance.(*BenthosMonitorInstance)
			if !ok {
				return fmt.Errorf("instance not an BenthosMonitorInstance")
			}
			bi.config = fc
			return bi.SetDesiredFSMState(fc.DesiredFSMState)
		},
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			bi, ok := instance.(*BenthosMonitorInstance)
			if !ok {
				return 0, fmt.Errorf("instance not an BenthosMonitorInstance")
			}
			return bi.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	logger.For(managerName).Info("Created BenthosMonitorManager with mocked service.")
	return &BenthosMonitorManager{
		BaseFSMManager: baseMgr,
	}
}
