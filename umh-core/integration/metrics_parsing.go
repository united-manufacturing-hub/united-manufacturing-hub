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

// parseSummaryQuantile scans the metrics for a summary line, e.g.:
//
//	metricName{component="control_loop",instance="main",quantile="0.99"}  12
//
// We'll look for lines that start with metricName and contain 'quantile="<quantile>"'
// (and optionally we can also filter by component=..., instance=..., if needed).
func parseSummaryQuantile(metricsBody, metricName, quantile, component, instance string) (float64, bool) {
	// Build a regex that matches the line with specific labels including component and instance
	// For example:
	//   umh_core_reconcile_duration_milliseconds{component="control_loop",instance="main",quantile="0.99"}  31
	pattern := fmt.Sprintf(`^%s\{[^}]*component="%s"[^}]*instance="%s"[^}]*quantile="%s"[^}]*\}\s+([0-9.+-eE]+)$`,
		regexp.QuoteMeta(metricName), regexp.QuoteMeta(component), regexp.QuoteMeta(instance), regexp.QuoteMeta(quantile))

	re := regexp.MustCompile(pattern)

	lines := strings.Split(metricsBody, "\n")
	for _, line := range lines {
		if strings.Contains(line, metricName) && strings.Contains(line, quantile) {
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
	GinkgoWriter.Println("No exact match found. Relevant lines:")

	for _, line := range lines {
		if strings.Contains(line, metricName) && strings.Contains(line, "quantile=\"0.99\"") {
			GinkgoWriter.Printf("  %s\n", line)
		}
	}

	return 0, false
}

// printAllReconcileDurations prints all reconcile duration p99 metrics that are over a threshold.
func printAllReconcileDurations(metricsBody string, thresholdMs float64) {
	lines := strings.Split(metricsBody, "\n")

	GinkgoWriter.Printf("\nReconcile p99 durations over %.1f ms:\n", thresholdMs)

	for _, line := range lines {
		if strings.Contains(line, "umh_core_reconcile_duration_milliseconds") && strings.Contains(line, `quantile="0.99"`) {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}

			valStr := fields[len(fields)-1]

			val, err := strconv.ParseFloat(valStr, 64)
			if err == nil && val > thresholdMs {
				GinkgoWriter.Printf("  %s\n", line)
			}
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

// displayReconcileTimeQuantiles displays the reconcile time quantiles for the control loop.
func displayReconcileTimeQuantiles(body string) error {
	GinkgoWriter.Println("\nControl loop reconcile time quantiles:")

	quantiles := []string{"0.5", "0.9", "0.95", "0.99"}
	for _, q := range quantiles {
		val, found := parseSummaryQuantile(body,
			"umh_core_reconcile_duration_milliseconds", q, "control_loop", "main")
		if found {
			GinkgoWriter.Printf("  %s quantile: %.2f ms\n", q, val)
		} else {
			GinkgoWriter.Printf("  %s quantile: not found\n", q)
		}
	}

	return nil
}

// checkReconcileTimeThresholds checks the reconcile time thresholds based on configuration.
func checkReconcileTimeThresholds(body string, enforceP99ReconcileTime bool, enforceP95ReconcileTime bool) error {
	if enforceP99ReconcileTime {
		// Enforce the 99th percentile threshold
		recon99, found := parseSummaryQuantile(body,
			"umh_core_reconcile_duration_milliseconds", "0.99", "control_loop", "main")
		if !found {
			return errors.New("expected to find 0.99 quantile for control_loop's reconcile time")
		}

		if recon99 > maxReconcileTime99th {
			return fmt.Errorf("99th percentile reconcile time (%.2f ms) exceeded %.1f ms", recon99, maxReconcileTime99th)
		}
	} else if enforceP95ReconcileTime {
		// Enforce the 95th percentile threshold
		recon95, found := parseSummaryQuantile(body,
			"umh_core_reconcile_duration_milliseconds", "0.95", "control_loop", "main")
		if !found {
			return errors.New("expected to find 0.95 quantile for control_loop's reconcile time")
		}

		if recon95 > maxReconcileTime99th { // For now this uses the same limit as the 99th percentile
			return fmt.Errorf("95th percentile reconcile time (%.2f ms) exceeded %.1f ms", recon95, maxReconcileTime99th)
		}
	}

	return nil
}
