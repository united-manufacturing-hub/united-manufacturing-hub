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

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// --- helpers ---------------------------------------------------------------

// TailInt returns the integer found after the final space in a Prometheus
// text-formatted line.
//
// Benthos' metric stream is deliberately terse; for counters and gauges the
// value we need is always the token after the last space.  A micro-parser that
// walks backwards from the end lets us avoid a full `strings.Fields` split
// (~3× faster and zero allocations on the hot-path).
func TailInt(line []byte) (int64, error) {
	i := bytes.LastIndexByte(line, ' ')
	if i == -1 {
		return 0, errors.New("failed to find space in line")
	}

	s := string(line[i+1:])
	if strings.ContainsAny(s, "eE") {
		f, err := strconv.ParseFloat(s, 64)
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

// ---------------------------------------------------------------------------

// ParseMetricsFromBytes parses the prometheus text-format payload benthos
// returns and produces a per-path Metrics. For switch/broker/fallback
// outputs the input emits N distinct series per metric, one per leaf
// path; the parser stores each in its own InputInstance / OutputInstance
// entry.
func ParseMetricsFromBytes(raw []byte) (Metrics, error) {
	var m Metrics

	m.Process.Processors = make(map[string]ProcessorMetrics, 8)
	m.Inputs = make(map[string]InputInstance, 1)
	m.Outputs = make(map[string]OutputInstance, 1)

	lineStart := 0

	for i, b := range raw {
		if b != '\n' && i != len(raw)-1 {
			continue
		}

		end := i
		if b != '\n' { // EOF w/o \n
			end++
		}

		line := raw[lineStart:end]
		lineStart = i + 1

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

		name := string(line[:nameEnd])

		// All input_* and output_* lines carry a `path` label.
		labelsRegion := line[nameEnd:]

		switch {
		case strings.HasPrefix(name, "input_"):
			path := extractLabel(labelsRegion, "path")
			if path == "" {
				path = "root.input"
			}

			label := extractLabel(labelsRegion, "label")

			in := m.Inputs[path]
			if in.Path == "" {
				in.Path = path
			}

			if in.Label == "" && label != "" {
				in.Label = label
			}

			switch name {
			case "input_connection_failed":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input connection failed: %w", err)
				}

				in.ConnectionFailed = count
			case "input_connection_lost":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input connection lost: %w", err)
				}

				in.ConnectionLost = count
			case "input_connection_up":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input connection up: %w", err)
				}

				in.ConnectionUp = count
			case "input_received":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input received: %w", err)
				}

				in.Received = count
			case "input_latency_ns":
				switch extractLabel(labelsRegion, "quantile") {
				case "0.5":
					p50, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse input latency ns p50: %w", err)
					}

					in.LatencyNS.P50 = float64(p50)
				case "0.9":
					p90, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse input latency ns p90: %w", err)
					}

					in.LatencyNS.P90 = float64(p90)
				case "0.99":
					p99, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse input latency ns p99: %w", err)
					}

					in.LatencyNS.P99 = float64(p99)
				}
			case "input_latency_ns_sum":
				sum, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input latency ns sum: %w", err)
				}

				in.LatencyNS.Sum = float64(sum)
			case "input_latency_ns_count":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse input latency ns count: %w", err)
				}

				in.LatencyNS.Count = count
			}

			m.Inputs[path] = in

		case strings.HasPrefix(name, "output_"):
			path := extractLabel(labelsRegion, "path")
			if path == "" {
				path = "root.output"
			}

			label := extractLabel(labelsRegion, "label")

			out := m.Outputs[path]
			if out.Path == "" {
				out.Path = path
			}

			if out.Label == "" && label != "" {
				out.Label = label
			}

			switch name {
			case "output_batch_sent":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output batch sent: %w", err)
				}

				out.BatchSent = count
			case "output_connection_failed":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output connection failed: %w", err)
				}

				out.ConnectionFailed = count
			case "output_connection_lost":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output connection lost: %w", err)
				}

				out.ConnectionLost = count
			case "output_connection_up":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output connection up: %w", err)
				}

				out.ConnectionUp = count
			case "output_error":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output error: %w", err)
				}

				out.Error = count
			case "output_sent":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output sent: %w", err)
				}

				out.Sent = count
			case "output_latency_ns":
				switch extractLabel(labelsRegion, "quantile") {
				case "0.5":
					p50, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse output latency ns p50: %w", err)
					}

					out.LatencyNS.P50 = float64(p50)
				case "0.9":
					p90, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse output latency ns p90: %w", err)
					}

					out.LatencyNS.P90 = float64(p90)
				case "0.99":
					p99, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse output latency ns p99: %w", err)
					}

					out.LatencyNS.P99 = float64(p99)
				}
			case "output_latency_ns_sum":
				sum, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output latency ns sum: %w", err)
				}

				out.LatencyNS.Sum = float64(sum)
			case "output_latency_ns_count":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse output latency ns count: %w", err)
				}

				out.LatencyNS.Count = count
			}

			m.Outputs[path] = out

		default:
			if !bytes.HasPrefix(line, []byte("processor_")) {
				continue
			}

			path := extractLabel(labelsRegion, "path")
			if path == "" {
				continue
			}

			pm := m.Process.Processors[path]
			if pm.Label == "" {
				pm.Label = extractLabel(labelsRegion, "label")
			}

			switch name {
			case "processor_received":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor received: %w", err)
				}

				pm.Received = count
			case "processor_batch_received":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor batch received: %w", err)
				}

				pm.BatchReceived = count
			case "processor_sent":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor sent: %w", err)
				}

				pm.Sent = count
			case "processor_batch_sent":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor batch sent: %w", err)
				}

				pm.BatchSent = count
			case "processor_error":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor error: %w", err)
				}

				pm.Error = count
			case "processor_latency_ns":
				switch extractLabel(labelsRegion, "quantile") {
				case "0.5":
					p50, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse processor latency ns p50: %w", err)
					}

					pm.LatencyNS.P50 = float64(p50)
				case "0.9":
					p90, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse processor latency ns p90: %w", err)
					}

					pm.LatencyNS.P90 = float64(p90)
				case "0.99":
					p99, err := TailInt(line)
					if err != nil {
						return Metrics{}, fmt.Errorf("failed to parse processor latency ns p99: %w", err)
					}

					pm.LatencyNS.P99 = float64(p99)
				}
			case "processor_latency_ns_sum":
				sum, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor latency ns sum: %w", err)
				}

				pm.LatencyNS.Sum = float64(sum)
			case "processor_latency_ns_count":
				count, err := TailInt(line)
				if err != nil {
					return Metrics{}, fmt.Errorf("failed to parse processor latency ns count: %w", err)
				}

				pm.LatencyNS.Count = count
			}

			m.Process.Processors[path] = pm
		}
	}

	return m, nil
}

// extractLabel scans the raw label-set of a Prometheus series (the `{…}`
// portion) and returns the *value* of a given label key.
//
// Reads only the labels it cares about (e.g. `path` or `quantile`), so this
// hand-rolled matcher is an order of magnitude faster than allocating a full
// `map[string]string` for every series.
func extractLabel(b []byte, key string) string {
	// expects {label="...",path="..."} order irrelevant
	key += "=\""

	i := bytes.Index(b, []byte(key))
	if i == -1 {
		return ""
	}

	i += len(key)

	j := bytes.IndexByte(b[i:], '"')
	if j == -1 {
		return ""
	}
	// return unsafeString(b[i : i+j])
	return string(b[i : i+j])
}

// unsafeString converts a []byte to string without allocation.
// Use only for read-only parsing.
// func unsafeString(b []byte) string { return *(*string)(unsafe.Pointer(&b)) }
