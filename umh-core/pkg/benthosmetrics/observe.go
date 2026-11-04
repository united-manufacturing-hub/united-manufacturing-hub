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

// Turning one observed scrape into rates and failures.

package benthosmetrics

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

// observation is what observe keeps from one scrape to compare the next one
// against: when, from which port, the totals it read, and the result observe
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

	// outputBlockedFor is how long the flow's outputs have sent nothing
	// while their writes keep failing at this scrape, 0 while they are not
	// blocked (see blockedFor).
	outputBlockedFor time.Duration

	// before holds the error counters this scrape is compared against, for
	// the entries' LastErrorAt: the previous scrape's counters on the same
	// counter series. It is nil on the first scrape and after a restart,
	// where a counter missing from it reads as 0.
	before map[string]int64

	// prevEntries holds the previous scrape's failure entries, for carrying
	// an open episode's LastErrorAt forward. It is nil on the first scrape
	// and after a restart.
	prevEntries []FailureEntry

	// rates is the rates this scrape produced.
	rates rates

	// failures is the failures this scrape produced.
	failures Failures
}

// observe records one successful scrape of the Benthos at port, taken at time
// at, and returns the rates and failures as of that scrape. A scrape no newer
// than the previous one is ignored and the previous result is returned.
// Errors counted before the first scrape are not reported.
func (t *Tracker) observe(port int, m Metrics, at time.Time) (rates, Failures) {
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
		// Errors the instance already counted do not count; only writes
		// after umh-core started watching it do.
		t.writes = outputWrites{lastSentAt: at, errorsAtLastSent: cur.errors[outputError]}
	case !at.After(prev.at):
		return prev.rates, prev.failures
	case prev.port != port:
		// A new counter series: both engines and the offsets start over.
		// The error engine is seeded at zero the same way a restart seeds
		// it, so errors the new port's Benthos already counted still count.
		t.engine = newEngine(rateTable())
		t.offsets = offsets{}
		rebuildErrors(t, nil, at)
		t.writes = outputWrites{lastSentAt: at}
	case counterSeriesChanged(prev, &cur):
		// A restart: fold the input and output drops into their offsets, so
		// the rate windows survive it, and start the error state over. The
		// restarted Benthos counts its writes from 0 again.
		if cur.inputReceived < prev.inputReceived {
			t.offsets.input += prev.inputReceived
		}
		if cur.outputSent < prev.outputSent {
			t.offsets.output += prev.outputSent
		}
		rebuildErrors(t, nil, at)
		t.writes = outputWrites{lastSentAt: at}
	case at.Sub(prev.at) > ErrorWindow:
		cur.before = prev.errors
		rebuildErrors(t, prev.errors, at)
		t.writes.observe(prev, &cur)
	default:
		cur.before = prev.errors
		cur.prevEntries = prev.failures.Entries
		prevNotConnectedSince = prev.notConnectedSince
		t.writes.observe(prev, &cur)
	}

	cur.notConnectedSince = firstNotConnectedSince(prevNotConnectedSince, cur.connections, at)
	cur.outputBlockedFor = blockedFor(t.writes, cur.errors[outputError], at)

	// The rate engine holds no signals, so its discarded (Fired, Readiness)
	// is always zero; observe feeds it for the windows alone.
	_, _ = t.engine.Observe(readings{
		inputReceived: diagnosis.Known(float64(cur.inputReceived + t.offsets.input)),
		outputSent:    diagnosis.Known(float64(cur.outputSent + t.offsets.output)),
	}, diagnosis.Environment{}, at)

	fired, _ := t.errorEngine.Observe(errorReadings(cur), diagnosis.Environment{}, at)

	cur.rates = rates{
		Input:  rate{MessagesPerSecond: rateOf(t.engine.Measurement(inputRate))},
		Output: rate{MessagesPerSecond: rateOf(t.engine.Measurement(outputRate))},
	}
	// IsActive keeps FSMv1's definition of an active Benthos (its
	// metrics_state.go: the input moves); the output rate does not count.
	cur.rates.IsActive = cur.rates.Input.MessagesPerSecond > 0

	cur.failures = judgeFailures(t.errorEngine, fired, cur)
	t.prev = &cur

	return cur.rates, cur.failures
}

// counterSeriesChanged reports whether Benthos restarted between prev and
// cur, so their counters cannot be compared: any one counter (input, output
// or error) went down. umh-core recognises a restart only this way, so
// failures between the restart and the first scrape after it still count: a
// new instance that has already counted as many messages and errors as the
// old one by then shows no drop, and only the errors above the old count
// count as new.
func counterSeriesChanged(prev, cur *observation) bool {
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
	t.errorEngine = newEngine(errorTable())
	t.errorEngine.Observe(seedErrors(errorCounts), diagnosis.Environment{}, at.Add(-ScrapeInterval))
}

// rateOf reads a rate measurement's slope, or 0 while the window cannot yet
// answer: fewer than two samples, or none newer than rateWindow.
func rateOf(r diagnosis.Reduced) float64 {
	v, state := r.Get()
	if state != diagnosis.StateValue {
		return 0
	}

	return v
}
