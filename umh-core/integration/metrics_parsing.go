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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

// printAllReconcileDurations prints all reconcile duration p99 metrics that are over a threshold
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

// checkWhetherMetricsHealthy checks that the metrics are healthy
func checkWhetherMetricsHealthy(body string) {
	// 1) Check memory usage: go_memstats_alloc_bytes
	alloc, found := parseMetricValue(body, "go_memstats_alloc_bytes")
	Expect(found).To(BeTrue(), "Expected to find go_memstats_alloc_bytes in metrics")
	Expect(alloc).To(BeNumerically("<", maxAllocBytes),
		"Heap allocation (alloc_bytes=%.2f) should be below %d bytes", alloc, maxAllocBytes)
	GinkgoWriter.Printf("✓ Memory: %.2f MB (limit: %.2f MB)\n", alloc/1024/1024, float64(maxAllocBytes)/1024/1024)

	// 2) Check total starved time: umh_core_reconcile_starved_total_seconds
	starved, found := parseMetricValue(body, "umh_core_reconcile_starved_total_seconds")
	Expect(found).To(BeTrue(), "Expected to find umh_core_reconcile_starved_total_seconds")
	Expect(starved).To(BeNumerically("==", float64(maxStarvedSeconds)),
		"Expected starved seconds (%.2f) to be == %.2f", starved, float64(maxStarvedSeconds))
	GinkgoWriter.Printf("✓ Starved seconds: %.2f (limit: %d)\n", starved, maxStarvedSeconds)

	// 3) Check error counters: umh_core_errors_total
	errorFound := false
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
				//GinkgoWriter.Printf("Checking error metric: %s = %.0f (limit: %d)\n", line, val, maxErrorCount)
				if val > float64(maxErrorCount) {
					errorFound = true
				}
				Expect(val).To(BeNumerically("<=", float64(maxErrorCount)),
					"Error counter (%.0f) exceeded %d: %s", val, maxErrorCount, line)
			}
		}
	}
	if !errorFound {
		GinkgoWriter.Printf("✓ No errors found above limit\n")
	}

	// 4) Check reconcile time quantiles for control loop
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

	// Print all reconcile durations over threshold for debugging
	printAllReconcileDurations(body, 20.0)

	// Still enforce the 99th percentile threshold
	recon99, found := parseSummaryQuantile(body,
		"umh_core_reconcile_duration_milliseconds", "0.99", "control_loop", "main")
	Expect(found).To(BeTrue(), "Expected to find 0.99 quantile for control_loop's reconcile time")
	Expect(recon99).To(BeNumerically("<=", maxReconcileTime99th),
		"99th percentile reconcile time (%.2f ms) exceeded %.1f ms", recon99, maxReconcileTime99th)
}

// parseFloat is a small helper to parse a string to float64
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
