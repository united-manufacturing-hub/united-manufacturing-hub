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

// The parsed metrics of one scrape, and the totals read off them.

package benthosmetrics

// Metrics is the parsed /metrics of one Benthos instance. Inputs and Outputs
// are keyed by Benthos' path label, such as "root.output"; a switch, broker
// or fallback has one entry per leaf input or output. The *Total methods sum
// across them. names.go cites where Benthos defines the path and label labels.
type Metrics struct {
	Process ProcessMetrics           `json:"process"`
	Outputs map[string]OutputMetrics `json:"outputs,omitempty"`
	Inputs  map[string]InputMetrics  `json:"inputs,omitempty"`
}

// InputMetrics contains the metrics of one leaf input path.
type InputMetrics struct {
	Path             string  `json:"path"`
	Label            string  `json:"label"`
	ConnectionFailed int64   `json:"connection_failed"`
	ConnectionLost   int64   `json:"connection_lost"`
	ConnectionUp     int64   `json:"connection_up"`
	LatencyNS        Latency `json:"latency_ns"`
	Received         int64   `json:"received"`
}

// OutputMetrics contains the metrics of one leaf output path.
type OutputMetrics struct {
	Path             string  `json:"path"`
	Label            string  `json:"label"`
	BatchSent        int64   `json:"batch_sent"`
	ConnectionFailed int64   `json:"connection_failed"`
	ConnectionLost   int64   `json:"connection_lost"`
	ConnectionUp     int64   `json:"connection_up"`
	Error            int64   `json:"error"`
	LatencyNS        Latency `json:"latency_ns"`
	Sent             int64   `json:"sent"`
}

// ProcessMetrics holds one entry per processor, keyed by processor path.
type ProcessMetrics struct {
	Processors map[string]ProcessorMetrics `json:"processors"` // key is the processor path (e.g. "root.pipeline.processors.0")
}

// ProcessorMetrics contains metrics for a single processor.
type ProcessorMetrics struct {
	Label         string  `json:"label"`
	Received      int64   `json:"received"`
	BatchReceived int64   `json:"batch_received"`
	Sent          int64   `json:"sent"`
	BatchSent     int64   `json:"batch_sent"`
	Error         int64   `json:"error"`
	LatencyNS     Latency `json:"latency_ns"`
}

// Latency summarises one latency series in nanoseconds: percentiles, sum and
// count.
type Latency struct {
	P50   float64 `json:"p50"`
	P90   float64 `json:"p90"`
	P99   float64 `json:"p99"`
	Sum   float64 `json:"sum"`
	Count int64   `json:"count"`
}

// InputReceivedTotal returns the sum of Received across every leaf input.
func (m Metrics) InputReceivedTotal() int64 {
	var total int64
	for _, in := range m.Inputs {
		total += in.Received
	}

	return total
}

// OutputSentTotal returns the sum of Sent across every leaf output.
func (m Metrics) OutputSentTotal() int64 {
	var total int64
	for _, out := range m.Outputs {
		total += out.Sent
	}

	return total
}
