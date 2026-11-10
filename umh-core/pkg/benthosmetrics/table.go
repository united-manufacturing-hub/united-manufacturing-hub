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

// What the engines measure: the diagnosis tables and the readings they read.

package benthosmetrics

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// rateWindow is how far back the rates look. It is a duration, not a readings
// count, so a skipped poll does not stretch the window past one minute.
const rateWindow = 60 * time.Second

// ErrorWindow is how long an error entry stays after the last new error: the
// same 10 minutes the log check reads (constants.BenthosLogWindow).
const ErrorWindow = constants.BenthosLogWindow

// Names of the engine's measurements.
const (
	inputRate  = "input_rate"
	outputRate = "output_rate"
)

// readings is one scrape's counters as the engines read them.
type readings struct {
	inputReceived diagnosis.Reading
	outputSent    diagnosis.Reading

	// errors holds one reading per errorCounters entry, keyed by the
	// counter's name.
	errors map[string]diagnosis.Reading
}

// rateTable declares what the rate engine measures: the input and output
// rates, each the slope of its counter over rateWindow. fsmv2/diagnosis
// computes the time windows; this table says what to measure and over how
// long.
func rateTable() diagnosis.Table[readings] {
	return diagnosis.Table[readings]{
		Interval: ScrapeInterval,
		Measurements: []diagnosis.Measurement[readings]{
			{
				Name:      inputRate,
				Extract:   func(s readings) diagnosis.Reading { return s.inputReceived },
				Reduction: diagnosis.Slope,
				Span:      rateWindow,
			},
			{
				Name:      outputRate,
				Extract:   func(s readings) diagnosis.Reading { return s.outputSent },
				Reduction: diagnosis.Slope,
				Span:      rateWindow,
			},
		},
	}
}

// errorTable declares what the error engine measures: one error signal per
// errorCounters entry. An error signal is on from a new error until
// ErrorWindow passes without one, and comes back at once on the next error.
func errorTable() diagnosis.Table[readings] {
	increase := newIncrease()
	signals := make([]diagnosis.Signal[readings], 0, len(errorCounters))
	for _, counter := range errorCounters {
		signals = append(signals, diagnosis.Signal[readings]{
			Name: counter.name,
			Instruments: []diagnosis.Instrument[readings]{{
				Measurement: diagnosis.Measurement[readings]{
					Name:    counter.name,
					Extract: func(s readings) diagnosis.Reading { return s.errors[counter.name] },
					// Counter is false: observe restarts the error engine
					// whenever a counter drops (see counterSeriesChanged), so
					// this window never sees a counter reset.
					Reduction: increase,
					Span:      ErrorWindow,
				},
				Marks: diagnosis.Marks{
					// Fire once the counter rose by 1 or more; clear once the
					// increase is back to 0.
					Fire:  diagnosis.Mark{At: 0.5},
					Clear: diagnosis.Mark{At: 0, Inclusive: true},
					// NewEngine requires a Worst; nothing reads the severity
					// it scales, judgeFailures orders entries by Count.
					Worst:    100,
					Polarity: diagnosis.HigherIsWorse,
				},
			}},
			DemoteSpan: ErrorWindow,
		})
	}

	return diagnosis.Table[readings]{
		Interval: ScrapeInterval,
		Signals:  signals,
	}
}

// newEngine builds an engine from tbl. Both tables are fixed at compile time,
// so a construction error is a bug in this package, not a runtime condition.
func newEngine(tbl diagnosis.Table[readings]) *diagnosis.Engine[readings] {
	e, err := diagnosis.NewEngine(tbl)
	if err != nil {
		panic(fmt.Sprintf("benthosmetrics: invalid diagnosis table: %v", err))
	}

	return e
}
