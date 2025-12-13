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

package example_panic

import (
	"sync"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

type Connection interface{}

type ConnectionPool interface {
	Acquire() (Connection, error)
	Release(Connection) error
	HealthCheck(Connection) error
}

type DefaultConnectionPool struct{}

func (d *DefaultConnectionPool) Acquire() (Connection, error) {
	return nil, nil
}

func (d *DefaultConnectionPool) Release(_ Connection) error {
	return nil
}

func (d *DefaultConnectionPool) HealthCheck(_ Connection) error {
	return nil
}

type ExamplepanicDependencies struct {
	*fsmv2.BaseDependencies
	connectionPool ConnectionPool

	mu          sync.RWMutex
	shouldPanic bool
	isConnected bool
}

func NewExamplepanicDependencies(connectionPool ConnectionPool, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, identity fsmv2.Identity) *ExamplepanicDependencies {
	return &ExamplepanicDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, stateReader, identity),
		connectionPool:   connectionPool,
	}
}

func (d *ExamplepanicDependencies) GetConnectionPool() ConnectionPool {
	return d.connectionPool
}

// SetShouldPanic sets whether the worker should panic during connect action.
func (d *ExamplepanicDependencies) SetShouldPanic(shouldPanic bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.shouldPanic = shouldPanic
}

// IsShouldPanic returns whether the worker should panic during connect action.
func (d *ExamplepanicDependencies) IsShouldPanic() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.shouldPanic
}

// SetConnected sets the connection state.
func (d *ExamplepanicDependencies) SetConnected(connected bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.isConnected = connected
}

// IsConnected returns the current connection state.
func (d *ExamplepanicDependencies) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isConnected
}
