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

package retry

import (
	"sync"
	"time"
)

// tracker is the default thread-safe implementation of Tracker.
type tracker struct {
	mu                sync.RWMutex
	consecutiveErrors int
	degradedSince     time.Time
	lastError         ErrorInfo
}

// New creates a new Tracker.
func New() Tracker {
	return &tracker{}
}

// --- Recorder implementation ---

func (t *tracker) RecordError(opts ...ErrorOption) {
	cfg := &errorConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.consecutiveErrors == 0 {
		t.degradedSince = time.Now()
	}

	t.consecutiveErrors++
	t.lastError = ErrorInfo{
		Class:      cfg.class,
		RetryAfter: cfg.retryAfter,
		OccurredAt: time.Now(),
	}
}

func (t *tracker) RecordSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.consecutiveErrors = 0
	t.degradedSince = time.Time{}
	t.lastError = ErrorInfo{}
}

func (t *tracker) Attempt() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.consecutiveErrors++
	// Note: Does NOT set degradedSince - this is intentional.
	// Attempt() is used by recovery actions (like ResetTransport) to advance
	// past modulo-N triggers without marking the system as newly degraded.
}

// --- State implementation ---

func (t *tracker) ConsecutiveErrors() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.consecutiveErrors
}

func (t *tracker) DegradedSince() (time.Time, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.degradedSince, !t.degradedSince.IsZero()
}

func (t *tracker) LastError() ErrorInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.lastError
}

// --- Policy implementation ---

func (t *tracker) ShouldReset(threshold int) bool {
	if threshold <= 0 {
		return false // Guard against division by zero
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.consecutiveErrors > 0 && t.consecutiveErrors%threshold == 0
}

func (t *tracker) BackoffElapsed(delay time.Duration) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.degradedSince.IsZero() {
		return true
	}

	return time.Since(t.degradedSince) >= delay
}
