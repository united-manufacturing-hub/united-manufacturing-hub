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

//go:build testmanual
// +build testmanual

package manual_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// Constants for the test
const (
	TestDuration        = 30 * time.Minute
	ContainerName       = "umh-redpanda-benthos-perf-test"
	DefaultMemory       = "4096m"
	DefaultCPUs         = 2
	MetricsPort         = 8080
	RedpandaKafkaPort   = 9092
	RedpandaAdminPort   = 9644
	BenthosCount        = 10
	MessagesPerSecond   = 10 // Each Benthos instance sends 10 messages per second
	TestTopic           = "test-messages"
	StabilizationPeriod = 5 * time.Minute
)

var _ = Describe("UMH Redpanda with Benthos Performance Test", Ordered, Label("manual"), func() {
	var configPath string
	var startTime time.Time

	BeforeAll(func() {
		// Create a temporary directory for config
		tempDir, err := os.MkdirTemp("", "umh-perf-test")
		Expect(err).NotTo(HaveOccurred())
		configPath = tempDir + "/config.yaml"

		// Build the test configuration
		config := buildTestConfig()

		// Write config to file
		configBytes, err := yaml.Marshal(config)
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile(configPath, configBytes, 0644)
		Expect(err).NotTo(HaveOccurred())

		// Stop any existing container with same name
		exec.Command("docker", "rm", "-f", ContainerName).Run()

		// Start container with our configuration
		cmd := exec.Command(
			"docker", "run", "-d",
			"--name", ContainerName,
			"-p", fmt.Sprintf("%d:8080", MetricsPort),
			"-p", fmt.Sprintf("%d:9092", RedpandaKafkaPort),
			"-p", fmt.Sprintf("%d:9644", RedpandaAdminPort),
			"-v", fmt.Sprintf("%s:/data/config.yaml", configPath),
			"-m", DefaultMemory,
			"--cpus", fmt.Sprintf("%d", DefaultCPUs),
			"united-manufacturing-hub/umh-core:latest",
		)

		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to start container: %s", string(output))

		startTime = time.Now()

		// Wait for container to be ready
		Eventually(func() bool {
			return checkContainerHealth()
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Container should be healthy")

		GinkgoWriter.Printf("Container started successfully. Beginning %s test...\n", TestDuration)
	})

	AfterAll(func() {
		GinkgoWriter.Println("Test completed. Stopping container...")
		exec.Command("docker", "logs", ContainerName).Run()
		exec.Command("docker", "rm", "-f", ContainerName).Run()
		// Clean up temp config
		if configPath != "" {
			os.Remove(configPath)
		}
	})

	It("should maintain stable performance for 30 minutes", func() {
		// Wait for stabilization period
		GinkgoWriter.Printf("Waiting for %v stabilization period...\n", StabilizationPeriod)
		time.Sleep(StabilizationPeriod)

		// Monitor system health for the test duration minus stabilization period
		remainingDuration := TestDuration - StabilizationPeriod
		monitorUntil := time.Now().Add(remainingDuration)

		GinkgoWriter.Printf("Beginning monitoring period for %v...\n", remainingDuration)

		// Check health every minute and report stats
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for time.Now().Before(monitorUntil) {
			select {
			case <-ticker.C:
				elapsedTime := time.Since(startTime)
				remainingTime := TestDuration - elapsedTime

				// Get container health and metrics
				redpandaMetrics := getRedpandaMetrics()
				metricsHealth := checkMetricsHealth()

				GinkgoWriter.Printf("Health check (%v elapsed, %v remaining):\n",
					elapsedTime.Round(time.Second),
					remainingTime.Round(time.Second))
				GinkgoWriter.Printf("  - Metrics endpoint: %v\n", metricsHealth)
				GinkgoWriter.Printf("  - Redpanda - Topics: %d, Storage: %.2f%% used\n",
					redpandaMetrics.Topics,
					100.0-(float64(redpandaMetrics.FreeBytes)/float64(redpandaMetrics.TotalBytes)*100.0))

				// Verify system is healthy
				Expect(metricsHealth).To(BeTrue(), "Metrics endpoint should be healthy")
				Expect(redpandaMetrics.FreeSpaceAlert).To(BeFalse(), "Redpanda storage should not have space alerts")
			default:
				time.Sleep(5 * time.Second)
			}
		}

		// Test completed successfully, now count messages
		GinkgoWriter.Println("Test duration completed successfully, counting messages...")

		// Calculate expected messages
		expectedMsgCount := int64(TestDuration.Seconds() * MessagesPerSecond * BenthosCount)

		// Get actual message count using rpk
		actualMsgCount := getMessageCount()

		// Calculate allowed deviation (10%)
		maxDeviation := float64(expectedMsgCount) * 0.1
		minExpected := int64(float64(expectedMsgCount) - maxDeviation)
		maxExpected := int64(float64(expectedMsgCount) + maxDeviation)

		GinkgoWriter.Printf("Message count verification:\n")
		GinkgoWriter.Printf("  - Expected messages: %d (±10%%)\n", expectedMsgCount)
		GinkgoWriter.Printf("  - Actual messages: %d\n", actualMsgCount)
		GinkgoWriter.Printf("  - Acceptable range: %d to %d\n", minExpected, maxExpected)

		// Verify message count is within acceptable range
		Expect(actualMsgCount).To(BeNumerically(">=", minExpected),
			"Message count should be at least %d", minExpected)
		Expect(actualMsgCount).To(BeNumerically("<=", maxExpected),
			"Message count should not exceed %d", maxExpected)
	})
})

// Helper functions

// buildTestConfig creates the test configuration with Redpanda and 10 Benthos instances
func buildTestConfig() map[string]interface{} {
	// Create base configuration
	config := map[string]interface{}{
		"agent": map[string]interface{}{
			"metricsPort": MetricsPort,
		},
		"internal": map[string]interface{}{
			"services": []interface{}{},
			"benthos":  []interface{}{},
			"redpanda": map[string]interface{}{
				"name":            "redpanda",
				"desiredFSMState": "active",
				"topic": map[string]interface{}{
					"defaultTopicRetentionMs":    -1, // Infinite retention
					"defaultTopicRetentionBytes": 0,  // Unlimited size
				},
				"resources": map[string]interface{}{
					"maxCores":             DefaultCPUs - 1,
					"memoryPerCoreInBytes": 1024 * 1024 * 1024, // 1GB per core
				},
			},
		},
	}

	// Add Benthos instances
	benthosInstances := []interface{}{}
	for i := 0; i < BenthosCount; i++ {
		benthosInstance := map[string]interface{}{
			"name":            fmt.Sprintf("benthos-%d", i),
			"desiredFSMState": "active",
			"metricsPort":     0, // Auto-assign
			"logLevel":        "ERROR",
			"input": map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  fmt.Sprintf(`root = {"id":"%d","value":"test message from benthos-%d","timestamp":%v}`, i, i, "${!timestamp_unix_nano}"),
					"interval": "0.1s", // 10 messages per second
					"count":    0,      // Unlimited
				},
			},
			"output": map[string]interface{}{
				"kafka": map[string]interface{}{
					"addresses":     []string{"localhost:9092"},
					"topic":         TestTopic,
					"max_in_flight": 1000,
					"metadata": map[string]interface{}{
						"key": "${!json(\"id\")}",
					},
					"batching": map[string]interface{}{
						"count":      100,
						"period":     "1s",
						"processors": []interface{}{},
					},
				},
			},
		}
		benthosInstances = append(benthosInstances, benthosInstance)
	}

	// Add Benthos instances to config
	config["internal"].(map[string]interface{})["benthos"] = benthosInstances

	return config
}

