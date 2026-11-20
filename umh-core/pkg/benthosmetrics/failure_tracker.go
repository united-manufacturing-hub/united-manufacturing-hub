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

// The error-side state kept across scrapes, and when it starts over.

package benthosmetrics

import (
	"slices"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// failureTracker is everything the Failures of one Benthos instance depend on
// from earlier scrapes. Tracker.observe calls reset when the error state has
// to start over, and observe once per scrape newer than the previous one.
type failureTracker struct {
	// engine holds the windows that turn error counters into fired error
	// signals.
	engine *diagnosis.Engine[errorReadings]

	// lastCounts holds the previous scrape's error counters, keyed by the
	// counter's name. It is nil only before the first scrape, which is the
	// baseline. After a reset for a restart or a port change it is empty,
	// and a counter missing from it reads as 0.
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

// newFailureTracker returns the failureTracker for a Tracker's first scrape.
// Its engine has no seed, so the first scrape is the baseline and errors
// counted before it are not reported.
func newFailureTracker() *failureTracker {
	return &failureTracker{engine: mustNewEngine(errorTable())}
}

// errorRise is one scrape at which an error counter rose, and by how much.
type errorRise struct {
	at time.Time
	by int64
}

// reset starts the error state over at time at. The new engine's first
// reading is each error counter at its value in errorCounts, or 0 when
// errorCounts has none. Pass nil after a restart or a port change, where
// every error the new counters hold is new, and the previous counts after a
// gap longer than ErrorWindow, where only errors after the gap count.
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

// observe judges the scrape m, taken at time at, and keeps what the next
// scrape is judged against.
func (ft *failureTracker) observe(m Metrics, at time.Time) Failures {
	current := judgedScrape{
		at:          at,
		errorCounts: readErrors(m),
		connections: readConnections(m),
	}

	ft.notConnectedSince = firstNotConnectedSince(ft.notConnectedSince, current.connections, at)
	ft.recordRises(current)
	fired, _ := ft.engine.Observe(errorReadingsOf(current.errorCounts), diagnosis.Environment{}, at)
	failures := ft.judge(fired, current)

	ft.lastCounts = current.errorCounts
	ft.lastEntries = failures.Entries

	return failures
}

// recordRises adds each error counter's rise since the previous scrape to
// ft.rises, dated at current, and drops every rise older than ErrorWindow.
// The first scrape is the baseline, so it records nothing.
func (ft *failureTracker) recordRises(current judgedScrape) {
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
