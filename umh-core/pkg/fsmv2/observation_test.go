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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// testStateEnteredAtUnix is a fixed Unix timestamp (2026-01-01 00:00:00 UTC) used
// in observation tests to provide a deterministic, non-zero StateEnteredAtUnix value.
const testStateEnteredAtUnix = int64(1767225600)

var _ = Describe("Observation", func() {
	type TestStatus struct {
		Reachable bool  `json:"reachable"`
		LatencyMs int64 `json:"latencyMs"`
	}

	// Compile-time assertions.
	var _ fsmv2.ObservedState = fsmv2.Observation[TestStatus]{}
	var _ fsmv2.TimestampProvider = fsmv2.Observation[TestStatus]{}

	Describe("NewObservation", func() {
		It("returns Observation with correct Status", func() {
			status := TestStatus{Reachable: true, LatencyMs: 42}
			obs := fsmv2.NewObservation(status)

			typed, ok := obs.(fsmv2.Observation[TestStatus])
			Expect(ok).To(BeTrue(), "NewObservation should return Observation[TStatus]")
			Expect(typed.Status.Reachable).To(BeTrue())
			Expect(typed.Status.LatencyMs).To(Equal(int64(42)))
		})

		It("leaves CollectedAt as zero value (set later by collector)", func() {
			obs := fsmv2.NewObservation(TestStatus{})

			typed := obs.(fsmv2.Observation[TestStatus])
			Expect(typed.CollectedAt.IsZero()).To(BeTrue(),
				"NewObservation must not set CollectedAt  -  the collector sets it")
		})

		It("satisfies the ObservedState interface", func() {
			obs := fsmv2.NewObservation(TestStatus{})
			_, ok := obs.(fsmv2.ObservedState)
			Expect(ok).To(BeTrue())
		})

		It("satisfies all collector duck-type setter interfaces", func() {
			obs := fsmv2.NewObservation(TestStatus{})

			_, ok := obs.(interface {
				SetState(string) fsmv2.ObservedState
			})
			Expect(ok).To(BeTrue(), "must satisfy SetState duck-type")

			_, ok = obs.(interface {
				SetShutdownRequested(bool) fsmv2.ObservedState
			})
			Expect(ok).To(BeTrue(), "must satisfy SetShutdownRequested duck-type")

			_, ok = obs.(interface {
				SetChildrenCounts(int, int) fsmv2.ObservedState
			})
			Expect(ok).To(BeTrue(), "must satisfy SetChildrenCounts duck-type")

			_, ok = obs.(interface {
				SetChildrenView(config.ChildrenView) fsmv2.ObservedState
			})
			Expect(ok).To(BeTrue(), "must satisfy SetChildrenView duck-type")

			_, ok = obs.(interface {
				SetWorkerMetrics(deps.Metrics) fsmv2.ObservedState
			})
			Expect(ok).To(BeTrue(), "must satisfy SetWorkerMetrics duck-type")

			_, ok = obs.(interface {
				SetFrameworkMetrics(deps.FrameworkMetrics) fsmv2.ObservedState
			})
			Expect(ok).To(BeTrue(), "must satisfy SetFrameworkMetrics duck-type")

			_, ok = obs.(interface {
				SetActionHistory([]deps.ActionResult) fsmv2.ObservedState
			})
			Expect(ok).To(BeTrue(), "must satisfy SetActionHistory duck-type")

			_, ok = obs.(interface {
				SetParentMappedState(string) fsmv2.ObservedState
			})
			Expect(ok).To(BeTrue(), "must satisfy SetParentMappedState duck-type")
		})
	})

	Describe("ObservedState interface", func() {
		It("GetTimestamp returns CollectedAt", func() {
			ts := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
			obs := fsmv2.Observation[TestStatus]{
				CollectedAt: ts,
			}
			Expect(obs.GetTimestamp()).To(Equal(ts))
		})
	})


	Describe("MarshalJSON", func() {
		It("produces flat JSON with framework and business fields at same level", func() {
			obs := fsmv2.Observation[TestStatus]{
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
			original := fsmv2.Observation[TestStatus]{
				CollectedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				State:             "running",
				ShutdownRequested: true,
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
				StateEnteredAtUnix:    testStateEnteredAtUnix,
			}
			original.Metrics.Worker = deps.Metrics{
				Counters: map[string]int64{"pull_ops": 10},
				Gauges:   map[string]float64{"latency": 1.5},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored fsmv2.Observation[TestStatus]
			Expect(json.Unmarshal(data, &restored)).To(Succeed())

			// Framework fields
			Expect(restored.CollectedAt).To(Equal(original.CollectedAt))
			Expect(restored.State).To(Equal("running"))
			Expect(restored.ShutdownRequested).To(BeTrue())
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
			Expect(restored.Metrics.Framework.StateEnteredAtUnix).To(Equal(testStateEnteredAtUnix))
			Expect(restored.Metrics.Worker.Counters["pull_ops"]).To(Equal(int64(10)))
			Expect(restored.Metrics.Worker.Gauges["latency"]).To(Equal(1.5))
		})

		It("includes ShutdownRequested=false in JSON (not omitted)", func() {
			obs := fsmv2.Observation[TestStatus]{
				State:             "stopped",
				ShutdownRequested: false,
			}

			data, err := json.Marshal(obs)
			Expect(err).NotTo(HaveOccurred())

			// ShutdownRequested intentionally has no omitempty  -  false must appear in wire format.
			Expect(string(data)).To(ContainSubstring(`"ShutdownRequested":false`),
				"ShutdownRequested=false must be present in JSON, got: %s", string(data))
		})

		It("includes MetricsEmbedder fields at top level with correct values", func() {
			obs := fsmv2.Observation[TestStatus]{
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

			obs := fsmv2.Observation[BadStatus]{
				State: "running",
			}
			obs.Status = BadStatus{State: "conflict"}

			_, err := json.Marshal(obs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("collides with framework field"))
		})

		It("returns error when TStatus is not a JSON object", func() {
			obs := fsmv2.Observation[string]{
				State: "running",
			}
			obs.Status = "not-an-object"

			_, err := json.Marshal(obs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must marshal to a JSON object"))
		})
	})
})

var _ = Describe("DetectFieldCollisions", func() {
	type CleanStatus struct {
		Reachable bool  `json:"reachable"`
		LatencyMs int64 `json:"latencyMs"`
	}

	type StateCollision struct {
		State string `json:"state"`
	}

	type CollectedAtCollision struct {
		CollectedAt string `json:"collected_at"`
	}

	type MetricsCollision struct {
		Metrics string `json:"metrics"`
	}

	type ChildrenViewCollision struct {
		ChildrenView string `json:"childrenView"`
	}

	type SkippedField struct {
		InternalOnly string `json:"-"`
		Reachable    bool   `json:"reachable"`
	}

	It("returns nil for clean status with no collisions", func() {
		err := fsmv2.DetectFieldCollisions[CleanStatus]()
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns error when TStatus has 'state' field colliding with framework", func() {
		err := fsmv2.DetectFieldCollisions[StateCollision]()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("state"))
	})

	It("returns error when TStatus has 'collected_at' field colliding with framework", func() {
		err := fsmv2.DetectFieldCollisions[CollectedAtCollision]()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("collected_at"))
	})

	It("returns error when TStatus has 'metrics' field colliding with MetricsEmbedder", func() {
		err := fsmv2.DetectFieldCollisions[MetricsCollision]()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("metrics"))
	})

	It("returns error when TStatus has 'childrenView' field colliding with framework", func() {
		err := fsmv2.DetectFieldCollisions[ChildrenViewCollision]()
		Expect(err).To(HaveOccurred(),
			"ChildrenView is now a serialized framework field; TStatus declaring 'childrenView' must be rejected (CHANGE-6)")
		Expect(err.Error()).To(ContainSubstring("childrenView"))
	})

	It("skips fields with json:\"-\" tag", func() {
		err := fsmv2.DetectFieldCollisions[SkippedField]()
		Expect(err).NotTo(HaveOccurred())
	})

	// Embedded structs with option-only tags (e.g., `json:",omitempty"`) still
	// promote their inner fields to the parent JSON object. The detector must
	// recurse into them, otherwise a collision via a promoted field is missed.
	It("detects collisions through embedded structs tagged with options only", func() {
		type stateHolder struct {
			State string `json:"state"`
		}

		type EmbeddedOmitempty struct {
			stateHolder `json:",omitempty"`
		}

		err := fsmv2.DetectFieldCollisions[EmbeddedOmitempty]()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("state"))
	})
})
