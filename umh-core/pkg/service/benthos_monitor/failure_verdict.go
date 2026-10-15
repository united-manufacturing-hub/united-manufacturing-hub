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
	// Fired lists the error entries first, then the connection entries, each
	// group with the larger Count first. pkg/fsmv2/benthos_monitor
	// (failureVerdict) sets this order. An error entry is present once its
	// counter rose and until its Window passed with no further rise. A
	// connection entry is present while an input or output is not connected.
	Fired []FailureEntry

	// ObservedAt is the scrape the verdict was computed for.
	ObservedAt time.Time

	// LastIncreaseAt is the latest scrape at which any failure counter was
	// higher than at the scrape before it. After a restart the scrape before
	// counts as 0. It is zero until the first increase, and it is computed from
	// the raw error counters, not from the fired signals. An error signal
	// fires only on a rise, so whenever Fired holds an entry with a Window,
	// LastIncreaseAt is set. Connection entries do not move it.
	LastIncreaseAt time.Time
}

// FailureEntry is one fired failure signal as the FSM reads it.
type FailureEntry struct {
	// Signal is the name the worker's table declared, e.g. "output_error".
	Signal string

	// Label is the human wording for one failure, e.g. "output error".
	// ErrorFree adds an "s" for any other count.
	Label string

	// Count is the number the reason names, read on every scrape so it is
	// the current number, not the one at firing: how much an error counter
	// rose inside its window, or how many paths a connection entry finds not
	// connected.
	Count float64

	// Since is the scrape the signal fired at.
	Since time.Time

	// Window is how long after an increase the signal stays fired. It is
	// zero for a connection entry, which holds only while the path is not
	// connected.
	Window time.Duration
}

// ErrorFree reports whether no failure signal fired. The reason names the
// first entry's current value, so a later rise raises the number even while
// the entry keeps the Since of the first one.
func (v *FailureVerdict) ErrorFree() (bool, string) {
	if len(v.Fired) == 0 {
		return true, ""
	}

	e := v.Fired[0]
	n := int(e.Count)
	label := e.Label
	if n != 1 {
		label += "s"
	}

	if e.Window == 0 {
		return false, fmt.Sprintf("benthos reports %d %s not connected", n, label)
	}

	return false, fmt.Sprintf("benthos reported %d %s in the last %s", n, label, e.Window)
}
