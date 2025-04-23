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

package s6

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
	baseS6Dir = constants.S6BaseDir
)

// S6Manager implements FSM management for S6 services.
type S6Manager struct {
	*public_fsm.BaseFSMManager[config.S6FSMConfig]
}

// S6ManagerSnapshot extends the base manager snapshot to hold any s6-specific info
type S6ManagerSnapshot struct {
	*public_fsm.BaseManagerSnapshot
}

// NewS6Manager creates a new S6Manager
// The name is used to identify the manager in logs, as other components that leverage s6 will sue their own instance of this manager
func NewS6Manager(name string) *S6Manager {

	managerName := fmt.Sprintf("%s%s", logger.ComponentS6Manager, name)

	baseManager := public_fsm.NewBaseFSMManager[config.S6FSMConfig](
		managerName,
		baseS6Dir,
		// Extract S6 configs from full config
		func(fullConfig config.FullConfig) ([]config.S6FSMConfig, error) {
			return fullConfig.Internal.Services, nil
		},
		// Get name from S6 config
		func(cfg config.S6FSMConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state from S6 config
		func(cfg config.S6FSMConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create S6 instance from config
		func(cfg config.S6FSMConfig) (public_fsm.FSMInstance, error) {
			return NewS6Instance(baseS6Dir, cfg)
		},
		// Compare S6 configs
		func(instance public_fsm.FSMInstance, cfg config.S6FSMConfig) (bool, error) {
			s6Instance, ok := instance.(*S6Instance)
			if !ok {
				return false, fmt.Errorf("instance is not an S6Instance")
			}
			return s6Instance.config.S6ServiceConfig.Equal(cfg.S6ServiceConfig), nil
		},
		// Set S6 config
		func(instance public_fsm.FSMInstance, cfg config.S6FSMConfig) error {
			s6Instance, ok := instance.(*S6Instance)
			if !ok {
				return fmt.Errorf("instance is not an S6Instance")
			}
			s6Instance.config.S6ServiceConfig = cfg.S6ServiceConfig
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			s6Instance, ok := instance.(*S6Instance)
			if !ok {
				return 0, fmt.Errorf("instance is not an S6Instance")
			}
			return s6Instance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentS6Manager, name)

	return &S6Manager{
		BaseFSMManager: baseManager,
	}
}

// CreateSnapshot overrides the base to add s6-specific fields if desired
func (m *S6Manager) CreateSnapshot() public_fsm.ManagerSnapshot {
	baseSnap := m.BaseFSMManager.CreateSnapshot()
	baseSnapshot, ok := baseSnap.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentS6Manager).Errorf("Could not create manager snapshot to BaseManagerSnapshot.")
		return baseSnap
	}
	snap := &S6ManagerSnapshot{
		BaseManagerSnapshot: baseSnapshot,
	}
	return snap
}
