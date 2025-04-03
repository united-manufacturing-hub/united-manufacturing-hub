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

package connection

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	public_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

const (
	baseDir = constants.S6BaseDir
	// Define component name constants
	ComponentConnectionManager = "ConnectionManager"
	// Define reconcile interval
	reconcileInterval = 5 * time.Second
)

// ConnectionManager implements FSM management for Connection testing instances.
type ConnectionManager struct {
	*public_fsm.BaseFSMManager[ConnectionConfig]
}

// ConnectionManagerSnapshot extends the base ManagerSnapshot with Connection-specific information
type ConnectionManagerSnapshot struct {
	// Embed the BaseManagerSnapshot to inherit its methods
	*public_fsm.BaseManagerSnapshot
}

// NewConnectionManager creates a new ConnectionManager
func NewConnectionManager(name string) *ConnectionManager {
	managerName := fmt.Sprintf("%s%s", ComponentConnectionManager, name)

	baseManager := public_fsm.NewBaseFSMManager[ConnectionConfig](
		managerName,
		baseDir,
		// Extract Connection configs from full config
		func(fullConfig config.FullConfig) ([]ConnectionConfig, error) {
			// For now, return an empty list - in real implementation, this would
			// retrieve connection test configurations from somewhere
			return []ConnectionConfig{}, nil
		},
		// Get name from Connection config
		func(cfg ConnectionConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state from Connection config
		func(cfg ConnectionConfig) (string, error) {
			return cfg.DesiredState, nil
		},
		// Create Connection instance from config
		func(cfg ConnectionConfig) (public_fsm.FSMInstance, error) {
			return NewConnection(cfg), nil
		},
		// Compare Connection configs
		func(instance public_fsm.FSMInstance, cfg ConnectionConfig) (bool, error) {
			connInstance, ok := instance.(*Connection)
			if !ok {
				return false, fmt.Errorf("instance is not a Connection")
			}
			// Consider the instance unchanged if target, port, and type are the same
			return connInstance.Config.Target == cfg.Target &&
				connInstance.Config.Port == cfg.Port &&
				connInstance.Config.Type == cfg.Type, nil
		},
		// Set Connection config
		func(instance public_fsm.FSMInstance, cfg ConnectionConfig) error {
			connInstance, ok := instance.(*Connection)
			if !ok {
				return fmt.Errorf("instance is not a Connection")
			}
			connInstance.Config = cfg
			return nil
		},
		// Get expected max p95 execution time per instance
		func(instance public_fsm.FSMInstance) (time.Duration, error) {
			connInstance, ok := instance.(*Connection)
			if !ok {
				return 0, fmt.Errorf("instance is not a Connection")
			}
			return connInstance.GetExpectedMaxP95ExecutionTimePerInstance(), nil
		},
	)

	return &ConnectionManager{
		BaseFSMManager: baseManager,
	}
}

// ReconcileLoop runs a continuous reconciliation loop for the connection manager
func (m *ConnectionManager) ReconcileLoop(ctx context.Context) error {
	log := logger.For("ConnectionManager.ReconcileLoop")
	log.Infof("Starting connection manager reconcile loop")

	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	var tick uint64 = 0
	for {
		select {
		case <-ctx.Done():
			log.Infof("Connection manager reconcile loop stopped due to context cancellation")
			return ctx.Err()
		case <-ticker.C:
			tick++

			// For now, we just reconcile the instances we have without loading config
			// In a full implementation, we would call m.BaseFSMManager.Reconcile with config

			// Get all instances
			instances := m.BaseFSMManager.GetInstances()

			// Reconcile each instance
			for name, instance := range instances {
				// Create a context with timeout for this instance
				instanceCtx, cancel := context.WithTimeout(ctx, 2*time.Second)

				err, _ := instance.Reconcile(instanceCtx, tick)
				if err != nil {
					log.Errorf("Error reconciling connection %s: %v", name, err)
				}

				cancel()
			}
		}
	}
}

// CreateConnectionTest creates a new connection test with the specified parameters
func (m *ConnectionManager) CreateConnectionTest(ctx context.Context, name, target string, port int, connType, parentDFC string) (*Connection, error) {
	config := ConnectionConfig{
		Name:         name,
		DesiredState: OperationalStateTesting, // Start testing immediately
		Target:       target,
		Port:         port,
		Type:         connType,
		ParentDFC:    parentDFC,
	}

	// Create a new instance directly
	instance := NewConnection(config)

	// Add the instance to our instances map
	m.BaseFSMManager.AddInstanceForTest(name, instance)

	return instance, nil
}

// GetConnectionByName returns a connection test by name if it exists
func (m *ConnectionManager) GetConnectionByName(ctx context.Context, name string) (*Connection, bool) {
	instance, exists := m.BaseFSMManager.GetInstance(name)
	if !exists {
		return nil, false
	}

	connInstance, ok := instance.(*Connection)
	if !ok {
		return nil, false
	}

	return connInstance, true
}

// GetConnectionByParentDFC returns a connection test by parent DFC name if it exists
func (m *ConnectionManager) GetConnectionByParentDFC(ctx context.Context, parentDFC string) (*Connection, bool) {
	// Get all instances
	instances := m.BaseFSMManager.GetInstances()

	// Look for a connection with matching parent DFC
	for _, instance := range instances {
		connInstance, ok := instance.(*Connection)
		if !ok {
			continue
		}

		if connInstance.Config.ParentDFC == parentDFC {
			return connInstance, true
		}
	}

	return nil, false
}
