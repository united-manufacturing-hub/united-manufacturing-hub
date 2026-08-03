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

// Rung 5 (capstone) of PR B: the positive-controlled end-to-end proof for
// property 1 of the design -  a REAL nmap worker, built through its real
// registration, driven through a REAL Collector and a REAL TriangularStore,
// carrying framework metrics on its saved Observation.
//
// nmap is the sharpest case the design exists to fix: it registers
// MonitorSpec[config.NmapConfig, NmapStatus, struct{}] with no NewDeps
// (nmap/nmap.go:101), so its bound deps are struct{} and fail the
// baseDepsAccessor assertion. Before rung 1 the collector's guard sat in front
// of the framework-metrics injection, so nmap's Observation carried a zero
// FrameworkMetrics struct and its drained action history was discarded. After
// rung 1 the collector injects both from its own locals before the deps guard.
//
// The assertion is a sentinel: the collector's FrameworkMetricsProvider returns
// a value carrying TimeInCurrentStateMs = 987654, and recovering exactly that
// back off the saved Observation proves framework metrics are present, not
// merely that the worker produced an Observation. The sentinel is what makes
// the positive control meaningful: the same spec reports ABSENT (the sentinel
// is lost) against the pre-rung-1 parent commit 47062a084, where the guard
// still sits in front of step 3.

package collection_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"

	// Populates the factory registry via init(); without it NewWorkerByType
	// returns "unknown worker type" and the test fails loudly rather than
	// silently proving nothing.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/nmap"
)

// nmapCapstoneSentinel is the value injected through the collector's
// FrameworkMetricsProvider. Recovering exactly this off the saved Observation
// is the positive proof that framework metrics reached nmap's Observation.
const nmapCapstoneSentinel int64 = 987654

// nmapObservedProbe mirrors the JSON shape the collector persists for a
// framework-flattened Observation, so a generic LoadObservedTyped read can
// recover just the metrics block.
type nmapObservedProbe struct {
	Metrics deps.MetricsContainer `json:"metrics"`
}

var _ = Describe("Capstone: real nmap worker gets framework metrics through a real collector", func() {
	It("persists framework metrics on the Observation of a real nmap worker carrying sentinel TimeInCurrentStateMs", func() {
		const workerType = "nmap"

		id := deps.Identity{ID: "nmap-capstone", WorkerType: workerType, Name: "nmap-capstone"}

		// Build a REAL nmap worker through its real registration. deps nil:
		// nmap's MonitorSpec has no NewDeps, so it needs no dependency payload.
		worker, err := factory.NewWorkerByType(workerType, id, deps.NewNopFSMLogger(), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(worker).NotTo(BeNil())

		store := supervisor.CreateTestTriangularStoreForWorkerType(workerType)

		cfg := collection.CollectorConfig[fsmv2.ObservedState]{
			Worker:   worker,
			Identity: id,
			Store:    store,
			Logger:   deps.NewNopFSMLogger(),
			// No background ticks; exactly one synchronous tick below.
			ObservationInterval: time.Hour,
			ObservationTimeout:  3 * time.Second,
			DesiredStateProvider: func() (fsmv2.DesiredState, error) {
				return &fsmv2.WrappedDesiredState[config.NmapConfig]{Config: config.NmapConfig{}}, nil
			},
			// The sentinel: what the supervisor captured for this worker this
			// tick. Recovering it proves the collector's own values landed on the
			// Observation rather than something read back out of the worker's deps
			// (nmap has a struct{} deps that cannot carry framework state).
			FrameworkMetricsProvider: func() *deps.FrameworkMetrics {
				return &deps.FrameworkMetrics{TimeInCurrentStateMs: nmapCapstoneSentinel}
			},
		}

		c := collection.NewCollector[fsmv2.ObservedState](cfg)

		ctx := context.Background()
		Expect(c.Start(ctx)).To(Succeed())
		defer c.Stop(context.Background())
		Expect(c.CollectFinalObservation(context.Background())).To(Succeed())

		var probe nmapObservedProbe
		Expect(store.LoadObservedTyped(ctx, workerType, id.ID, &probe)).To(Succeed())

		// The positive assertion: the sentinel survived the full pipeline. If the
		// guard still sat in front of step 3, TimeInCurrentStateMs would be 0.
		Expect(probe.Metrics.Framework.TimeInCurrentStateMs).To(Equal(nmapCapstoneSentinel),
			"framework TimeInCurrentStateMs sentinel did not survive on the real nmap worker's Observation")
	})
})
