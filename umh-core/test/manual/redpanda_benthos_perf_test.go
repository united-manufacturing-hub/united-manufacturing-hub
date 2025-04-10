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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// Constants for the test
const (
	TestDuration        = 30 * time.Minute
	DefaultMemory       = "8192m" // 8GB to ensure enough memory for Redpanda
	DefaultCPUs         = 4       // More CPUs for better performance
	MetricsPort         = 8080
	RedpandaKafkaPort   = 9092
	RedpandaAdminPort   = 9644
	BenthosCount        = 10
	MessagesPerSecond   = 10 // Each Benthos instance sends 10 messages per second
	TestTopic           = "test-messages"
	StabilizationPeriod = 5 * time.Minute
)

// Variables for container management - copied from integration_test to avoid import cycle
var (
	containerName     string
	containerOnce     sync.Once
	tmpDirOnce        sync.Once
	tmpDir            string
	metricsPort       int
	redpandaKafkaPort int
	redpandaAdminPort int
)

var _ = Describe("UMH Redpanda with Benthos Performance Test", Ordered, Label("manual"), func() {
	var startTime time.Time

	BeforeAll(func() {
		// Build the test configuration
		config := buildTestConfig()

		// Convert to YAML
		configYaml, err := yaml.Marshal(config)
		Expect(err).NotTo(HaveOccurred())

		// Build and run the container using the common helper function
		err = BuildAndRunContainer(string(configYaml), DefaultMemory, DefaultCPUs)
		Expect(err).NotTo(HaveOccurred(), "Container should start successfully")

		startTime = time.Now()

		GinkgoWriter.Printf("Container started successfully. Beginning %s test...\n", TestDuration)
	})

	AfterAll(func() {
		GinkgoWriter.Println("Test completed. Stopping container...")
		PrintLogsAndStopContainer()
		cleanupTmpDirs(getContainerName())
	})

	It("should maintain stable performance for 30 minutes", func() {
		// Wait for system stabilization
		GinkgoWriter.Printf("Waiting for system to stabilize (max %v)...\n", StabilizationPeriod)

		stabilityTimeout := time.Now().Add(StabilizationPeriod)
		requiredStabilityDuration := 30 * time.Second
		stabilityStartTime := time.Time{}
		lastReportTime := time.Now()
		reportInterval := 15 * time.Second

		// Monitor until stable for required duration or timeout
		for {
			// Check if we've exceeded the maximum stabilization timeout
			if time.Now().After(stabilityTimeout) {
				GinkgoWriter.Println("Stabilization period timed out, continuing with test anyway")
				break
			}

			// Check system health
			redpandaMetrics := getRedpandaMetrics()
			metricsHealth := checkMetricsHealth()

			// Report progress periodically
			if time.Since(lastReportTime) > reportInterval {
				elapsedStabilization := time.Since(startTime)
				remainingTimeout := stabilityTimeout.Sub(time.Now())

				if !stabilityStartTime.IsZero() {
					stabilityProgress := time.Since(stabilityStartTime)

					GinkgoWriter.Printf("Stabilization progress: %v/%v stable, %v until timeout\n",
						stabilityProgress.Round(time.Second),
						requiredStabilityDuration,
						remainingTimeout.Round(time.Second))
				} else {
					GinkgoWriter.Printf("Waiting for stability: %v elapsed, %v until timeout\n",
						elapsedStabilization.Round(time.Second),
						remainingTimeout.Round(time.Second))
				}

				GinkgoWriter.Printf("  - Metrics endpoint: %v\n", metricsHealth)
				GinkgoWriter.Printf("  - Redpanda - Topics: %d, Storage: %.2f%% used\n",
					redpandaMetrics.Topics,
					100.0-(float64(redpandaMetrics.FreeBytes)/float64(redpandaMetrics.TotalBytes)*100.0))

				lastReportTime = time.Now()
			}

			// Check system health and track stability
			if metricsHealth && !redpandaMetrics.FreeSpaceAlert && redpandaMetrics.Topics > 0 {
				// System is healthy, start or continue stability timer
				if stabilityStartTime.IsZero() {
					stabilityStartTime = time.Now()
					GinkgoWriter.Println("System appears healthy, starting stability timer")
				} else if time.Since(stabilityStartTime) >= requiredStabilityDuration {
					GinkgoWriter.Printf("System stable for %v, proceeding with test\n", requiredStabilityDuration)
					break
				}
			} else {
				// System is not healthy, reset stability timer
				if !stabilityStartTime.IsZero() {
					GinkgoWriter.Println("System health check failed, resetting stability timer")
					stabilityStartTime = time.Time{}
				}
			}

			time.Sleep(1 * time.Second)
		}

		// Monitor system health for the test duration minus elapsed time
		elapsedTime := time.Since(startTime)
		remainingDuration := TestDuration - elapsedTime
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

// BuildAndRunContainer rebuilds the Docker image and starts the container
// Adapted from integration_test package to avoid import cycle
func BuildAndRunContainer(configYaml string, memory string, cpus uint) error {
	// Get unique container name
	containerName := getContainerName()

	// Validate inputs
	if memory == "" {
		return fmt.Errorf("memory limit is not set")
	}
	if cpus == 0 {
		return fmt.Errorf("cpu limit is not set")
	}

	GinkgoWriter.Printf("\n=== STARTING CONTAINER BUILD AND RUN ===\n")
	GinkgoWriter.Printf("Container name: %s\n", containerName)
	GinkgoWriter.Printf("Memory limit: %s\n", memory)
	GinkgoWriter.Printf("CPU limit: %d\n", cpus)

	// Generate random ports to avoid conflicts
	// Use PID to create somewhat unique ports, but with a limited range to avoid system port limits
	metricsPrt := 8081 + (os.Getpid() % 1000) // Base port 8081 + offset based on PID
	kafkaPort := 9092 + (os.Getpid() % 1000)  // Base port 9092 + offset based on PID
	adminPort := 9644 + (os.Getpid() % 1000)  // Base port 9644 + offset based on PID

	GinkgoWriter.Printf("Port mappings - Host:Container\n")
	GinkgoWriter.Printf("- Metrics: %d:8080\n", metricsPrt)
	GinkgoWriter.Printf("- Kafka: %d:9092\n", kafkaPort)
	GinkgoWriter.Printf("- Admin: %d:9644\n", adminPort)

	// Store these ports for later use
	metricsPort = metricsPrt
	redpandaKafkaPort = kafkaPort
	redpandaAdminPort = adminPort

	// 1. Stop/Remove any old container
	GinkgoWriter.Println("Cleaning up any previous containers with the same name...")
	out, err := runDockerCommand("rm", "-f", containerName) // ignoring error
	if err != nil {
		GinkgoWriter.Printf("Note: Container removal returned: %v, output: %s (this may be normal)\n", err, out)
	}

	// 2. Build image
	GinkgoWriter.Println("Building Docker image...")
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Navigate to umh-core root directory (up 2 levels from test/manual)
	coreDir := filepath.Dir(filepath.Dir(currentDir))
	dockerfilePath := filepath.Join(coreDir, "Dockerfile")

	GinkgoWriter.Printf("Core directory: %s\n", coreDir)
	GinkgoWriter.Printf("Dockerfile path: %s\n", dockerfilePath)

	out, err = runDockerCommand("build", "-t", "umh-core:latest", "-f", dockerfilePath, coreDir)
	if err != nil {
		GinkgoWriter.Printf("Docker build failed: %v\n", err)
		GinkgoWriter.Printf("Build output:\n%s\n", out)
		return fmt.Errorf("Docker build failed: %s", out)
	}
	GinkgoWriter.Println("Docker build successful")

	// 3. Create temp directories
	tmpRedpandaDir := filepath.Join(getTmpDir(), containerName, "redpanda")
	tmpLogsDir := filepath.Join(getTmpDir(), containerName, "logs")
	configPath := filepath.Join(getTmpDir(), containerName, "config.yaml")

	// Create the directories
	if err := os.MkdirAll(tmpRedpandaDir, 0o777); err != nil {
		return fmt.Errorf("failed to create redpanda dir: %w", err)
	}
	if err := os.MkdirAll(tmpLogsDir, 0o777); err != nil {
		return fmt.Errorf("failed to create logs dir: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, []byte(configYaml), 0o666); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// 4. Run container
	GinkgoWriter.Println("Starting container...")
	out, err = runDockerCommand(
		"run", "-d",
		"--name", containerName,
		"-e", "LOGGING_LEVEL=debug",
		// Map the host ports to the container's fixed ports
		"-p", fmt.Sprintf("%d:8080", metricsPrt),
		"-p", fmt.Sprintf("%d:9092", kafkaPort),
		"-p", fmt.Sprintf("%d:9644", adminPort),
		"--memory", memory,
		"--cpus", fmt.Sprintf("%d", cpus),
		"-v", fmt.Sprintf("%s:/data/redpanda", tmpRedpandaDir),
		"-v", fmt.Sprintf("%s:/data/logs", tmpLogsDir),
		"-v", fmt.Sprintf("%s:/data/config.yaml", configPath),
		"umh-core:latest",
	)
	if err != nil {
		GinkgoWriter.Printf("Container start failed: %v\n", err)
		GinkgoWriter.Printf("Container run output:\n%s\n", out)
		return fmt.Errorf("could not run container: %s", out)
	}
	GinkgoWriter.Printf("Container started with ID: %s\n", strings.TrimSpace(out))

	// 5. Verify the container is running
	out, err = runDockerCommand("inspect", "--format", "{{.State.Status}}", containerName)
	if err != nil {
		GinkgoWriter.Printf("Failed to inspect container: %v\n", err)
	} else {
		containerState := strings.TrimSpace(out)
		GinkgoWriter.Printf("Container state: %s\n", containerState)
		if containerState != "running" {
			GinkgoWriter.Println("WARNING: Container is not in running state!")
			printContainerDebugInfo()
			return fmt.Errorf("container is in %s state, not running", containerState)
		}
	}

	// 6. Wait for the container to be healthy
	GinkgoWriter.Println("Waiting for metrics endpoint to become available...")
	return waitForMetrics()
}

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

// Helper functions copied from integration_test
func getContainerName() string {
	containerOnce.Do(func() {
		// Generate a random 8-character suffix
		suffix := make([]byte, 4)
		_, err := rand.Read(suffix)
		if err != nil {
			// Fallback to timestamp if random fails
			containerName = fmt.Sprintf("umh-core-perf-%d", os.Getpid())
			return
		}
		containerName = fmt.Sprintf("umh-core-perf-%s", hex.EncodeToString(suffix))
	})
	return containerName
}

func getTmpDir() string {
	tmpDirOnce.Do(func() {
		tmpDir = "/tmp"
		// If we are in a devcontainer, use the workspace as tmp dir
		if os.Getenv("REMOTE_CONTAINERS") != "" || os.Getenv("CODESPACE_NAME") != "" || os.Getenv("USER") == "vscode" {
			tmpDir = "/workspaces/united-manufacturing-hub/umh-core/tmp"
		}
	})
	return tmpDir
}

func cleanupTmpDirs(containerName string) {
	filepath := filepath.Join(getTmpDir(), containerName)
	GinkgoWriter.Printf("Cleaning up temporary directories for container %s (%s)\n", containerName, filepath)
	os.RemoveAll(filepath)
}

func waitForMetrics() error {
	startTime := time.Now()
	timeout := 2 * time.Minute
	interval := 1 * time.Second

	for time.Since(startTime) < timeout {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", metricsPort))
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			GinkgoWriter.Printf("Metrics endpoint is available after %v\n", time.Since(startTime))
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("metrics endpoint not available after %v", timeout)
}

func runDockerCommand(args ...string) (string, error) {
	GinkgoWriter.Printf("Running docker command: %v\n", args)
	// Check if we use docker or podman
	dockerCmd := "docker"
	if _, err := exec.LookPath("podman"); err == nil {
		dockerCmd = "podman"
	}
	cmd := exec.Command(dockerCmd, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func PrintLogsAndStopContainer() {
	containerName := getContainerName()
	if CurrentSpecReport().Failed() {
		GinkgoWriter.Println("Test failed, printing container logs:")
		printContainerLogs()
	}
	// First stop the container
	runDockerCommand("stop", containerName)
	// Then remove it
	runDockerCommand("rm", "-f", containerName)
}

func printContainerLogs() {
	containerName := getContainerName()
	GinkgoWriter.Printf("Test failed, printing container logs:\n")

	// 1. Regular container logs (stdout/stderr)
	GinkgoWriter.Printf("\n=== DOCKER CONTAINER LOGS ===\n")
	out, err := runDockerCommand("logs", containerName)
	if err != nil {
		GinkgoWriter.Printf("Failed to get container logs: %v\n", err)
	} else {
		GinkgoWriter.Printf("%s\n", out)
	}

	// 2. Internal UMH Core logs
	GinkgoWriter.Printf("\n=== UMH CORE INTERNAL LOGS ===\n")
	out, err = runDockerCommand("exec", containerName, "cat", "/data/logs/umh-core/current")
	if err != nil {
		GinkgoWriter.Printf("Failed to get UMH Core internal logs: %v\n", err)
	} else if out == "" {
		GinkgoWriter.Printf("UMH Core internal logs are empty or not found\n")
	} else {
		GinkgoWriter.Printf("%s\n", out)
	}
}

func printContainerDebugInfo() {
	containerName := getContainerName()

	GinkgoWriter.Println("\n==== CONTAINER DEBUG INFO ====")

	// 1. List all running containers
	GinkgoWriter.Println("\n[ALL RUNNING CONTAINERS]")
	out, err := runDockerCommand("ps", "-a")
	if err != nil {
		GinkgoWriter.Printf("Failed to list containers: %v\n", err)
	} else {
		GinkgoWriter.Println(out)
	}

	// 2. Check our container's status
	GinkgoWriter.Printf("\n[CONTAINER STATUS: %s]\n", containerName)
	out, err = runDockerCommand("inspect", "--format", "{{.State.Status}}", containerName)
	if err != nil {
		GinkgoWriter.Printf("Failed to get container status: %v\n", err)
	} else {
		GinkgoWriter.Printf("Status: %s\n", strings.TrimSpace(out))
	}

	// 3. Container logs
	GinkgoWriter.Println("\n[CONTAINER LOGS (last 30 lines)]")
	out, err = runDockerCommand("logs", "--tail", "30", containerName)
	if err != nil {
		GinkgoWriter.Printf("Failed to get container logs: %v\n", err)
	} else {
		GinkgoWriter.Println(out)
	}

	GinkgoWriter.Println("\n==== END DEBUG INFO ====")
}

// checkContainerHealth verifies the container is running and metrics endpoint is reachable
func checkContainerHealth() bool {
	// Check container is running
	cmd := exec.Command("docker", "inspect", "--format={{.State.Running}}", getContainerName())
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return false
	}

	// Check metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", metricsPort))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// checkMetricsHealth verifies the metrics endpoint is healthy
func checkMetricsHealth() bool {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", metricsPort))
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

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/public_metrics", redpandaAdminPort))
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
	cmd := exec.CommandContext(ctx, "docker", "exec", getContainerName(),
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
