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

// FailureVerdict is the FSMv2 benthos monitor's judgement of one scrape: which
// failures it reports, and since when. A failure is an error counter that
// rose recently, or an input or output that is not connected. The zero
// verdict, with no entry, is the error-free case.
type FailureVerdict struct {
	// Fired lists the error entries first, then the connection entries, each
	// group with the larger Count first. pkg/fsmv2/benthos_monitor
	// (failureVerdict) sets this order. An error entry is present once its
	// counter rose and until its Window passed with no further rise. A
	// connection entry is present while an input or output is not connected.
	Fired []FailureEntry

	// ObservedAt is the scrape the verdict was computed for.
	ObservedAt time.Time

	// LastIncreaseAt is the latest scrape at which any error counter was
	// higher than at the scrape before it. After a restart, each counter is
	// compared with 0. It is zero until the first increase, and it is computed
	// from the raw error counters, not from Fired. An error entry appears only
	// on a rise, so whenever Fired holds an error entry, LastIncreaseAt is
	// set. Connection entries do not move it.
	LastIncreaseAt time.Time
}

// FailureEntry is one failure in the verdict: an error counter that rose, or
// inputs or outputs that are not connected.
type FailureEntry struct {
	// Signal names the error counter or connection check, e.g. "output_error"
	// or "input_connection".
	Signal string

	// Label is the human wording for one failure, e.g. "output error".
	// ErrorFree adds an "s" for any other count.
	Label string

	// Count is the number the reason names, read on every scrape so it is
	// the current number, not the one when the entry appeared: how much an
	// error counter rose inside its window, or how many inputs or outputs are
	// not connected.
	Count int64

	// Since is the scrape at which the entry appeared: the counter's first
	// rise, or the first scrape that found an input or output not connected.
	Since time.Time

	// Window is how long after an increase an error entry stays. It is zero
	// for a connection entry, which stays only while an input or output is
	// not connected.
	Window time.Duration

	// Blocked is set on an output error entry whose output sent nothing for
	// 2 minutes or more while at least 3 of its writes failed. It is how long
	// the output has sent nothing. It is zero otherwise, and on every other
	// entry. blockedFor in pkg/fsmv2/benthos_monitor sets it.
	Blocked time.Duration
}

// IsConnection reports whether e is a connection entry. A connection entry
// has no Window: it stays while an input or output is not connected.
func (e FailureEntry) IsConnection() bool { return e.Window == 0 }

// ErrorFree reports whether the verdict has no entry: no error counter rose
// inside its window and every input and output is connected. The name matches
// IBenthosService.IsMetricsErrorFree, which reads it. The reason uses the first
// entry's current Count. A rise after the entry appeared raises that number,
// while Since stays at the scrape where the entry first appeared.
func (v *FailureVerdict) ErrorFree() (bool, string) {
	if len(v.Fired) == 0 {
		return true, ""
	}

	e := v.Fired[0]
	if e.Blocked > 0 {
		return false, fmt.Sprintf("benthos output blocked: no message written for %v", e.Blocked.Truncate(time.Minute))
	}

	n := e.Count
	label := e.Label
	if n != 1 {
		label += "s"
	}

	if e.IsConnection() {
		return false, fmt.Sprintf("benthos reports %d %s not connected", n, label)
	}

	return false, fmt.Sprintf("benthos reported %d %s in the last %s", n, label, e.Window)
}
