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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// windowSpan is how far back the throughput rates look. It is a duration, not a
// sample count, so a skipped poll does not stretch the window past one minute.
const windowSpan = 60 * time.Second

// Names of the engine's measurements.
const (
	inputRate  = "input_rate"
	outputRate = "output_rate"
)

// sample is one scrape's counters as the engine reads them.
type sample struct {
	inputReceived diagnosis.Reading
	outputSent    diagnosis.Reading

	// errors holds one reading per errorCounters entry, keyed by the
	// counter's name.
	errors map[string]diagnosis.Reading
}

// table declares what the engine measures and judges: the input and output
// rates, each the slope of its counter over windowSpan, and one error signal
// per errorCounters entry, firing when its counter rose inside the last
// constants.BenthosLogWindow.
func table() diagnosis.Table[sample] {
	signals := make([]diagnosis.Signal[sample], 0, len(errorCounters))
	for _, counter := range errorCounters {
		signals = append(signals, diagnosis.Signal[sample]{
			Name: counter.name,
			Instruments: []diagnosis.Instrument[sample]{{
				Measurement: diagnosis.Measurement[sample]{
					Name:    counter.name,
					Extract: func(s sample) diagnosis.Reading { return s.errors[counter.name] },
					// Counter is false: observe rebuilds the engine
					// whenever a counter drops (see counterSeriesChanged),
					// so this window never sees a counter reset.
					Reduction: increase,
					Span:      constants.BenthosLogWindow,
					Counter:   false,
				},
				Marks: diagnosis.Marks{
					// Fire once the counter rose by 1 or more (an increase
					// above 0.5). Clear once the increase is back to 0.
					// Inclusive makes exactly 0 count.
					Fire:  diagnosis.Mark{At: 0.5},
					Clear: diagnosis.Mark{At: 0, Inclusive: true},
					// NewEngine requires Worst. Nothing reads the severity
					// it scales: failureVerdict orders entries by Count.
					Worst:    100,
					Polarity: diagnosis.HigherIsWorse,
				},
			}},
			DemoteSpan:   constants.BenthosLogWindow,
			RefireAtOnce: true, // a new error right after the signal clears shows at once, not after a 10-minute wait
		})
	}

	return diagnosis.Table[sample]{
		Interval: pollInterval,
		Signals:  signals,
		Measurements: []diagnosis.Measurement[sample]{
			{
				Name:      inputRate,
				Extract:   func(s sample) diagnosis.Reading { return s.inputReceived },
				Reduction: diagnosis.Slope,
				Span:      windowSpan,
			},
			{
				Name:      outputRate,
				Extract:   func(s sample) diagnosis.Reading { return s.outputSent },
				Reduction: diagnosis.Slope,
				Span:      windowSpan,
			},
		},
	}
}

// newEngine builds an engine from table. table is fixed at compile time, so an
// error here is a bug in table.
func newEngine() *diagnosis.Engine[sample] {
	e, err := diagnosis.NewEngine(table())
	if err != nil {
		panic(fmt.Sprintf("benthos_monitor: invalid diagnosis table: %v", err))
	}

	return e
}
