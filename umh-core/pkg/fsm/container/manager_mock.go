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

package container

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
)

// NewContainerManagerWithMockedService creates a ContainerManager that uses a mock container_monitor.Service
func NewContainerManagerWithMockedService(name string, mockSvc container_monitor.MockService) *ContainerManager {
	managerName := fmt.Sprintf("%s_mock_%s", ContainerManagerComponentName, name)

	baseMgr := public_fsm.NewBaseFSMManager[config.ContainerConfig](
		managerName,
		"/dev/null",
		// For the mock, we'll just pretend to parse from FullConfig
		func(fc config.FullConfig) ([]config.ContainerConfig, error) {
			// In a real test, you'd define fc.Container with test data
			return []config.ContainerConfig{
				{
					Name:            "Core",
					DesiredFSMState: "active",
				},
			}, nil
		},
		func(cc config.ContainerConfig) (string, error) {
			return cc.Name, nil
		},
		func(cc config.ContainerConfig) (string, error) {
			return cc.DesiredFSMState, nil
		},
		func(cc config.ContainerConfig) (public_fsm.FSMInstance, error) {
			inst := NewContainerInstanceWithService(cc, &mockSvc)
			return inst, nil
		},
		func(instance public_fsm.FSMInstance, cc config.ContainerConfig) (bool, error) {
			ci, ok := instance.(*ContainerInstance)
			if !ok {
				return false, fmt.Errorf("instance not a ContainerInstance")
			}
			return ci.config.DesiredFSMState == cc.DesiredFSMState, nil
		},
		func(instance public_fsm.FSMInstance, cc config.ContainerConfig) error {
			ci, ok := instance.(*ContainerInstance)
			if !ok {
				return fmt.Errorf("instance not a ContainerInstance")
			}
			ci.config = cc
			return ci.SetDesiredFSMState(cc.DesiredFSMState)
		},
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			ci, ok := instance.(*ContainerInstance)
			if !ok {
				return 0, fmt.Errorf("instance not a ContainerInstance")
			}
			return ci.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	logger.For(managerName).Info("Created ContainerManager with mocked service.")
	return &ContainerManager{
		BaseFSMManager: baseMgr,
	}
}
