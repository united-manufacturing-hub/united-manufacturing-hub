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

import (
	"maps"
	"slices"
)

// Series-name prefixes of the three families, and the default paths Benthos
// uses when an input or output series has no path label.
const (
	inputPrefix     = "input_"
	outputPrefix    = "output_"
	processorPrefix = "processor_"

	rootInputPath  = "root.input"
	rootOutputPath = "root.output"
)

// The latency series of a family, as they follow its prefix. The _sum and
// _count stems are the summary exposition client_golang writes.
const (
	latencyStem      = "latency_ns"
	latencySumStem   = "latency_ns_sum"
	latencyCountStem = "latency_ns_count"
)

// inputCounters maps each input counter series, from
// internal/component/input/async_reader.go, to the setter of the field it fills.
var inputCounters = map[string]func(InputMetrics, int64) InputMetrics{
	"input_connection_failed": func(m InputMetrics, v int64) InputMetrics {
		m.ConnectionFailed = v

		return m
	},
	"input_connection_lost": func(m InputMetrics, v int64) InputMetrics {
		m.ConnectionLost = v

		return m
	},
	"input_connection_up": func(m InputMetrics, v int64) InputMetrics {
		m.ConnectionUp = v

		return m
	},
	"input_received": func(m InputMetrics, v int64) InputMetrics {
		m.Received = v

		return m
	},
}

// outputCounters maps each output counter series, from
// internal/component/output/async_writer.go, to the setter of the field it fills.
var outputCounters = map[string]func(OutputMetrics, int64) OutputMetrics{
	"output_batch_sent": func(m OutputMetrics, v int64) OutputMetrics {
		m.BatchSent = v

		return m
	},
	"output_connection_failed": func(m OutputMetrics, v int64) OutputMetrics {
		m.ConnectionFailed = v

		return m
	},
	"output_connection_lost": func(m OutputMetrics, v int64) OutputMetrics {
		m.ConnectionLost = v

		return m
	},
	"output_connection_up": func(m OutputMetrics, v int64) OutputMetrics {
		m.ConnectionUp = v

		return m
	},
	"output_error": func(m OutputMetrics, v int64) OutputMetrics {
		m.Error = v

		return m
	},
	"output_sent": func(m OutputMetrics, v int64) OutputMetrics {
		m.Sent = v

		return m
	},
}

// processorCounters maps each processor counter series, from
// internal/component/processor/auto_observed.go, to the setter of the field it fills.
var processorCounters = map[string]func(ProcessorMetrics, int64) ProcessorMetrics{
	"processor_batch_received": func(m ProcessorMetrics, v int64) ProcessorMetrics {
		m.BatchReceived = v

		return m
	},
	"processor_batch_sent": func(m ProcessorMetrics, v int64) ProcessorMetrics {
		m.BatchSent = v

		return m
	},
	"processor_error": func(m ProcessorMetrics, v int64) ProcessorMetrics {
		m.Error = v

		return m
	},
	"processor_received": func(m ProcessorMetrics, v int64) ProcessorMetrics {
		m.Received = v

		return m
	},
	"processor_sent": func(m ProcessorMetrics, v int64) ProcessorMetrics {
		m.Sent = v

		return m
	},
}

// The HTTP endpoints Benthos serves: /ping, /version and /metrics from
// internal/api/api.go, /ready from internal/stream/type.go.
const (
	pingPath    = "/ping"
	readyPath   = "/ready"
	versionPath = "/version"
	metricsPath = "/metrics"
)

// knownSeriesNames lists every series name the parser reads, sorted: the
// counters of each family and its three latency series.
func knownSeriesNames() []string {
	var names []string

	names = slices.AppendSeq(names, maps.Keys(inputCounters))
	names = slices.AppendSeq(names, maps.Keys(outputCounters))

	names = slices.AppendSeq(names, maps.Keys(processorCounters))
	for _, prefix := range []string{inputPrefix, outputPrefix, processorPrefix} {
		names = append(names, prefix+latencyStem, prefix+latencySumStem, prefix+latencyCountStem)
	}

	slices.Sort(names)

	return names
}
