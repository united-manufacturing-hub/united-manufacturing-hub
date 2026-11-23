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

// throughputWindow is how far back the throughput looks. It is a duration,
// not a readings count, so a skipped poll does not stretch the window past
// one minute.
const throughputWindow = 60 * time.Second

// ErrorWindow is how long an error entry stays after the last new error: the
// same 10 minutes the log check reads (constants.BenthosLogWindow).
const ErrorWindow = constants.BenthosLogWindow

// Names of the engine's measurements.
const (
	inputMessagesPerSecond  = "input_messages_per_second"
	outputMessagesPerSecond = "output_messages_per_second"
)

// readings is one scrape's throughput counters as the throughput engine
// reads them.
type readings struct {
	inputReceived diagnosis.Reading
	outputSent    diagnosis.Reading
}

// errorReadings is one scrape's error counters as the error engine reads
// them: one reading per errorCounters entry, keyed by the counter's name.
type errorReadings struct {
	errors map[Signal]diagnosis.Reading
}

// table declares what the engine measures: the input and output messages per
// second, each the slope of its counter over throughputWindow. fsmv2/diagnosis
// computes the time windows; this table says what to measure and over how
// long.
func table() diagnosis.Table[readings] {
	return diagnosis.Table[readings]{
		Interval: ScrapeInterval,
		Measurements: []diagnosis.Measurement[readings]{
			{
				Name:      inputMessagesPerSecond,
				Extract:   func(s readings) diagnosis.Reading { return s.inputReceived },
				Reduction: diagnosis.Slope,
				Span:      throughputWindow,
			},
			{
				Name:      outputMessagesPerSecond,
				Extract:   func(s readings) diagnosis.Reading { return s.outputSent },
				Reduction: diagnosis.Slope,
				Span:      throughputWindow,
			},
		},
	}
}

// errorTable declares what the error engine measures: one error signal per
// errorCounters entry. An error signal is on from a new error until
// ErrorWindow passes without one, and comes back at once on the next error.
func errorTable() diagnosis.Table[errorReadings] {
	increase := newIncrease()

	signals := make([]diagnosis.Signal[errorReadings], 0, len(errorCounters))
	for _, counter := range errorCounters {
		signals = append(signals, diagnosis.Signal[errorReadings]{
			Name: string(counter.name),
			Instruments: []diagnosis.Instrument[errorReadings]{{
				Measurement: diagnosis.Measurement[errorReadings]{
					Name:    string(counter.name),
					Extract: func(s errorReadings) diagnosis.Reading { return s.errors[counter.name] },
					// Counter is false: observe resets the failureTracker
					// whenever a counter drops (see restarted), so this
					// window never sees a counter reset.
					Reduction: increase,
					Span:      ErrorWindow,
				},
				Marks: diagnosis.Marks{
					// Fire once the counter rose by 1 or more; clear once the
					// increase is back to 0.
					Fire:  diagnosis.Mark{At: 0.5},
					Clear: diagnosis.Mark{At: 0, Inclusive: true},
					// NewEngine requires a Worst; nothing reads the severity
					// it scales, failureTracker.judge orders entries by Count.
					Worst:    100,
					Polarity: diagnosis.HigherIsWorse,
				},
			}},
			DemoteSpan: ErrorWindow,
		})
	}

	return diagnosis.Table[errorReadings]{
		Interval: ScrapeInterval,
		Signals:  signals,
	}
}

// newIncrease returns the diagnosis reduction for error signals: how much a
// counter went up from the oldest to the newest reading in its window. It
// needs at least two readings, and panics on a constructor error, a bug here.
func newIncrease() diagnosis.Reduction {
	r, err := diagnosis.NewReduction("increase", 2, func(p []diagnosis.Point) float64 {
		return p[len(p)-1].Value - p[0].Value
	})
	if err != nil {
		panic(fmt.Sprintf("benthosmetrics: invalid increase reduction: %v", err))
	}

	return r
}

// errorReadingsOf turns one scrape's error counters into the readings the
// error engine reads.
func errorReadingsOf(errorCounts map[Signal]int64) errorReadings {
	s := errorReadings{errors: make(map[Signal]diagnosis.Reading, len(errorCounts))}
	for name, v := range errorCounts {
		s.errors[name] = diagnosis.Known(float64(v))
	}

	return s
}

// seedErrors is the readings a reset error engine observes first: each
// error counter at its value in errorCounts, or 0 when errorCounts has none.
func seedErrors(errorCounts map[Signal]int64) errorReadings {
	s := errorReadings{errors: make(map[Signal]diagnosis.Reading, len(errorCounters))}
	for _, c := range errorCounters {
		s.errors[c.name] = diagnosis.Known(float64(errorCounts[c.name]))
	}

	return s
}

// mustNewEngine builds an engine from tbl and panics on error: both tables
// are fixed at compile time, so an error is a bug in this package.
func mustNewEngine[S any](tbl diagnosis.Table[S]) *diagnosis.Engine[S] {
	e, err := diagnosis.NewEngine(tbl)
	if err != nil {
		panic(fmt.Sprintf("benthosmetrics: invalid diagnosis table: %v", err))
	}

	return e
}
