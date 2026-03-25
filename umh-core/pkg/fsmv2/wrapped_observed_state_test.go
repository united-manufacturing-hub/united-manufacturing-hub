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

package fsmv2_test

import (
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var _ = Describe("WrappedObservedState", func() {
	type TestStatus struct {
		Reachable bool  `json:"reachable"`
		LatencyMs int64 `json:"latencyMs"`
	}

	Describe("MarshalJSON", func() {
		It("produces flat JSON with framework and business fields at same level", func() {
			obs := fsmv2.WrappedObservedState[TestStatus]{
				CollectedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				State:       "running",
			}
			obs.Status = TestStatus{Reachable: true, LatencyMs: 42}

			data, err := json.Marshal(obs)
			Expect(err).NotTo(HaveOccurred())

			var flat map[string]interface{}
			Expect(json.Unmarshal(data, &flat)).To(Succeed())

			// Framework fields at top level
			Expect(flat).To(HaveKey("collected_at"))
			Expect(flat["state"]).To(Equal("running"))

			// Business fields at top level (NOT nested)
			Expect(flat["reachable"]).To(BeTrue())
			Expect(flat["latencyMs"]).To(BeNumerically("==", 42))

			// No nested keys
			Expect(flat).NotTo(HaveKey("Status"))
			Expect(flat).NotTo(HaveKey("status"))
		})

		It("round-trips via UnmarshalJSON preserving ALL fields", func() {
			original := fsmv2.WrappedObservedState[TestStatus]{
				CollectedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				State:             "running",
				ShutdownRequested: true,
				ParentMappedState: "running",
				ChildrenHealthy:   3,
				ChildrenUnhealthy: 1,
				LastActionResults: []deps.ActionResult{
					{
						Timestamp:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
						ActionType: "SayHelloAction",
						Success:    true,
						Latency:    42 * time.Millisecond,
					},
				},
			}
			original.Status = TestStatus{Reachable: true, LatencyMs: 42}
			original.Metrics.Framework = deps.FrameworkMetrics{
				StateTransitionsTotal: 5,
				TimeInCurrentStateMs:  12000,
				StateEnteredAtUnix:    1767225600,
			}
			original.Metrics.Worker = deps.Metrics{
				Counters: map[string]int64{"pull_ops": 10},
				Gauges:   map[string]float64{"latency": 1.5},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored fsmv2.WrappedObservedState[TestStatus]
			Expect(json.Unmarshal(data, &restored)).To(Succeed())

			// Framework fields
			Expect(restored.CollectedAt).To(Equal(original.CollectedAt))
			Expect(restored.State).To(Equal("running"))
			Expect(restored.ShutdownRequested).To(BeTrue())
			Expect(restored.ParentMappedState).To(Equal("running"))
			Expect(restored.ChildrenHealthy).To(Equal(3))
			Expect(restored.ChildrenUnhealthy).To(Equal(1))

			// Action results
			Expect(restored.LastActionResults).To(HaveLen(1))
			Expect(restored.LastActionResults[0].ActionType).To(Equal("SayHelloAction"))
			Expect(restored.LastActionResults[0].Success).To(BeTrue())

			// Status fields
			Expect(restored.Status.Reachable).To(BeTrue())
			Expect(restored.Status.LatencyMs).To(Equal(int64(42)))

			// MetricsEmbedder round-trip
			Expect(restored.Metrics.Framework.StateTransitionsTotal).To(Equal(int64(5)))
			Expect(restored.Metrics.Framework.TimeInCurrentStateMs).To(Equal(int64(12000)))
			Expect(restored.Metrics.Framework.StateEnteredAtUnix).To(Equal(int64(1767225600)))
			Expect(restored.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(10)))
			Expect(restored.Metrics.Worker.Gauges["latency"]).To(Equal(1.5))
		})

		It("includes ShutdownRequested=false in JSON (not omitted)", func() {
			obs := fsmv2.WrappedObservedState[TestStatus]{
				State:             "stopped",
				ShutdownRequested: false,
			}

			data, err := json.Marshal(obs)
			Expect(err).NotTo(HaveOccurred())

			// ShutdownRequested intentionally has no omitempty — false must appear in wire format.
			Expect(strings.Contains(string(data), `"ShutdownRequested":false`)).To(BeTrue(),
				"ShutdownRequested=false must be present in JSON, got: %s", string(data))
		})

		It("includes MetricsEmbedder fields at top level with correct values", func() {
			obs := fsmv2.WrappedObservedState[TestStatus]{
				State: "running",
			}
			obs.Status = TestStatus{Reachable: true}
			obs.Metrics.Framework = deps.FrameworkMetrics{
				StateTransitionsTotal: 5,
			}

			data, err := json.Marshal(obs)
			Expect(err).NotTo(HaveOccurred())

			var flat map[string]interface{}
			Expect(json.Unmarshal(data, &flat)).To(Succeed())

			// MetricsEmbedder should not produce nested "MetricsEmbedder" key
			Expect(flat).NotTo(HaveKey("MetricsEmbedder"))

			// "metrics" key present at top level with framework data
			Expect(flat).To(HaveKey("metrics"))
			metricsMap, ok := flat["metrics"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "metrics should be a JSON object")
			Expect(metricsMap).To(HaveKey("framework"))
			fwMap, ok := metricsMap["framework"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "framework should be a JSON object")
			Expect(fwMap["state_transitions_total"]).To(BeNumerically("==", 5))
		})

		It("returns error on TStatus field collision with framework field", func() {
			type BadStatus struct {
				State string `json:"state"` // collides with framework "state"
			}

			obs := fsmv2.WrappedObservedState[BadStatus]{
				State: "running",
			}
			obs.Status = BadStatus{State: "conflict"}

			_, err := json.Marshal(obs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("collides with framework field"))
		})

		It("returns error when TStatus is not a JSON object", func() {
			obs := fsmv2.WrappedObservedState[string]{
				State: "running",
			}
			obs.Status = "not-an-object"

			_, err := json.Marshal(obs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must marshal to a JSON object"))
		})
	})
})
