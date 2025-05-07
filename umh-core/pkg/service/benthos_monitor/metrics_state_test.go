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

package benthos_monitor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

var _ = Describe("MetricsState", Label("metrics_state"), func() {
	var (
		state *benthos_monitor.BenthosMetricsState
		tick  uint64
	)

	BeforeEach(func() {
		state = benthos_monitor.NewBenthosMetricsState()
		tick = 0
	})

	Context("NewBenthosMetricsState", func() {
		It("should initialize with empty processors map", func() {
			Expect(state.Processors).NotTo(BeNil())
			Expect(state.Processors).To(BeEmpty())
			Expect(state.LastTick).To(BeZero())
			Expect(state.IsActive).To(BeFalse())
		})
	})

	Context("UpdateFromMetrics", func() {
		It("should handle first update as baseline", func() {
			metrics := benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 100,
				},
				Output: benthos_monitor.OutputMetrics{
					Sent:      90,
					BatchSent: 10,
				},
			}

			state.UpdateFromMetrics(metrics, tick)
			tick++

			Expect(state.Input.LastCount).To(Equal(int64(100)))
			Expect(state.Input.MessagesPerTick).To(Equal(float64(100))) // First tick has throughput of input.received
			Expect(state.Output.LastCount).To(Equal(int64(90)))
			Expect(state.Output.MessagesPerTick).To(Equal(float64(90))) // First tick has throughput of output.sent
			Expect(state.Output.LastBatchCount).To(Equal(int64(10)))
			Expect(state.Output.BatchesPerTick).To(Equal(float64(10))) // First tick has throughput of output.batch_sent
			Expect(state.LastTick).To(Equal(uint64(0)))
			Expect(state.IsActive).To(BeTrue()) // Active since we have throughput
		})

		It("should handle counter reset", func() {
			// First update
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 100,
				},
			}, tick)
			tick++

			// Second update to establish throughput
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 150,
				},
			}, tick)
			tick++

			// Counter reset (new count lower than last count)
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 50, // Reset to lower value
				},
			}, tick)
			tick++

			Expect(state.Input.LastCount).To(Equal(int64(50)))
			Expect(state.Input.MessagesPerTick).To(Equal(float64(50))) // Should reset window and start fresh
			Expect(state.IsActive).To(BeTrue())
		})

		It("should calculate rates correctly over multiple ticks", func() {
			// First update at tick 0
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 100,
				},
				Output: benthos_monitor.OutputMetrics{
					Sent:      90,
					BatchSent: 10,
				},
			}, tick)
			tick++

			// Second update at tick 1
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 160, // +60 over 1 tick = 60 per tick
				},
				Output: benthos_monitor.OutputMetrics{
					Sent:      140, // +50 over 1 tick = 50 per tick
					BatchSent: 20,  // +10 over 1 tick = 10 per tick
				},
			}, tick)
			tick++

			Expect(state.Input.MessagesPerTick).To(Equal(float64(60)))  // (160-100)/1
			Expect(state.Output.MessagesPerTick).To(Equal(float64(50))) // (140-90)/1
			Expect(state.IsActive).To(BeTrue())
		})

		It("should handle processor metrics", func() {
			metrics := benthos_monitor.Metrics{
				Process: benthos_monitor.ProcessMetrics{
					Processors: map[string]benthos_monitor.ProcessorMetrics{
						"proc1": {
							Sent:      100,
							BatchSent: 10,
						},
						"proc2": {
							Sent:      200,
							BatchSent: 20,
						},
					},
				},
			}

			state.UpdateFromMetrics(metrics, tick)
			tick++
			Expect(state.Processors).To(HaveLen(2))
			Expect(state.Processors["proc1"].LastCount).To(Equal(int64(100)))
			Expect(state.Processors["proc1"].LastBatchCount).To(Equal(int64(10)))
			Expect(state.Processors["proc2"].LastCount).To(Equal(int64(200)))
			Expect(state.Processors["proc2"].LastBatchCount).To(Equal(int64(20)))
		})

		It("should update activity status correctly", func() {
			// No activity
			state.UpdateFromMetrics(benthos_monitor.Metrics{}, tick)
			tick++
			Expect(state.IsActive).To(BeFalse())

			// First input update
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 100,
				},
			}, tick)
			tick++
			Expect(state.IsActive).To(BeTrue()) // Active since we have throughput

			// Second update shows activity
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 200,
				},
			}, tick)
			tick++
			Expect(state.IsActive).To(BeTrue()) // Still active since we have throughput

			// No new input activity (same count)
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 200, // Same as last tick
				},
			}, tick)
			tick++
			Expect(state.IsActive).To(BeTrue()) // Still active as we had throughput previously

			// No new input activity for a while
			for i := uint64(0); i < benthos_monitor.ThroughputWindowSize+1; i++ {
				state.UpdateFromMetrics(benthos_monitor.Metrics{Input: benthos_monitor.InputMetrics{Received: 200}}, tick)
				tick++
			}
			Expect(state.IsActive).To(BeFalse()) // Not active as we had no throughput for a while

			// New input activity
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 300, // New messages
				},
			}, tick)
			tick++
			Expect(state.IsActive).To(BeTrue()) // Active again due to throughput
		})

		It("should handle high throughput (60k msg/sec) without counter issues", func() {
			// Simulate a baseline
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 0,
				},
			}, tick)
			tick++

			// Simulate 60,000 messages received in one tick
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 60000,
				},
			}, tick)
			tick++

			Expect(state.Input.LastCount).To(Equal(int64(60000)))
			Expect(state.Input.MessagesPerTick).To(Equal(float64(60000)))
			Expect(state.IsActive).To(BeTrue())

			// Simulate another 60,000 messages in the next tick (total 120,000)
			state.UpdateFromMetrics(benthos_monitor.Metrics{
				Input: benthos_monitor.InputMetrics{
					Received: 120000,
				},
			}, tick)
			tick++

			Expect(state.Input.LastCount).To(Equal(int64(120000)))
			Expect(state.Input.MessagesPerTick).To(Equal(float64(60000))) // (120000-60000)/1
			Expect(state.IsActive).To(BeTrue())
		})
	})
})
