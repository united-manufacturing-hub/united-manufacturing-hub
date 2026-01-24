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

func (d *DefaultConnectionPool) Acquire() (Connection, error) {
	return nil, nil
}

func (d *DefaultConnectionPool) Release(_ Connection) error {
	return nil
}

func (d *DefaultConnectionPool) HealthCheck(_ Connection) error {
	return nil
}

// FailingDependencies provides access to tools needed by failing worker actions.
type FailingDependencies struct {
	connectionPool ConnectionPool
	*deps.BaseDependencies
	maxFailures           int
	attempts              int
	restartAfterFailures  int
	failureCycles         int // Total number of failure cycles to perform
	currentCycle          int // Current failure cycle (0-indexed)
	ticksInConnectedState int // Number of ticks spent in Connected state
	mu                    sync.RWMutex
	shouldFail            bool
	connected             bool
}

func NewFailingDependencies(connectionPool ConnectionPool, logger *zap.SugaredLogger, stateReader deps.StateReader, identity deps.Identity) *FailingDependencies {
	return &FailingDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		connectionPool:   connectionPool,
		maxFailures:      3, // Default: fail 3 times before success
		failureCycles:    1, // Default: single failure cycle (backward compatible)
	}
}

func (d *FailingDependencies) GetConnectionPool() ConnectionPool {
	return d.connectionPool
}

func (d *FailingDependencies) SetShouldFail(shouldFail bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.shouldFail = shouldFail
}

func (d *FailingDependencies) GetShouldFail() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.shouldFail
}

func (d *FailingDependencies) SetMaxFailures(maxFailures int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.maxFailures = maxFailures
}

func (d *FailingDependencies) GetMaxFailures() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.maxFailures
}

func (d *FailingDependencies) IncrementAttempts() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.attempts++

	return d.attempts
}

func (d *FailingDependencies) GetAttempts() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.attempts
}

func (d *FailingDependencies) ResetAttempts() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.attempts = 0
}

func (d *FailingDependencies) SetConnected(connected bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.connected = connected
}

func (d *FailingDependencies) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.connected
}

func (d *FailingDependencies) SetRestartAfterFailures(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.restartAfterFailures = n
}

// GetRestartAfterFailures returns the restart threshold (0 = no restart).
func (d *FailingDependencies) GetRestartAfterFailures() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.restartAfterFailures
}

func (d *FailingDependencies) SetFailureCycles(cycles int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.failureCycles = cycles
}

func (d *FailingDependencies) GetFailureCycles() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.failureCycles
}

func (d *FailingDependencies) GetCurrentCycle() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.currentCycle
}

func (d *FailingDependencies) AllCyclesComplete() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.currentCycle >= d.failureCycles
}

func (d *FailingDependencies) AdvanceCycle() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.currentCycle++

	d.attempts = 0

	return d.currentCycle
}

func (d *FailingDependencies) IncrementTicksInConnected() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ticksInConnectedState++

	return d.ticksInConnectedState
}

func (d *FailingDependencies) GetTicksInConnected() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.ticksInConnectedState
}

func (d *FailingDependencies) ResetTicksInConnected() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ticksInConnectedState = 0
}
