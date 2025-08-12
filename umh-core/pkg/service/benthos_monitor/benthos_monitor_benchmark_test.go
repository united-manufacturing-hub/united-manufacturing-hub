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

// ────────────────────────────────────────────────────────────────────────────────
// ENG-2893  Faster metric parsing for Benthos/Redpanda
// Context: Slack thread 2025-04-30 (this file ↔ ENG-2893, ENG-2884)
// ────────────────────────────────────────────────────────────────────────────────
//
// Benchmarks (Go 1.22, Ryzen 9-5900X, pkg/service/benthos_monitor):
// BenchmarkGzipDecode-24                             83491             14535 ns/op           49169 B/op         11 allocs/op
// BenchmarkHexDecode-24                            2826033               419.2 ns/op           416 B/op          1 allocs/op
// BenchmarkMetricsParsing-24                         24597             47635 ns/op           28392 B/op        781 allocs/op
// BenchmarkCompleteProcessing-24                     17631             70915 ns/op           75673 B/op        792 allocs/op
// BenchmarkParseBenthosLogsWithPercentiles-24         7845            142591 ns/op            144292 p50ns            469733 p95ns            706459 p99ns          217352 B/op        911 allocs/op
// BenchmarkUpdateFromMetrics-24                    2992988               399.1 ns/op           925 B/op          2 allocs/op
//
// Findings
// --------
// • ParseMetricsFromBytes ≈ 73 % of end-to-end time (42 µs of 58 µs).
// • Total cost to handle one Benthos metrics payload (gzip→hex→parse) ≈ 0.05 ms.
// • Hex decode is negligible; gzip decompression is minor; Prometheus text parse
//   is the clear hotspot.
//
// Options under evaluation
// ------------------------
// 1. Avoid parsing every tick for every Benthos instance (parse only when needed).
// 2. Replace Prometheus text parser with minimal hand-rolled parser.
//
// Related tickets
//  • ENG-2893 – performance work.
//  • ENG-2884 – parsing errors / observed-state update.
//
// Sample metrics used for the benchmark are below (taken from test_metrics.txt).
// ────────────────────────────────────────────────────────────────────────────────

package benthos_monitor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"io"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
)

var sampleMetrics = `# HELP input_connection_failed Benthos Counter metric
# TYPE input_connection_failed counter
input_connection_failed{label="",path="root.input"} 0
# HELP input_connection_lost Benthos Counter metric
# TYPE input_connection_lost counter
input_connection_lost{label="",path="root.input"} 0
# HELP input_connection_up Benthos Counter metric
# TYPE input_connection_up counter
input_connection_up{label="",path="root.input"} 1
# HELP input_latency_ns Benthos Timing metric
# TYPE input_latency_ns summary
input_latency_ns{label="",path="root.input",quantile="0.5"} 127167
input_latency_ns{label="",path="root.input",quantile="0.9"} 378375
input_latency_ns{label="",path="root.input",quantile="0.99"} 858666
input_latency_ns_sum{label="",path="root.input"} 3.629208e+06
input_latency_ns_count{label="",path="root.input"} 18
# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 18
# HELP output_batch_sent Benthos Counter metric
# TYPE output_batch_sent counter
output_batch_sent{label="",path="root.output"} 18
# HELP output_connection_failed Benthos Counter metric
# TYPE output_connection_failed counter
output_connection_failed{label="",path="root.output"} 0
# HELP output_connection_lost Benthos Counter metric
# TYPE output_connection_lost counter
output_connection_lost{label="",path="root.output"} 0
# HELP output_connection_up Benthos Counter metric
# TYPE output_connection_up counter
output_connection_up{label="",path="root.output"} 1
# HELP output_error Benthos Counter metric
# TYPE output_error counter
output_error{label="",path="root.output"} 0
# HELP output_latency_ns Benthos Timing metric
# TYPE output_latency_ns summary
output_latency_ns{label="",path="root.output",quantile="0.5"} 33250
output_latency_ns{label="",path="root.output",quantile="0.9"} 94709
output_latency_ns{label="",path="root.output",quantile="0.99"} 138250
output_latency_ns_sum{label="",path="root.output"} 816919
output_latency_ns_count{label="",path="root.output"} 18
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output"} 18
`

var samplePing = `pong`
var sampleReady = `{"statuses":[{"label":"tcp_server","path":"root.input","connected":true},{"label":"http_client","path":"root.output","connected":true}]}`
var sampleVersion = `{"version":"3.71.0","built":"2023-08-15T12:00:00Z"}`

