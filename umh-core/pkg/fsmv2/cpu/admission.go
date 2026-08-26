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
// measured is reported rather than waited on any longer. The synthetic-clock
// tests step the sample clock in whole seconds, so the window is a whole number
// of them.
const admissionWindow = 10 * time.Second

// admission is the admission window's per-worker state: where the window
// started, and whether its deadline has already been reported.
type admission struct {
	// startedAt is the first sample timestamp the worker ever saw, and zero
	// until then.
	startedAt time.Time

	// reported records whether the deadline report has already fired.
	reported bool
}

// shortfallAtDeadline anchors the window on the first tick and answers what it
// says about this one. at is the tick's sample timestamp; measured and capable
// are its evidence counts. The answer is the package function of the same name,
// which is where the rule itself lives.
//
// Elapsed is the delta between at and the anchor, so the window is driven by
// the sample clock rather than the wall clock: it advances only as fast as the
// samples do, which is what lets the synthetic-clock tests reach the deadline
// without waiting. Production sample timestamps come from monotonic time.Now(),
// so elapsed is never negative there.
func (a *admission) shortfallAtDeadline(at time.Time, measured, capable int) bool {
	if a.startedAt.IsZero() {
		a.startedAt = at
	}

	return shortfallAtDeadline(at.Sub(a.startedAt), admissionWindow, measured, capable)
}

// reportOnce returns true on its first call and false on every later one, so a
// caller that guards its report with it reports once per worker rather than
// once per tick.
func (a *admission) reportOnce() bool {
	if a.reported {
		return false
	}

	a.reported = true

	return true
}

// shortfallAtDeadline answers whether one tick has reached the admission
// deadline with a shortfall still open. It reads nothing but its arguments — no
// worker, no sampler, no clock.
//
// elapsed is how much sample time has passed since the worker's first sample.
// capable and measured are the tick's evidence counts: how many signals this box
// can answer at all, and how many of those have ever produced a reading. A
// shortfall is measured below capable — some signal this box can answer has
// never once answered.
//
// It takes both to be true here: a shortfall alone is not enough, because early
// ticks are expected to be missing readings, and a closed window alone is not
// enough, because most boxes reach it having measured everything. A box that
// cannot fully see its own CPU is therefore reported rather than waited on
// forever, and a box that can is never reported at all.
func shortfallAtDeadline(elapsed, window time.Duration, measured, capable int) bool {
	return measured < capable && elapsed >= window
}
