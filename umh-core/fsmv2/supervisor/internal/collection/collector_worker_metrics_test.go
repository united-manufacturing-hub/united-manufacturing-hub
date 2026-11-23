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

package collection_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor/internal/collection"
)

const (
	collectorClaimGauge       = "collector_claim_test_gauge"
	collectorClaimGaugeMetric = "umh_fsmv2_worker_" + collectorClaimGauge
)

// collectorClaimGaugeValues returns the values of the test gauge's series for hierarchyPath.
func collectorClaimGaugeValues(hierarchyPath string) []float64 {
	families, err := prometheus.DefaultGatherer.Gather()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	var values []float64

	for _, family := range families {
		if family.GetName() != collectorClaimGaugeMetric {
			continue
		}

		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "hierarchy_path" && label.GetValue() == hierarchyPath {
					values = append(values, metric.GetGauge().GetValue())
				}
			}
		}
	}

	return values
}

var _ = Describe("Collector worker-metric export", func() {
	identity := deps.Identity{
		ID:            "collector-claim",
		Name:          "collector-claim",
		WorkerType:    "testwrapping",
		HierarchyPath: "collector-claim(testwrapping)",
	}

	var (
		coll         *collection.Collector[fsmv2.Observation[struct{}]]
		dependencies *deps.BaseDependencies
	)

	BeforeEach(func() {
		dependencies = deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity)
		dependencies.MetricsRecorder().SetGauge(collectorClaimGauge, 7)
		worker := &wrappingTestWorker{baseDeps: dependencies, observed: fsmv2.Observation[struct{}]{}}

		coll = collection.NewCollector[fsmv2.Observation[struct{}]](collection.CollectorConfig[fsmv2.Observation[struct{}]]{
			Worker:               worker,
			Identity:             identity,
			Store:                supervisor.CreateTestTriangularStoreForWorkerType("testwrapping"),
			Logger:               deps.NewNopFSMLogger(),
			ObservationInterval:  time.Hour,
			ObservationTimeout:   3 * time.Second,
			DesiredStateProvider: wrappingDesiredProvider(),
		})

		Expect(coll.Start(context.Background())).To(Succeed())
		DeferCleanup(coll.Stop, context.Background())
		Expect(coll.CollectFinalObservation(context.Background())).To(Succeed())
		Expect(collectorClaimGaugeValues(identity.HierarchyPath)).To(ConsistOf(7.0))
	})

	It("keeps exporting the worker metrics after the observation loop restarts", func() {
		coll.Restart()
		Expect(collectorClaimGaugeValues(identity.HierarchyPath)).To(ConsistOf(7.0))

		dependencies.MetricsRecorder().SetGauge(collectorClaimGauge, 8)
		Expect(coll.CollectFinalObservation(context.Background())).To(Succeed())

		Expect(collectorClaimGaugeValues(identity.HierarchyPath)).To(ConsistOf(8.0))
	})

	It("deletes the worker metrics on Stop and drops a collection that returns after it", func() {
		handle := coll.WorkerMetrics()

		coll.Stop(context.Background())
		handle.Export(fsmv2.Observation[struct{}]{}.SetWorkerMetrics(deps.Metrics{
			Gauges: map[string]float64{collectorClaimGauge: 7},
		}))

		Expect(collectorClaimGaugeValues(identity.HierarchyPath)).To(BeEmpty())
	})
})
