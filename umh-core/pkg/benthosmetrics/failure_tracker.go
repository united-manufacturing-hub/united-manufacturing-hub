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
// reading becomes Failures.

package benthosmetrics

import (
	"cmp"
	"slices"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// failureTracker answers, for one Benthos instance, which failures are
// active as of the latest reading, and why. Tracker.observe detects restarts,
// port changes and gaps; on one it calls resetForNewInstance or
// resetAfterGap, then observe once per scrape newer than the previous one.
type failureTracker struct {
	// engine holds the windows that turn error counters into fired error
	// signals.
	engine *diagnosis.Engine[errorReadings]

	// lastCounts holds the previous scrape's error counters, keyed by the
	// counter's name. It is nil only before the first scrape, which is the
	// baseline. After resetForNewInstance it is empty, and a counter missing
	// from it reads as 0.
	lastCounts map[string]int64

	// rises holds, per error counter, each rise still within ErrorWindow,
	// oldest first. An entry's Count is their sum.
	rises map[string][]errorRise

	// lastEntries holds the previous scrape's failure entries, for carrying
	// an open entry's LastErrorAt forward. It is nil before the first scrape
	// and after any reset.
	lastEntries []FailureEntry

	// notConnectedSince holds, for each connection check with something not
	// connected, the scrape at which that started. Like lastEntries, it is
	// nil before the first scrape and after any reset.
	notConnectedSince map[string]time.Time
}

// failureReading is what failureTracker.observe reads off one successful
// scrape.
type failureReading struct {
	at time.Time

	// errorCounts holds the raw error counters, keyed by the counter's name.
	errorCounts map[string]int64

	// connections holds the number of inputs or outputs not connected,
	// keyed by the connection check's name.
	connections map[string]int64
}

// errorRise is one scrape at which an error counter rose, and by how much.
type errorRise struct {
	at time.Time
	by int64
}

// newFailureTracker returns the failureTracker for a Tracker's first scrape.
// Its engine has no seed, so the first scrape is the baseline and errors
// counted before it are not reported.
func newFailureTracker() *failureTracker {
	return &failureTracker{engine: mustNewEngine(errorTable())}
}

// resetForNewInstance forgets every failure after a restart or a port change
// detected at time at. The new instance's counters count from 0, so every
// error they hold at the next reading is new.
func (ft *failureTracker) resetForNewInstance(at time.Time) {
	ft.reset(nil, at)
}

// resetAfterGap forgets every failure after a gap longer than ErrorWindow
// that ended at time at. The previous scrape's error counters stay the
// baseline, so only errors counted after them are new.
func (ft *failureTracker) resetAfterGap(at time.Time) {
	ft.reset(ft.lastCounts, at)
}

// reset starts the error state over at time at. The new engine's first
// reading is each error counter at its value in errorCounts, or 0 when
// errorCounts has none.
//
// The seed is dated one poll before at. Dating it at the previous scrape
// fails after a long gap: the engine drops a window whose newest reading is
// older than DemoteSpan, so the seed would be gone before the next reading.
func (ft *failureTracker) reset(errorCounts map[string]int64, at time.Time) {
	ft.engine = mustNewEngine(errorTable())
	ft.engine.Observe(seedErrors(errorCounts), diagnosis.Environment{}, at.Add(-ScrapeInterval))

	if errorCounts == nil {
		errorCounts = map[string]int64{}
	}

	ft.lastCounts = errorCounts
	ft.rises = nil
	ft.lastEntries = nil
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
func (ft *failureTracker) observe(current failureReading) Failures {
	ft.notConnectedSince = firstNotConnectedSince(ft.notConnectedSince, current.connections, current.at)
	ft.recordRises(current)
	fired, _ := ft.engine.Observe(errorReadingsOf(current.errorCounts), diagnosis.Environment{}, current.at)
	failures := ft.judge(fired, current)

	ft.lastCounts = current.errorCounts
	ft.lastEntries = failures.Entries

	return failures
}

// recordRises adds each error counter's rise since the previous scrape to
// ft.rises, dated at current, and drops every rise older than ErrorWindow.
// The first scrape is the baseline, so it records nothing.
func (ft *failureTracker) recordRises(current failureReading) {
	if ft.rises == nil {
		ft.rises = make(map[string][]errorRise, len(errorCounters))
	}

	for _, c := range errorCounters {
		kept := slices.DeleteFunc(ft.rises[c.name], func(r errorRise) bool {
			return current.at.Sub(r.at) > ErrorWindow
		})

		if ft.lastCounts != nil {
			if by := current.errorCounts[c.name] - ft.lastCounts[c.name]; by > 0 {
				kept = append(kept, errorRise{at: current.at, by: by})
			}
		}

		ft.rises[c.name] = kept
	}
}

// recentErrors returns how many errors the counter named signal rose by
// within ErrorWindow, each rise timed by the scrape that saw it, so a rise
// whose baseline reading is older than the window still counts.
func (ft *failureTracker) recentErrors(signal string) int64 {
	var n int64
	for _, r := range ft.rises[signal] {
		n += r.by
	}

	return n
}

// judge builds the Failures for the reading current: error entries first,
// then connection entries, each group with the larger Count first. ft still
// holds the previous scrape's counts and entries, or what a reset left of
// them; only notConnectedSince and rises are already updated for current.
func (ft *failureTracker) judge(fired []diagnosis.Fired, current failureReading) Failures {
	errorEntries := append(ft.firedErrorEntries(fired, current), ft.heldErrorEntries(fired, current)...)
	connectionEntries := ft.notConnectedEntries(current)

	sortByCountDesc(errorEntries)
	sortByCountDesc(connectionEntries)

	return Failures{
		Entries:    append(errorEntries, connectionEntries...),
		ObservedAt: current.at,
	}
}

// firedErrorEntries returns one error entry per fired error signal. Its
// Count is how much the counter went up within ErrorWindow (see
// recentErrors), and its LastErrorAt is the last scrape at which the counter
// rose.
func (ft *failureTracker) firedErrorEntries(fired []diagnosis.Fired, current failureReading) []FailureEntry {
	var entries []FailureEntry

	for _, f := range fired {
		c := errorCounterOf(f.Signal)
		entries = append(entries, FailureEntry{
			Signal:      c.name,
			Label:       c.label,
			Count:       ft.recentErrors(c.name),
			Since:       f.Since,
			LastErrorAt: ft.lastRise(current, c.name, f.Since),
		})
	}

	return entries
}

// heldErrorEntries returns the previous scrape's error entries whose signal no
// longer fires but whose LastErrorAt is still within ErrorWindow. The engine
// measures a rise from the oldest reading in its window, the one before the
// error, so after sparse scrapes it stops firing before ErrorWindow has
// passed since the error itself. A held entry's Count is recounted, so errors
// that aged out of the window no longer count. ft.lastEntries is nil after a
// reset, so nothing is held across a restart or a gap longer than ErrorWindow.
func (ft *failureTracker) heldErrorEntries(fired []diagnosis.Fired, current failureReading) []FailureEntry {
	var held []FailureEntry

	for _, previous := range ft.lastEntries {
		if previous.Connection || isFiring(fired, previous.Signal) {
			continue
		}

		if current.at.Sub(previous.LastErrorAt) > ErrorWindow {
			continue
		}

		previous.Count = ft.recentErrors(previous.Signal)
		previous.LastErrorAt = redate(current.at, previous.LastErrorAt)
		held = append(held, previous)
	}

	return held
}

// notConnectedEntries returns one connection entry per connection check with
// an input or output that is not connected.
func (ft *failureTracker) notConnectedEntries(current failureReading) []FailureEntry {
	var entries []FailureEntry

	for _, c := range connectionChecks {
		n := current.connections[c.name]
		if n == 0 {
			continue
		}

		entries = append(entries, FailureEntry{
			Signal:     c.name,
			Label:      c.label,
			Count:      n,
			Since:      ft.notConnectedSince[c.name],
			Connection: true,
		})
	}

	return entries
}

// lastRise returns when the error counter named signal last rose, for the
// entry that opened at since.
func (ft *failureTracker) lastRise(current failureReading, signal string, since time.Time) time.Time {
	// A counter missing from ft.lastCounts reads as 0, so after a restart
	// every non-zero counter rose.
	if current.errorCounts[signal] > ft.lastCounts[signal] {
		return current.at
	}

	previous, ok := ft.lastEntry(signal, since)
	if !ok {
		return since
	}

	return redate(current.at, previous.LastErrorAt)
}

// lastEntry returns the previous scrape's entry with the same signal and
// Since.
func (ft *failureTracker) lastEntry(signal string, since time.Time) (FailureEntry, bool) {
	for _, previous := range ft.lastEntries {
		if previous.Signal == signal && previous.Since.Equal(since) {
			return previous, true
		}
	}

	return FailureEntry{}, false
}

// firstNotConnectedSince returns, per connection check with something not
// connected, when that started: the time from previous, or at if previous
// has none.
func firstNotConnectedSince(previous map[string]time.Time, counts map[string]int64, at time.Time) map[string]time.Time {
	out := make(map[string]time.Time, len(counts))
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

// isFiring reports whether fired holds the signal named signal.
func isFiring(fired []diagnosis.Fired, signal string) bool {
	return slices.ContainsFunc(fired, func(f diagnosis.Fired) bool { return f.Signal == signal })
}

// redate returns t re-dated from at by the monotonic time elapsed between
// them, so the result's wall clock matches at's even after a wall-clock step.
// HasErrorNotClearedInTime on Failures that went through JSON, which drops
// monotonic readings, then still measures real time.
func redate(at, t time.Time) time.Time {
	return at.Add(-at.Sub(t))
}

// sortByCountDesc sorts entries in place by the larger Count first, keeping
// equals in list order.
func sortByCountDesc(entries []FailureEntry) {
	slices.SortStableFunc(entries, func(a, b FailureEntry) int {
		return cmp.Compare(b.Count, a.Count)
	})
}
