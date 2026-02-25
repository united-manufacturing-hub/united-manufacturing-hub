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
	"math"
	"regexp"
	"sort"
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

// bucket represents a single histogram bucket with an upper bound and cumulative count.
type bucket struct {
	le    float64
	count float64
}

// parseHistogramBuckets extracts histogram buckets for a given metric, component, and instance.
// It parses lines like:
//
//	metricName_bucket{component="control_loop",instance="main",le="10"} 42
//	metricName_bucket{component="control_loop",instance="main",le="+Inf"} 100
func parseHistogramBuckets(metricsBody, metricName, component, instance string) []bucket {
	bucketMetric := metricName + "_bucket"
	pattern := fmt.Sprintf(`^%s\{[^}]*component="%s"[^}]*instance="%s"[^}]*le="([^"]+)"[^}]*\}\s+([0-9.+-eE]+)$`,
		regexp.QuoteMeta(bucketMetric), regexp.QuoteMeta(component), regexp.QuoteMeta(instance))
	re := regexp.MustCompile(pattern)

	var buckets []bucket
	lines := strings.Split(metricsBody, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, bucketMetric) {
			continue
		}
		match := re.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		leStr := match[1]
		countStr := match[2]

		var le float64
		if leStr == "+Inf" {
			le = math.Inf(1)
		} else {
			var err error
			le, err = strconv.ParseFloat(leStr, 64)
			if err != nil {
				continue
			}
		}

		count, err := strconv.ParseFloat(countStr, 64)
		if err != nil {
			continue
		}
		buckets = append(buckets, bucket{le: le, count: count})
	}

	sort.Slice(buckets, func(i, j int) bool { return buckets[i].le < buckets[j].le })
	return buckets
}

// parseHistogramQuantile computes an approximate quantile from histogram bucket data,
// using the same linear interpolation algorithm as Prometheus's histogram_quantile().
//
// This replaces the old parseSummaryQuantile function after the SummaryVec→HistogramVec migration.
func parseHistogramQuantile(metricsBody, metricName, quantile, component, instance string) (float64, bool) {
	q, err := strconv.ParseFloat(quantile, 64)
	if err != nil || q < 0 || q > 1 {
		return 0, false
	}

	buckets := parseHistogramBuckets(metricsBody, metricName, component, instance)
	if len(buckets) == 0 {
		GinkgoWriter.Println("No histogram buckets found. Relevant lines:")
		lines := strings.Split(metricsBody, "\n")
		for _, line := range lines {
			if strings.Contains(line, metricName) {
				GinkgoWriter.Printf("  %s\n", line)
			}
		}
		return 0, false
	}

	// Find the +Inf bucket to get total count
	var total float64
	for _, b := range buckets {
		if math.IsInf(b.le, 1) {
			total = b.count
			break
		}
	}
	if total == 0 {
		return 0, false
	}

	// Target rank
	rank := q * total

	// Linear interpolation between buckets (Prometheus histogram_quantile algorithm)
	var prevCount float64
	var prevLE float64
	for _, b := range buckets {
		if math.IsInf(b.le, 1) {
			break
		}
		if b.count >= rank {
			// Linear interpolation within this bucket
			bucketCount := b.count - prevCount
			if bucketCount == 0 {
				return prevLE, true
			}
			fraction := (rank - prevCount) / bucketCount
			return prevLE + fraction*(b.le-prevLE), true
		}
		prevCount = b.count
		prevLE = b.le
	}

	// If we didn't find it in finite buckets, return the last finite upper bound
	if len(buckets) >= 2 {
		lastFinite := buckets[len(buckets)-2] // second-to-last is last finite bucket
		if !math.IsInf(lastFinite.le, 1) {
			return lastFinite.le, true
		}
	}

	return 0, false
}

// printAllReconcileDurations prints estimated p99 reconcile durations for all components
// that exceed the given threshold.
func printAllReconcileDurations(metricsBody string, thresholdMs float64) {
	GinkgoWriter.Printf("\nReconcile estimated p99 durations over %.1f ms:\n", thresholdMs)

	// Find all unique component/instance pairs from bucket lines
	bucketMetric := "umh_core_reconcile_duration_milliseconds_bucket"
	labelPattern := regexp.MustCompile(`component="([^"]+)"[^}]*instance="([^"]+)"`)

	type labelPair struct{ component, instance string }
	seen := make(map[labelPair]bool)

	lines := strings.Split(metricsBody, "\n")
	for _, line := range lines {
		if !strings.Contains(line, bucketMetric) {
			continue
		}
		match := labelPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		pair := labelPair{match[1], match[2]}
		if seen[pair] {
			continue
		}
		seen[pair] = true

		val, found := parseHistogramQuantile(metricsBody,
			"umh_core_reconcile_duration_milliseconds", "0.99", pair.component, pair.instance)
		if found && val > thresholdMs {
			GinkgoWriter.Printf("  %s/%s: ~%.2f ms\n", pair.component, pair.instance, val)
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
	GinkgoWriter.Println("\nControl loop reconcile time quantiles (estimated from histogram):")

	quantiles := []string{"0.5", "0.9", "0.95", "0.99"}
	for _, q := range quantiles {
		val, found := parseHistogramQuantile(body,
			"umh_core_reconcile_duration_milliseconds", q, "control_loop", "main")
		if found {
			GinkgoWriter.Printf("  p%s: ~%.2f ms\n", strings.TrimPrefix(q, "0."), val)
		} else {
			GinkgoWriter.Printf("  p%s: not found\n", strings.TrimPrefix(q, "0."))
		}
	}

	return nil
}

// checkReconcileTimeThresholds checks the reconcile time thresholds based on configuration.
func checkReconcileTimeThresholds(body string, enforceP99ReconcileTime bool, enforceP95ReconcileTime bool) error {
	if enforceP99ReconcileTime {
		// Enforce the 99th percentile threshold
		recon99, found := parseHistogramQuantile(body,
			"umh_core_reconcile_duration_milliseconds", "0.99", "control_loop", "main")
		if !found {
			return errors.New("expected to find histogram buckets for control_loop's reconcile time")
		}

		if recon99 > maxReconcileTime99th {
			return fmt.Errorf("99th percentile reconcile time (~%.2f ms) exceeded %.1f ms", recon99, maxReconcileTime99th)
		}
	} else if enforceP95ReconcileTime {
		// Enforce the 95th percentile threshold
		recon95, found := parseHistogramQuantile(body,
			"umh_core_reconcile_duration_milliseconds", "0.95", "control_loop", "main")
		if !found {
			return errors.New("expected to find histogram buckets for control_loop's reconcile time")
		}

		if recon95 > maxReconcileTime99th { // For now this uses the same limit as the 99th percentile
			return fmt.Errorf("95th percentile reconcile time (~%.2f ms) exceeded %.1f ms", recon95, maxReconcileTime99th)
		}
	}

	return nil
}
