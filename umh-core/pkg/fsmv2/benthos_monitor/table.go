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

	// failures holds one reading per failureCounters entry, keyed by the
	// counter's name.
	failures map[string]diagnosis.Reading
}

// table declares what the engine measures and judges: the input and output
// rates, each the slope of its counter over windowSpan, and one failure signal
// per failureCounters entry, firing when its counter rose inside the last
// constants.BenthosLogWindow.
func table() diagnosis.Table[sample] {
	signals := make([]diagnosis.Signal[sample], 0, len(failureCounters))
	for _, c := range failureCounters {
		// counter is loop-local so the closure below reads this entry.
		counter := c
		signals = append(signals, diagnosis.Signal[sample]{
			Name: counter.name,
			Instruments: []diagnosis.Instrument[sample]{{
				Measurement: diagnosis.Measurement[sample]{
					Name:    counter.name,
					Extract: func(s sample) diagnosis.Reading { return s.failures[counter.name] },
					// Counter false, not true: a drop means benthos
					// restarted, and observe rebuilds the engine there,
					// so the window never spans two counter series.
					Reduction: increase,
					Span:      constants.BenthosLogWindow,
					Counter:   false,
				},
				Marks: diagnosis.Marks{
					// Fire at the first rise, clear when the window holds no
					// rise again. An inclusive clear reads a flat window as
					// error-free.
					Fire:     diagnosis.Mark{At: 0.5},
					Clear:    diagnosis.Mark{At: 0, Inclusive: true},
					Worst:    100,
					Polarity: diagnosis.HigherIsWorse,
				},
			}},
			DemoteSpan:   constants.BenthosLogWindow,
			Tier:         counter.tier,
			RefireAtOnce: true, // with the 10-minute window, a failure every 11 minutes would otherwise be hidden for 9 of them
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
