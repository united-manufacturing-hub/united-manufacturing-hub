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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	. "github.com/onsi/ginkgo/v2" // nolint: staticcheck // Ginkgo is designed to be used with dot imports
	. "github.com/onsi/gomega"    // nolint: staticcheck // Gomega is designed to be used with dot imports
)

// monitorHealth checks the metrics and golden service.
func monitorHealth() {
	// 1) Check metrics
	failOnMetricsHealthIssue()
	GinkgoWriter.Println("✅ Metrics are healthy")

	// 2) Check Golden service
	checkGoldenServiceWithFailure()
	GinkgoWriter.Println("✅ Golden service is running")

	// 3) Check and print system information (CPU, Memory, etc.)
	printSystemInformation()
}
func printSystemInformation() {
	// Get CPU information
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		GinkgoWriter.Printf("Failed to get CPU usage: %v\n", err)
	} else {
		GinkgoWriter.Printf("CPU usage: %.2f%%\n", cpuPercent[0])
	}

	// Get CPU core count
	cpuCounts, err := cpu.Counts(true)
	if err != nil {
		GinkgoWriter.Printf("Failed to get CPU count: %v\n", err)
	} else {
		GinkgoWriter.Printf("CPU cores: %d\n", cpuCounts)
	}

	// Get memory information
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		GinkgoWriter.Printf("Failed to get memory usage: %v\n", err)
	} else {
		GinkgoWriter.Printf("Memory usage: %.2f%% (Used: %d MB, Total: %d MB)\n",
			vmStat.UsedPercent,
			vmStat.Used/1024/1024,
			vmStat.Total/1024/1024)
	}
}

// failOnMetricsHealthIssue expects the metrics to be healthy, otherwise it fails the test
func failOnMetricsHealthIssue() {
	data, err := getMetricsHealth()
	Expect(err).NotTo(HaveOccurred(), "Metrics endpoint should be healthy")
	metricsErrors := checkWhetherMetricsHealthy(string(data), true, true)
	Expect(metricsErrors).To(BeEmpty(), "Metrics should be healthy")
}

// reportOnMetricsHealthIssue is similar to failOnMetricsHealthIssue, but it returns an error instead of failing the test, allowing the caller to handle it
func reportOnMetricsHealthIssue(enforceP99ReconcileTime bool, enforceP95ReconcileTime bool) error {
	data, err := getMetricsHealth()
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}
	metricsErrors := checkWhetherMetricsHealthy(string(data), enforceP99ReconcileTime, enforceP95ReconcileTime)
	if len(metricsErrors) > 0 {
		return fmt.Errorf("metrics are not healthy: %v", metricsErrors)
	}
	return nil
}

func getMetricsHealth() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", GetMetricsURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			Fail(fmt.Sprintf("Error closing response body: %v\n", err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint returned non-200: %v", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics: %w", err)
	}

	return data, nil
}

// checkGoldenService sends a test request to the golden service and checks that it returns a 200 status code
func checkGoldenService() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", GetGoldenServiceURL(), bytes.NewBuffer([]byte(`{"message": "test"}`)))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	checkResp, e := http.DefaultClient.Do(req)
	if e != nil {
		return 0, fmt.Errorf("failed to send request: %w", e)
	}
	defer func() {
		if err := checkResp.Body.Close(); err != nil {
			Fail(fmt.Sprintf("Error closing response body: %v\n", err))
		}
	}()

	return checkResp.StatusCode, nil
}

// checkGoldenServiceWithFailure sends a test request to the golden service and fails the test if it doesn't return 200
func checkGoldenServiceWithFailure() {
	statusCode, err := checkGoldenService()
	if err != nil {
		Fail(fmt.Sprintf("failed to check golden service: %v\n", err))
	}
	if statusCode != 200 {
		Fail(fmt.Sprintf("Golden service returned status: %d", statusCode))
	}
}

// checkGoldenServiceStatusOnly sends a test request to the golden service and returns only the status code.
// It handles errors using Expect() so it can be used directly with Eventually().
func checkGoldenServiceStatusOnly() int {
	statusCode, _ := checkGoldenService()
	// We ignore any errors here as they can happen during initial startup
	return statusCode
}

// waitForMetrics polls the /metrics endpoint until it returns 200
func waitForMetrics() error {
	startTime := time.Now()

	// Print initial debug info before we start polling
	fmt.Printf("Starting to wait for metrics at %s\n", startTime.Format(time.RFC3339))
	fmt.Printf("Container name: %s\n", getContainerName())

	// Track errors for better debugging
	var lastError error
	var consecutiveErrors int
	var totalAttempts int
	var lastURL string

	Eventually(func() error {
		totalAttempts++
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		url := GetMetricsURL()
		lastURL = url

		// The URL detection and logging is now handled by GetMetricsURL()
		fmt.Printf("Attempt %d: Connecting to metrics...\n", totalAttempts)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			lastError = fmt.Errorf("failed to create request to %s: %w", url, err)
			consecutiveErrors++

			// Every 5 consecutive errors, print diagnostic info
			if consecutiveErrors%5 == 0 {
				fmt.Printf("Attempt %d: Still failing after %v. Last error: %v\n",
					totalAttempts, time.Since(startTime), lastError)

				// After 15 consecutive errors, print container debug info
				if consecutiveErrors == 15 {
					printContainerDebugInfo()
				}
			}
			return lastError
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastError = fmt.Errorf("failed to connect to %s: %w", url, err)
			consecutiveErrors++

			// Every 5 consecutive errors, print more info
			if consecutiveErrors%5 == 0 {
				fmt.Printf("Attempt %d: Still failing after %v. Last error: %v\n",
					totalAttempts, time.Since(startTime), lastError)

				// After 15 consecutive errors, print container debug info
				if consecutiveErrors == 15 {
					printContainerDebugInfo()
				}
			}
			return lastError
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				Fail(fmt.Sprintf("Error closing response body: %v\n", err))
			}
		}()

		if resp.StatusCode != http.StatusOK {
			lastError = fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
			consecutiveErrors++
			return lastError
		}

		// Success! Reset error counter
		consecutiveErrors = 0
		fmt.Printf("Successfully connected to metrics endpoint (%s) after %v (%d attempts)\n",
			url, time.Since(startTime), totalAttempts)
		return nil
	}, 60*time.Second, 1*time.Second).Should(Succeed(), func() string {
		// If we're still failing after 60 seconds, print detailed debug info
		printContainerDebugInfo()

		// Return a detailed error message
		return fmt.Sprintf("Failed to connect to metrics endpoint (%s) after %v (%d attempts). Last error: %v",
			lastURL, time.Since(startTime), totalAttempts, lastError)
	})

	return nil
}