// checkContainerHealth verifies the container is running and metrics endpoint is reachable
func checkContainerHealth() bool {
	// Check container is running
	cmd := exec.Command("docker", "inspect", "--format={{.State.Running}}", ContainerName)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return false
	}

	// Check metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", MetricsPort))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// checkMetricsHealth verifies the metrics endpoint is healthy
func checkMetricsHealth() bool {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", MetricsPort))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	return strings.Contains(string(body), "umh_core_reconcile_duration_milliseconds")
}

// RedpandaMetrics contains key metrics from Redpanda
type RedpandaMetrics struct {
	Topics         int64
	FreeBytes      int64
	TotalBytes     int64
	FreeSpaceAlert bool
}

// getRedpandaMetrics retrieves key metrics from Redpanda
func getRedpandaMetrics() RedpandaMetrics {
	metrics := RedpandaMetrics{}

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/public_metrics", RedpandaAdminPort))
	if err != nil {
		return metrics
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return metrics
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return metrics
	}

	// Extract metrics using regex
	topicsRegex := regexp.MustCompile(`redpanda_cluster_topics\s+(\d+)`)
	if matches := topicsRegex.FindStringSubmatch(string(body)); len(matches) > 1 {
		metrics.Topics, _ = strconv.ParseInt(matches[1], 10, 64)
	}

	freeBytesRegex := regexp.MustCompile(`redpanda_storage_disk_free_bytes\s+(\d+)`)
	if matches := freeBytesRegex.FindStringSubmatch(string(body)); len(matches) > 1 {
		metrics.FreeBytes, _ = strconv.ParseInt(matches[1], 10, 64)
	}

	totalBytesRegex := regexp.MustCompile(`redpanda_storage_disk_total_bytes\s+(\d+)`)
	if matches := totalBytesRegex.FindStringSubmatch(string(body)); len(matches) > 1 {
		metrics.TotalBytes, _ = strconv.ParseInt(matches[1], 10, 64)
	}

	freeSpaceAlertRegex := regexp.MustCompile(`redpanda_storage_disk_free_space_alert\s+(\d+)`)
	if matches := freeSpaceAlertRegex.FindStringSubmatch(string(body)); len(matches) > 1 {
		alertValue, _ := strconv.ParseInt(matches[1], 10, 64)
		metrics.FreeSpaceAlert = alertValue > 0
	}

	return metrics
}

// getMessageCount runs rpk to count messages in the test topic
func getMessageCount() int64 {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Run rpk to consume messages with --timeout
	cmd := exec.CommandContext(ctx, "docker", "exec", ContainerName,
		"/opt/redpanda/bin/rpk", "topic", "consume", TestTopic,
		"--num", "-1", // consume all messages
		"--brokers", "localhost:9092",
		"--timeout", "2m", // timeout after 2 minutes
	)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	GinkgoWriter.Printf("Running RPK to count messages...\n")
	err := cmd.Run()
	if err != nil {
		GinkgoWriter.Printf("Error running RPK: %v\n", err)
		return 0
	}

	// Count non-empty lines to get message count
	messageCount := int64(0)
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.TrimSpace(line) != "" {
			messageCount++
		}
	}

	return messageCount
}
