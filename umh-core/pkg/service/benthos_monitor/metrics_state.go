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

package benthos_monitor

// ComponentThroughput tracks throughput metrics for a single component
type ComponentThroughput struct {
	// LastTick is the last tick when metrics were updated
	LastTick uint64
	// LastCount is the last message count seen
	LastCount int64
	// LastBatchCount is the last batch count seen
	LastBatchCount int64
	// MessagesPerTick is the number of messages processed per tick (averaged over window)
	MessagesPerTick float64
	// BatchesPerTick is the number of batches processed per tick (averaged over window)
	BatchesPerTick float64
	// Window stores the last N message counts for calculating sliding window average
	Window []MessageCount
}

// MessageCount stores a count at a specific tick
type MessageCount struct {
	Tick       uint64
	Count      int64
	BatchCount int64
}

// BenthosMetricsState tracks the state of Benthos metrics over time
type BenthosMetricsState struct {
	// Input tracks input throughput
	Input ComponentThroughput
	// Output tracks output throughput
	Output ComponentThroughput
	// Processors tracks processor throughput
	Processors map[string]ComponentThroughput
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

// NewBenthosMetricsState creates a new BenthosMetricsState
func NewBenthosMetricsState() *BenthosMetricsState {
	return &BenthosMetricsState{
		Processors:      make(map[string]ComponentThroughput),
		LastTick:        0,
		IsActive:        false,
		LastInputChange: 0,
	}
}

// UpdateFromMetrics updates the metrics state based on new metrics
func (s *BenthosMetricsState) UpdateFromMetrics(metrics BenthosMetrics, tick uint64) {
	// Update component throughput
	s.updateComponentThroughput(&s.Input, metrics.Input.Received, 0, tick)
	s.updateComponentThroughput(&s.Output, metrics.Output.Sent, metrics.Output.BatchSent, tick)

	// Update processor throughput
	newProcessors := make(map[string]ComponentThroughput)
	for path, processor := range metrics.Process.Processors {
		var throughput ComponentThroughput
		if existing, exists := s.Processors[path]; exists {
			throughput = existing
		}
		s.updateComponentThroughput(&throughput, processor.Sent, processor.BatchSent, tick)
		newProcessors[path] = throughput
	}
	s.Processors = newProcessors

	// Update activity status based on input throughput
	s.IsActive = s.Input.MessagesPerTick > 0

	// Update last tick
	s.LastTick = tick
}

// updateComponentThroughput updates throughput metrics for a single component
func (s *BenthosMetricsState) updateComponentThroughput(throughput *ComponentThroughput, count, batchCount int64, tick uint64) {
	// Initialize window if needed
	if throughput.Window == nil {
		throughput.Window = make([]MessageCount, 0, ThroughputWindowSize)
	}

	// If this is the first update or if the counter has reset (new count is lower than last count),
	// clear the window and start fresh
	if (throughput.LastTick == 0 && throughput.LastCount == 0) || count < throughput.LastCount {
		throughput.Window = throughput.Window[:0]
		throughput.LastCount = count
		throughput.LastBatchCount = batchCount
		throughput.MessagesPerTick = float64(count)
		throughput.BatchesPerTick = float64(batchCount)
		throughput.Window = append(throughput.Window, MessageCount{Tick: tick, Count: count, BatchCount: batchCount})
	} else {
		// Add new count to window
		throughput.Window = append(throughput.Window, MessageCount{Tick: tick, Count: count, BatchCount: batchCount})

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
			batchDiff := last.BatchCount - first.BatchCount

			if tickDiff > 0 {
				throughput.MessagesPerTick = float64(countDiff) / float64(tickDiff)
				throughput.BatchesPerTick = float64(batchDiff) / float64(tickDiff)
			} else {
				throughput.MessagesPerTick = 0
				throughput.BatchesPerTick = 0
			}
		}

		throughput.LastCount = count
		throughput.LastBatchCount = batchCount
	}
	throughput.LastTick = tick
}
