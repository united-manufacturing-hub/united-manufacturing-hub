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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"go.uber.org/zap"
)

// Connection represents a connection to an external resource.
type Connection interface{}

// ConnectionPool is a mock interface for managing connections.
type ConnectionPool interface {
	Acquire() (Connection, error)
	Release(Connection) error
	HealthCheck(Connection) error
}

// DefaultConnectionPool is a no-op connection pool for testing and examples.
type DefaultConnectionPool struct{}

// Acquire returns a nil connection.
func (d *DefaultConnectionPool) Acquire() (Connection, error) {
	return nil, nil
}

// Release releases a connection.
func (d *DefaultConnectionPool) Release(_ Connection) error {
	return nil
}

// HealthCheck checks the connection health.
func (d *DefaultConnectionPool) HealthCheck(_ Connection) error {
	return nil
}

// ExamplechildDependencies provides access to tools needed by child worker actions.
type ExamplechildDependencies struct {
	*deps.BaseDependencies
	connectionPool ConnectionPool

	mu          sync.RWMutex
	isConnected bool
}

// NewExamplechildDependencies creates new dependencies for the child worker.
func NewExamplechildDependencies(connectionPool ConnectionPool, logger *zap.SugaredLogger, stateReader deps.StateReader, identity deps.Identity) *ExamplechildDependencies {
	return &ExamplechildDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
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
