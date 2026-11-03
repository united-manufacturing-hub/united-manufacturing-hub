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

// The Benthos-defined series, label and endpoint names this package reads,
// in the one place they are spelled. They are unexported string literals in
// Benthos' internal/ packages, which Go forbids importing, so they are
// copied here instead; the fixture spec (metrics_fixture_test.go) fails
// when a version bump renames one.
//
// Version chain: umh-core/Makefile BENTHOS_UMH_VERSION 0.16.1 pins
// benthos-umh v0.16.1, whose go.mod pins github.com/redpanda-data/benthos/v4
// v4.84.0. Every name below is defined in that tree:
//
//   - input series: internal/component/input/async_reader.go L98-L102
//     (https://github.com/redpanda-data/benthos/blob/v4.84.0/internal/component/input/async_reader.go#L98-L102);
//   - output series: internal/component/output/async_writer.go L105-L112
//     (https://github.com/redpanda-data/benthos/blob/v4.84.0/internal/component/output/async_writer.go#L105-L112);
//   - processor series: internal/component/processor/auto_observed.go L81-L86
//     (https://github.com/redpanda-data/benthos/blob/v4.84.0/internal/component/processor/auto_observed.go#L81-L86);
//   - the path and label labels: internal/manager/type.go L410-L432
//     (https://github.com/redpanda-data/benthos/blob/v4.84.0/internal/manager/type.go#L410-L432);
//   - the /ping, /version and /metrics endpoints: internal/api/api.go L98-L226
//     (https://github.com/redpanda-data/benthos/blob/v4.84.0/internal/api/api.go#L98-L226);
//   - the /ready endpoint: internal/stream/type.go L60-L109
//     (https://github.com/redpanda-data/benthos/blob/v4.84.0/internal/stream/type.go#L60-L109).
//
// The public docs list the same names:
// https://docs.redpanda.com/redpanda-connect/components/metrics/about/ and
// https://docs.redpanda.com/redpanda-connect/components/http/about/.

package benthosmetrics

// Input series, from internal/component/input/async_reader.go.
const (
	inputConnectionFailed = "input_connection_failed"
	inputConnectionLost   = "input_connection_lost"
	inputConnectionUp     = "input_connection_up"
	inputLatency          = "input_latency_ns"
	inputLatencySum       = "input_latency_ns_sum"
	inputLatencyCount     = "input_latency_ns_count"
	inputReceived         = "input_received"
)

// Output series, from internal/component/output/async_writer.go.
const (
	outputBatchSent        = "output_batch_sent"
	outputConnectionFailed = "output_connection_failed"
	outputConnectionLost   = "output_connection_lost"
	outputConnectionUp     = "output_connection_up"
	outputError            = "output_error"
	outputLatency          = "output_latency_ns"
	outputLatencySum       = "output_latency_ns_sum"
	outputLatencyCount     = "output_latency_ns_count"
	outputSent             = "output_sent"
)

// Processor series, from internal/component/processor/auto_observed.go.
const (
	processorBatchReceived = "processor_batch_received"
	processorBatchSent     = "processor_batch_sent"
	processorError         = "processor_error"
	processorLatency       = "processor_latency_ns"
	processorLatencySum    = "processor_latency_ns_sum"
	processorLatencyCount  = "processor_latency_ns_count"
	processorReceived      = "processor_received"
	processorSent          = "processor_sent"
)

// Series-name prefixes, the default paths Benthos uses when a series has no
// path label, and the latency stems as they follow a family prefix. The
// _sum and _count stems are the summary exposition client_golang writes.
const (
	inputPrefixName     = "input_"
	outputPrefixName    = "output_"
	processorPrefixName = "processor_"

	rootInputPath  = "root.input"
	rootOutputPath = "root.output"

	latencyStem      = "latency_ns"
	latencySumStem   = "latency_ns_sum"
	latencyCountStem = "latency_ns_count"
)

// The HTTP endpoints Benthos serves: /ping, /version and /metrics from
// internal/api/api.go, /ready from internal/stream/type.go.
const (
	pingPath    = "/ping"
	readyPath   = "/ready"
	versionPath = "/version"
	metricsPath = "/metrics"
)

// knownSeriesNames lists every series name the parser reads, so the fixture
// spec can require each one in a recorded /metrics body.
func knownSeriesNames() []string {
	return []string{
		inputConnectionFailed, inputConnectionLost, inputConnectionUp, inputReceived,
		inputLatency, inputLatencySum, inputLatencyCount,

		outputBatchSent, outputConnectionFailed, outputConnectionLost,
		outputConnectionUp, outputError, outputSent,
		outputLatency, outputLatencySum, outputLatencyCount,

		processorBatchReceived, processorReceived, processorSent,
		processorBatchSent, processorError,
		processorLatency, processorLatencySum, processorLatencyCount,
	}
}
