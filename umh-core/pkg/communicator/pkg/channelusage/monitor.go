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
	"sync"
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

	// Window is the span the p95 is taken over.
	Window = 30 * time.Second
	// PeakWindow is how long the peak is remembered. A full queue delays its own
	// report, so the peak must outlive the backlog to reach the frontend.
	PeakWindow = 30 * time.Second
	// SampleInterval is the rate callers must sample at; the engine validates
	// the table against it. Backlogs shorter than one interval are invisible.
	SampleInterval = 100 * time.Millisecond
	// demoteSpan is how long the signal may go unsampled before its window
	// empties.
	demoteSpan = 60 * time.Second
)

// Sample is one fill-level reading of the tracked channel, in percent.
type Sample struct {
	FillPercent diagnosis.Reading
}

// Verdict is the monitor's judgement of the channel after a sample.
type Verdict struct {
	// P95FillPercent is the 95th percentile fill level over Window, 0 to 100.
	P95FillPercent float64
	// PeakFillPercent is the highest fill level over PeakWindow, 0 to 100.
	PeakFillPercent float64
	// Degraded reports whether the p95 or the peak has fired and not yet
	// cleared.
	Degraded bool
}

// Monitor judges one channel's fill level. Observe must be called from a single
// goroutine; Verdict is safe from any.
type Monitor struct {
	engine  *diagnosis.Engine[Sample]
	env     diagnosis.Environment
	mu      sync.RWMutex
	verdict Verdict
	known   bool
}

// NewMonitor builds a monitor. It fails only on a malformed table.
func NewMonitor() (*Monitor, error) {
	declared, err := table()
	if err != nil {
		return nil, err
	}

	engine, err := diagnosis.NewEngine(declared)
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
func table() (diagnosis.Table[Sample], error) {
	peak, err := diagnosis.NewReduction("max", 1, func(points []diagnosis.Point) float64 {
		highest := points[0].Value
		for _, p := range points[1:] {
			if p.Value > highest {
				highest = p.Value
			}
		}

		return highest
	})
	if err != nil {
		return diagnosis.Table[Sample]{}, err
	}

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
				Marks: diagnosis.Marks{
					Unit:     "%",
					Polarity: diagnosis.HigherIsWorse,
					Fire:     diagnosis.Mark{At: FirePercent},
					Clear:    diagnosis.Mark{At: ClearPercent},
					Worst:    100,
				},
			}},
		}, {
			Name:       signalOutboundPeak,
			DemoteSpan: demoteSpan,
			Instruments: []diagnosis.Instrument[Sample]{{
				Measurement: diagnosis.Measurement[Sample]{
					Name:      instrumentPeak,
					Extract:   func(s Sample) diagnosis.Reading { return s.FillPercent },
					Reduction: peak,
					Span:      PeakWindow,
				},
				Marks: diagnosis.Marks{
					Unit:     "%",
					Polarity: diagnosis.HigherIsWorse,
					Fire:     diagnosis.Mark{At: PeakFirePercent},
					Clear:    diagnosis.Mark{At: PeakClearPercent},
					Worst:    100,
				},
			}},
		}},
	}, nil
}

// Observe records one reading from len(ch) and cap(ch). A capacity of 0 records
// as 0%.
func (m *Monitor) Observe(length, capacity int, at time.Time) {
	if m == nil {
		return
	}

	var fillPercent float64
	if capacity > 0 {
		fillPercent = float64(length) / float64(capacity) * 100
	}

	fired, _ := m.engine.Observe(Sample{FillPercent: diagnosis.Known(fillPercent)}, m.env, at)

	p95FillPercent, state := m.engine.Reduction(signalOutboundFull, instrumentFillPercent).Get()
	peakFillPercent, peakState := m.engine.Reduction(signalOutboundPeak, instrumentPeak).Get()

	m.mu.Lock()
	defer m.mu.Unlock()

	if state != diagnosis.StateValue {
		m.known = false

		return
	}

	if peakState != diagnosis.StateValue {
		peakFillPercent = p95FillPercent
	}

	m.verdict = Verdict{
		P95FillPercent:  p95FillPercent,
		PeakFillPercent: peakFillPercent,
		Degraded:        len(fired) > 0,
	}
	m.known = true
}

// Verdict returns the last verdict, or false until enough samples exist to
// compute the p95.
func (m *Monitor) Verdict() (Verdict, bool) {
	if m == nil {
		return Verdict{}, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.verdict, m.known
}
