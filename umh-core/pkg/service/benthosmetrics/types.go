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

package benthosmetrics

// Metrics contains information about the metrics of the Benthos service.
// Inputs and Outputs are keyed by benthos `path` label (one entry per leaf
// AsyncReader / AsyncWriter). For switch/broker/fallback configs, multiple
// entries exist; for a single-input or single-output config there is one
// entry at "root.input" / "root.output". Aggregate values are exposed via
// the *Total helpers on Metrics.
type Metrics struct {
	Process ProcessMetrics            `json:"process,omitempty"`
	Outputs map[string]OutputInstance `json:"outputs,omitempty"`
	Inputs  map[string]InputInstance  `json:"inputs,omitempty"`
}

// InputInstance contains per-path input metrics for one benthos AsyncReader.
type InputInstance struct {
	Path             string  `json:"path"`
	Label            string  `json:"label"`
	ConnectionFailed int64   `json:"connection_failed"`
	ConnectionLost   int64   `json:"connection_lost"`
	ConnectionUp     int64   `json:"connection_up"`
	LatencyNS        Latency `json:"latency_ns"`
	Received         int64   `json:"received"`
}

// OutputInstance contains per-path output metrics for one benthos AsyncWriter.
type OutputInstance struct {
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

// ProcessMetrics contains processor-specific metrics.
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

// Latency contains latency metrics.
type Latency struct {
	P50   float64 `json:"p50"`   // 50th percentile
	P90   float64 `json:"p90"`   // 90th percentile
	P99   float64 `json:"p99"`   // 99th percentile
	Sum   float64 `json:"sum"`   // Total sum
	Count int64   `json:"count"` // Number of samples
}

// InputReceivedTotal returns the sum of Received across every input instance.
func (m Metrics) InputReceivedTotal() int64 {
	var total int64
	for _, in := range m.Inputs {
		total += in.Received
	}

	return total
}

// InputConnectionUpTotal returns the sum of ConnectionUp across every input.
func (m Metrics) InputConnectionUpTotal() int64 {
	var total int64
	for _, in := range m.Inputs {
		total += in.ConnectionUp
	}

	return total
}

// InputConnectionLostTotal returns the sum of ConnectionLost across every input.
func (m Metrics) InputConnectionLostTotal() int64 {
	var total int64
	for _, in := range m.Inputs {
		total += in.ConnectionLost
	}

	return total
}

// OutputSentTotal returns the sum of Sent across every output instance.
func (m Metrics) OutputSentTotal() int64 {
	var total int64
	for _, out := range m.Outputs {
		total += out.Sent
	}

	return total
}

// OutputBatchSentTotal returns the sum of BatchSent across every output.
func (m Metrics) OutputBatchSentTotal() int64 {
	var total int64
	for _, out := range m.Outputs {
		total += out.BatchSent
	}

	return total
}

// OutputConnectionUpTotal returns the sum of ConnectionUp across every output.
func (m Metrics) OutputConnectionUpTotal() int64 {
	var total int64
	for _, out := range m.Outputs {
		total += out.ConnectionUp
	}

	return total
}

// OutputConnectionLostTotal returns the sum of ConnectionLost across every output.
func (m Metrics) OutputConnectionLostTotal() int64 {
	var total int64
	for _, out := range m.Outputs {
		total += out.ConnectionLost
	}

	return total
}

// OutputErrorTotal returns the sum of Error across every output. Used by
// benthos.IsMetricsErrorFree to decide whether the FSM should treat the
// component as healthy.
func (m Metrics) OutputErrorTotal() int64 {
	var total int64
	for _, out := range m.Outputs {
		total += out.Error
	}

	return total
}

// HealthCheck contains information about the health of the Benthos service
// https://docs.redpanda.com/redpanda-connect/guides/monitoring/
type HealthCheck struct {
	// Version contains the version of the Benthos service
	Version string
	// ReadyError contains any error message from the ready check
	ReadyError string `json:"ready_error,omitempty"`
	// ConnectionStatuses contains the detailed connection status of inputs and outputs
	ConnectionStatuses []ConnStatus `json:"connection_statuses,omitempty"`
	// IsLive is true if the Benthos service is live
	IsLive bool
	// IsReady is true if the Benthos service is ready to process data
	IsReady bool
}

// VersionResponse represents the JSON structure returned by the /version endpoint.
type VersionResponse struct {
	Version string `json:"version"`
	Built   string `json:"built"`
}

// ReadyResponse represents the JSON structure returned by the /ready endpoint.
type ReadyResponse struct {
	Error    string       `json:"error,omitempty"`
	Statuses []ConnStatus `json:"statuses"`
}

// ConnStatus represents the connection status of a single input or output.
type ConnStatus struct {
	Label     string `json:"label"`
	Path      string `json:"path"`
	Error     string `json:"error,omitempty"`
	Connected bool   `json:"connected"`
}

// BenthosMetrics contains information about the metrics of the Benthos service.
type BenthosMetrics struct {
	// MetricsState contains the state of the metrics
	MetricsState *BenthosMetricsState
	// Metrics contains the metrics of the Benthos service
	Metrics Metrics
}

// Scan is one observation of a benthos instance: the parsed metrics, the
// health check, and whether the /metrics scrape succeeded this observation.
type Scan struct {
	Metrics          Metrics
	HealthCheck      HealthCheck
	MetricsAvailable bool
}
