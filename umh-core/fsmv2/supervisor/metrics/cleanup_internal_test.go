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
		exporter := &WorkerMetricsExporter{
			counters:     make(map[string]*prometheus.CounterVec),
			gauges:       make(map[string]*prometheus.GaugeVec),
			prevCounters: make(map[string]map[string]int64),
		}

		exporter.export(cleanupTestPath, &deps.Metrics{
			Counters: map[string]int64{"cleanup_internal_test_zero_counter": 0},
		})
		Expect(exporter.prevCounters).To(HaveKey(cleanupTestPath))

		exporter.cleanup(cleanupTestPath)

		Expect(exporter.prevCounters).NotTo(HaveKey(cleanupTestPath))
	})
})
