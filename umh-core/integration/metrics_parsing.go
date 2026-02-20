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
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint: staticcheck // Ginkgo is designed to be used with dot imports
)

// parseMetricValue scans the metrics body for a line that starts with `metricName`
// and tries to parse the last field as a float64. Returns (value, found).
func parseMetricValue(metricsBody, metricName string) (float64, bool) {
	// The line for a gauge or counter typically looks like:
	//  metricName 123.45
	// or
	//  metricName{...} 123.45
	lines := strings.Split(metricsBody, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, metricName) {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}

			valStr := fields[len(fields)-1]

			f, err := strconv.ParseFloat(valStr, 64)
			if err == nil {
				return f, true
			}
		}
	}

	return 0, false
}

// parseHistogramBucket scans the metrics for a histogram bucket line, e.g.:
//
//	metricName_bucket{component="control_loop",instance="main",le="90"}  450
//
// Returns the cumulative count for the given bucket boundary.
func parseHistogramBucket(metricsBody, metricName, le, component, instance string) (float64, bool) {
	bucketMetric := metricName + "_bucket"
	pattern := fmt.Sprintf(`^%s\{[^}]*component="%s"[^}]*instance="%s"[^}]*le="%s"[^}]*\}\s+([0-9.+-eE]+)$`,
		regexp.QuoteMeta(bucketMetric), regexp.QuoteMeta(component), regexp.QuoteMeta(instance), regexp.QuoteMeta(le))

	re := regexp.MustCompile(pattern)

	lines := strings.Split(metricsBody, "\n")
	for _, line := range lines {
		if strings.Contains(line, bucketMetric) && strings.Contains(line, fmt.Sprintf(`le="%s"`, le)) {
			if match := re.FindStringSubmatch(line); match != nil {
				valStr := match[1]

				f, err := strconv.ParseFloat(valStr, 64)
				if err == nil {
					return f, true
				}
			}
		}
	}

	// If no match, dump all relevant lines for debugging
	GinkgoWriter.Println("No exact match found. Relevant bucket lines:")

	for _, line := range lines {
		if strings.Contains(line, bucketMetric) && strings.Contains(line, fmt.Sprintf(`component="%s"`, component)) {
			GinkgoWriter.Printf("  %s\n", line)
		}
	}

	return 0, false
}

// parseHistogramCount scans the metrics for a histogram _count line, e.g.:
//
//	metricName_count{component="control_loop",instance="main"}  500
func parseHistogramCount(metricsBody, metricName, component, instance string) (float64, bool) {
	countMetric := metricName + "_count"
	pattern := fmt.Sprintf(`^%s\{[^}]*component="%s"[^}]*instance="%s"[^}]*\}\s+([0-9.+-eE]+)$`,
		regexp.QuoteMeta(countMetric), regexp.QuoteMeta(component), regexp.QuoteMeta(instance))

	re := regexp.MustCompile(pattern)

	lines := strings.Split(metricsBody, "\n")
	for _, line := range lines {
		if strings.Contains(line, countMetric) {
			if match := re.FindStringSubmatch(line); match != nil {
				valStr := match[1]

				f, err := strconv.ParseFloat(valStr, 64)
				if err == nil {
					return f, true
				}
			}
		}
	}

	return 0, false
}

// printAllReconcileDurations prints all reconcile duration histogram bucket data for debugging.
func printAllReconcileDurations(metricsBody string, _ float64) {
	lines := strings.Split(metricsBody, "\n")

	GinkgoWriter.Printf("\nReconcile duration histogram buckets:\n")

	for _, line := range lines {
		if strings.Contains(line, "umh_core_reconcile_duration_milliseconds_bucket") ||
			strings.Contains(line, "umh_core_reconcile_duration_milliseconds_count") ||
			strings.Contains(line, "umh_core_reconcile_duration_milliseconds_sum") {
			GinkgoWriter.Printf("  %s\n", line)
		}
	}
}

// checkWhetherMetricsHealthy checks that the metrics are healthy and returns an error if they are not.
func checkWhetherMetricsHealthy(body string, enforceP99ReconcileTime bool, enforceP95ReconcileTime bool) []error {
	// Collect all errors instead of returning on the first one
	errors := make([]error, 0)

	// Check each metric category
	if err := checkMemoryUsage(body); err != nil {
		errors = append(errors, err)
	}

	if err := checkStarvedTime(body); err != nil {
		errors = append(errors, err)
	}

	if err := checkErrorCounters(body); err != nil {
		errors = append(errors, err)
	}

	if err := displayReconcileTimeQuantiles(body); err != nil {
		errors = append(errors, err)
	}

	// Print all reconcile durations over threshold for debugging
	printAllReconcileDurations(body, 20.0)

	if err := checkReconcileTimeThresholds(body, enforceP99ReconcileTime, enforceP95ReconcileTime); err != nil {
		errors = append(errors, err)
	}

	return errors
}

