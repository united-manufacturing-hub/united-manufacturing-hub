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

package examplefailing

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

// FailingDependencies provides access to tools needed by failing worker actions.
type FailingDependencies struct {
	*fsmv2.BaseDependencies
	connectionPool ConnectionPool
	mu             sync.RWMutex
	shouldFail     bool
}

// NewFailingDependencies creates new dependencies for the failing worker.
func NewFailingDependencies(connectionPool ConnectionPool, logger *zap.SugaredLogger, identity fsmv2.Identity) *FailingDependencies {
	return &FailingDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, identity),
		connectionPool:   connectionPool,
	}
}

// GetConnectionPool returns the connection pool.
func (d *FailingDependencies) GetConnectionPool() ConnectionPool {
	return d.connectionPool
}

// SetShouldFail sets the failure flag for the connect action.
func (d *FailingDependencies) SetShouldFail(shouldFail bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.shouldFail = shouldFail
}

// GetShouldFail returns the current failure flag.
func (d *FailingDependencies) GetShouldFail() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.shouldFail
}
