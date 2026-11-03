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

// Parsing of the Prometheus text format that Benthos serves on /metrics:
// https://prometheus.io/docs/instrumenting/exposition_formats/

package benthosmetrics

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// Series-name prefixes, label keys and latency-name suffixes, held as byte
// slices so the per-line reads below allocate nothing. The names themselves
// are defined, with their upstream citations, in names.go.
var (
	inputPrefix       = []byte(inputPrefixName)
	outputPrefix      = []byte(outputPrefixName)
	processorPrefix   = []byte(processorPrefixName)
	defaultInputPath  = []byte(rootInputPath)
	defaultOutputPath = []byte(rootOutputPath)

	pathKey     = []byte(`path="`)
	labelKey    = []byte(`label="`)
	quantileKey = []byte(`quantile="`)

	latencySumSuffix   = []byte("_" + latencySumStem)
	latencyCountSuffix = []byte("_" + latencyCountStem)
)

// tailInt returns the integer found after the final space in a Prometheus
// text-formatted line. Benthos' metric stream is deliberately terse: for
// counters and gauges the value we need is always that last token.
//
// The token is read leniently, not parsed: the digits up to the first
// non-digit are taken, so a trailing timestamp (input_received 5
// 1700000000000) reads as the timestamp, a fraction truncates (12.9 reads
// 12), and NaN, ±Inf and a negative number read 0 on the digit walk. Idle
// Benthos sends NaN, so the leniency is load-bearing. The one form the digit
// walk cannot read, exponent notation (1.234567e+06), is parsed as a float
// and so keeps its sign: -1e3 reads -1000, and an out-of-range exponent
// (1e999) is an error.
func tailInt(line []byte) (int64, error) {
	i := bytes.LastIndexByte(line, ' ')
	if i == -1 {
		return 0, errors.New("failed to find space in line")
	}

	s := line[i+1:]
	if bytes.ContainsAny(s, "eE") {
		// Exponent notation carries no spaces, so the token is one float.
		f, err := strconv.ParseFloat(string(s), 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse float: %w", err)
		}

		return int64(f), nil
	}

	var v int64

	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}

		v = v*10 + int64(c-'0')
	}

	return v, nil
}

// setQuantile folds one latency_ns sample into the percentile its quantile
// label names.
func setQuantile(l *Latency, quantile []byte, v int64) {
	switch string(quantile) {
	case "0.5":
		l.P50 = float64(v)
	case "0.9":
		l.P90 = float64(v)
	case "0.99":
		l.P99 = float64(v)
	}
}

// setLatency folds the value of one latency series into l. Which of the three
// series the line carries — the quantile sample named by the quantile label,
// the running sum, or the sample count — is said by the metric's full name,
// which ends in _latency_ns, _latency_ns_sum or _latency_ns_count. The three
// families (input, output, processor) share this one handler.
func setLatency(l *Latency, name, labels []byte, v int64) {
	switch {
	case bytes.HasSuffix(name, latencySumSuffix):
		l.Sum = float64(v)
	case bytes.HasSuffix(name, latencyCountSuffix):
		l.Count = v
	default: // "_latency_ns"
		setQuantile(l, extractLabel(labels, quantileKey), v)
	}
}

// knownLatencyName reports whether name is one of the three latency series
// of the family with this prefix: the quantile samples (prefix_latency_ns),
// the running sum (prefix_latency_ns_sum) or the sample count
// (prefix_latency_ns_count). Only those exact names pass, as in FSMv1: a
// user metric whose name merely ends in a latency suffix, such as
// input_custom_latency_ns, is not one of them.
func knownLatencyName(prefix, name []byte) bool {
	rest, ok := bytes.CutPrefix(name, prefix)
	if !ok {
		return false
	}
	switch string(rest) {
	case latencyStem, latencySumStem, latencyCountStem:
		return true
	}
	return false
}

// knownInputName reports whether the parser reads an input_* metric with this
// name: one of the per-path input counters, or one of the three latency
// series. A line with any other input_* name is ignored, whatever its value,
// as in FSMv1.
func knownInputName(name []byte) bool {
	switch string(name) {
	case inputConnectionFailed, inputConnectionLost, inputConnectionUp, inputReceived:
		return true
	}
	return knownLatencyName(inputPrefix, name)
}

// knownOutputName reports whether the parser reads an output_* metric with
// this name: one of the per-path output counters, or one of the three latency
// series. A line with any other output_* name is ignored, whatever its
// value, as in FSMv1.
func knownOutputName(name []byte) bool {
	switch string(name) {
	case outputBatchSent, outputConnectionFailed, outputConnectionLost,
		outputConnectionUp, outputError, outputSent:
		return true
	}
	return knownLatencyName(outputPrefix, name)
}

