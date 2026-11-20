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

package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
)

const (
	claimTestPath          = "claim-internal-test(worker)"
	claimTestGauge         = "claim_internal_test_gauge"
	claimTestGaugeMetric   = "umh_fsmv2_worker_" + claimTestGauge
	claimTestCounter       = "claim_internal_test_counter"
	claimTestCounterMetric = "umh_fsmv2_worker_" + claimTestCounter
)

var _ = Describe("WorkerMetricsExporter claims", func() {
	var (
		registry *prometheus.Registry
		exporter *WorkerMetricsExporter
	)

	BeforeEach(func() {
		registry = prometheus.NewRegistry()
		exporter = newWorkerMetricsExporter(registry)
	})

	gaugeValues := func() []float64 {
		families, err := registry.Gather()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		var values []float64

		for _, family := range families {
			if family.GetName() != claimTestGaugeMetric {
				continue
			}

			for _, metric := range family.GetMetric() {
				values = append(values, metric.GetGauge().GetValue())
			}
		}

		return values
	}

	counterValues := func() []float64 {
		families, err := registry.Gather()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		var values []float64

		for _, family := range families {
			if family.GetName() != claimTestCounterMetric {
				continue
			}

			for _, metric := range family.GetMetric() {
				values = append(values, metric.GetCounter().GetValue())
			}
		}

		return values
	}

	exportGauge := func(claim WorkerMetricsClaim, value float64) {
		exporter.export(claimTestPath, claim, &deps.Metrics{Gauges: map[string]float64{claimTestGauge: value}})
	}

	It("starts a worker added again at a removed worker's path from its own counter values", func() {
		removedWorker := exporter.claim(claimTestPath)
		exporter.export(claimTestPath, removedWorker, &deps.Metrics{Counters: map[string]int64{claimTestCounter: 100}})
		exporter.cleanup(claimTestPath, removedWorker)

		addedWorker := exporter.claim(claimTestPath)
		exporter.export(claimTestPath, addedWorker, &deps.Metrics{Counters: map[string]int64{claimTestCounter: 3}})

		Expect(counterValues()).To(ConsistOf(3.0))
	})

	It("drops an export from a removed worker", func() {
		removedWorker := exporter.claim(claimTestPath)
		exportGauge(removedWorker, 1)

		exporter.cleanup(claimTestPath, removedWorker)
		exportGauge(removedWorker, 1)

		Expect(gaugeValues()).To(BeEmpty())
	})

	It("keeps a worker added later at the same path from the removed worker's exports and cleanup", func() {
		removedWorker := exporter.claim(claimTestPath)
		addedWorker := exporter.claim(claimTestPath)
		exportGauge(addedWorker, 1)

		exportGauge(removedWorker, 99)
		exporter.cleanup(claimTestPath, removedWorker)

		Expect(gaugeValues()).To(ConsistOf(1.0))
	})

	It("drops an export under claim 0 for a path whose worker was removed", func() {
		removedWorker := exporter.claim(claimTestPath)
		exporter.cleanup(claimTestPath, removedWorker)

		exportGauge(0, 1)

		Expect(gaugeValues()).To(BeEmpty())
	})

	It("deletes the previous owner's series and counter baselines when a new worker claims the path", func() {
		previousOwner := exporter.claim(claimTestPath)
		exporter.export(claimTestPath, previousOwner, &deps.Metrics{
			Gauges:   map[string]float64{claimTestGauge: 5},
			Counters: map[string]int64{claimTestCounter: 100},
		})

		newOwner := exporter.claim(claimTestPath)
		Expect(gaugeValues()).To(BeEmpty())

		exporter.export(claimTestPath, newOwner, &deps.Metrics{Counters: map[string]int64{claimTestCounter: 3}})

		Expect(counterValues()).To(ConsistOf(3.0))
	})

	It("drops an export for a path no worker has claimed", func() {
		exportGauge(0, 1)

		Expect(gaugeValues()).To(BeEmpty())
	})
})
