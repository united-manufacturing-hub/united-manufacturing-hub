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

// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package integration_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics Parsing", Label("metrics_parsing"), func() {
	// Sample metrics from real system
	var testMetrics string

	BeforeEach(func() {
		// This is our test metrics data with various violations
		// Uses histogram format (buckets) instead of summary (quantiles).
		// The control_loop/main has 100 total observations: 95 under 90ms, 99 under 100ms, 1 over 1000ms.
		// The base_fsm_manager/S6ManagerCore has 50 total observations all under 1ms.
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
umh_core_reconcile_duration_milliseconds_sum{component="base_fsm_manager",instance="S6ManagerCore"} 10
umh_core_reconcile_duration_milliseconds_count{component="base_fsm_manager",instance="S6ManagerCore"} 50
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="1"} 60
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="2"} 75
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="5"} 85
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="10"} 90
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="20"} 93
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="50"} 95
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="90"} 95
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="100"} 99
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="200"} 99
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="500"} 99
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="1000"} 99
umh_core_reconcile_duration_milliseconds_bucket{component="control_loop",instance="main",le="+Inf"} 100
umh_core_reconcile_duration_milliseconds_sum{component="control_loop",instance="main"} 450
umh_core_reconcile_duration_milliseconds_count{component="control_loop",instance="main"} 100
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

	Describe("parseHistogramBucket", Label("integration"), func() {
		It("should parse histogram bucket metrics correctly when filtering by component and instance", func() {
			// Test finding the le="90" bucket for control_loop/main (95 of 100 observations)
			bucket90, found := parseHistogramBucket(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "90", "control_loop", "main")
			Expect(found).To(BeTrue(), "Should find the le=90 bucket for control_loop/main")
			Expect(bucket90).To(BeNumerically("==", 95), "Should parse the le=90 bucket correctly")

			// Test finding the le="100" bucket for control_loop/main (99 of 100 observations)
			bucket100, found := parseHistogramBucket(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "100", "control_loop", "main")
			Expect(found).To(BeTrue(), "Should find the le=100 bucket for control_loop/main")
			Expect(bucket100).To(BeNumerically("==", 99), "Should parse the le=100 bucket correctly")

			// Test finding a bucket for a different component (all 50 observations under 1ms)
			s6Bucket, found := parseHistogramBucket(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "1", "base_fsm_manager", "S6ManagerCore")
			Expect(found).To(BeTrue(), "Should find the le=1 bucket for base_fsm_manager/S6ManagerCore")
			Expect(s6Bucket).To(BeNumerically("==", 50), "Should parse the S6 manager le=1 bucket correctly")

			// Test a non-existent component/instance
			_, found = parseHistogramBucket(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "90", "non_existent", "component")
			Expect(found).To(BeFalse(), "Should not find a non-existent component")
		})

		It("should return false for a non-existent bucket boundary", func() {
			_, found := parseHistogramBucket(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "999", "control_loop", "main")
			Expect(found).To(BeFalse(), "Should not find le=999 bucket")
		})

		It("should return false for empty metrics string", func() {
			_, found := parseHistogramBucket("",
				"umh_core_reconcile_duration_milliseconds", "90", "control_loop", "main")
			Expect(found).To(BeFalse(), "Should not find anything in empty metrics")
		})
	})

	Describe("parseHistogramCount", Label("integration"), func() {
		It("should parse histogram count correctly", func() {
			count, found := parseHistogramCount(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "control_loop", "main")
			Expect(found).To(BeTrue(), "Should find the count for control_loop/main")
			Expect(count).To(BeNumerically("==", 100), "Should parse the count correctly")

			s6Count, found := parseHistogramCount(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "base_fsm_manager", "S6ManagerCore")
			Expect(found).To(BeTrue(), "Should find the count for base_fsm_manager/S6ManagerCore")
			Expect(s6Count).To(BeNumerically("==", 50), "Should parse the S6 manager count correctly")
		})

		It("should return false when _count metric is missing", func() {
			_, found := parseHistogramCount(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "non_existent", "instance")
			Expect(found).To(BeFalse(), "Should not find count for non-existent component")
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
			// Modify metrics to be healthy: no errors, no starved seconds, and >=99% within 90ms.
			// Change le="90" bucket from 95 to 100 so ratio is 100/100 = 100% (passes p99).
			healthyMetrics := strings.ReplaceAll(testMetrics, "umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 3",
				"umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 0")
			healthyMetrics = strings.ReplaceAll(healthyMetrics, "umh_core_reconcile_starved_total_seconds 3",
				"umh_core_reconcile_starved_total_seconds 0")
			healthyMetrics = strings.ReplaceAll(healthyMetrics,
				"umh_core_reconcile_duration_milliseconds_bucket{component=\"control_loop\",instance=\"main\",le=\"90\"} 95",
				"umh_core_reconcile_duration_milliseconds_bucket{component=\"control_loop\",instance=\"main\",le=\"90\"} 100")

			metricsErrors := checkWhetherMetricsHealthy(healthyMetrics, true, true)
			Expect(metricsErrors).To(BeEmpty(), "Should not have any failures with healthy metrics")
		})

		It("should fail when p99 reconcile time exceeds max", func() {
			// Modify metrics to have no errors/starved seconds but too many observations above 90ms.
			// The default fixture has le="90" = 95 out of 100 = 95%, which fails the 99% threshold.
			noErrorMetrics := strings.ReplaceAll(testMetrics, "umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 3",
				"umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 0")
			highReconcileMetrics := strings.ReplaceAll(noErrorMetrics, "umh_core_reconcile_starved_total_seconds 3",
				"umh_core_reconcile_starved_total_seconds 0")

			metricsErrors := checkWhetherMetricsHealthy(highReconcileMetrics, true, true)
			Expect(metricsErrors).NotTo(BeEmpty(), "Should have detected high reconcile time")

			// Check that we caught the reconcile time issue
			foundReconcileTimeError := false
			for _, failure := range metricsErrors {
				if strings.Contains(failure.Error(), "reconciliations completed within") {
					foundReconcileTimeError = true

					break
				}
			}
			Expect(foundReconcileTimeError).To(BeTrue(), "Should have caught the reconcile time error")
		})
	})

	Describe("checkReconcileTimeThresholds", Label("integration"), func() {
		It("should return error when total count is zero", func() {
			zeroCountMetrics := strings.ReplaceAll(testMetrics,
				"umh_core_reconcile_duration_milliseconds_count{component=\"control_loop\",instance=\"main\"} 100",
				"umh_core_reconcile_duration_milliseconds_count{component=\"control_loop\",instance=\"main\"} 0")

			err := checkReconcileTimeThresholds(zeroCountMetrics, true, false)
			Expect(err).To(HaveOccurred(), "Should return error when count is zero")
			Expect(err.Error()).To(ContainSubstring("zero"), "Error should mention zero count")
		})
	})
})
