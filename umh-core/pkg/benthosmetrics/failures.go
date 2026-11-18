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

	// Connection is true for an input or output that is not connected, false
	// for an error counter.
	Connection bool
}

// IsConnection reports whether e is a connection entry: an entry for an
// input or output that is not connected, which stays until Benthos
// reconnects it. Every other entry is an error entry.
func (e FailureEntry) IsConnection() bool {
	return e.Connection
}

// ErrorFree reports whether nothing is failing. Otherwise reason describes
// the first entry, for example "benthos reported 3 output errors in the
// last 10 minutes" or "benthos reports 1 input not connected".
func (f *Failures) ErrorFree() (ok bool, reason string) {
	if len(f.Entries) == 0 {
		return true, ""
	}

	e := f.Entries[0]
	label := e.Label
	if e.Count != 1 {
		label += "s"
	}

	if e.IsConnection() {
		return false, fmt.Sprintf("benthos reports %d %s not connected", e.Count, label)
	}

	return false, fmt.Sprintf("benthos reported %d %s in the last %s", e.Count, label, windowWords(ErrorWindow))
}

// NotCleared reports whether f, as of its own ObservedAt, still holds an
// error entry more than its ErrorWindow plus margin after that entry's own
// LastErrorAt, which means the entry did not clear in time. A connection
// entry never counts.
func (f *Failures) NotCleared(margin time.Duration) bool {
	for _, e := range f.Entries {
		if e.IsConnection() {
			continue
		}

		if f.ObservedAt.Sub(e.LastErrorAt) > ErrorWindow+margin {
			return true
		}
	}

	return false
}

// windowWords renders one of the package's durations for a reason a user
// reads: whole minutes in words below an hour — "1 minute", "10 minutes" —
// and, from an hour on, hours with the minutes left over, "1 hour",
// "1 hour 15 minutes", "12 hours"; never Go's "10m0s" or "75m0s".
func windowWords(d time.Duration) string {
	minutes := int(d / time.Minute)

	words := func(n int, word string) string {
		if n == 1 {
			return "1 " + word
		}
		return fmt.Sprintf("%d %ss", n, word)
	}

	if minutes < 60 {
		return words(minutes, "minute")
	}

	hours, rest := minutes/60, minutes%60
	if rest == 0 {
		return words(hours, "hour")
	}
	return words(hours, "hour") + " " + words(rest, "minute")
}
