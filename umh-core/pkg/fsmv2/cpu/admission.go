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

package fsmv2cpu

import "time"

// admissionWindow is how long a fresh worker waits for a capable signal to
// produce its first measurement before giving up on it. Once this much sample
// time has passed since the worker's first sample, a signal that still has not
// measured is reported rather than waited on any longer.
const admissionWindow = 10 * time.Second

// admission is the admission window's per-worker state.
type admission struct {
	// startedAt is the first sample timestamp the worker ever saw, and zero
	// until then.
	startedAt time.Time

	reported bool
}

// shortfallAtDeadline anchors the window on the first tick and answers what it
// says about this one. at is the tick's sample timestamp; measured and capable
// are its evidence counts.
//
// Elapsed is at minus the anchor, so the window runs on the sample clock rather
// than the wall clock. Production sample timestamps come from monotonic
// time.Now(), so elapsed is never negative there.
func (a *admission) shortfallAtDeadline(at time.Time, measured, capable int) bool {
	if a.startedAt.IsZero() {
		a.startedAt = at
	}

	return shortfallAtDeadline(at.Sub(a.startedAt), admissionWindow, measured, capable)
}

// reportOnce returns true on its first call and false on every later one, so a
// caller that guards its report with it reports once per worker.
func (a *admission) reportOnce() bool {
	if a.reported {
		return false
	}

	a.reported = true

	return true
}

// shortfallAtDeadline answers whether one tick has reached the admission
// deadline with a shortfall still open.
//
// elapsed is how much sample time has passed since the worker's first sample.
// capable and measured are the tick's evidence counts. A shortfall is measured
// below capable — some signal this box can answer has never once answered.
func shortfallAtDeadline(elapsed, window time.Duration, measured, capable int) bool {
	return measured < capable && elapsed >= window
}
