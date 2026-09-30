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

// Package channelusage judges how full an outbound message channel is.
//
// Depth is a sawtooth (the drain empties the channel in one pass), so a mean
// hides bursts. The channel is judged on its p95 and, separately, on its peak:
// the p95 catches sustained pressure, the peak a single near-overflow.
package channelusage

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

const (
	signalOutboundFull    = "outbound-queue-full"
	instrumentFillPercent = "fill-percent"
	signalOutboundPeak    = "outbound-queue-peak"
	instrumentPeak        = "peak-fill-percent"

	// FirePercent is the p95 fill level above which the queue is degraded:
	// one more burst can overflow it.
	FirePercent = 50.0
	// ClearPercent is where a degraded p95 recovers, below FirePercent to
	// avoid flapping.
	ClearPercent = 30.0

	// PeakFirePercent is the single-reading fill level that degrades the queue
	// regardless of its p95.
	PeakFirePercent = 90.0
	// PeakClearPercent is where a degraded peak recovers.
	PeakClearPercent = 70.0

	// Window is the span the p95 and the peak are taken over. A full queue
	// delays its own report, so the peak must outlive the backlog to reach the
	// frontend.
	Window = 30 * time.Second
	// SampleInterval is the rate callers must sample at; the engine validates
	// the table against it. The transport worker samples once per observation,
	// so this is also its observation interval. Each sample is the highest
	// depth since the previous one, so backlogs that fill and drain between
	// samples still count.
	SampleInterval = 1 * time.Second
	// demoteSpan is how long the signal may go unsampled before its window
	// empties.
	demoteSpan = 60 * time.Second
)

// Sample is one fill-level reading of the tracked channel, in percent.
type Sample struct {
	FillPercent diagnosis.Reading
}

// Verdict is the monitor's judgement of the channel after a sample. The figures
// are zero until Measured is true.
type Verdict struct {
	// Measured reports whether enough samples exist to compute the p95.
	Measured bool `json:"measured"`
	// Degraded reports whether the p95 or the peak has fired and not yet
	// cleared.
	Degraded bool `json:"degraded"`
	// FillDegraded reports whether the p95 has fired and not yet cleared.
	FillDegraded bool `json:"fill_degraded"`
	// PeakDegraded reports whether the peak has fired and not yet cleared.
	PeakDegraded bool `json:"peak_degraded"`
	// FillPercent is the 95th percentile fill level over Window, 0 to 100.
	FillPercent float64 `json:"fill_percent"`
	// PeakPercent is the highest fill level over Window, 0 to 100.
	PeakPercent float64 `json:"peak_percent"`
}

// FillMarks returns the threshold pair the p95 fill level is judged against.
func FillMarks() diagnosis.Marks {
	return diagnosis.Marks{
		Unit:     "%",
		Polarity: diagnosis.HigherIsWorse,
		Fire:     diagnosis.Mark{At: FirePercent},
		Clear:    diagnosis.Mark{At: ClearPercent},
		Worst:    100,
	}
}

// PeakMarks returns the threshold pair the peak fill level is judged against.
func PeakMarks() diagnosis.Marks {
	return diagnosis.Marks{
		Unit:     "%",
		Polarity: diagnosis.HigherIsWorse,
		Fire:     diagnosis.Mark{At: PeakFirePercent},
		Clear:    diagnosis.Mark{At: PeakClearPercent},
		Worst:    100,
	}
}

// FillThreshold states the mark that changes the p95's verdict, such as
// "degrades above 50%".
func (v Verdict) FillThreshold() string {
	return threshold(FillMarks(), v.FillDegraded)
}

// PeakThreshold states the mark that changes the peak's verdict, such as
// "recovers below 70%".
func (v Verdict) PeakThreshold() string {
	return threshold(PeakMarks(), v.PeakDegraded)
}

// threshold names the fire mark while a figure is healthy and the clear mark
// while it is degraded.
func threshold(marks diagnosis.Marks, degraded bool) string {
	mark, verb := marks.Fire, "degrades"
	if degraded {
		mark, verb = marks.Clear, "recovers"
	}

	return fmt.Sprintf("%s %s %.0f%s", verb, diagnosis.MarkSide(mark, marks.Polarity, degraded), mark.At, marks.Unit)
}

// Monitor judges one channel's fill level. Observe must be called from a single
// goroutine.
type Monitor struct {
	engine *diagnosis.Engine[Sample]
	env    diagnosis.Environment
}

// NewMonitor builds a monitor. It fails only on a malformed table.
func NewMonitor() (*Monitor, error) {
	engine, err := diagnosis.NewEngine(table())
	if err != nil {
		return nil, err
	}

	return &Monitor{
		engine: engine,
		env:    diagnosis.NewEnvironment(),
	}, nil
}

// table declares p95 and peak as sibling signals, since the engine judges only
// the first ready instrument of a signal. Either one degrades the queue.
func table() diagnosis.Table[Sample] {
	return diagnosis.Table[Sample]{
		Interval: SampleInterval,
		Signals: []diagnosis.Signal[Sample]{{
			Name:       signalOutboundFull,
			DemoteSpan: demoteSpan,
			Instruments: []diagnosis.Instrument[Sample]{{
				Measurement: diagnosis.Measurement[Sample]{
					Name:      instrumentFillPercent,
					Extract:   func(s Sample) diagnosis.Reading { return s.FillPercent },
					Reduction: diagnosis.P95,
					Span:      Window,
				},
				Marks: FillMarks(),
			}},
		}, {
			Name:       signalOutboundPeak,
			DemoteSpan: demoteSpan,
			Instruments: []diagnosis.Instrument[Sample]{{
				Measurement: diagnosis.Measurement[Sample]{
					Name:      instrumentPeak,
					Extract:   func(s Sample) diagnosis.Reading { return s.FillPercent },
					Reduction: diagnosis.Max,
					Span:      Window,
				},
				Marks: PeakMarks(),
			}},
		}},
	}
}

// Observe records one reading of a channel length against its capacity and
// returns the resulting verdict. A capacity of 0 records as 0%. A nil monitor
// returns an unmeasured verdict.
func (m *Monitor) Observe(length, capacity int, at time.Time) Verdict {
	if m == nil {
		return Verdict{}
	}

	var fillPercent float64
	if capacity > 0 {
		fillPercent = float64(length) / float64(capacity) * 100
	}

	fired, _ := m.engine.Observe(Sample{FillPercent: diagnosis.Known(fillPercent)}, m.env, at)

	p95, state := m.engine.Reduction(signalOutboundFull, instrumentFillPercent).Get()
	if state != diagnosis.StateValue {
		return Verdict{}
	}

	peak, _ := m.engine.Reduction(signalOutboundPeak, instrumentPeak).Get()

	verdict := Verdict{
		Measured:    true,
		Degraded:    len(fired) > 0,
		FillPercent: p95,
		PeakPercent: peak,
	}

	for _, f := range fired {
		switch f.Signal {
		case signalOutboundFull:
			verdict.FillDegraded = true
		case signalOutboundPeak:
			verdict.PeakDegraded = true
		}
	}

	return verdict
}
