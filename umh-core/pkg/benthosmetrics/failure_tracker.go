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

// The error-side state kept across scrapes, when it starts over, and how one
// reading becomes a FailureReport.

package benthosmetrics

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// failureTracker answers, for one Benthos instance, which failures are
// active as of the latest reading, and why. Tracker.observe calls observe
// once per scrape newer than the previous one. It detects restarts, port
// changes and gaps itself, and before observing such a scrape calls
// resetForNewInstance or resetAfterGap.
type failureTracker struct {
	// engine holds one window per error counter. Each scrape feeds it how
	// much each counter rose since lastCounts, dated at that scrape, and the
	// window sums those increases over ErrorWindow. So each increase counts
	// for ErrorWindow after the scrape that saw it, however old the reading
	// before it is.
	engine *diagnosis.Engine[errorReadings]

	// lastCounts is the baseline the next scrape's increases are measured
	// from: the previous scrape's error counters, keyed by the counter's
	// name. It is nil only before the first scrape, which is the baseline
	// itself. After resetForNewInstance it is empty, and a counter missing
	// from it reads as 0.
	lastCounts map[signal]int64

	// lastErrorAt holds, per error counter, the last scrape at which it rose
	// since the last reset.
	lastErrorAt map[signal]time.Time

	// notConnectedSince holds, for each connection check with something not
	// connected, the scrape at which that started. It is nil before the
	// first scrape and after any reset.
	notConnectedSince map[signal]time.Time
}

// failureReading is what failureTracker.observe reads off one successful
// scrape.
type failureReading struct {
	at time.Time

	// errorCounts holds the raw error counters, keyed by the counter's name.
	errorCounts map[signal]int64

	// connections holds the number of inputs or outputs not connected,
	// keyed by the connection check's name.
	connections map[signal]int64
}

// newFailureTracker returns the failureTracker for a Tracker's first scrape.
// It has no baseline yet, so the first scrape becomes it and errors counted
// before it are not reported.
func newFailureTracker() *failureTracker {
	return &failureTracker{
		engine:      mustNewEngine(errorTable()),
		lastErrorAt: map[signal]time.Time{},
	}
}

// resetForNewInstance forgets every failure after a restart or a port change.
// The new instance's counters count from 0, so every error they hold at the
// next reading is new.
func (ft *failureTracker) resetForNewInstance() {
	ft.reset(map[signal]int64{})
}

// resetAfterGap forgets every failure after a gap longer than ErrorWindow.
// The previous scrape's error counters stay the baseline, so only errors
// counted after them are new.
func (ft *failureTracker) resetAfterGap() {
	ft.reset(ft.lastCounts)
}

// reset starts the error state over with baseline as the counts the next
// scrape's increases are measured from.
func (ft *failureTracker) reset(baseline map[signal]int64) {
	ft.engine = mustNewEngine(errorTable())
	ft.lastCounts = baseline
	ft.lastErrorAt = map[signal]time.Time{}
	ft.notConnectedSince = nil
}

// countsDropped reports whether any error counter of m reads lower than at
// the previous scrape, one of the ways a Benthos restart shows.
func (ft *failureTracker) countsDropped(m Metrics) bool {
	for name, v := range readErrors(m) {
		if v < ft.lastCounts[name] {
			return true
		}
	}

	return false
}

// observe judges the reading current and keeps what the next reading is
// judged against.
func (ft *failureTracker) observe(current failureReading) FailureReport {
	ft.notConnectedSince = firstNotConnectedSince(ft.notConnectedSince, current.connections, current.at)

	increases := ft.increases(current)
	ft.recordLastErrors(increases, current.at)

	fired, _ := ft.engine.Observe(errorReadingsOf(increases), diagnosis.Environment{}, current.at)
	failures := ft.judge(fired, current)

	ft.lastCounts = current.errorCounts

	return failures
}

// increases returns how much each error counter rose since ft.lastCounts.
// The first scrape is the baseline, so every increase is 0 there. The
// counters never drop here: observe resets the tracker first (see restarted).
func (ft *failureTracker) increases(current failureReading) map[signal]int64 {
	out := make(map[signal]int64, len(errorCounters))
	for _, c := range errorCounters {
		var by int64
		if ft.lastCounts != nil {
			by = current.errorCounts[c.name] - ft.lastCounts[c.name]
		}

		out[c.name] = by
	}

	return out
}

// recordLastErrors dates each error counter that rose at the scrape at.
func (ft *failureTracker) recordLastErrors(increases map[signal]int64, at time.Time) {
	for name, by := range increases {
		if by > 0 {
			ft.lastErrorAt[name] = at
		}
	}
}

// judge builds the FailureReport for the reading current: a RecentErrors per
// fired error signal, and a NotConnected per connection check with something
// not connected. A RecentErrors' Count is read off the engine's window as it
// stands now: a Fired's own Value is frozen at the tick the signal fired.
func (ft *failureTracker) judge(fired []diagnosis.Fired, current failureReading) FailureReport {
	report := FailureReport{ObservedAt: current.at}

	firedSince := make(map[signal]time.Time, len(fired))
	for _, f := range fired {
		firedSince[signal(f.Signal)] = f.Since
	}

	for _, c := range errorCounters {
		since, ok := firedSince[c.name]
		if !ok {
			continue
		}

		count, _ := ft.engine.Reduction(string(c.name), string(c.name)).Get()
		// Through a local: nilaway panics on *c.field(...) = ... because
		// field is a func-typed struct field, not a declared function.
		slot := c.field(&report)
		*slot = &RecentErrors{
			Count:       int64(count),
			Since:       since,
			LastErrorAt: redate(current.at, ft.lastErrorAt[c.name]),
		}
	}

	for _, c := range connectionChecks {
		n := current.connections[c.name]
		if n == 0 {
			continue
		}

		slot := c.field(&report)
		*slot = &NotConnected{Count: n, Since: ft.notConnectedSince[c.name]}
	}

	return report
}

// firstNotConnectedSince returns, per connection check with something not
// connected, when that started: the time from previous, or at if previous
// has none.
func firstNotConnectedSince(previous map[signal]time.Time, counts map[signal]int64, at time.Time) map[signal]time.Time {
	out := make(map[signal]time.Time, len(counts))
	for name, n := range counts {
		if n == 0 {
			continue
		}

		since, ok := previous[name]
		if !ok {
			since = at
		}

		out[name] = since
	}

	return out
}

// redate returns t re-dated from at by the monotonic time elapsed between
// them, so the result's wall clock matches at's even after a wall-clock step.
// ErrorFree on a FailureReport that went through JSON, which drops monotonic
// readings, then still measures real time since the last error.
func redate(at, t time.Time) time.Time {
	return at.Add(-at.Sub(t))
}
