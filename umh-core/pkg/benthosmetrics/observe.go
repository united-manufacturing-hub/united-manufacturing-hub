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

// Turning one observed scrape into message throughput and failures.

package benthosmetrics

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// observation is what observe keeps from one scrape to compare the next one
// against: when, from which port, the totals it read, and the Sample observe
// returned for it.
type observation struct {
	at            time.Time
	port          int
	inputReceived int64
	outputSent    int64

	// errors holds the raw error counters, keyed by the counter's name.
	errors map[string]int64

	// connections holds the number of inputs or outputs not connected, keyed
	// by the connection check's name.
	connections map[string]int64

	// notConnectedSince holds, for each connection check whose count is above
	// 0, the scrape at which that count went above 0 (see
	// firstNotConnectedSince).
	notConnectedSince map[string]time.Time

	// before holds the error counters this scrape is compared against, for
	// the entries' LastErrorAt: the previous scrape's counters on the same
	// counter series. It is nil on the first scrape and after a restart,
	// where a counter missing from it reads as 0.
	before map[string]int64

	// prevEntries holds the previous scrape's failure entries, for carrying
	// an open entry's LastErrorAt forward. It is nil on the first scrape,
	// after a restart and after a gap longer than ErrorWindow.
	prevEntries []FailureEntry

	sample Sample
}

// observe records one successful scrape of the Benthos at port, taken at time
// at, and returns its Sample. A scrape no newer than the previous one keeps
// the previous throughput and Failures, because the windows accept readings
// only in time order. Errors counted before the first scrape are not
// reported.
func (t *Tracker) observe(port int, m Metrics, at time.Time) Sample {
	cur := observation{
		at:            at,
		port:          port,
		inputReceived: m.InputReceivedTotal(),
		outputSent:    m.OutputSentTotal(),
		errors:        readErrors(m),
		connections:   readConnections(m),
	}

	prev := t.prev
	// prevNotConnectedSince carries an outage's start forward only from a
	// scrape of the same Benthos with no long gap, as the error signals do.
	var prevNotConnectedSince map[string]time.Time

	switch {
	case prev == nil:
	case !at.After(prev.at):
		s := prev.sample
		s.At, s.Metrics = at, m

		return s
	case prev.port != port:
		// A new counter series: both engines and the offsets start over.
		// The error engine is seeded at zero the same way a restart seeds
		// it, so errors the new port's Benthos already counted still count.
		t.engine = mustNewEngine(table())
		t.offsets = offsets{}
		rebuildErrors(t, nil, at)
	case restarted(prev, &cur):
		// A restart (a Benthos restart resets every counter to 0): a counter
		// that reads lower than before adds its previous value to its
		// offset, so the series the engine is fed never goes down and both
		// throughput windows keep their samples. The error state starts over.
		if cur.inputReceived < prev.inputReceived {
			t.offsets.input += prev.inputReceived
		}

		if cur.outputSent < prev.outputSent {
			t.offsets.output += prev.outputSent
		}

		rebuildErrors(t, nil, at)
	case at.Sub(prev.at) > ErrorWindow:
		cur.before = prev.errors
		rebuildErrors(t, prev.errors, at)
	default:
		cur.before = prev.errors
		cur.prevEntries = prev.sample.Failures.Entries
		prevNotConnectedSince = prev.notConnectedSince
	}

	cur.notConnectedSince = firstNotConnectedSince(prevNotConnectedSince, cur.connections, at)

	// The throughput engine holds no signals, so the discarded (Fired,
	// Readiness) is always zero; observe feeds it for the windows alone.
	_, _ = t.engine.Observe(readings{
		inputReceived: diagnosis.Known(float64(cur.inputReceived + t.offsets.input)),
		outputSent:    diagnosis.Known(float64(cur.outputSent + t.offsets.output)),
	}, diagnosis.Environment{}, at)

	fired, _ := t.errorEngine.Observe(errorReadings(cur), diagnosis.Environment{}, at)
	failures := judgeFailures(t.errorEngine, fired, cur)

	s := Sample{
		At:                      at,
		Metrics:                 m,
		InputMessagesPerSecond:  messagesPerSecond(t.engine.Measurement(inputMessagesPerSecond)),
		OutputMessagesPerSecond: messagesPerSecond(t.engine.Measurement(outputMessagesPerSecond)),
		Failures:                &failures,
	}
	inputPerSecond, measured := s.InputMessagesPerSecond.Get()
	s.IsActive = measured && inputPerSecond > 0

	cur.sample = s
	t.prev = &cur

	return s
}

// restarted reports whether any counter of cur (input, output or error) is
// lower than in prev, which is how umh-core detects a Benthos restart.
// Errors the new instance counted before its first scrape count only above
// the old instance's count.
func restarted(prev, cur *observation) bool {
	if cur.inputReceived < prev.inputReceived ||
		cur.outputSent < prev.outputSent {
		return true
	}

	for name, v := range cur.errors {
		if v < prev.errors[name] {
			return true
		}
	}

	return false
}

// rebuildErrors gives t a new error engine whose first reading is
// seedErrors(errorCounts), dated one poll before at. Dating it at the
// previous scrape fails after a long gap: the engine drops a window whose
// newest reading is older than DemoteSpan, so the seed would be gone before
// the next reading.
func rebuildErrors(t *Tracker, errorCounts map[string]int64, at time.Time) {
	t.errorEngine = mustNewEngine(errorTable())
	t.errorEngine.Observe(seedErrors(errorCounts), diagnosis.Environment{}, at.Add(-ScrapeInterval))
}

// messagesPerSecond reads a measurement's slope, Unknown while the window
// cannot answer: fewer than two samples, or none newer than throughputWindow.
func messagesPerSecond(r diagnosis.Reduced) diagnosis.Reading {
	v, state := r.Get()
	if state != diagnosis.StateValue {
		return diagnosis.Unknown()
	}

	return diagnosis.Known(v)
}
