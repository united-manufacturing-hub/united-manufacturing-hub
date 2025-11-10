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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"go.uber.org/zap"
)

// Connection represents a connection to an external resource
type Connection interface{}

// ConnectionPool is a mock interface for managing connections
// In a real implementation, this would manage actual network connections
type ConnectionPool interface {
	Acquire() (Connection, error)
	Release(Connection) error
	HealthCheck(Connection) error
}

// ChildDependencies provides access to tools needed by child worker actions
type ChildDependencies struct {
	*fsmv2.BaseDependencies
	connectionPool ConnectionPool
}

// NewChildDependencies creates new dependencies for the child worker
func NewChildDependencies(connectionPool ConnectionPool, logger *zap.SugaredLogger) *ChildDependencies {
	return &ChildDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger),
		connectionPool:   connectionPool,
	}
}

// GetConnectionPool returns the connection pool
func (d *ChildDependencies) GetConnectionPool() ConnectionPool {
	return d.connectionPool
}
