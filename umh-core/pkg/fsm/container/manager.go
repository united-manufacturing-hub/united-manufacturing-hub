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
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

const (
	ContainerManagerComponentName = "ContainerManager"
)

// ContainerManager is the FSM manager for multiple container monitor instances.
type ContainerManager struct {
	*public_fsm.BaseFSMManager[config.ContainerConfig]
}

// ContainerManagerSnapshot extends the base manager snapshot to hold any container-specific info.
type ContainerManagerSnapshot struct {
	*public_fsm.BaseManagerSnapshot
}

// Ensure it satisfies fsm.ObservedStateSnapshot.
func (c *ContainerManagerSnapshot) IsObservedStateSnapshot() {}

// NewContainerManager constructs a manager.
// You might keep `sharedMonitorService` if you want one global service instance.
func NewContainerManager(name string) *ContainerManager {
	managerName := fmt.Sprintf("%s_%s", ContainerManagerComponentName, name)

	baseMgr := public_fsm.NewBaseFSMManager[config.ContainerConfig](
		managerName,
		"/dev/null", // no actual S6 base dir needed for a pure monitor
		// Extract config.ContainerConfig slice from FullConfig
		func(fc config.FullConfig) ([]config.ContainerConfig, error) {
			// Always return a single config
			var configs []config.ContainerConfig

			configs = append(configs, config.ContainerConfig{
				Name:            constants.DefaultInstanceName,
				DesiredFSMState: OperationalStateActive,
			})

			return configs, nil
		},
		// Get name from config
		func(cc config.ContainerConfig) (string, error) {
			return cc.Name, nil
		},
		// Desired state from config
		func(cc config.ContainerConfig) (string, error) {
			return cc.DesiredFSMState, nil
		},
		// Create instance
		func(cc config.ContainerConfig) (public_fsm.FSMInstance, error) {
			// Typically create a new container_monitor.Service or reuse shared
			// Here let's reuse the shared:
			inst := NewContainerInstance(cc)

			return inst, nil
		},
		// Compare config => if same, no recreation needed
		func(instance public_fsm.FSMInstance, containerConfig config.ContainerConfig) (bool, error) {
			containerInstance, ok := instance.(*ContainerInstance)
			if !ok {
				return false, errors.New("instance is not a ContainerInstance")
			}
			// If same config => return true, else false
			// Minimal check:
			return containerInstance.config.DesiredFSMState == containerConfig.DesiredFSMState, nil
		},
		// Set config if only small changes
		func(instance public_fsm.FSMInstance, containerConfig config.ContainerConfig) error {
			containerInstance, ok := instance.(*ContainerInstance)
			if !ok {
				return errors.New("instance is not a ContainerInstance")
			}

			containerInstance.config = containerConfig

			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			ci, ok := instance.(*ContainerInstance)
			if !ok {
				return 0, errors.New("instance is not a ContainerInstance")
			}

			return ci.GetMinimumRequiredTime(), nil
		},
	)

	metrics.InitErrorCounter(ContainerManagerComponentName, name)

	return &ContainerManager{
		BaseFSMManager: baseMgr,
	}
}

// CreateSnapshot overrides the base to add container-specific fields if desired.
func (m *ContainerManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	baseSnap := m.BaseFSMManager.CreateSnapshot()

	baseSnapshot, ok := baseSnap.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(ContainerManagerComponentName).Errorf("Could not cast manager snapshot to BaseManagerSnapshot.")

		return baseSnap
	}

	snap := &ContainerManagerSnapshot{
		BaseManagerSnapshot: baseSnapshot,
	}

	return snap
}
