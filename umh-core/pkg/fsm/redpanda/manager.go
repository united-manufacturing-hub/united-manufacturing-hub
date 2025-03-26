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

package redpanda

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

const (
	baseRedpandaDir = constants.S6BaseDir
)

// RedpandaManager implements FSM management for Redpanda services.
type RedpandaManager struct {
	*public_fsm.BaseFSMManager[config.RedpandaConfig]
}

// RedpandaManagerSnapshot extends the base ManagerSnapshot with Redpanda-specific information
type RedpandaManagerSnapshot struct {
	// Embed the BaseManagerSnapshot to inherit its methods
	*public_fsm.BaseManagerSnapshot
}

func NewRedpandaManager(name string) *RedpandaManager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentRedpandaManager, name)

	baseManager := public_fsm.NewBaseFSMManager[config.RedpandaConfig](
		managerName,
		baseRedpandaDir,
		// Extract Redpanda configs from full config
		func(fullConfig config.FullConfig) ([]config.RedpandaConfig, error) {
			return []config.RedpandaConfig{fullConfig.Redpanda}, nil
		},
		// Get name from Redpanda config
		func(cfg config.RedpandaConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state from Redpanda config
		func(cfg config.RedpandaConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create Redpanda instance from config
		func(cfg config.RedpandaConfig) (public_fsm.FSMInstance, error) {
			return NewRedpandaInstance(baseRedpandaDir, cfg), nil
		},
		// Compare Redpanda configs
		func(instance public_fsm.FSMInstance, cfg config.RedpandaConfig) (bool, error) {
			RedpandaInstance, ok := instance.(*RedpandaInstance)
			if !ok {
				return false, fmt.Errorf("instance is not a RedpandaInstance")
			}
			return RedpandaInstance.config.Equal(cfg.RedpandaServiceConfig), nil
		},
		// Set Redpanda config
		func(instance public_fsm.FSMInstance, cfg config.RedpandaConfig) error {
			RedpandaInstance, ok := instance.(*RedpandaInstance)
			if !ok {
				return fmt.Errorf("instance is not a RedpandaInstance")
			}
			RedpandaInstance.config = cfg.RedpandaServiceConfig
			return nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentRedpandaManager, name)

	return &RedpandaManager{
		BaseFSMManager: baseManager,
	}
}

func (m *RedpandaManager) Reconcile(ctx context.Context, cfg config.FullConfig, tick uint64, tickStartTime time.Time) (error, bool) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.ObserveReconcileTime(logger.ComponentRedpandaManager, m.BaseFSMManager.GetManagerName(), duration)
	}()

	// We do not need to manage ports for Redpanda, therefore we can directly reconcile
	return m.BaseFSMManager.Reconcile(ctx, cfg, tick, tickStartTime)
}

func (m *RedpandaManager) CreateSnapshot() public_fsm.ManagerSnapshot {
	// Get base snapshot from parent
	baseSnapshot := m.BaseFSMManager.CreateSnapshot()

	// We need to convert the interface to the concrete type
	baseManagerSnapshot, ok := baseSnapshot.(*public_fsm.BaseManagerSnapshot)
	if !ok {
		logger.For(logger.ComponentBenthosManager).Errorf(
			"Failed to convert base snapshot to BaseManagerSnapshot, using generic snapshot")
		return baseSnapshot
	}

	// Create Benthos-specific snapshot
	redpandaSnapshot := &RedpandaManagerSnapshot{
		BaseManagerSnapshot: baseManagerSnapshot,
	}

	return redpandaSnapshot
}

func (s *RedpandaManagerSnapshot) IsObservedStateSnapshot() {
	// Marker method implementation
}