// checkMemoryUsage checks the memory usage metrics.
func checkMemoryUsage(body string) error {
	alloc, found := parseMetricValue(body, "go_memstats_alloc_bytes")
	if !found {
		return errors.New("expected to find go_memstats_alloc_bytes in metrics")
	}

	if alloc >= maxAllocBytes {
		return fmt.Errorf("heap allocation (alloc_bytes=%.2f) should be below %d bytes", alloc, maxAllocBytes)
	}

	GinkgoWriter.Printf("✓ Memory: %.2f MB (limit: %.2f MB)\n", alloc/1024/1024, float64(maxAllocBytes)/1024/1024)

	return nil
}

// checkStarvedTime checks the total starved time metrics.
func checkStarvedTime(body string) error {
	starved, found := parseMetricValue(body, "umh_core_reconcile_starved_total_seconds")
	if !found {
		return errors.New("expected to find umh_core_reconcile_starved_total_seconds")
	}

	if starved != float64(maxStarvedSeconds) {
		return fmt.Errorf("expected starved seconds (%.2f) to be == %.2f", starved, float64(maxStarvedSeconds))
	}

	GinkgoWriter.Printf("✓ Starved seconds: %.2f (limit: %d)\n", starved, maxStarvedSeconds)

	return nil
}

// checkErrorCounters checks the error counter metrics.
func checkErrorCounters(body string) error {
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "umh_core_errors_total") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}

			valStr := fields[len(fields)-1]

			val, err := strconv.ParseFloat(valStr, 64)
			if err == nil {
				if val > float64(maxErrorCount) {
					return fmt.Errorf("error counter (%.0f) exceeded %d: %s", val, maxErrorCount, line)
				}
			}
		}
	}

	GinkgoWriter.Printf("✓ No errors found above limit\n")

	return nil
}

// displayReconcileTimeQuantiles displays the reconcile time histogram bucket distribution for the control loop.
func displayReconcileTimeQuantiles(body string) error {
	GinkgoWriter.Println("\nControl loop reconcile time histogram buckets:")

	buckets := []string{"1", "2", "5", "10", "20", "50", "90", "100", "200", "500", "1000", "+Inf"}
	for _, b := range buckets {
		val, found := parseHistogramBucket(body,
			"umh_core_reconcile_duration_milliseconds", b, "control_loop", "main")
		if found {
			GinkgoWriter.Printf("  le=%s: %.0f\n", b, val)
		} else {
			GinkgoWriter.Printf("  le=%s: not found\n", b)
		}
	}

	count, found := parseHistogramCount(body, "umh_core_reconcile_duration_milliseconds", "control_loop", "main")
	if found {
		GinkgoWriter.Printf("  count: %.0f\n", count)
	}

	return nil
}

// checkReconcileTimeThresholds checks reconcile time thresholds using histogram bucket ratios.
// For p99 enforcement: verifies that >=99% of observations fall within the le="90" bucket.
// For p95 enforcement: verifies that >=95% of observations fall within the le="90" bucket.
func checkReconcileTimeThresholds(body string, enforceP99ReconcileTime bool, enforceP95ReconcileTime bool) error {
	if !enforceP99ReconcileTime && !enforceP95ReconcileTime {
		return nil
	}

	// Use the bucket boundary matching our threshold (90ms)
	thresholdBucket := fmt.Sprintf("%g", maxReconcileTime99th)
	bucketCount, foundBucket := parseHistogramBucket(body,
		"umh_core_reconcile_duration_milliseconds", thresholdBucket, "control_loop", "main")

	totalCount, foundCount := parseHistogramCount(body,
		"umh_core_reconcile_duration_milliseconds", "control_loop", "main")

	if !foundBucket || !foundCount {
		return fmt.Errorf("expected to find histogram bucket le=%s and count for control_loop's reconcile time", thresholdBucket)
	}

	if totalCount == 0 {
		return errors.New("histogram count is zero for control_loop's reconcile time")
	}

	ratio := bucketCount / totalCount

	if enforceP99ReconcileTime {
		if ratio < 0.99 {
			return fmt.Errorf("less than 99%% of reconciliations completed within %.0f ms (%.1f%% did)", maxReconcileTime99th, ratio*100)
		}
	} else if enforceP95ReconcileTime {
		if ratio < 0.95 {
			return fmt.Errorf("less than 95%% of reconciliations completed within %.0f ms (%.1f%% did)", maxReconcileTime99th, ratio*100)
		}
	}

	return nil
}
