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

package supervisor

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type panicRecovery struct {
	timestamps []time.Time
	window     time.Duration
	maxPanics  int
	mu         sync.Mutex
}

func newPanicRecovery(window time.Duration, maxPanics int) *panicRecovery {
	if window <= 0 {
		panic("panicRecovery: window must be positive")
	}
	if maxPanics <= 0 {
		panic("panicRecovery: maxPanics must be positive")
	}
	return &panicRecovery{
		window:    window,
		maxPanics: maxPanics,
	}
}

// RecordPanic records a panic event and returns true if the escalation threshold has been reached.
func (p *panicRecovery) RecordPanic() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	p.timestamps = append(p.timestamps, now)

	cutoff := now.Add(-p.window)
	pruned := p.timestamps[:0]
	for _, ts := range p.timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	p.timestamps = pruned

	return len(p.timestamps) >= p.maxPanics
}

func (p *panicRecovery) PanicCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-p.window)
	count := 0
	for _, ts := range p.timestamps {
		if ts.After(cutoff) {
			count++
		}
	}

	return count
}

func (p *panicRecovery) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.timestamps = nil
}

// classifyPanic converts a recovered panic value into a typed error and a classification string.
// Used by the tick() panic recovery handler. The collector has a local copy
// (internal/collection/collector.go) due to circular import constraints.
func classifyPanic(r interface{}) (panicType string, panicErr error) {
	switch v := r.(type) {
	case error:
		return "error", v
	case string:
		return "string", errors.New(v)
	default:
		return "unknown", fmt.Errorf("%v", r)
	}
}
