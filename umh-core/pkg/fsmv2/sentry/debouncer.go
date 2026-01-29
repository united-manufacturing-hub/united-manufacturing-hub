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

package sentry

import (
	"sync"
	"time"
)

// FingerprintDebouncer provides per-fingerprint debouncing for Sentry error capture.
// This prevents duplicate errors with the same fingerprint from being captured
// within the debounce window.
type FingerprintDebouncer struct {
	lastSeen map[string]time.Time
	mu       sync.Mutex
	window   time.Duration
}

// NewFingerprintDebouncer creates a new debouncer with the specified debounce window.
// The cleanup goroutine is started automatically to prevent memory leaks.
func NewFingerprintDebouncer(window time.Duration) *FingerprintDebouncer {
	d := &FingerprintDebouncer{
		lastSeen: make(map[string]time.Time),
		window:   window,
	}
	go d.cleanupLoop()

	return d
}

// ShouldCapture returns true if this fingerprint should be captured to Sentry.
// Returns false if the same fingerprint was seen within the debounce window.
func (d *FingerprintDebouncer) ShouldCapture(fingerprint string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	lastTime, exists := d.lastSeen[fingerprint]
	if exists && time.Since(lastTime) < d.window {
		return false
	}

	d.lastSeen[fingerprint] = time.Now()

	return true
}

// cleanupLoop periodically removes old entries to prevent memory leaks.
func (d *FingerprintDebouncer) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		d.mu.Lock()

		cutoff := time.Now().Add(-d.window * 2)
		for fp, t := range d.lastSeen {
			if t.Before(cutoff) {
				delete(d.lastSeen, fp)
			}
		}

		d.mu.Unlock()
	}
}
