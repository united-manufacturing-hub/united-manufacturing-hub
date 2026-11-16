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
	"math"
	"strconv"
)

// Label keys as they open a label value in a raw label set.
var (
	pathKey     = []byte(`path="`)
	labelKey    = []byte(`label="`)
	quantileKey = []byte(`quantile="`)
)

// parseMetrics parses the Prometheus text-format payload Benthos returns into
// per-path Metrics. Lines whose name the parser does not know are ignored,
// whatever their value.
func parseMetrics(raw []byte) (Metrics, error) {
	m := Metrics{
		Process: ProcessMetrics{Processors: make(map[string]ProcessorMetrics, 8)},
		Inputs:  make(map[string]InputMetrics, 1),
		Outputs: make(map[string]OutputMetrics, 1),
	}

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

		name, labels := line[:nameEnd], line[nameEnd:]

		var err error

		switch {
		case hasPrefix(name, inputPrefix):
			err = parseInputLine(&m, name, labels, line)
		case hasPrefix(name, outputPrefix):
			err = parseOutputLine(&m, name, labels, line)
		case hasPrefix(name, processorPrefix):
			err = parseProcessorLine(&m, name, labels, line)
		}

		if err != nil {
			return Metrics{}, fmt.Errorf("failed to parse %s: %w", name, err)
		}
	}

	return m, nil
}

// parseInputLine folds one input_* line into m.Inputs, keyed by its path.
func parseInputLine(m *Metrics, name, labels, line []byte) error {
	field, isCounter := inputCounters[string(name)]
	if !isCounter && !isLatencyName(inputPrefix, name) {
		return nil
	}

	v, err := parseSampleValue(line)
	if err != nil {
		return err
	}

	path := extractLabel(labels, pathKey)

	in, ok := m.Inputs[string(path)]
	if len(path) == 0 {
		in, ok = m.Inputs[rootInputPath]
	}

	if !ok {
		in.Path = rootInputPath
		if len(path) > 0 {
			in.Path = string(path)
		}
	}

	if in.Label == "" {
		in.Label = string(extractLabel(labels, labelKey))
	}

	if isCounter {
		var n int64

		n, err = counterValue(v)
		in = field(in, n)
	} else {
		err = setLatency(&in.LatencyNS, inputPrefix, name, labels, v)
	}

	if err != nil {
		return err
	}

	m.Inputs[in.Path] = in

	return nil
}

// parseOutputLine folds one output_* line into m.Outputs, keyed by its path.
func parseOutputLine(m *Metrics, name, labels, line []byte) error {
	field, isCounter := outputCounters[string(name)]
	if !isCounter && !isLatencyName(outputPrefix, name) {
		return nil
	}

	v, err := parseSampleValue(line)
	if err != nil {
		return err
	}

	path := extractLabel(labels, pathKey)

	out, ok := m.Outputs[string(path)]
	if len(path) == 0 {
		out, ok = m.Outputs[rootOutputPath]
	}

	if !ok {
		out.Path = rootOutputPath
		if len(path) > 0 {
			out.Path = string(path)
		}
	}

	if out.Label == "" {
		out.Label = string(extractLabel(labels, labelKey))
	}

	if isCounter {
		var n int64

		n, err = counterValue(v)
		out = field(out, n)
	} else {
		err = setLatency(&out.LatencyNS, outputPrefix, name, labels, v)
	}

	if err != nil {
		return err
	}

	m.Outputs[out.Path] = out

	return nil
}

// parseProcessorLine folds one processor_* line into m.Process.Processors,
// keyed by its path. A line without a path is ignored.
func parseProcessorLine(m *Metrics, name, labels, line []byte) error {
	field, isCounter := processorCounters[string(name)]
	if !isCounter && !isLatencyName(processorPrefix, name) {
		return nil
	}

	v, err := parseSampleValue(line)
	if err != nil {
		return err
	}

	path := extractLabel(labels, pathKey)
	if len(path) == 0 {
		return nil
	}

	pm := m.Process.Processors[string(path)]
	if pm.Label == "" {
		pm.Label = string(extractLabel(labels, labelKey))
	}

	if isCounter {
		var n int64

		n, err = counterValue(v)
		pm = field(pm, n)
	} else {
		err = setLatency(&pm.LatencyNS, processorPrefix, name, labels, v)
	}

	if err != nil {
		return err
	}

	m.Process.Processors[string(path)] = pm

	return nil
}

// parseSampleValue returns the sample value of a line in the Prometheus text
// format, metric_name [{labels}] value [timestamp]:
// https://prometheus.io/docs/instrumenting/exposition_formats/#text-format-details
func parseSampleValue(line []byte) (float64, error) {
	// A label value may hold spaces and braces, but the value and the
	// timestamp hold neither, so the last } ends the label set.
	rest := line
	if i := bytes.LastIndexByte(line, '}'); i != -1 {
		rest = line[i+1:]
	} else if i := bytes.IndexByte(line, ' '); i != -1 {
		rest = line[i:]
	} else {
		return 0, errors.New("no sample value")
	}

	rest = bytes.TrimLeft(rest, " ")
	if i := bytes.IndexByte(rest, ' '); i != -1 {
		rest = rest[:i]
	}

	if len(rest) == 0 {
		return 0, errors.New("no sample value")
	}

	return strconv.ParseFloat(string(rest), 64)
}

// counterValue converts a counter's sample value, truncating a fraction.
// NaN, a negative value and one past int64 are errors.
func counterValue(v float64) (int64, error) {
	if math.IsNaN(v) || v < 0 || v >= math.MaxInt64 {
		return 0, fmt.Errorf("counter value %v is not a non-negative int64", v)
	}

	return int64(v), nil
}

// setLatency folds the sample value of one of the family's latency series
// into l: a quantile sample, the sum or the count. A NaN quantile leaves its
// percentile nil; any other non-finite value is an error.
func setLatency(l *Latency, prefix string, name, labels []byte, v float64) error {
	switch string(name[len(prefix):]) {
	case latencySumStem:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("latency sum %v is not finite", v)
		}

		l.Sum = v
	case latencyCountStem:
		n, err := counterValue(v)
		if err != nil {
			return err
		}

		l.Count = n
	default: // latencyStem
		if math.IsNaN(v) {
			return nil
		}

		if math.IsInf(v, 0) {
			return fmt.Errorf("latency quantile %v is not finite", v)
		}

		switch string(extractLabel(labels, quantileKey)) {
		case "0.5":
			l.P50 = new(v)
		case "0.9":
			l.P90 = new(v)
		case "0.99":
			l.P99 = new(v)
		}
	}

	return nil
}

// isLatencyName reports whether name is one of the three latency series of
// the family with this prefix. A user metric that only ends in a latency
// stem, such as input_custom_latency_ns, is not.
func isLatencyName(prefix string, name []byte) bool {
	if !hasPrefix(name, prefix) {
		return false
	}

	switch string(name[len(prefix):]) {
	case latencyStem, latencySumStem, latencyCountStem:
		return true
	}

	return false
}

// hasPrefix is bytes.HasPrefix for a string prefix, without converting it.
func hasPrefix(b []byte, prefix string) bool {
	return len(b) >= len(prefix) && string(b[:len(prefix)]) == prefix
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
