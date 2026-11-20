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

const cleanupTestPath = "cleanup-internal-test(worker)"

var _ = Describe("WorkerMetricsExporter.cleanup", func() {
	It("forgets the last value of a counter that never went above 0", func() {
		exporter := newWorkerMetricsExporter()
		claim := exporter.claim(cleanupTestPath)

		exporter.export(cleanupTestPath, &deps.Metrics{
			Counters: map[string]int64{"cleanup_internal_test_zero_counter": 0},
		})
		Expect(exporter.prevCounters).To(HaveKey(cleanupTestPath))

		exporter.cleanup(cleanupTestPath, claim)

		Expect(exporter.prevCounters).NotTo(HaveKey(cleanupTestPath))
	})

	It("leaves the series of a worker added later at the same path", func() {
		const gaugeName = "cleanup_internal_test_claimed_gauge"

		exporter := newWorkerMetricsExporter()
		removedWorker := exporter.claim(cleanupTestPath)
		addedWorker := exporter.claim(cleanupTestPath)

		exporter.export(cleanupTestPath, &deps.Metrics{Gauges: map[string]float64{gaugeName: 1}})

		exporter.cleanup(cleanupTestPath, removedWorker)
		Expect(seriesCount("umh_fsmv2_worker_" + gaugeName)).To(Equal(1))

		exporter.cleanup(cleanupTestPath, addedWorker)
		Expect(seriesCount("umh_fsmv2_worker_" + gaugeName)).To(BeZero())
	})
})

func seriesCount(metricName string) int {
	families, err := prometheus.DefaultGatherer.Gather()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, family := range families {
		if family.GetName() == metricName {
			return len(family.GetMetric())
		}
	}

	return 0
}
