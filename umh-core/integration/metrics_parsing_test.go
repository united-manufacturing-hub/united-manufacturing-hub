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
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics Parsing", Label("metrics_parsing"), func() {
	// Sample metrics from real system
	var testMetrics string

	BeforeEach(func() {
		// This is our test metrics data with various violations
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
# TYPE umh_core_reconcile_duration_milliseconds summary
umh_core_reconcile_duration_milliseconds{component="base_fsm_manager",instance="S6ManagerCore",quantile="0.5"} 0
umh_core_reconcile_duration_milliseconds{component="base_fsm_manager",instance="S6ManagerCore",quantile="0.9"} 0
umh_core_reconcile_duration_milliseconds{component="base_fsm_manager",instance="S6ManagerCore",quantile="0.95"} 0
umh_core_reconcile_duration_milliseconds{component="base_fsm_manager",instance="S6ManagerCore",quantile="0.99"} 0
umh_core_reconcile_duration_milliseconds{component="control_loop",instance="main",quantile="0.5"} 1
umh_core_reconcile_duration_milliseconds{component="control_loop",instance="main",quantile="0.9"} 1
umh_core_reconcile_duration_milliseconds{component="control_loop",instance="main",quantile="0.95"} 2
umh_core_reconcile_duration_milliseconds{component="control_loop",instance="main",quantile="0.99"} 16
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

	Describe("parseSummaryQuantile", Label("integration"), func() {
		It("should parse quantile metrics correctly when filtering by component and instance", func() {
			// Test finding the 99th percentile for control_loop/main
			p99, found := parseSummaryQuantile(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "0.99", "control_loop", "main")
			Expect(found).To(BeTrue(), "Should find the 0.99 quantile for control_loop/main")
			Expect(p99).To(BeNumerically("==", 16), "Should parse the 99th percentile correctly")

			// Test finding the 95th percentile for control_loop/main
			p95, found := parseSummaryQuantile(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "0.95", "control_loop", "main")
			Expect(found).To(BeTrue(), "Should find the 0.95 quantile for control_loop/main")
			Expect(p95).To(BeNumerically("==", 2), "Should parse the 95th percentile correctly")

			// Test finding a metric for a different component
			p99s6, found := parseSummaryQuantile(testMetrics,
				"umh_core_reconcile_duration_milliseconds", "0.99", "base_fsm_manager", "S6ManagerCore")
			Expect(found).To(BeTrue(), "Should find the 0.99 quantile for base_fsm_manager/S6ManagerCore")
			Expect(p99s6).To(BeNumerically("==", 0), "Should parse the S6 manager 99th percentile correctly")

			// Test a non-existent component/instance
			_, found = parseSummaryQuantile(testMetrics,
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
			// Using the non-error metrics but changing the reconcile time to exceed the threshold
			noErrorMetrics := strings.ReplaceAll(testMetrics, "umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 3",
				"umh_core_errors_total{component=\"s6_instance\",instance=\"golden-service\"} 0")
			noErrorStarvedMetrics := strings.ReplaceAll(noErrorMetrics, "umh_core_reconcile_starved_total_seconds 3",
				"umh_core_reconcile_starved_total_seconds 0")

			highReconcileMetrics := strings.ReplaceAll(noErrorStarvedMetrics,
				"umh_core_reconcile_duration_milliseconds{component=\"control_loop\",instance=\"main\",quantile=\"0.99\"} 16",
				"umh_core_reconcile_duration_milliseconds{component=\"control_loop\",instance=\"main\",quantile=\"0.99\"} "+strconv.FormatFloat(maxReconcileTime99th+1, 'f', -1, 64))

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
