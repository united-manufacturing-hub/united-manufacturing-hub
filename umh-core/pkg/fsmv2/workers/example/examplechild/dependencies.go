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

package example_child

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"go.uber.org/zap"
)

// Connection represents a connection to an external resource.
type Connection interface{}

// ConnectionPool is a mock interface for managing connections
// In a real implementation, this would manage actual network connections.
type ConnectionPool interface {
	Acquire() (Connection, error)
	Release(Connection) error
	HealthCheck(Connection) error
}

// DefaultConnectionPool is a no-op connection pool used for factory registration.
// It always returns a nil connection successfully, suitable for testing and examples.
type DefaultConnectionPool struct{}

// Acquire returns a nil connection (no-op implementation).
func (d *DefaultConnectionPool) Acquire() (Connection, error) {
	return nil, nil
}

// Release is a no-op.
func (d *DefaultConnectionPool) Release(_ Connection) error {
	return nil
}

// HealthCheck always returns success (no-op implementation).
func (d *DefaultConnectionPool) HealthCheck(_ Connection) error {
	return nil
}

// ExamplechildDependencies provides access to tools needed by child worker actions.
// The isConnected flag is shared between actions and CollectObservedState.
type ExamplechildDependencies struct {
	*fsmv2.BaseDependencies
	connectionPool ConnectionPool

	// isConnected tracks connection state, updated by ConnectAction/DisconnectAction
	// and read by CollectObservedState. Protected by mu.
	mu          sync.RWMutex
	isConnected bool
}

// NewExamplechildDependencies creates new dependencies for the child worker.
func NewExamplechildDependencies(connectionPool ConnectionPool, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, identity fsmv2.Identity) *ExamplechildDependencies {
	return &ExamplechildDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, stateReader, identity),
		connectionPool:   connectionPool,
		isConnected:      false,
	}
}

// GetConnectionPool returns the connection pool.
func (d *ExamplechildDependencies) GetConnectionPool() ConnectionPool {
	return d.connectionPool
}

// SetConnected updates the connection state. Called by ConnectAction/DisconnectAction.
func (d *ExamplechildDependencies) SetConnected(connected bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.isConnected = connected
}

// IsConnected returns the current connection state. Called by CollectObservedState.
func (d *ExamplechildDependencies) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isConnected
}