// Returns a gzip compressed string of the sample metrics
// Also returns the same data but hex encoded.
func prepareDataForBenchmark() ([]byte, string) {
	var buf bytes.Buffer

	gzipWriter := gzip.NewWriter(&buf)
	_, _ = gzipWriter.Write([]byte(sampleMetrics))

	err := gzipWriter.Close()
	if err != nil {
		panic(err)
	}

	hexData := hex.EncodeToString(buf.Bytes())

	return buf.Bytes(), hexData
}

// Prepare sample log entries for ParseBenthosLogs benchmark.
func prepareSampleLogEntries() []s6_shared.LogEntry {
	entries := []s6_shared.LogEntry{}

	// Add BLOCK_START_MARKER
	entries = append(entries, s6_shared.LogEntry{Content: BLOCK_START_MARKER})

	// Add ping data
	pingGzip := compressAndHex(samplePing)
	entries = append(entries, s6_shared.LogEntry{Content: pingGzip})

	// Add PING_END_MARKER
	entries = append(entries, s6_shared.LogEntry{Content: PING_END_MARKER})

	// Add ready data
	readyGzip := compressAndHex(sampleReady)
	entries = append(entries, s6_shared.LogEntry{Content: readyGzip})

	// Add READY_END
	entries = append(entries, s6_shared.LogEntry{Content: READY_END})

	// Add version data
	versionGzip := compressAndHex(sampleVersion)
	entries = append(entries, s6_shared.LogEntry{Content: versionGzip})

	// Add VERSION_END
	entries = append(entries, s6_shared.LogEntry{Content: VERSION_END})

	// Add metrics data
	_, metricsHex := prepareDataForBenchmark()
	entries = append(entries, s6_shared.LogEntry{Content: metricsHex})

	// Add METRICS_END_MARKER
	entries = append(entries, s6_shared.LogEntry{Content: METRICS_END_MARKER})

	// Add timestamp (unix timestamp in nanoseconds)
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	entries = append(entries, s6_shared.LogEntry{Content: timestamp})

	// Add BLOCK_END_MARKER
	entries = append(entries, s6_shared.LogEntry{Content: BLOCK_END_MARKER})

	return entries
}

// Helper to compress and hex encode a string.
func compressAndHex(data string) string {
	var buf bytes.Buffer

	gzipWriter := gzip.NewWriter(&buf)
	_, _ = gzipWriter.Write([]byte(data))

	err := gzipWriter.Close()
	if err != nil {
		panic(err)
	}

	return hex.EncodeToString(buf.Bytes())
}

