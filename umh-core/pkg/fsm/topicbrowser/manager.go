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

package topicbrowser

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

// Manager implements the FSM management for Topic Browser services
type Manager struct {
	*public_fsm.BaseFSMManager[config.TopicBrowserConfig]
}

// Snapshot extends the base ManagerSnapshot with Topic Browser specific information
type Snapshot struct {
	// Embed BaseManagerSnapshot to include its methods using composition
	*public_fsm.BaseManagerSnapshot
}

func NewTopicBrowserManager(name string) *Manager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentTopicBrowserManager, name)
	baseManager := public_fsm.NewBaseFSMManager[config.TopicBrowserConfig](
		managerName,
		baseConnectionDir,
		// Extract the topic browser config from fullConfig
		func(fullConfig config.FullConfig) ([]config.TopicBrowserConfig, error) {
			tbConfig := fullConfig.Internal.TopicBrowser
			return []config.TopicBrowserConfig{tbConfig}, nil
		},
		// Get name for topic browser config
		func(cfg config.TopicBrowserConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state for topic browser config
		func(cfg config.TopicBrowserConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create topic browser instance from config
		func(cfg config.TopicBrowserConfig) (public_fsm.FSMInstance, error) {
			return NewInstance(baseConnectionDir, cfg), nil
		},
		// Compare Connection configs
		func(instance public_fsm.FSMInstance, cfg config.TopicBrowserConfig) (bool, error) {
			tbInstance, ok := instance.(*Instance)
			if !ok {
				return false, fmt.Errorf("instance is not a Topic Browser Instance")
			}
			return tbInstance.config.Equal(cfg.ServiceConfig), nil
		},
		// Set Connection config
		func(instance public_fsm.FSMInstance, cfg config.TopicBrowserConfig) error {
			tbInstance, ok := instance.(*Instance)
			if !ok {
				return fmt.Errorf("instance is not a Topic Browser Instance")
			}
			tbInstance.config = cfg.ServiceConfig
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			tbInstance, ok := instance.(*Instance)
			if !ok {
				return 0, fmt.Errorf("instance is not a Topic Browser Instance")
			}
			return tbInstance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)
	metrics.InitErrorCounter(metrics.ComponentTopicBrowserManager, name)
	return &Manager{
		BaseFSMManager: baseManager,
	}
}

// CreateSnapshot overrides the base CreateSnapshot to include ConnectionManager-specific information
func (m *Manager) CreateSnapshot() public_fsm.ManagerSnapshot {
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
	snap := &Snapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
	}
	return snap
}

// IsObservedStateSnapshot implements the fsm.ObservedStateSnapshot interface
func (s *Snapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}
