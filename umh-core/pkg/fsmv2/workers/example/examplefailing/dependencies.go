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
// It implements ExamplefailingDependenciesWithFailure interface for failure simulation.
type FailingDependencies struct {
	connectionPool ConnectionPool
	*fsmv2.BaseDependencies
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

// NewFailingDependencies creates new dependencies for the failing worker.
func NewFailingDependencies(connectionPool ConnectionPool, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, identity fsmv2.Identity) *FailingDependencies {
	return &FailingDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, stateReader, identity),
		connectionPool:   connectionPool,
		maxFailures:      3, // Default: fail 3 times before success
		failureCycles:    1, // Default: single failure cycle (backward compatible)
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

// SetMaxFailures sets the maximum number of failures before success.
func (d *FailingDependencies) SetMaxFailures(maxFailures int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.maxFailures = maxFailures
}

// GetMaxFailures returns the configured maximum number of failures before success.
func (d *FailingDependencies) GetMaxFailures() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.maxFailures
}

// IncrementAttempts increments and returns the current attempt count.
func (d *FailingDependencies) IncrementAttempts() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.attempts++

	return d.attempts
}

// GetAttempts returns the current attempt count.
func (d *FailingDependencies) GetAttempts() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.attempts
}

// ResetAttempts resets the attempt counter to zero.
func (d *FailingDependencies) ResetAttempts() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.attempts = 0
}

// SetConnected marks the worker as connected or disconnected.
func (d *FailingDependencies) SetConnected(connected bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.connected = connected
}

// IsConnected returns whether the worker is currently connected.
func (d *FailingDependencies) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.connected
}

// SetRestartAfterFailures sets the restart threshold.
func (d *FailingDependencies) SetRestartAfterFailures(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.restartAfterFailures = n
}

// GetRestartAfterFailures returns the restart threshold.
// 0 means no restart.
func (d *FailingDependencies) GetRestartAfterFailures() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.restartAfterFailures
}

// SetFailureCycles sets the total number of failure cycles to perform.
func (d *FailingDependencies) SetFailureCycles(cycles int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.failureCycles = cycles
}

// GetFailureCycles returns the configured number of failure cycles.
func (d *FailingDependencies) GetFailureCycles() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.failureCycles
}

// GetCurrentCycle returns the current failure cycle (0-indexed).
func (d *FailingDependencies) GetCurrentCycle() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.currentCycle
}

// AllCyclesComplete returns true if all failure cycles have been completed.
func (d *FailingDependencies) AllCyclesComplete() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.currentCycle >= d.failureCycles
}

// AdvanceCycle advances to the next failure cycle and resets the attempt counter.
// Returns the new cycle number.
func (d *FailingDependencies) AdvanceCycle() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.currentCycle++

	d.attempts = 0

	return d.currentCycle
}

// IncrementTicksInConnected increments and returns the ticks spent in Connected state.
func (d *FailingDependencies) IncrementTicksInConnected() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ticksInConnectedState++

	return d.ticksInConnectedState
}

// GetTicksInConnected returns the number of ticks spent in Connected state.
func (d *FailingDependencies) GetTicksInConnected() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.ticksInConnectedState
}

// ResetTicksInConnected resets the ticks counter to zero.
func (d *FailingDependencies) ResetTicksInConnected() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ticksInConnectedState = 0
}
