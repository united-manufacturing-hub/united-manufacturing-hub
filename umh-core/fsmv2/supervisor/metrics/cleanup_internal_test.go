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
	handleTestPath          = "handle-internal-test(worker)"
	handleTestGauge         = "handle_internal_test_gauge"
	handleTestGaugeMetric   = "umh_fsmv2_worker_" + handleTestGauge
	handleTestCounter       = "handle_internal_test_counter"
	handleTestCounterMetric = "umh_fsmv2_worker_" + handleTestCounter
)

var _ = Describe("WorkerMetricsHandle", func() {
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
			if family.GetName() != handleTestGaugeMetric {
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
			if family.GetName() != handleTestCounterMetric {
				continue
			}

			for _, metric := range family.GetMetric() {
				values = append(values, metric.GetCounter().GetValue())
			}
		}

		return values
	}

	exportGauge := func(handle *WorkerMetricsHandle, value float64) {
		exporter.export(handle, &deps.Metrics{Gauges: map[string]float64{handleTestGauge: value}})
	}

	It("starts a worker added again at a removed worker's path from its own counter values", func() {
		removedWorker := exporter.claim(handleTestPath)
		exporter.export(removedWorker, &deps.Metrics{Counters: map[string]int64{handleTestCounter: 100}})
		removedWorker.Release()

		addedWorker := exporter.claim(handleTestPath)
		exporter.export(addedWorker, &deps.Metrics{Counters: map[string]int64{handleTestCounter: 3}})

		Expect(counterValues()).To(ConsistOf(3.0))
	})

	It("drops an export from a removed worker", func() {
		removedWorker := exporter.claim(handleTestPath)
		exportGauge(removedWorker, 1)

		removedWorker.Release()
		exportGauge(removedWorker, 1)

		Expect(gaugeValues()).To(BeEmpty())
	})

	It("keeps a worker added later at the same path from the removed worker's exports and cleanup", func() {
		removedWorker := exporter.claim(handleTestPath)
		addedWorker := exporter.claim(handleTestPath)
		exportGauge(addedWorker, 1)

		exportGauge(removedWorker, 99)
		removedWorker.Release()

		Expect(gaugeValues()).To(ConsistOf(1.0))
	})

	It("deletes the previous owner's series and counter baselines when a new worker claims the path", func() {
		previousOwner := exporter.claim(handleTestPath)
		exporter.export(previousOwner, &deps.Metrics{
			Gauges:   map[string]float64{handleTestGauge: 5},
			Counters: map[string]int64{handleTestCounter: 100},
		})

		newOwner := exporter.claim(handleTestPath)
		Expect(gaugeValues()).To(BeEmpty())

		exporter.export(newOwner, &deps.Metrics{Counters: map[string]int64{handleTestCounter: 3}})

		Expect(counterValues()).To(ConsistOf(3.0))
	})

	It("treats a nil handle as one that exports and releases nothing", func() {
		var noHandle *WorkerMetricsHandle

		Expect(func() {
			noHandle.Export(nil)
			noHandle.Release()
		}).NotTo(Panic())
	})
})
