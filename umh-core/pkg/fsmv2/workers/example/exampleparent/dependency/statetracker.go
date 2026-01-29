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

package dependency

import (
	"sync"
	"time"

	"github.com/benbjohnson/clock"
)

// StateTracker provides state transition time tracking.
type StateTracker interface {
	// RecordStateChange updates the entry time if state changed
	RecordStateChange(newState string)
	// GetStateEnteredAt returns when current state was entered
	GetStateEnteredAt() time.Time
	// GetCurrentState returns the current recorded state
	GetCurrentState() string
	// Elapsed returns time since state was entered (uses injected Clock)
	Elapsed() time.Duration
}

// DefaultStateTracker is the production implementation.
type DefaultStateTracker struct {
	enteredAt    time.Time
	clock        clock.Clock
	currentState string
	mu           sync.RWMutex
}

// NewDefaultStateTracker creates a new StateTracker with the given clock.
// If clock is nil, uses the real system clock.
func NewDefaultStateTracker(c clock.Clock) *DefaultStateTracker {
	if c == nil {
		c = clock.New()
	}

	return &DefaultStateTracker{
		clock:     c,
		enteredAt: c.Now(),
	}
}

// RecordStateChange updates the entry time if state changed.
func (t *DefaultStateTracker) RecordStateChange(newState string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.currentState != newState {
		t.currentState = newState
		t.enteredAt = t.clock.Now()
	}
}

// GetStateEnteredAt returns when current state was entered.
func (t *DefaultStateTracker) GetStateEnteredAt() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.enteredAt
}

// GetCurrentState returns the current recorded state.
func (t *DefaultStateTracker) GetCurrentState() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.currentState
}

// Elapsed returns time since state was entered.
func (t *DefaultStateTracker) Elapsed() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.clock.Since(t.enteredAt)
}
