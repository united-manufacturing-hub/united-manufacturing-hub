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

// A switch output has one output_sent series per route, and the parse keeps
// each.

package benthosmetrics

import (
	"bytes"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// switchOutputMetrics models the Prometheus output Benthos emits for a
// three-route switch config. Coordinators (switch, broker, fallback) emit no
// output_sent of their own; every leaf output keeps its own series under
// its own path label, so a switch has one series per leaf and no aggregate.
const switchOutputMetrics = `# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 20
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output.switch.cases.0.output.fallback.0"} 10
output_sent{label="",path="root.output.switch.cases.0.output.fallback.1"} 0
output_sent{label="",path="root.output.switch.cases.1.output.fallback.0"} 7
output_sent{label="",path="root.output.switch.cases.1.output.fallback.1"} 0
output_sent{label="",path="root.output.switch.cases.2.output.fallback.0"} 3
output_sent{label="",path="root.output.switch.cases.2.output.fallback.1"} 0
# HELP output_batch_sent Benthos Counter metric
# TYPE output_batch_sent counter
output_batch_sent{label="",path="root.output.switch.cases.0.output.fallback.0"} 10
output_batch_sent{label="",path="root.output.switch.cases.0.output.fallback.1"} 0
output_batch_sent{label="",path="root.output.switch.cases.1.output.fallback.0"} 7
output_batch_sent{label="",path="root.output.switch.cases.1.output.fallback.1"} 0
output_batch_sent{label="",path="root.output.switch.cases.2.output.fallback.0"} 3
output_batch_sent{label="",path="root.output.switch.cases.2.output.fallback.1"} 0
# HELP output_error Benthos Counter metric
# TYPE output_error counter
output_error{label="",path="root.output.switch.cases.0.output.fallback.0"} 1
output_error{label="",path="root.output.switch.cases.1.output.fallback.0"} 2
output_error{label="",path="root.output.switch.cases.2.output.fallback.0"} 0
# HELP output_connection_up Benthos Counter metric
# TYPE output_connection_up counter
output_connection_up{label="",path="root.output.switch.cases.0.output.fallback.0"} 1
output_connection_up{label="",path="root.output.switch.cases.1.output.fallback.0"} 1
output_connection_up{label="",path="root.output.switch.cases.2.output.fallback.0"} 1
`

var _ = Describe("Parsing a switch config's output_sent", func() {
	It("aggregates output_sent across the switch routes", func() {
		m, err := parseMetrics([]byte(switchOutputMetrics))
		Expect(err).NotTo(HaveOccurred())

		By("summing output_sent across all three routes via the total")
		Expect(m.OutputSentTotal()).To(Equal(int64(20)), "10 + 7 + 3 across the routes")

		By("summing the other counters straight off the map")

		var batchSent, errorTotal, connectionUp int64
		for _, out := range m.Outputs {
			batchSent += out.BatchSent
			errorTotal += out.Error
			connectionUp += out.ConnectionUp
		}

		Expect(batchSent).To(Equal(int64(20)))
		Expect(errorTotal).To(Equal(int64(3)), "1 + 2 + 0")
		Expect(connectionUp).To(Equal(int64(3)), "1 + 1 + 1")

		By("keeping one entry per output path so a collapsed map fails even when totals add up")
		Expect(m.Outputs).To(HaveLen(6), "3 cases x 2 fallback leaves")

		By("preserving the per-leaf value so a route-summing parser fails even when totals are correct")

		const leafPath = "root.output.switch.cases.0.output.fallback.0"
		Expect(m.Outputs).To(HaveKey(leafPath))
		Expect(m.Outputs[leafPath].Sent).To(Equal(int64(10)), "the case-0 first leaf")
		Expect(m.Outputs[leafPath].Path).To(Equal(leafPath), "the parser populates Path on the entry")

		By("keeping the input entry under its own path")
		Expect(m.Inputs).To(HaveKey("root.input"))
		Expect(m.Inputs["root.input"].Received).To(Equal(int64(20)))
	})
})

var _ = Describe("Parsing the counter values", func() {
	It("reads an exponent-form counter", func() {
		m, err := parseMetrics([]byte("output_sent{label=\"\",path=\"root.output\"} 1.234567e+06\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.OutputSentTotal()).To(Equal(int64(1234567)))
	})

	It("reads a NaN quantile from an idle summary as no measurement", func() {
		m, err := parseMetrics([]byte(
			"input_received{label=\"\",path=\"root.input\"} 7\n" +
				"input_latency_ns{label=\"\",path=\"root.input\",quantile=\"0.5\"} NaN\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.InputReceivedTotal()).To(Equal(int64(7)))
		Expect(m.Inputs["root.input"].LatencyNS.P50).To(BeNil(),
			"an idle series is not a latency of 0 ns")
	})

	It("reads a fractional quantile as it is", func() {
		m, err := parseMetrics([]byte(
			"output_latency_ns{label=\"\",path=\"root.output\",quantile=\"0.99\"} 12.5\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Outputs["root.output"].LatencyNS.P99).To(HaveValue(Equal(12.5)))
	})

	It("reads the sample value, not a trailing timestamp", func() {
		m, err := parseMetrics([]byte(
			"input_received{label=\"a b}\",path=\"root.input\"} 5 1700000000000\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.InputReceivedTotal()).To(Equal(int64(5)))
	})

	It("rejects a NaN counter", func() {
		_, err := parseMetrics([]byte("input_received{label=\"\",path=\"root.input\"} NaN\n"))
		Expect(err).To(MatchError(ContainSubstring("input_received")))
	})

	It("rejects a negative counter", func() {
		_, err := parseMetrics([]byte("output_sent{label=\"\",path=\"root.output\"} -3\n"))
		Expect(err).To(MatchError(ContainSubstring("output_sent")))
	})

	It("sums Received across inputs", func() {
		m := Metrics{Inputs: map[string]InputMetrics{"a": {Received: 3}, "b": {Received: 4}}}
		Expect(m.InputReceivedTotal()).To(Equal(int64(7)))
	})

	It("parses processor counters, latency quantiles and labels", func() {
		m, err := parseMetrics([]byte(
			"processor_received{label=\"p\",path=\"root.pipeline.processors.0\"} 9\n" +
				"output_latency_ns{label=\"out\",path=\"root.output\",quantile=\"0.9\"} 42\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Process.Processors["root.pipeline.processors.0"].Received).To(Equal(int64(9)))
		Expect(m.Outputs["root.output"].LatencyNS.P90).To(HaveValue(Equal(float64(42))))
		Expect(m.Outputs["root.output"].Label).To(Equal("out"))
	})

	It("walks the body without allocating per line", func() {
		nl := bytes.Repeat([]byte{'\n'}, 4<<20)

		runtime.GC()

		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)

		_, _ = parseMetrics(nl)

		runtime.ReadMemStats(&b)
		Expect(b.TotalAlloc-a.TotalAlloc).To(BeNumerically("<=", 1<<20),
			"a body of newlines must not allocate a slice header per line")
	})

	It("files repeated lines without allocating a map key per line", func() {
		// The maps are keyed by the Path string the entry already holds;
		// keying by string(path) would allocate a fresh key on every line.
		body := bytes.Repeat([]byte("input_received{label=\"\",path=\"root.input\"} 1\n"), 1000)
		allocs := testing.AllocsPerRun(10, func() {
			m, err := parseMetrics(body)
			Expect(err).NotTo(HaveOccurred())
			Expect(m.Inputs).To(HaveLen(1))
		})
		Expect(allocs).To(BeNumerically("<", 100),
			"1000 lines of one path cost a handful of allocations, not one per line")
	})
})

var _ = Describe("Parsing the latency sum and count series", func() {
	It("keeps _latency_ns_sum and _latency_ns_count for an input, an output and a processor", func() {
		m, err := parseMetrics([]byte(
			"input_latency_ns_sum{label=\"\",path=\"root.input\"} 100000\n" +
				"input_latency_ns_count{label=\"\",path=\"root.input\"} 50\n" +
				"output_latency_ns_sum{label=\"\",path=\"root.output\"} 200000\n" +
				"output_latency_ns_count{label=\"\",path=\"root.output\"} 80\n" +
				"processor_latency_ns_sum{label=\"p\",path=\"root.pipeline.processors.0\"} 300000\n" +
				"processor_latency_ns_count{label=\"p\",path=\"root.pipeline.processors.0\"} 120\n"))
		Expect(err).NotTo(HaveOccurred())

		By("folding the input's sum and count into its latency entry")
		Expect(m.Inputs).To(HaveKey("root.input"))
		Expect(m.Inputs["root.input"].LatencyNS.Sum).To(Equal(float64(100000)))
		Expect(m.Inputs["root.input"].LatencyNS.Count).To(Equal(int64(50)))

		By("folding the output's sum and count into its latency entry")
		Expect(m.Outputs).To(HaveKey("root.output"))
		Expect(m.Outputs["root.output"].LatencyNS.Sum).To(Equal(float64(200000)))
		Expect(m.Outputs["root.output"].LatencyNS.Count).To(Equal(int64(80)))

		By("folding the processor's sum and count into its latency entry")
		Expect(m.Process.Processors).To(HaveKey("root.pipeline.processors.0"))
		Expect(m.Process.Processors["root.pipeline.processors.0"].LatencyNS.Sum).To(Equal(float64(300000)))
		Expect(m.Process.Processors["root.pipeline.processors.0"].LatencyNS.Count).To(Equal(int64(120)))
	})
})

var _ = Describe("Parsing label values and unknown names", func() {
	It("keeps the path when a label value contains a closing brace", func() {
		// A } inside a label value must not end the label set: the path is
		// read past it.
		m, err := parseMetrics([]byte(
			"input_received{label=\"a}b\",path=\"root.input.x\"} 5\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Inputs).To(HaveKey("root.input.x"),
			"the } in the label value is data, not the end of the label set")
		Expect(m.Inputs["root.input.x"]).To(Equal(InputMetrics{
			Path: "root.input.x", Label: "a}b", Received: 5,
		}))
	})

	It("ignores an unknown input_* line whose value does not parse", func() {
		// The value is parsed only for a name the parser knows, so a user
		// metric processor's overflowing gauge cannot fail the whole scrape.
		m, err := parseMetrics([]byte(
			"input_received{label=\"\",path=\"root.input\"} 7\n" +
				"input_custom_gauge{label=\"\",path=\"root.input\"} 1e999\n" +
				"output_sent{label=\"\",path=\"root.output\"} 4\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.InputReceivedTotal()).To(Equal(int64(7)),
			"the valid counters are kept")
		Expect(m.OutputSentTotal()).To(Equal(int64(4)))
		Expect(m.Inputs).To(HaveLen(1), "the unknown line leaves no entry behind")
	})

	It("ignores an unknown output_* line whose value does not parse", func() {
		// The gate mirrors the input gate, so a user metric's overflowing
		// gauge cannot fail the whole scrape either.
		m, err := parseMetrics([]byte(
			"input_received{label=\"\",path=\"root.input\"} 7\n" +
				"output_custom_gauge{label=\"\",path=\"root.output\"} 1e999\n" +
				"output_sent{label=\"\",path=\"root.output\"} 4\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.OutputSentTotal()).To(Equal(int64(4)),
			"the valid counters are kept")
		Expect(m.Outputs).To(HaveLen(1), "the unknown line leaves no entry behind")
	})

	It("ignores an unknown processor_* line whose value does not parse", func() {
		m, err := parseMetrics([]byte(
			"processor_received{label=\"\",path=\"root.pipeline.processors.0\"} 7\n" +
				"processor_custom_gauge{label=\"\",path=\"root.pipeline.processors.0\"} 1e999\n" +
				"processor_sent{label=\"\",path=\"root.pipeline.processors.0\"} 4\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Process.Processors).To(HaveLen(1),
			"the unknown line leaves no entry behind")
		Expect(m.Process.Processors["root.pipeline.processors.0"].Received).To(Equal(int64(7)))
		Expect(m.Process.Processors["root.pipeline.processors.0"].Sent).To(Equal(int64(4)))
	})

	It("ignores a user metric whose name ends in a latency suffix", func() {
		// Only the exact series names pass the gates, so a
		// user metric such as input_custom_latency_ns can neither overwrite
		// a real latency value nor fail the scrape.
		m, err := parseMetrics([]byte(
			"input_latency_ns{label=\"\",path=\"root.input\",quantile=\"0.5\"} 111\n" +
				"input_custom_latency_ns{path=\"root.input\",quantile=\"0.5\"} 999\n" +
				"output_latency_ns_sum{label=\"\",path=\"root.output\"} 5\n" +
				"output_custom_latency_ns_sum 1e999\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(m.Inputs["root.input"].LatencyNS.P50).To(HaveValue(Equal(float64(111))),
			"the real latency sample is unchanged")
		Expect(m.Outputs["root.output"].LatencyNS.Sum).To(Equal(float64(5)),
			"the real sum is unchanged")
	})
})
