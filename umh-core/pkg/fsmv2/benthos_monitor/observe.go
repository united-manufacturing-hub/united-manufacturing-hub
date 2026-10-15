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

package fsmv2benthosmonitor

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// throughput is what one scrape's counters say about message flow.
type throughput struct {
	Input    ComponentThroughput
	Output   ComponentThroughput
	IsActive bool
}

// observation is one observed scrape: when, from which port, the raw counters,
// and the result observe returned for it.
type observation struct {
	at            time.Time
	port          int
	inputReceived int
	outputSent    int

	// failures holds the raw failure counters, keyed by the counter's name.
	failures map[string]int64

	// connections holds the number of inputs or outputs not connected, keyed
	// by the connection check's name.
	connections map[string]int64

	// notConnectedSince holds, for each connection check whose count is above
	// 0, the scrape at which that count went above 0 (see notConnectedSince).
	notConnectedSince map[string]time.Time

	result  throughput
	verdict benthosmonitorservice.FailureVerdict
}

// observe records one successful scrape of the benthos at port, taken at time
// at, and returns the throughput over the window that ends at the newest
// observed scrape, and the failure verdict of that scrape.
//
// A scrape whose at is not after the previous one's is not observed, and the
// previous result and verdict are returned. The engine's windows must receive
// points in time order.
//
// The engine is rebuilt when the port changed or when a counter dropped. A new
// port is a different benthos, and a dropped counter means benthos restarted,
// so the old points belong to a different counter series. Without the rebuild,
// the rate would be the slope from the old count to the new one, a negative
// number.
//
// The engine is also rebuilt when more than constants.BenthosLogWindow passed
// since the previous scrape with no counter drop. The rebuild drops every fired
// signal, so a signal that fired before the gap releases unless its counter
// rose during it. A drop during the gap is a restart, handled as above.
//
// A rebuilt engine first observes a seed (see seed). After a restart its
// failure counters are 0, so errors counted between the restart and this scrape
// count as new. After a long gap they are the previous scrape's values, so only
// a rise during the gap counts. The seed's throughput is unknown, so the first
// rate after a rebuild is 0.
//
// The first scrape on fresh deps is observed alone: failure counters that were
// already high before it have no older point to rise against, so they are not
// judged. The connection checks read that scrape alone and are judged at once.
func observe(d *benthosMonitorDeps, port int, m benthosmonitorservice.Metrics, at time.Time) (throughput, benthosmonitorservice.FailureVerdict) {
	cur := observation{
		at:            at,
		port:          port,
		inputReceived: int(m.InputReceivedTotal()),
		outputSent:    int(m.OutputSentTotal()),
		failures:      readFailures(m),
		connections:   readConnections(m),
	}

	prev := d.prev
	// before holds the failure counters this scrape is compared with for
	// LastIncreaseAt: the previous scrape's, or 0 after a restart, the same
	// values the engine compares this scrape against.
	var before map[string]int64
	// prevNotConnectedSince carries an outage's start forward only from a
	// scrape of the same benthos with no long gap, as the error signals do.
	var prevNotConnectedSince map[string]time.Time
	switch {
	case prev == nil:
		d.engine = newEngine()
	case !at.After(prev.at):
		return prev.result, prev.verdict
	case counterSeriesChanged(prev, &cur):
		rebuild(d, nil, at)
	case at.Sub(prev.at) > constants.BenthosLogWindow:
		before = prev.failures
		rebuild(d, prev.failures, at)
	default:
		before = prev.failures
		prevNotConnectedSince = prev.notConnectedSince
	}

	cur.notConnectedSince = notConnectedSince(prevNotConnectedSince, cur.connections, at)

	fired, _ := d.engine.Observe(sampleFrom(cur), diagnosis.Environment{}, at)

	cur.result = throughput{
		Input: ComponentThroughput{
			MessagesPerSecond: rate(d.engine.Measurement(inputRate)),
			LastCount:         cur.inputReceived,
		},
		Output: ComponentThroughput{
			MessagesPerSecond: rate(d.engine.Measurement(outputRate)),
			LastCount:         cur.outputSent,
		},
	}
	cur.result.IsActive = cur.result.Input.MessagesPerSecond > 0
	cur.verdict = failureVerdict(d.engine, fired, cur)
	if prev != nil {
		cur.verdict.LastIncreaseAt = prev.verdict.LastIncreaseAt
		if rose(before, cur.failures) {
			cur.verdict.LastIncreaseAt = at
		}
	}
	d.prev = &cur

	return cur.result, cur.verdict
}

// counterSeriesChanged reports whether cur's counters cannot be compared with
// prev's: the port changed, or a counter went down because benthos restarted. A
// drop in the output counter alone counts, because a reconfigured benthos can
// leave the input counter unchanged. A dropped failure counter counts for the
// same reason. A new port always comes with a new benthos process: the port is
// part of the benthos service config, so changing it restarts benthos.
func counterSeriesChanged(prev, cur *observation) bool {
	if cur.port != prev.port ||
		cur.inputReceived < prev.inputReceived ||
		cur.outputSent < prev.outputSent {
		return true
	}

	for name, v := range cur.failures {
		if v < prev.failures[name] {
			return true
		}
	}

	return false
}

// A counter missing from before counts as 0. before is nil after a restart,
// so any counter above 0 on the first scrape after it is a rise.
func rose(before, cur map[string]int64) bool {
	for name, v := range cur {
		if v > before[name] {
			return true
		}
	}

	return false
}

// rebuild replaces d's engine with a new one that has observed seed(failures)
// one poll before at. The seed sits one poll before at, not at the previous
// scrape's time, because the engine empties a window whose newest point is
// older than its demote span (pkg/diagnosis, SlidingWindow.age); after a long
// gap the seed would be gone before the real sample arrives.
func rebuild(d *benthosMonitorDeps, failures map[string]int64, at time.Time) {
	d.engine = newEngine()
	d.engine.Observe(seed(failures), diagnosis.Environment{}, at.Add(-pollInterval))
}

// seed is the sample a rebuilt engine observes first: unknown throughput, and
// each failure counter at its value in failures, or 0 when failures has none.
func seed(failures map[string]int64) sample {
	s := sample{
		inputReceived: diagnosis.Unknown(),
		outputSent:    diagnosis.Unknown(),
		failures:      make(map[string]diagnosis.Reading, len(errorCounters)),
	}
	for _, c := range errorCounters {
		s.failures[c.name] = diagnosis.Known(float64(failures[c.name]))
	}

	return s
}

// rate is a measurement's slope, or 0 while the engine marks it untrusted or
// absent, as with fewer than two points.
func rate(r diagnosis.Reduced) float64 {
	v, state := r.Get()
	if state != diagnosis.StateValue {
		return 0
	}

	return v
}
