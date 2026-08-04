// Copyright 2026 UMH Systems GmbH
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

package benthos_monitor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

const mixedDropMetrics = `# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 4676
# HELP messages_dropped Benthos Counter metric
# TYPE messages_dropped counter
messages_dropped{label="",path="root.output",reason="contract_mismatch"} 3666
messages_dropped{label="",path="root.output",reason="missing_timestamp"} 10
messages_dropped{label="",path="root.input",reason="unsupported_datatype"} 5
messages_dropped{label="",path="root.pipeline.processors.0",reason="deliberate"} 20
# HELP output_connection_up Benthos Counter metric
# TYPE output_connection_up counter
output_connection_up{label="",path="root.output"} 1
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output"} 4676
# HELP output_batch_sent Benthos Counter metric
# TYPE output_batch_sent counter
output_batch_sent{label="",path="root.output"} 999
`

const allDroppedMetrics = `# HELP input_received Benthos Counter metric
# TYPE input_received counter
input_received{label="",path="root.input"} 4676
# HELP messages_dropped Benthos Counter metric
# TYPE messages_dropped counter
messages_dropped{label="",path="root.output",reason="contract_mismatch"} 4676
# HELP output_connection_up Benthos Counter metric
# TYPE output_connection_up counter
output_connection_up{label="",path="root.output"} 1
# HELP output_sent Benthos Counter metric
# TYPE output_sent counter
output_sent{label="",path="root.output"} 4676
`

var _ = Describe("Dropped output messages", Label("metrics_state"), func() {
	Context("parsing", func() {
		It("attributes output-path drops to the output and ignores the rest", func() {
			m, err := benthos_monitor.ParseMetricsFromBytes([]byte(mixedDropMetrics))
			Expect(err).NotTo(HaveOccurred())

			Expect(m.OutputSentTotal()).To(Equal(int64(4676)))
			Expect(m.OutputDroppedTotal()).To(Equal(int64(3676)), "both output-path reasons sum; the input and processor drops are excluded")
			Expect(m.Outputs["root.output"].Dropped).To(Equal(int64(3676)))
		})

		It("reports the messages actually written", func() {
			m, err := benthos_monitor.ParseMetricsFromBytes([]byte(mixedDropMetrics))
			Expect(err).NotTo(HaveOccurred())

			Expect(m.OutputWrittenTotal()).To(Equal(int64(1000)))
		})

		It("reports zero written when every accepted message was dropped", func() {
			m, err := benthos_monitor.ParseMetricsFromBytes([]byte(allDroppedMetrics))
			Expect(err).NotTo(HaveOccurred())

			Expect(m.OutputSentTotal()).To(Equal(int64(4676)), "benthos still counts them as sent")
			Expect(m.OutputWrittenTotal()).To(BeZero())
		})

		It("does not create an output instance for a drop on a path that has no output", func() {
			m, err := benthos_monitor.ParseMetricsFromBytes([]byte(mixedDropMetrics))
			Expect(err).NotTo(HaveOccurred())

			Expect(m.Outputs).To(HaveLen(1))
			Expect(m.Outputs).To(HaveKey("root.output"))
		})

		It("never reports a negative number of written messages", func() {
			m := benthos_monitor.Metrics{
				Outputs: map[string]benthos_monitor.OutputInstance{
					"root.output": {Sent: 4, Dropped: 10},
				},
			}

			Expect(m.OutputWrittenTotal()).To(BeZero())
		})
	})

	Context("throughput", func() {
		It("counts written messages, not accepted ones", func() {
			state := benthos_monitor.NewBenthosMetricsState()

			m, err := benthos_monitor.ParseMetricsFromBytes([]byte(mixedDropMetrics))
			Expect(err).NotTo(HaveOccurred())

			state.UpdateFromMetrics(m, 0)

			Expect(state.Output.LastCount).To(Equal(int64(1000)))
			Expect(state.Output.MessagesPerTick).To(Equal(float64(1000)))
		})

		It("reports no write throughput for a bridge that drops everything", func() {
			state := benthos_monitor.NewBenthosMetricsState()

			m, err := benthos_monitor.ParseMetricsFromBytes([]byte(allDroppedMetrics))
			Expect(err).NotTo(HaveOccurred())

			state.UpdateFromMetrics(m, 0)

			Expect(state.Output.LastCount).To(BeZero())
			Expect(state.Output.MessagesPerTick).To(BeZero())
		})

		It("holds write throughput at zero while the drops keep pace with the sends", func() {
			state := benthos_monitor.NewBenthosMetricsState()

			m, err := benthos_monitor.ParseMetricsFromBytes([]byte(allDroppedMetrics))
			Expect(err).NotTo(HaveOccurred())

			state.UpdateFromMetrics(m, 0)

			later := benthos_monitor.Metrics{
				Inputs: map[string]benthos_monitor.InputInstance{
					"root.input": {Received: 9352},
				},
				Outputs: map[string]benthos_monitor.OutputInstance{
					"root.output": {Sent: 9352, Dropped: 9352, BatchSent: 2000},
				},
			}

			state.UpdateFromMetrics(later, 1)

			Expect(state.Output.MessagesPerTick).To(BeZero())
			Expect(state.Input.MessagesPerTick).NotTo(BeZero(), "the bridge is still receiving; only its writes are zero")
		})
	})
})
