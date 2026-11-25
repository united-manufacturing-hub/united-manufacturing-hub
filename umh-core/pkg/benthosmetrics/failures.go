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

// FailureReport, the failures it holds, and the questions callers ask of it.

package benthosmetrics

import (
	"fmt"
	"strings"
	"time"
)

// FailureReport is the result of judging one scrape: what is wrong with one
// Benthos instance as of that scrape, namely recent errors and inputs or
// outputs that are not connected. It has one field per thing that can fail,
// nil while that thing is fine, so the zero value means nothing is wrong.
//
// A FailureReport holds no fsmv2/diagnosis types, so the FSMv2 store can
// save it as JSON.
type FailureReport struct {
	// OutputErrors and ProcessorErrors are the errors of that counter within
	// ErrorWindow (10 minutes). Each stays until ErrorWindow passes with no
	// new error.
	OutputErrors    *RecentErrors
	ProcessorErrors *RecentErrors

	// InputsNotConnected and OutputsNotConnected are the inputs or outputs
	// that are not connected. Each stays while one of them is not connected.
	InputsNotConnected  *NotConnected
	OutputsNotConnected *NotConnected

	// ObservedAt is when the scrape behind these failures ran.
	ObservedAt time.Time
}

// RecentErrors is one error counter that rose within ErrorWindow.
type RecentErrors struct {
	// Count is the number of errors within ErrorWindow.
	Count int64

	// Since is the first new error since the counter last cleared.
	Since time.Time

	// LastErrorAt is the last scrape at which the counter rose.
	LastErrorAt time.Time
}

// NotConnected is the inputs, or the outputs, of one Benthos instance that
// are not connected.
type NotConnected struct {
	// Count is the number of inputs or outputs not connected.
	Count int64

	// Since is the first scrape that found one of them not connected.
	Since time.Time
}

// ErrorFree reports whether nothing is failing. Otherwise reason describes
// one failure: the error counter with the most errors, or, with no errors,
// the inputs or outputs with the most not connected, for example "benthos
// reported 3 output errors in the last 10m, last one 4m ago" or "benthos
// reports 1 input not connected". On a tie it names output errors before
// processor errors, and inputs before outputs.
func (f *FailureReport) ErrorFree() (ok bool, reason string) {
	if counter, errs := f.mostErrors(); errs != nil {
		return false, errs.reason(counter.label, f.ObservedAt)
	}

	if check, down := f.mostNotConnected(); down != nil {
		return false, down.reason(check.label)
	}

	return true, ""
}

// HasErrorNotClearedInTime reports whether f, as of its own ObservedAt,
// still holds errors more than ErrorWindow plus margin after their own
// LastErrorAt: errors that should have cleared by then.
func (f *FailureReport) HasErrorNotClearedInTime(margin time.Duration) bool {
	for _, c := range errorCounters {
		errs := *c.field(f)
		if errs != nil && f.ObservedAt.Sub(errs.LastErrorAt) > ErrorWindow+margin {
			return true
		}
	}

	return false
}

// mostErrors returns the error counter with the most errors in f and those
// errors, the first in errorCounters on a tie. errs is nil when f holds no
// errors.
func (f *FailureReport) mostErrors() (counter errorCounter, errs *RecentErrors) {
	for _, c := range errorCounters {
		if e := *c.field(f); e != nil && (errs == nil || e.Count > errs.Count) {
			counter, errs = c, e
		}
	}

	return counter, errs
}

// mostNotConnected returns the connection check with the most inputs or
// outputs not connected in f and that failure, the first in connectionChecks
// on a tie. down is nil when everything is connected.
func (f *FailureReport) mostNotConnected() (check connectionCheck, down *NotConnected) {
	for _, c := range connectionChecks {
		if n := *c.field(f); n != nil && (down == nil || n.Count > down.Count) {
			check, down = c, n
		}
	}

	return check, down
}

// reason words these errors as of observedAt, as ErrorFree returns it, with
// label the wording for one of them, e.g. "output error".
func (e *RecentErrors) reason(label string, observedAt time.Time) string {
	// Only a count above one needs saying which error the time belongs to.
	when := e.timeSinceLastErrorText(observedAt)
	if e.Count != 1 {
		when = "last one " + when
	}

	return fmt.Sprintf("benthos reported %d %s in the last %s, %s",
		e.Count, plural(label, e.Count), shortDuration(ErrorWindow), when)
}

// timeSinceLastErrorText words how long before observedAt the counter last
// rose, e.g. "4m ago". It counts up rather than down to the clear, because a
// countdown jumps backwards on each new error. Anything under a second,
// including a negative duration, reads "just now".
func (e *RecentErrors) timeSinceLastErrorText(observedAt time.Time) string {
	ago := observedAt.Sub(e.LastErrorAt)
	if ago < time.Second {
		return "just now"
	}

	return shortDuration(ago) + " ago"
}

// reason words this failure as ErrorFree returns it, with label the wording
// for one input or output. It carries no time: it lasts until Benthos
// reconnects, not over a window.
func (n *NotConnected) reason(label string) string {
	return fmt.Sprintf("benthos reports %d %s not connected", n.Count, plural(label, n.Count))
}

// plural returns label for a count of 1, and label with an "s" otherwise.
func plural(label string, count int64) string {
	if count == 1 {
		return label
	}

	return label + "s"
}

// shortDuration renders a duration for a user to read, rounded to whole
// seconds as the log-window reason in pkg/fsm/benthos does and without the
// trailing zero units Go's own String leaves behind: "10m0s" reads "10m" and
// "1h0m0s" reads "1h", while "9m12s" is already short.
func shortDuration(d time.Duration) string {
	s := d.Round(time.Second).String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}

	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}

	return s
}
