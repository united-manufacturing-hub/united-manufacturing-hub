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
	mu       sync.Mutex
	wg       sync.WaitGroup
	lastSeen map[string]time.Time
	done     chan struct{}
	window   time.Duration
}

// NewFingerprintDebouncer creates a debouncer with the specified debounce window.
// A cleanup goroutine starts automatically to prevent memory leaks.
// Call Stop() when the debouncer is no longer needed to release the goroutine.
func NewFingerprintDebouncer(window time.Duration) *FingerprintDebouncer {
	d := &FingerprintDebouncer{
		lastSeen: make(map[string]time.Time),
		done:     make(chan struct{}),
		window:   window,
	}

	d.wg.Add(1)

	go d.cleanupLoop()

	return d
}

// ShouldCapture returns true if the debouncer has not recorded this fingerprint
// within the debounce window; false if this fingerprint appeared recently.
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

// Stop signals the cleanup goroutine to exit and waits for it to finish.
// Call this when the debouncer is no longer needed to prevent goroutine leaks.
func (d *FingerprintDebouncer) Stop() {
	close(d.done)
	d.wg.Wait()
}

// cleanupLoop periodically removes old entries to prevent memory leaks.
func (d *FingerprintDebouncer) cleanupLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-d.done:
			return
		case <-ticker.C:
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
}
