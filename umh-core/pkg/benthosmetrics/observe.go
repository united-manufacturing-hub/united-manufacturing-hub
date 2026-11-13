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

// Turning one observed scrape into message throughput.

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
	sample        Sample
}

// observe records one successful scrape of the Benthos at port, taken at time
// at, and returns its Sample. A scrape no newer than the previous one keeps
// the previous throughput, because the windows accept readings only in time
// order.
func (t *Tracker) observe(port int, m Metrics, at time.Time) Sample {
	input := m.InputReceivedTotal()
	output := m.OutputSentTotal()

	if t.prev != nil {
		if !at.After(t.prev.at) {
			s := t.prev.sample
			s.At, s.Metrics = at, m

			return s
		}

		t.adjustForRestart(port, input, output)
	}

	// The engine holds no signals, so the discarded (Fired, Readiness) is
	// always zero; observe feeds it for the windows alone.
	_, _ = t.engine.Observe(readings{
		inputReceived: diagnosis.Known(float64(input + t.offsets.input)),
		outputSent:    diagnosis.Known(float64(output + t.offsets.output)),
	}, diagnosis.Environment{}, at)

	s := Sample{
		At:                      at,
		Metrics:                 m,
		InputMessagesPerSecond:  messagesPerSecond(t.engine.Measurement(inputMessagesPerSecond)),
		OutputMessagesPerSecond: messagesPerSecond(t.engine.Measurement(outputMessagesPerSecond)),
	}
	inputPerSecond, measured := s.InputMessagesPerSecond.Get()
	s.IsActive = measured && inputPerSecond > 0

	t.prev = &observation{
		at:            at,
		port:          port,
		inputReceived: input,
		outputSent:    output,
		sample:        s,
	}

	return s
}

// adjustForRestart compares the totals of a newer scrape with t.prev. A new
// port is a new counter series, so the engine and both offsets start over. A
// counter that reads lower than before restarted (a Benthos restart resets it
// to 0): its previous value is added to its offset, so the series the engine
// is fed never goes down and both windows keep their samples.
func (t *Tracker) adjustForRestart(port int, input, output int64) {
	if port != t.prev.port {
		t.engine = mustNewEngine()
		t.offsets = offsets{}

		return
	}

	if input < t.prev.inputReceived {
		t.offsets.input += t.prev.inputReceived
	}

	if output < t.prev.outputSent {
		t.offsets.output += t.prev.outputSent
	}
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
