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

package integration_test

import (
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics Parsing", Label("metrics_parsing"), func() {
	// Sample metrics from real system (histogram format)
	var testMetrics string

	BeforeEach(func() {
		// This is our test metrics data with various violations.
		// The reconcile duration uses HistogramVec with buckets [1,2,5,10,20,50,90,100,200,500,1000].
		//
		// control_loop/main distribution: 100 total observations
		//   - 50 observations <= 1ms
		//   - 70 observations <= 2ms
		//   - 85 observations <= 5ms
		//   - 90 observations <= 10ms
		//   - 95 observations <= 20ms
		//   - 97 observations <= 50ms
		//   - 99 observations <= 90ms  (so p99 ~ within 50-90 bucket, well under 90ms threshold)
		//   - 100 observations <= 100ms
		//   - 100 observations total
		//
		// base_fsm_manager/S6ManagerCore: all 50 observations <= 1ms (fast)
		testMetrics = `# HELP go_memstats_alloc_bytes Number of bytes allocated in heap and currently in use.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 3.05356e+06
# HELP umh_core_errors_total Total number of errors encountered by component
# TYPE umh_core_errors_total counter
umh_core_errors_total{component="base_fsm_manager",instance="BenthosManagerCore"} 0
umh_core_errors_total{component="base_fsm_manager",instance="S6ManagerCore"} 0
umh_core_errors_total{component="benthos_manager",instance="Core"} 0
umh_core_errors_total{component="control_loop",instance="main"} 0
umh_core_errors_total{component="s6_instance",instance="golden-service"} 3
umh_core_errors_total{component="s6_instance",instance="sleepy"} 0
umh_core_errors_total{component="s6_manager",instance="Core"} 0
# HELP umh_core_reconcile_duration_milliseconds Time taken to reconcile (in milliseconds)
# TYPE umh_core_reconcile_duration_milliseconds histogram
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="1"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="2"} 70
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="5"} 85
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="10"} 90
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="20"} 95
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="50"} 97
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="90"} 99
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="100"} 100
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="200"} 100
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="500"} 100
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="1000"} 100
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="+Inf"} 100
umh_core_reconcile_duration_milliseconds_sum{component="control_loop",instance="main"} 523
umh_core_reconcile_duration_milliseconds_count{component="control_loop",instance="main"} 100
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="1"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="2"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="5"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="10"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="20"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="50"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="90"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="100"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="200"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="500"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="1000"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="base_fsm_manager",instance="S6ManagerCore",le="+Inf"} 50
umh_core_reconcile_duration_milliseconds_sum{component="base_fsm_manager",instance="S6ManagerCore"} 12
umh_core_reconcile_duration_milliseconds_count{component="base_fsm_manager",instance="S6ManagerCore"} 50
# HELP umh_core_reconcile_starved_total_seconds Total seconds the reconcile loop was starved
# TYPE umh_core_reconcile_starved_total_seconds counter
umh_core_reconcile_starved_total_seconds 3`
	})

	Describe("parseMetricValue", Label("integration"), func() {
		It("should find and parse simple metric values correctly", func() {
			// Test finding the alloc bytes
			alloc, found := parseMetricValue(testMetrics, "go_memstats_alloc_bytes")
			Expect(found).To(BeTrue(), "Should find go_memstats_alloc_bytes metric")
			Expect(alloc).To(BeNumerically("==", 3.05356e+06), "Should parse the value correctly")

			// Test finding the starved seconds
			starved, found := parseMetricValue(testMetrics, "umh_core_reconcile_starved_total_seconds")
			Expect(found).To(BeTrue(), "Should find umh_core_reconcile_starved_total_seconds metric")
			Expect(starved).To(BeNumerically("==", 3), "Should parse the starved seconds correctly")

			// Test a non-existent metric
			_, found = parseMetricValue(testMetrics, "non_existent_metric")
			Expect(found).To(BeFalse(), "Should not find a non-existent metric")
		})
	})

	Describe("parseHistogramQuantile", Label("integration"), func() {
		It("should compute quantiles from histogram buckets correctly", func() {
			// Test p99 for control_loop/main
			// rank = 0.99 * 100 = 99. Bucket le=50 has count=97, le=90 has count=99.
			// Interpolation: 50 + (99 - 97)/(99 - 97) * (90 - 50) = 50 + 40 = 90
			p99, found := parseHistogramQuantile(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "0.99", "control_loop", "main")
			Expect(found).To(BeTrue(), "Should find the 0.99 quantile for control_loop/main")
			Expect(p99).To(BeNumerically("~", 90, 0.1), "Should compute the 99th percentile correctly")

			// Test p95 for control_loop/main
			// rank = 0.95 * 100 = 95. Bucket le=20 has count=95.
			// Interpolation: prevLE=10, prevCount=90, le=20, count=95.
			// 10 + (95 - 90)/(95 - 90) * (20 - 10) = 10 + 10 = 20
			p95, found := parseHistogramQuantile(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "0.95", "control_loop", "main")
			Expect(found).To(BeTrue(), "Should find the 0.95 quantile for control_loop/main")
			Expect(p95).To(BeNumerically("~", 20, 0.1), "Should compute the 95th percentile correctly")

			// Test p50 for control_loop/main
			// rank = 0.5 * 100 = 50. Bucket le=1 has count=50.
			// Interpolation: prevLE=0, prevCount=0, le=1, count=50.
			// 0 + (50 / 50) * 1 = 1
			p50, found := parseHistogramQuantile(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "0.5", "control_loop", "main")
			Expect(found).To(BeTrue(), "Should find the 0.50 quantile for control_loop/main")
			Expect(p50).To(BeNumerically("~", 1, 0.1), "Should compute the 50th percentile correctly")

			// Test p99 for base_fsm_manager/S6ManagerCore (all in first bucket)
			// rank = 0.99 * 50 = 49.5. Bucket le=1 has count=50.
			// Interpolation: 0 + (49.5 / 50) * 1 = 0.99
			p99s6, found := parseHistogramQuantile(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "0.99", "base_fsm_manager", "S6ManagerCore")
			Expect(found).To(BeTrue(), "Should find the 0.99 quantile for base_fsm_manager/S6ManagerCore")
			Expect(p99s6).To(BeNumerically("~", 0.99, 0.1), "Should compute a low 99th percentile for the fast S6 manager")

			// Test a non-existent component/instance
			_, found = parseHistogramQuantile(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "0.99", "non_existent", "component")
			Expect(found).To(BeFalse(), "Should not find a non-existent component")
		})
	})

	Describe("checkWhetherMetricsHealthy", Label("integration"), func() {
		It("should fail when error count exceeds max", func() {
			// Original metrics have s6_instance with golden-service having 3 errors
			// which exceeds maxErrorCount (0)
			metricsErrors := checkWhetherMetricsHealthy(testMetrics, true, true)
			Expect(metricsErrors).NotTo(BeEmpty(), "Should have detected errors")

			// Check that we caught the golden-service error specifically
			foundGoldenServiceError := false
			for _, failure := range metricsErrors {
				if strings.Contains(failure.Error(), "golden-service") && strings.Contains(failure.Error(), "3") {
					foundGoldenServiceError = true

					break
				}
			}
			Expect(foundGoldenServiceError).To(BeTrue(), "Should have caught the golden-service error")
		})

		It("should fail when starved seconds exceeds max", func() {
			// We'll modify the metrics to have no errors but still have starved seconds
			noErrorMetrics := strings.ReplaceAll(testMetrics, "umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 3",
				"umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 0")

			metricsErrors := checkWhetherMetricsHealthy(noErrorMetrics, true, true)
			Expect(metricsErrors).NotTo(BeEmpty(), "Should have detected starved seconds violation")

			// Check that we caught the starved seconds issue
			foundStarvedSecondsError := false
			for _, failure := range metricsErrors {
				if strings.Contains(failure.Error(), "starved seconds") {
					foundStarvedSecondsError = true

					break
				}
			}
			Expect(foundStarvedSecondsError).To(BeTrue(), "Should have caught the starved seconds error")
		})

		It("should pass with healthy metrics", func() {
			// Modify metrics to be healthy (no errors, no starved seconds, p99 under threshold)
			healthyMetrics := strings.ReplaceAll(testMetrics, "umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 3",
				"umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 0")
			healthyMetrics = strings.ReplaceAll(healthyMetrics, "umh_core_reconcile_starved_total_seconds 3",
				"umh_core_reconcile_starved_total_seconds 0")

			metricsErrors := checkWhetherMetricsHealthy(healthyMetrics, true, true)
			Expect(metricsErrors).To(BeEmpty(), "Should not have any failures with healthy metrics")
		})

		It("should fail when p99 reconcile time exceeds max", func() {
			// Create metrics where p99 exceeds threshold by putting observations in high buckets
			noErrorMetrics := strings.ReplaceAll(testMetrics, "umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 3",
				"umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 0")
			highReconcileMetrics := strings.ReplaceAll(noErrorMetrics, "umh_core_reconcile_starved_total_seconds 3",
				"umh_core_reconcile_starved_total_seconds 0")

			// Replace the control_loop buckets so p99 lands well above maxReconcileTime99th (90ms).
			// Distribution: 90 obs in le=100, 95 in le=200, 100 in le=+Inf.
			// p99 rank = 0.99 * 100 = 99. Bucket le=200 has 95, le=500 has 100.
			// Interpolation: 200 + (99-95)/(100-95) * (500-200) = 200 + 240 = 440ms >> 90ms threshold.
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="1"} 50`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="1"} 10`)
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="2"} 70`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="2"} 20`)
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="5"} 85`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="5"} 40`)
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="10"} 90`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="10"} 60`)
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="20"} 95`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="20"} 70`)
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="50"} 97`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="50"} 80`)
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="90"} 99`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="90"} 85`)
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="100"} 100`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="100"} 90`)
			highReconcileMetrics = strings.ReplaceAll(highReconcileMetrics,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="200"} 100`,
				`umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="200"} 95`)

			metricsErrors := checkWhetherMetricsHealthy(highReconcileMetrics, true, true)
			Expect(metricsErrors).NotTo(BeEmpty(), "Should have detected high reconcile time")

			// Check that we caught the reconcile time issue
			foundReconcileTimeError := false
			for _, failure := range metricsErrors {
				if strings.Contains(failure.Error(), "reconcile time") && strings.Contains(failure.Error(), strconv.FormatFloat(maxReconcileTime99th, 'f', -1, 64)) {
					foundReconcileTimeError = true

					break
				}
			}
			Expect(foundReconcileTimeError).To(BeTrue(), "Should have caught the reconcile time error")
		})
	})
})
