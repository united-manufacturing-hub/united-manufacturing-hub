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
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
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
			// Force topic browser name to be "topic-browser"
			tbConfig.Name = constants.TopicBrowserServiceName
			return []config.TopicBrowserConfig{tbConfig}, nil
		},
		// Get name for topic browser config
		func(cfg config.TopicBrowserConfig) (string, error) {
			if cfg.Name == "" {
				return "", fmt.Errorf("topic browser config name cannot be empty")
			}
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
			tbInstance, ok := instance.(*TopicBrowserInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a Topic Browser Instance")
			}
			return tbInstance.config.Equal(cfg.TopicBrowserServiceConfig), nil
		},
		// Set Connection config
		func(instance public_fsm.FSMInstance, cfg config.TopicBrowserConfig) error {
			tbInstance, ok := instance.(*TopicBrowserInstance)
			if !ok {
				return fmt.Errorf("instance is not a Topic Browser Instance")
			}
			tbInstance.config = cfg.TopicBrowserServiceConfig
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			tbInstance, ok := instance.(*TopicBrowserInstance)
			if !ok {
				return 0, fmt.Errorf("instance is not a Topic Browser Instance")
			}
			return tbInstance.GetMinimumRequiredTime(), nil
		},
	)
	metrics.InitErrorCounter(metrics.ComponentTopicBrowserManager, name)
	return &Manager{
		BaseFSMManager: baseManager,
	}
}

func (m *Manager) Reconcile(ctx context.Context, snapshot public_fsm.SystemSnapshot, services serviceregistry.Provider) (error, bool) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(logger.ComponentTopicBrowserManager, m.GetManagerName(), duration)
	}()

	// We do not need to manage ports for Topic Browser, therefore we can directly reconcile
	return m.BaseFSMManager.Reconcile(ctx, snapshot, services)
}

// CreateSnapshot overrides the base CreateSnapshot to include ConnectionManager-specific information
func (m *Manager) CreateSnapshot() public_fsm.ManagerSnapshot {
	// Get base snapshot from parent
	baseSnapshot := m.BaseFSMManager.CreateSnapshot()

	// We need to convert the interface to the concrete type
	baseManagerSnapshot, ok := baseSnapshot.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentTopicBrowserManager).Errorf(
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
