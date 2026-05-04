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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
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
	*deps.BaseDependencies                // Embedded pointer (8 bytes)
	connectionPool         ConnectionPool // Interface (16 bytes)
	lastFailureTime        time.Time      // When the last failure occurred - kept for metrics (24 bytes)
	mu                     sync.RWMutex   // Protects mutable fields below (24 bytes)
	attempts               int
	currentCycle           int // Current failure cycle (0-indexed)
	ticksInConnectedState  int // Number of ticks spent in Connected state
	observationsSinceFailure  int // Counter incremented each time CollectObservedState is called
	connected              bool
}

func NewFailingDependencies(connectionPool ConnectionPool, baseDeps *deps.BaseDependencies) *FailingDependencies {
	return &FailingDependencies{
		BaseDependencies: baseDeps,
		connectionPool:   connectionPool,
	}
}

func (d *FailingDependencies) GetConnectionPool() ConnectionPool {
	return d.connectionPool
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

func (d *FailingDependencies) GetCurrentCycle() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.currentCycle
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

func (d *FailingDependencies) SetLastFailureTime(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastFailureTime = t
}

func (d *FailingDependencies) GetLastFailureTime() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastFailureTime
}

// IncrementObservationsSinceFailure increments the observation counter and returns the new value.
// This should be called each time CollectObservedState is called.
func (d *FailingDependencies) IncrementObservationsSinceFailure() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.observationsSinceFailure++

	return d.observationsSinceFailure
}

// GetObservationsSinceFailure returns the current observation counter value.
func (d *FailingDependencies) GetObservationsSinceFailure() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.observationsSinceFailure
}

// ResetObservationsSinceFailure resets the observation counter to 0.
// This should be called when a failure occurs.
func (d *FailingDependencies) ResetObservationsSinceFailure() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.observationsSinceFailure = 0
}
