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

// The fixture spec: a /metrics body recorded from the benthos-umh the
// Makefile pins, checked name by name and parsed. testdata/capture.yaml is
// the config it was recorded with; make benthosmetrics-fixture re-records.

package benthosmetrics

import (
	"bytes"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fixturePath is the recorded /metrics body of the pinned benthos-umh 0.16.1.
const fixturePath = "testdata/metrics_benthos_umh_0_16_1.txt"

// bodyNames returns every series name the body carries.
func bodyNames(raw []byte) map[string]struct{} {
	names := make(map[string]struct{})
	for line := range bytes.Lines(raw) {
		line = bytes.TrimSuffix(line, []byte("\n"))
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		end := bytes.IndexByte(line, '{')
		if end == -1 {
			end = bytes.IndexByte(line, ' ')
		}
		if end == -1 {
			continue
		}
		names[string(line[:end])] = struct{}{}
	}
	return names
}

var _ = Describe("The real benthos-umh 0.16.1 /metrics fixture", func() {
	var raw []byte

	BeforeEach(func() {
		var err error
		raw, err = os.ReadFile(fixturePath)
		Expect(err).NotTo(HaveOccurred())
	})

	It("carries every series name the parser reads", func() {
		names := bodyNames(raw)
		for _, name := range knownSeriesNames() {
			Expect(names).To(HaveKey(name),
				"the fixture must keep series %s so a rename by a version bump fails here", name)
		}
	})

	It("ignores only names outside the input/output/processor families", func() {
		names := bodyNames(raw)
		var ignored []string
		for name := range names {
			if !knownInputName([]byte(name)) &&
				!knownOutputName([]byte(name)) &&
				!knownProcessorName([]byte(name)) {
				ignored = append(ignored, name)
			}
		}
		Expect(ignored).To(ConsistOf(
			"http_request_latency_ns", "http_request_latency_ns_sum", "http_request_latency_ns_count",
			"output_rejected"),
			"everything else in the body must be read by the parser")
	})

	It("parses every family into nonzero values", func() {
		m, err := ParseMetricsFromBytes(raw)
		Expect(err).NotTo(HaveOccurred())

		By("input: received, connection counters and latency")
		in := m.Inputs["root.input"]
		Expect(in.Received).To(BeNumerically(">", int64(0)))
		Expect(in.ConnectionUp).To(BeNumerically(">", int64(0)))
		Expect(in.LatencyNS.Count).To(Equal(int64(0)), "the idle generate input has an empty latency series")

		By("output: sent, batch sent, error, connection counters and latency")
		var sent, batchSent, errs, connUp int64
		for _, out := range m.Outputs {
			sent += out.Sent
			batchSent += out.BatchSent
			errs += out.Error
			connUp += out.ConnectionUp
		}
		Expect(sent).To(BeNumerically(">", int64(0)))
		Expect(batchSent).To(BeNumerically(">", int64(0)))
		Expect(errs).To(BeNumerically(">", int64(0)))
		Expect(connUp).To(BeNumerically(">", int64(0)))
		stdout := m.Outputs["root.output.broker.outputs.0"]
		Expect(stdout.Label).To(Equal("spike_stdout"))
		Expect(stdout.LatencyNS.Count).To(BeNumerically(">", int64(0)))
		Expect(stdout.LatencyNS.P50).To(BeNumerically(">", float64(0)))
		Expect(m.Outputs["root.output.broker.outputs.1"].Error).To(BeNumerically(">", int64(0)), "the reject branch")
		Expect(m.Outputs["root.output.broker.outputs.2"].Error).To(BeNumerically(">", int64(0)), "the dead http_client branch")

		By("processor: received, sent, error and latency")
		proc := m.Process.Processors["root.pipeline.processors.0"]
		Expect(proc.Label).To(Equal("spike_throw_half"))
		Expect(proc.Received).To(BeNumerically(">", int64(0)))
		Expect(proc.Sent).To(BeNumerically(">", int64(0)))
		Expect(proc.Error).To(BeNumerically(">", int64(0)))
		Expect(proc.LatencyNS.Count).To(BeNumerically(">", int64(0)))
		Expect(proc.LatencyNS.P50).To(BeNumerically(">", float64(0)))
		Expect(proc.LatencyNS.P99).To(BeNumerically(">", float64(0)))
	})
})

// BenchmarkParseMetricsFromBytes parses the real fixture; the per-line path
// must stay allocation-free, so the allocs/op count is the number to watch.
func BenchmarkParseMetricsFromBytes(b *testing.B) {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseMetricsFromBytes(raw); err != nil {
			b.Fatal(err)
		}
	}
}
