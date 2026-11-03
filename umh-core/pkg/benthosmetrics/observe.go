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

// Turning one observed scrape into rates.

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
	result        rates
}

// observe records one successful scrape of the Benthos at port, taken at time
// at, and returns the rates as of that scrape. A scrape no newer than the
// previous one is ignored and the previous rates are returned, because the
// windows accept readings only in time order.
func (t *Tracker) observe(port int, m Metrics, at time.Time) rates {
	input := m.InputReceivedTotal()
	output := m.OutputSentTotal()

	prev := t.prev
	if prev != nil {
		if !at.After(prev.at) {
			return prev.result
		}

		if port != prev.port {
			// A port change is a new counter series: the engine and both
			// offsets start over.
			t.engine = newEngine()
			t.offsets = offsets{}
		} else {
			// A counter that reads lower than its previous value restarted
			// (a Benthos restart resets it to 0). The drop is folded into
			// that counter's offset, so the series the engine is fed never
			// goes down; both rate windows keep their samples.
			if input < prev.inputReceived {
				t.offsets.input += prev.inputReceived
			}

			if output < prev.outputSent {
				t.offsets.output += prev.outputSent
			}
		}
	}

	// The engine holds no signals, so the discarded (Fired, Readiness) is
	// always zero; observe feeds it for the windows alone.
	_, _ = t.engine.Observe(readings{
		inputReceived: diagnosis.Known(float64(input + t.offsets.input)),
		outputSent:    diagnosis.Known(float64(output + t.offsets.output)),
	}, diagnosis.Environment{}, at)

	result := rates{
		Input:  rate{MessagesPerSecond: rateOf(t.engine.Measurement(inputRate))},
		Output: rate{MessagesPerSecond: rateOf(t.engine.Measurement(outputRate))},
	}
	// IsActive keeps FSMv1's definition of an active Benthos (its
	// metrics_state.go: the input moves); the output rate does not count.
	result.IsActive = result.Input.MessagesPerSecond > 0

	t.prev = &observation{
		at:            at,
		port:          port,
		inputReceived: input,
		outputSent:    output,
		result:        result,
	}

	return result
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
