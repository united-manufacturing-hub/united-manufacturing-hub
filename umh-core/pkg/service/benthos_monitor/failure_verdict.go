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

package benthos_monitor

import (
	"fmt"
	"time"
)

// FailureVerdict is the worker's judgement of one scrape: which failure
// signals fired, and when. The zero verdict, with no entry, is the
// error-free case.
type FailureVerdict struct {
	// Fired lists every fired failure signal, most urgent first. A signal has
	// fired when its counter rose inside its window and has not yet cleared.
	// More urgent means a lower tier, then a larger current increase. The tier
	// is the counter's rank class, errors before connection failures; the
	// FSMv2 benthos monitor (pkg/fsmv2/benthos_monitor, failureCounters)
	// assigns it and sorts Fired.
	Fired []FailureEntry

	// ObservedAt is the scrape the verdict was computed for.
	ObservedAt time.Time

	// LastIncreaseAt is the latest scrape at which any failure counter was
	// higher than at the scrape before it. After a restart the scrape before
	// counts as 0. It is zero until the first increase, and it is computed from
	// the raw counters, not from the fired signals.
	LastIncreaseAt time.Time
}

// FailureEntry is one fired failure signal as the FSM reads it.
type FailureEntry struct {
	// Signal is the name the worker's table declared, e.g. "output_error".
	Signal string

	// Label is the human wording for the reason, e.g. "output errors".
	Label string

	// Increase is how much the counter rose inside the window, read on every
	// scrape so the reason names the current rise, not the one that fired.
	Increase float64

	// Since is the scrape the signal fired at.
	Since time.Time

	// Window is how long after an increase the signal stays fired.
	Window time.Duration
}

// ErrorFree reports whether no failure signal fired. The reason names the
// first entry's current increase, so a later rise raises the number even
// while the entry keeps the Since of the first one.
func (v *FailureVerdict) ErrorFree() (bool, string) {
	if len(v.Fired) == 0 {
		return true, ""
	}

	e := v.Fired[0]
	return false, fmt.Sprintf("benthos reported %d %s in the last %s", int(e.Increase), e.Label, e.Window)
}