// BenchmarkGzipDecode benchmarks just the gzip decoding operation.
func BenchmarkGzipDecode(b *testing.B) {
	encodedData, _ := prepareDataForBenchmark()

	b.ResetTimer()

	for range b.N {
		reader, err := gzip.NewReader(bytes.NewReader(encodedData))
		if err != nil {
			b.Fatal(err)
		}

		_, err = io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}

		err = reader.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHexDecode benchmarks just the hex decoding operation.
func BenchmarkHexDecode(b *testing.B) {
	_, encodedAndHexedData := prepareDataForBenchmark()

	b.ResetTimer()

	for range b.N {
		_, err := hex.DecodeString(encodedAndHexedData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMetricsParsing benchmarks the metrics parsing operation.
func BenchmarkMetricsParsing(b *testing.B) {
	b.ResetTimer()

	for range b.N {
		_, err := ParseMetricsFromBytesSlow([]byte(sampleMetrics))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMetricsParsingOpt benchmarks the metrics parsing operation.
func BenchmarkMetricsParsingOpt(b *testing.B) {
	b.ResetTimer()

	for range b.N {
		_, err := ParseMetricsFromBytes([]byte(sampleMetrics))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Test that both output the same result.
func TestMetricsParsing(t *testing.T) {
	metrics, err := ParseMetricsFromBytes([]byte(sampleMetrics))
	if err != nil {
		t.Fatal(err)
	}

	metricsOpt, err := ParseMetricsFromBytesSlow([]byte(sampleMetrics))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(metrics, metricsOpt) {
		t.Logf("metrics: %+v", metrics)
		t.Logf("metricsOpt: %+v", metricsOpt)
		t.Fatal("metrics do not match")
	}
}

// BenchmarkCompleteProcessing benchmarks the entire pipeline: hex decode -> gzip decode -> parse metrics.
func BenchmarkCompleteProcessing(b *testing.B) {
	_, encodedAndHexedData := prepareDataForBenchmark()

	b.ResetTimer()

	for range b.N {
		// Step 1: Hex decode
		decodedMetricsDataBytes, _ := hex.DecodeString(encodedAndHexedData)

		// Step 2: Gzip decompress
		gzipReader, err := gzip.NewReader(bytes.NewReader(decodedMetricsDataBytes))
		if err != nil {
			b.Fatal(err)
		}

		decompressedData, err := io.ReadAll(gzipReader)
		if err != nil {
			b.Fatal(err)
		}

		err = gzipReader.Close()
		if err != nil {
			b.Fatal(err)
		}

		// Step 3: Parse metrics
		_, err = ParseMetricsFromBytes(decompressedData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseBenthosLogs benchmarks the ParseBenthosLogs function.
func BenchmarkParseBenthosLogsWithPercentiles(b *testing.B) {
	// Create service and test data
	service := NewBenthosMonitorService("test-benthos")
	logs := prepareSampleLogEntries()
	ctx := context.Background()

	// Collect execution times
	times := make([]time.Duration, b.N)

	b.ResetTimer()
	b.StopTimer()

	for i := range b.N {
		start := time.Now()

		b.StartTimer()

		_, err := service.ParseBenthosLogs(ctx, logs, uint64(i))

		b.StopTimer()

		if err != nil {
			b.Fatal(err)
		}

		times[i] = time.Since(start)
	}

	// Sort times for percentile calculation
	sort.Slice(times, func(i, j int) bool {
		return times[i] < times[j]
	})

	// Calculate and report percentiles
	p50 := times[int(float64(len(times))*0.50)]
	p95 := times[int(float64(len(times))*0.95)]
	p99 := times[int(float64(len(times))*0.99)]

	b.ReportMetric(float64(p50.Nanoseconds()), "p50ns")
	b.ReportMetric(float64(p95.Nanoseconds()), "p95ns")
	b.ReportMetric(float64(p99.Nanoseconds()), "p99ns")
}

// BenchmarkUpdateFromMetricsWithPercentiles benchmarks the UpdateFromMetrics method and reports percentiles.
func BenchmarkUpdateFromMetricsWithPercentiles(b *testing.B) {
	// Create a metrics state
	metricsState := NewBenthosMetricsState()

	// Create sample metrics
	metrics := Metrics{
		Input: InputMetrics{
			ConnectionFailed: 0,
			ConnectionLost:   0,
			ConnectionUp:     1,
			LatencyNS: Latency{
				P50:   127167,
				P90:   378375,
				P99:   858666,
				Sum:   3629208,
				Count: 18,
			},
			Received: 18,
		},
		Output: OutputMetrics{
			BatchSent:        18,
			ConnectionFailed: 0,
			ConnectionLost:   0,
			ConnectionUp:     1,
			Error:            0,
			LatencyNS: Latency{
				P50:   33250,
				P90:   94709,
				P99:   138250,
				Sum:   816919,
				Count: 18,
			},
			Sent: 18,
		},
		Process: ProcessMetrics{
			Processors: map[string]ProcessorMetrics{
				"root.pipeline.processors.0": {
					Label:         "test_processor",
					Received:      18,
					BatchReceived: 18,
					Sent:          18,
					BatchSent:     18,
					Error:         0,
					LatencyNS: Latency{
						P50:   10000,
						P90:   20000,
						P99:   30000,
						Sum:   200000,
						Count: 18,
					},
				},
			},
		},
	}

	// Collect execution times
	times := make([]time.Duration, b.N)

	b.ResetTimer()
	b.StopTimer()

	for i := range b.N {
		// Simulate increasing counters for realistic benchmark
		metrics.Input.Received++
		metrics.Output.Sent++

		metrics.Output.BatchSent++
		for path := range metrics.Process.Processors {
			proc := metrics.Process.Processors[path]
			proc.Received++
			proc.BatchReceived++
			proc.Sent++
			proc.BatchSent++
			metrics.Process.Processors[path] = proc
		}

		start := time.Now()

		b.StartTimer()
		metricsState.UpdateFromMetrics(metrics, uint64(i))
		b.StopTimer()

		times[i] = time.Since(start)
	}

	// Sort times for percentile calculation
	sort.Slice(times, func(i, j int) bool {
		return times[i] < times[j]
	})

	// Calculate and report percentiles
	p50 := times[int(float64(len(times))*0.50)]
	p95 := times[int(float64(len(times))*0.95)]
	p99 := times[int(float64(len(times))*0.99)]

	b.ReportMetric(float64(p50.Nanoseconds()), "p50ns")
	b.ReportMetric(float64(p95.Nanoseconds()), "p95ns")
	b.ReportMetric(float64(p99.Nanoseconds()), "p99ns")
}
