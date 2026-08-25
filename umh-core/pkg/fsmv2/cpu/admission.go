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

// admissionWindow is how long a fresh worker may refuse admission while
// a capable signal has still not first-measured. Once this much sample time
// has passed since the worker's first sample, admission opens even if the
// counts are unchanged. The synthetic-clock tests step the sample clock in
// whole seconds, so the window is a whole number of them.
const admissionWindow = 10 * time.Second

// admissionDecision answers what the admission window says about one tick. It
// reads nothing but its arguments — no worker, no sampler, no clock.
//
// elapsed is how much sample time has passed since the worker's first sample.
// capable and measured are the tick's evidence counts: how many signals this box
// can answer at all, and how many of those have ever produced a reading. A
// shortfall is measured below capable — some signal this box can answer has
// never once answered.
//
// A shortfall does one of two things, and which one depends only on where the
// tick falls in the window:
//
//	refusing             inside the window: hold admission back and wait
//	shortfallAtDeadline  the window has closed: admit anyway, and report it
//
// So the refusal is bounded rather than fixed to the counts. A signal that never
// measures stops blocking admission once the window closes, which is what keeps
// a box that cannot fully see its own CPU from being blocked for its whole life;
// it is reported instead. The two results are the same shortfall split by the
// window, so they can never both be true, and with no shortfall neither is.
func admissionDecision(elapsed, window time.Duration, measured, capable int) (refusing, shortfallAtDeadline bool) {
	shortfall := measured < capable

	return shortfall && elapsed < window, shortfall && elapsed >= window
}
