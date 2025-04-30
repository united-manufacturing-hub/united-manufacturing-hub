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

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"io"
	"testing"
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

// Returns a gzip compressed string of the sample metrics
// Also returns the same data but hex encoded
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

// BenchmarkGzipDecode benchmarks just the gzip decoding operation
func BenchmarkGzipDecode(b *testing.B) {
	encodedData, _ := prepareDataForBenchmark()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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

// BenchmarkHexDecode benchmarks just the hex decoding operation
func BenchmarkHexDecode(b *testing.B) {
	_, encodedAndHexedData := prepareDataForBenchmark()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := hex.DecodeString(encodedAndHexedData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMetricsParsing benchmarks the metrics parsing operation
func BenchmarkMetricsParsing(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseMetricsFromBytes([]byte(sampleMetrics))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompleteProcessing benchmarks the entire pipeline: hex decode -> gzip decode -> parse metrics
func BenchmarkCompleteProcessing(b *testing.B) {
	_, encodedAndHexedData := prepareDataForBenchmark()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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

		// Step 3: Parse metrics
		_, err = ParseMetricsFromBytes(decompressedData)
		if err != nil {
			b.Fatal(err)
		}
	}
}
