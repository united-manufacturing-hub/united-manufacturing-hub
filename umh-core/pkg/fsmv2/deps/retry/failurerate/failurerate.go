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

package failurerate

import (
	"fmt"
	"math"
	"sync"
)

// Config controls the Tracker's window size, escalation threshold, and
// minimum sample count.
type Config struct {
	// WindowSize is the number of outcomes the circular buffer retains.
	// Older outcomes are evicted when the buffer is full.
	// Example: 600 ≈ 10 minutes at one outcome per 1 second tick.
	WindowSize int

	// Threshold is the failure rate (0.0–1.0) that triggers escalation.
	// Example: 0.9 means 90 % failures.
	Threshold float64

	// MinSamples is the minimum number of recorded outcomes before the
	// Tracker can escalate. This prevents spurious alerts during startup.
	MinSamples int
}

// Tracker tracks the failure rate of the last N outcomes using a fixed-size
// circular buffer. Call [Tracker.RecordOutcome] with true (success) or false
// (failure); the Tracker computes the rolling failure rate and detects
// threshold crossings.
//
// Tracker is safe for concurrent use.
type Tracker struct {
	outcomes  []bool
	cfg       Config
	mu        sync.RWMutex
	head      int
	count     int
	failures  int
	escalated bool
}

// New creates a Tracker with the given configuration. The circular buffer
// is pre-allocated to cfg.WindowSize. New panics if the configuration is
// invalid (WindowSize <= 0, Threshold out of (0,1] or NaN, MinSamples < 0,
// or MinSamples > WindowSize).
func New(cfg Config) *Tracker {
	if cfg.WindowSize <= 0 {
		panic(fmt.Sprintf("failurerate: WindowSize must be > 0, got %d", cfg.WindowSize))
	}
	if math.IsNaN(cfg.Threshold) || cfg.Threshold <= 0.0 || cfg.Threshold > 1.0 {
		panic(fmt.Sprintf("failurerate: Threshold must be in (0.0, 1.0], got %f", cfg.Threshold))
	}
	if cfg.MinSamples < 0 || cfg.MinSamples > cfg.WindowSize {
		panic(fmt.Sprintf("failurerate: MinSamples must be in [0, WindowSize], got %d (WindowSize=%d)", cfg.MinSamples, cfg.WindowSize))
	}

	return &Tracker{
		outcomes: make([]bool, cfg.WindowSize),
		cfg:      cfg,
	}
}

// RecordOutcome records a success (true) or failure (false) and updates the
// rolling window. It returns true exactly once when the failure rate first
// crosses the escalation threshold (one-shot). After the rate drops below
// the threshold, the one-shot rearms and can fire again on the next crossing.
func (t *Tracker) RecordOutcome(success bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Evict the oldest outcome if the buffer is full.
	if t.count == t.cfg.WindowSize {
		evicted := t.outcomes[t.head]
		if !evicted {
			t.failures--
		}
	} else {
		t.count++
	}

	// Write the new outcome.
	t.outcomes[t.head] = success
	if !success {
		t.failures++
	}
	t.head = (t.head + 1) % t.cfg.WindowSize

	// Check escalation transition.
	if t.count < t.cfg.MinSamples {
		return false
	}

	rate := float64(t.failures) / float64(t.count)
	aboveThreshold := rate >= t.cfg.Threshold

	if aboveThreshold && !t.escalated {
		t.escalated = true
		return true // one-shot: first crossing
	}
	if !aboveThreshold && t.escalated {
		t.escalated = false // rearm for next crossing
	}

	return false
}

// FailureRate returns the current failure rate (0.0–1.0). It returns 0.0 if
// no outcomes have been recorded or if fewer than [Config.MinSamples] have.
func (t *Tracker) FailureRate() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 || t.count < t.cfg.MinSamples {
		return 0.0
	}
	return float64(t.failures) / float64(t.count)
}

// IsEscalated reports whether the failure rate meets or exceeds the threshold
// and at least [Config.MinSamples] outcomes have been recorded.
func (t *Tracker) IsEscalated() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.escalated
}

// Reset clears all recorded outcomes. Use this when the tracked entity is
// recreated from scratch and historical data is no longer relevant.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.head = 0
	t.count = 0
	t.failures = 0
	t.escalated = false
}

// SetEscalatedForTest sets the escalated flag directly. This is only for
// use in external test packages that need to set up specific escalation states.
func (t *Tracker) SetEscalatedForTest(v bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.escalated = v
}
