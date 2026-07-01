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

// ENG-5006 reproduction: output metrics are broken for `switch` outputs.
//
// Benthos's `Manager.IntoPath()` injects a `path` label per output instance.
// Coordinators (switch/broker/fallback) do not emit `output_sent` themselves;
// only leaf `AsyncWriter`s do. So a switch with N cases emits N distinct
// `output_sent` series with different `path` labels and NO top-level aggregate.
//
// ParseMetricsFromBytes previously treated `output_sent` as a singleton
// (last-wins overwrite in the case branch), so the UI showed only one route's
// counter. The parser now aggregates per-path into Metrics.Outputs; this
// test pins that behavior.
package benthos_monitor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthosmetrics"
)

// switchOutputMetrics models the prometheus output benthos emits for the
// reporter's three-route switch config in ENG-5006 (job_start/job_end/logbook).
// Each case has a sql_raw → stdout fallback; the leaf AsyncWriters are
// sql_raw (path .../fallback.0) and stdout (path .../fallback.1).
//
// Numbers: 10 messages to case 0, 7 to case 1, 3 to case 2. Total 20.
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

const (
	wantSentTotal         int64 = 10 + 7 + 3 // 20
	wantBatchSentTotal    int64 = 10 + 7 + 3
	wantErrorTotal        int64 = 1 + 2 + 0 // 3
	wantConnectionUpTotal int64 = 1 + 1 + 1 // 3
)

var _ = Describe("ENG-5006 switch output metrics", func() {
	It("aggregates output_sent across switch routes", func() {
		m, err := benthosmetrics.ParseMetricsFromBytes([]byte(switchOutputMetrics))
		Expect(err).NotTo(HaveOccurred())

		By("summing totals across all three switch routes via Total helpers")
		Expect(m.OutputSentTotal()).To(Equal(wantSentTotal), "sum across switch routes")
		Expect(m.OutputBatchSentTotal()).To(Equal(wantBatchSentTotal))
		Expect(m.OutputConnectionUpTotal()).To(Equal(wantConnectionUpTotal))

		By("summing the map directly to keep the per-path assertion explicit alongside the total")
		var errorTotal int64
		for _, out := range m.Outputs {
			errorTotal += out.Error
		}
		Expect(errorTotal).To(Equal(wantErrorTotal))

		By("keeping one entry per output path so a collapsed map fails even when totals add up")
		Expect(m.Outputs).To(HaveLen(6), "3 cases x 2 fallback leaves")

		By("preserving the per-leaf value so a route-summing parser fails even when totals are correct")
		const leafPath = "root.output.switch.cases.0.output.fallback.0"
		Expect(m.Outputs).To(HaveKey(leafPath))
		Expect(m.Outputs[leafPath].Sent).To(Equal(int64(10)), "the case-0 sql_raw leaf")
		Expect(m.Outputs[leafPath].Path).To(Equal(leafPath), "parser must populate Path on the instance")
	})
})
