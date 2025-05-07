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

package redpanda_monitor

import "fmt"

// ComponentThroughput tracks throughput metrics for a single component
type ComponentThroughput struct {
	// LastTick is the last tick when metrics were updated
	LastTick uint64
	// LastCount is the last message count seen
	LastCount int64
	// BytesPerTick is the number of messages processed per tick (averaged over window)
	BytesPerTick float64
	// Window stores the last N message counts for calculating sliding window average
	// Do not deep-copy this field as it is not needed
	Window []MessageCount `copy:"-"`
}

// MessageCount stores a count at a specific tick
type MessageCount struct {
	Tick  uint64
	Count int64
}

// RedpandaMetricsState tracks the state of Redpanda metrics over time
type RedpandaMetricsState struct {
	// Input tracks input throughput
	Input ComponentThroughput
	// Output tracks output throughput
	Output ComponentThroughput
	// LastTick is the last tick when metrics were updated
	LastTick uint64
	// IsActive indicates if any component has shown activity in the last tick
	IsActive bool
	// LastInputChange tracks the tick when we last saw a change in input.received
	LastInputChange uint64
}

// Constants for throughput calculation
const (
	// ThroughputWindowSize is how many ticks to keep in the sliding window
	ThroughputWindowSize = 10 * 60 // assuming 100ms per tick, this is 1 minute
)

// NewRedpandaMetricsState creates a new RedpandaMetricsState
func NewRedpandaMetricsState() *RedpandaMetricsState {
	return &RedpandaMetricsState{
		LastTick:        0,
		IsActive:        false,
		LastInputChange: 0,
	}
}

// UpdateFromMetrics updates the metrics state based on new metrics
func (s *RedpandaMetricsState) UpdateFromMetrics(metrics Metrics, tick uint64) {
	s.updateComponentThroughput(&s.Input, metrics.Throughput.BytesIn, tick)
	s.updateComponentThroughput(&s.Output, metrics.Throughput.BytesOut, tick)

	// Update activity status based on input throughput
	s.IsActive = s.Input.BytesPerTick > 0 || s.Output.BytesPerTick > 0
	fmt.Println("Input:", s.Input.BytesPerTick, "Output:", s.Output.BytesPerTick)
	fmt.Println("Input:", s.Input.LastCount, "Output:", s.Output.LastCount)

	// Update last tick
	s.LastTick = tick
}

// updateComponentThroughput updates throughput metrics for a single component
func (s *RedpandaMetricsState) updateComponentThroughput(throughput *ComponentThroughput, count int64, tick uint64) {
	// Initialize window if needed
	if throughput.Window == nil {
		throughput.Window = make([]MessageCount, 0, ThroughputWindowSize)
	}

	// If this is the first update or if the counter has reset (new count is lower than last count),
	// clear the window and start fresh
	if (throughput.LastTick == 0 && throughput.LastCount == 0) || count < throughput.LastCount {
		throughput.Window = throughput.Window[:0]
		throughput.LastCount = count
		throughput.BytesPerTick = float64(count)
		throughput.Window = append(throughput.Window, MessageCount{Tick: tick, Count: count})
	} else {
		// Add new count to window
		throughput.Window = append(throughput.Window, MessageCount{Tick: tick, Count: count})

		// Keep only the last ThroughputWindowSize entries
		if len(throughput.Window) > ThroughputWindowSize {
			throughput.Window = throughput.Window[1:]
		}

		// Calculate average throughput over the window
		if len(throughput.Window) > 1 {
			first := throughput.Window[0]
			last := throughput.Window[len(throughput.Window)-1]
			tickDiff := last.Tick - first.Tick
			countDiff := last.Count - first.Count

			if tickDiff > 0 {
				throughput.BytesPerTick = float64(countDiff) / float64(tickDiff)
			} else {
				throughput.BytesPerTick = 0
			}
		}

		throughput.LastCount = count
	}
	throughput.LastTick = tick
}
