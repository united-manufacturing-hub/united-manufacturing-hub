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
// against: when, from which port, the throughput totals it read, and the
// Sample observe returned for it. The error side keeps its own state in
// Tracker.failures.
type observation struct {
	at            time.Time
	port          int
	inputReceived int64
	outputSent    int64

	sample Sample
}

// observe records one successful scrape of the Benthos at port, taken at time
// at, and returns its Sample. A scrape no newer than the previous one keeps
// the previous throughput and Failures, because the windows accept readings
// only in time order. Errors counted before the first scrape are not
// reported.
func (t *Tracker) observe(port int, m Metrics, at time.Time) Sample {
	current := observation{
		at:            at,
		port:          port,
		inputReceived: m.InputReceivedTotal(),
		outputSent:    m.OutputSentTotal(),
	}

	previous := t.previous

	switch {
	case previous == nil:
	case !at.After(previous.at):
		s := previous.sample
		s.At, s.Metrics = at, m

		return s
	case previous.port != port:
		// A new counter series: the throughput engine and the offsets start
		// over. The error state starts over the same way a restart resets
		// it, so errors the new port's Benthos already counted still count.
		t.engine = mustNewEngine(table())
		t.offsets = offsets{}
		t.failures.resetForNewInstance()
	case t.restarted(previous, &current, m):
		// A restart (a Benthos restart resets every counter to 0): a counter
		// that reads lower than before adds its previous value to its
		// offset, so the series the engine is fed never goes down and both
		// throughput windows keep their samples. The error state starts over.
		if current.inputReceived < previous.inputReceived {
			t.offsets.input += previous.inputReceived
		}

		if current.outputSent < previous.outputSent {
			t.offsets.output += previous.outputSent
		}

		t.failures.resetForNewInstance()
	case at.Sub(previous.at) > ErrorWindow:
		// Errors from before the gap no longer count; only rises above the
		// counts the gap started at do.
		t.failures.resetAfterGap()
	}

	// The throughput engine holds no signals, so the discarded (Fired,
	// Readiness) is always zero; observe feeds it for the windows alone.
	_, _ = t.engine.Observe(readings{
		inputReceived: diagnosis.Known(float64(current.inputReceived + t.offsets.input)),
		outputSent:    diagnosis.Known(float64(current.outputSent + t.offsets.output)),
	}, diagnosis.Environment{}, at)

	failures := t.failures.observe(readFailureReading(m, at))

	s := Sample{
		At:                      at,
		Metrics:                 m,
		InputMessagesPerSecond:  messagesPerSecond(t.engine.Measurement(inputMessagesPerSecond)),
		OutputMessagesPerSecond: messagesPerSecond(t.engine.Measurement(outputMessagesPerSecond)),
		Failures:                &failures,
	}
	inputPerSecond, measured := s.InputMessagesPerSecond.Get()
	s.IsActive = measured && inputPerSecond > 0

	current.sample = s
	t.previous = &current

	return s
}

// restarted reports whether any counter of m (input, output or error) is
// lower than at the previous scrape, which is how umh-core detects a Benthos
// restart. Errors the new instance counted before its first scrape count only
// above the old instance's count.
func (t *Tracker) restarted(previous, current *observation, m Metrics) bool {
	return current.inputReceived < previous.inputReceived ||
		current.outputSent < previous.outputSent ||
		t.failures.countsDropped(m)
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