// knownProcessorName reports whether the parser reads a processor_* metric
// with this name: one of the per-processor counters, or one of the three
// latency series. A line with any other processor_* name is ignored, whatever
// its value, as in FSMv1.
func knownProcessorName(name []byte) bool {
	switch string(name) {
	case processorReceived, processorBatchReceived, processorSent,
		processorBatchSent, processorError:
		return true
	}
	return knownLatencyName(processorPrefix, name)
}

// ParseMetricsFromBytes parses the Prometheus text-format payload Benthos
// returns into per-path Metrics. A switch/broker/fallback config has one
// leaf input / output per path, and each keeps its own entry.
func ParseMetricsFromBytes(raw []byte) (Metrics, error) {
	var m Metrics

	m.Process.Processors = make(map[string]ProcessorMetrics, 8)
	m.Inputs = make(map[string]InputMetrics, 1)
	m.Outputs = make(map[string]OutputMetrics, 1)

	for line := range bytes.Lines(raw) {
		line = bytes.TrimSuffix(line, []byte("\n"))

		if len(line) == 0 || line[0] == '#' {
			continue
		}

		nameEnd := bytes.IndexByte(line, '{')
		if nameEnd == -1 {
			nameEnd = bytes.IndexByte(line, ' ')
		}

		if nameEnd == -1 {
			continue
		}

		name := line[:nameEnd]
		labels := line[nameEnd:]

		switch {
		case bytes.HasPrefix(name, inputPrefix):
			// The value is parsed only when the name is one the parser
			// knows; an unknown input_* line is ignored, whatever its
			// value, as in FSMv1.
			if !knownInputName(name) {
				continue
			}

			count, err := tailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse %s: %w", name, err)
			}

			path := extractLabel(labels, pathKey)
			if len(path) == 0 {
				path = defaultInputPath
			}

			in := m.Inputs[string(path)]
			if in.Path == "" {
				in.Path = string(path)
			}

			if in.Label == "" {
				in.Label = string(extractLabel(labels, labelKey))
			}

			switch string(name) {
			case inputConnectionFailed:
				in.ConnectionFailed = count
			case inputConnectionLost:
				in.ConnectionLost = count
			case inputConnectionUp:
				in.ConnectionUp = count
			case inputReceived:
				in.Received = count
			default:
				setLatency(&in.LatencyNS, name, labels, count)
			}

			m.Inputs[in.Path] = in

		case bytes.HasPrefix(name, outputPrefix):
			// The value is parsed only when the name is one the parser
			// knows; an unknown output_* line is ignored, whatever its
			// value, as in FSMv1.
			if !knownOutputName(name) {
				continue
			}

			count, err := tailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse %s: %w", name, err)
			}

			path := extractLabel(labels, pathKey)
			if len(path) == 0 {
				path = defaultOutputPath
			}

			out := m.Outputs[string(path)]
			if out.Path == "" {
				out.Path = string(path)
			}

			if out.Label == "" {
				out.Label = string(extractLabel(labels, labelKey))
			}

			switch string(name) {
			case outputBatchSent:
				out.BatchSent = count
			case outputConnectionFailed:
				out.ConnectionFailed = count
			case outputConnectionLost:
				out.ConnectionLost = count
			case outputConnectionUp:
				out.ConnectionUp = count
			case outputError:
				out.Error = count
			case outputSent:
				out.Sent = count
			default:
				setLatency(&out.LatencyNS, name, labels, count)
			}

			m.Outputs[out.Path] = out

		case bytes.HasPrefix(name, processorPrefix):
			// The value is parsed only when the name is one the parser
			// knows; an unknown processor_* line is ignored, whatever its
			// value, as in FSMv1.
			if !knownProcessorName(name) {
				continue
			}

			count, err := tailInt(line)
			if err != nil {
				return Metrics{}, fmt.Errorf("failed to parse %s: %w", name, err)
			}

			path := extractLabel(labels, pathKey)
			if len(path) == 0 {
				continue
			}

			pm := m.Process.Processors[string(path)]
			if pm.Label == "" {
				pm.Label = string(extractLabel(labels, labelKey))
			}

			switch string(name) {
			case processorReceived:
				pm.Received = count
			case processorBatchReceived:
				pm.BatchReceived = count
			case processorSent:
				pm.Sent = count
			case processorBatchSent:
				pm.BatchSent = count
			case processorError:
				pm.Error = count
			default:
				setLatency(&pm.LatencyNS, name, labels, count)
			}

			m.Process.Processors[string(path)] = pm
		}
	}

	return m, nil
}

// extractLabel reads one label's value out of a raw Prometheus label set, the
// {…} region of a line, without allocating a map for the rest.
func extractLabel(b, key []byte) []byte {
	i := bytes.Index(b, key)
	if i == -1 {
		return nil
	}

	i += len(key)

	j := bytes.IndexByte(b[i:], '"')
	if j == -1 {
		return nil
	}

	return b[i : i+j]
}
