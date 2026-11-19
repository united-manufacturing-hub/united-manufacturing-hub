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

// Failures, its entries, and the questions callers ask of it.

package benthosmetrics

import (
	"fmt"
	"strings"
	"time"
)

// Failures is what is wrong with one Benthos instance as of one scrape:
// recent errors, and inputs or outputs that are not connected. The zero
// value means nothing is wrong.
type Failures struct {
	// Entries holds one entry per current failure, error entries first. An
	// error entry stays until ErrorWindow (10 minutes) passes with no new
	// error; a connection entry stays while an input or output is not connected.
	Entries []FailureEntry

	// ObservedAt is when the scrape behind these failures ran.
	ObservedAt time.Time
}

// FailureEntry is one failure: an error counter that rose, or inputs or
// outputs that are not connected.
type FailureEntry struct {
	// Signal names the error counter or connection check, e.g. "output_error"
	// or "input_connection".
	Signal string

	// Label is the human wording for one failure, e.g. "output error".
	// ErrorFree adds an "s" for any other count.
	Label string

	// Count is the current number behind the entry: errors within
	// ErrorWindow, or inputs or outputs not connected.
	Count int64

	// Since is when this entry appeared: the first new error since it last
	// cleared, or the first scrape that found the input or output not connected.
	Since time.Time

	// LastErrorAt is the last scrape at which this entry's error counter
	// rose; zero on a connection entry.
	LastErrorAt time.Time

	// Connection is true for a connection entry: inputs or outputs that are
	// not connected, which stays until Benthos reconnects them. It is false
	// for an error entry.
	Connection bool
}

// ErrorFree reports whether nothing is failing. Otherwise reason describes
// the first entry, for example "benthos reported 3 output errors in the last
// 10m, last one 4m ago" or "benthos reports 1 input not connected".
func (f *Failures) ErrorFree() (ok bool, reason string) {
	if len(f.Entries) == 0 {
		return true, ""
	}

	e := f.Entries[0]

	label := e.Label
	if e.Count != 1 {
		label += "s"
	}

	if e.Connection {
		return false, fmt.Sprintf("benthos reports %d %s not connected", e.Count, label)
	}

	// Only a count above one needs saying which error the time belongs to.
	when := e.lastErrorAgo(f.ObservedAt)
	if e.Count != 1 {
		when = "last one " + when
	}

	return false, fmt.Sprintf("benthos reported %d %s in the last %s, %s",
		e.Count, label, short(ErrorWindow), when)
}

// lastErrorAgo words how long before at, the Failures' ObservedAt, this
// entry's error counter last rose, e.g. "4m ago". It counts up rather than
// down to the clear, because a countdown jumps backwards on each new error.
// Anything under a second, including a negative duration, reads "just now".
func (e FailureEntry) lastErrorAgo(at time.Time) string {
	ago := at.Sub(e.LastErrorAt)
	if ago < time.Second {
		return "just now"
	}

	return short(ago) + " ago"
}

// short renders a duration for a user to read, rounded to whole seconds as
// the log-window reason in pkg/fsm/benthos does and without the trailing
// zero units Go's own String leaves behind: "10m0s" reads "10m" and
// "1h0m0s" reads "1h", while "9m12s" is already short.
func short(d time.Duration) string {
	s := d.Round(time.Second).String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}

	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}

	return s
}

// NotCleared reports whether f, as of its own ObservedAt, still holds an
// error entry more than its ErrorWindow plus margin after that entry's own
// LastErrorAt, which means the entry did not clear in time. A connection
// entry never counts.
func (f *Failures) NotCleared(margin time.Duration) bool {
	for _, e := range f.Entries {
		if e.Connection {
			continue
		}

		if f.ObservedAt.Sub(e.LastErrorAt) > ErrorWindow+margin {
			return true
		}
	}

	return false
}
