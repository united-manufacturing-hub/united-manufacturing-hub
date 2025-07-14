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

package redpanda_monitor_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// getTmpDir returns the temporary directory for a container
func getTmpDir() string {
	tmpDir := "/tmp"
	// If we are in a devcontainer, use the workspace as tmp dir
	// This is because in a devcontainer, the tmp dir is very small
	if os.Getenv("REMOTE_CONTAINERS") != "" || os.Getenv("CODESPACE_NAME") != "" || os.Getenv("USER") == "vscode" {
		tmpDir = "/workspaces/united-manufacturing-hub/umh-core/tmp"
	}
	return tmpDir
}

// newTimeoutContext creates a context with a 30-second timeout
func newTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func getMetricsReader() *bytes.Reader {
	// metrics references the redpanda_monitor_data_test.go file
	// Remove all newlines in string
	mX := strings.ReplaceAll(metrics, "\n", "")
	// Hex decode metrics
	metricsBytes, err := hex.DecodeString(mX)
	Expect(err).NotTo(HaveOccurred())
	// Gzip reader
	gzipReader, err := gzip.NewReader(bytes.NewReader(metricsBytes))
	Expect(err).NotTo(HaveOccurred())
	// Read all
	data, err := io.ReadAll(gzipReader)
	Expect(err).NotTo(HaveOccurred())
	dataReader := bytes.NewReader(data)
	return dataReader
}

var _ = Describe("Redpanda Monitor Service", func() {
	var (
		service         *redpanda_monitor.RedpandaMonitorService
		tick            uint64
		mockSvcRegistry *serviceregistry.Registry
		ctx             context.Context
		cancel          context.CancelFunc
	)

	BeforeEach(func() {
		service = redpanda_monitor.NewRedpandaMonitorService("test-redpanda")
		tick = 0

		mockSvcRegistry = serviceregistry.NewMockRegistry()
		// Cleanup the data directory
		ctx, cancel = newTimeoutContext()
		err := mockSvcRegistry.GetFileSystem().RemoveAll(ctx, getTmpDir())
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		cancel()
	})

	Describe("GenerateS6ConfigForRedpandaMonitor", func() {
		It("should generate valid S6 configuration", func() {
			s6Config, err := service.GenerateS6ConfigForRedpandaMonitor(service.GetS6ServiceName())
			Expect(err).NotTo(HaveOccurred())

			// Verify the config contains the expected command and script
			Expect(s6Config.Command).To(HaveLen(2))
			Expect(s6Config.Command[0]).To(Equal("/bin/sh"))
			Expect(s6Config.ConfigFiles).To(HaveKey("run_redpanda_monitor.sh"))

			// Verify the script content contains the necessary markers
			script := s6Config.ConfigFiles["run_redpanda_monitor.sh"]
			Expect(script).To(ContainSubstring(redpanda_monitor.BLOCK_START_MARKER))
			Expect(script).To(ContainSubstring(redpanda_monitor.METRICS_END_MARKER))
			Expect(script).To(ContainSubstring(redpanda_monitor.BLOCK_END_MARKER))
			Expect(script).To(ContainSubstring("curl -sSL"))
			Expect(script).To(ContainSubstring("sleep 1"))
		})
	})

	Describe("Service Status", func() {
		It("should return an error if service does not exist", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			_, err := service.Status(ctx, mockSvcRegistry.GetFileSystem(), tick)
			Expect(err).To(HaveOccurred())
		})

		It("should return service info when service exists", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Mock the S6 service to return some logs
			mockS6 := process_shared.NewMockService()

			// Create a new service with the mock S6 service
			service = redpanda_monitor.NewRedpandaMonitorService("test-redpanda", redpanda_monitor.WithS6Service(mockS6))

			// Add the service first
			err := service.AddRedpandaMonitorToS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Make sure the service exists by reconciling
			err, _ = service.ReconcileManager(ctx, mockSvcRegistry, 0)
			Expect(err).NotTo(HaveOccurred())

			// Explicitly mark the service as existing in the mock
			servicePath := fmt.Sprintf("%s/%s", constants.S6BaseDir, service.GetS6ServiceName())
			mockS6.ExistingServices[servicePath] = true

			// Set up mock logs that include our markers and some fake metrics data
			mockLogs := []process_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_START_MARKER)},
				{Content: "1f8b0800000000000003abcd4f2c492d2e516c0600000000ffff0300ee1f0e9e09000000\n"}, // Some hex-encoded gzipped data
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.METRICS_END_MARKER)},
				{Content: "1f8b0800000000000003abcd4f2c492d2e516c0600000000ffff0300ee1f0e9e09000000\n"}, // Some hex-encoded gzipped data
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_END_MARKER)},
			}

			// Set the mock logs result directly
			mockS6.GetLogsResult = mockLogs

			// Try getting status - we don't need to capture the result
			_, err = service.Status(ctx, mockSvcRegistry.GetFileSystem(), tick)
			Expect(err).To(HaveOccurred())
			// Check that this is a "failed to parse metrics" error
			Expect(err.Error()).To(ContainSubstring("failed to parse metrics"))

			// We expect an error due to the mock data not being real metrics data
			// but at least the service should report as existing
			Expect(service.ServiceExists(ctx, mockSvcRegistry.GetFileSystem())).To(BeTrue())
		})
	})

	Describe("ParseRedpandaLogs", func() {
		It("should return an error for empty logs", func() {
			logs := []process_shared.LogEntry{}
			_, err := service.ParseRedpandaLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no logs provided"))
		})

		It("should return an error if no block end marker is found", func() {
			logs := []process_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.METRICS_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.CLUSTERCONFIG_END_MARKER)},
				{Content: "timestamp data\n"},
			}
			_, err := service.ParseRedpandaLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse redpanda metrics/configuration: no sections found. This can happen when the redpanda service is not running, or the logs where rotate"))
		})

		It("should return an error if no start marker is found", func() {
			logs := []process_shared.LogEntry{
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.METRICS_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.CLUSTERCONFIG_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseRedpandaLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse redpanda metrics/configuration: no sections found. This can happen when the redpanda service is not running, or the logs where rotate"))
		})

		It("should return an error if no metrics end marker is found", func() {
			logs := []process_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.CLUSTERCONFIG_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseRedpandaLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse redpanda metrics/configuration: no sections found. This can happen when the redpanda service is not running, or the logs where rotate"))
		})

		It("should return an error if no config end marker is found", func() {
			logs := []process_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.METRICS_END_MARKER)},
				{Content: "more data\n"},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseRedpandaLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse redpanda metrics/configuration: no sections found. This can happen when the redpanda service is not running, or the logs where rotate"))
		})

		It("should return an error if markers are in incorrect order", func() {
			logs := []process_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_START_MARKER)},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.CLUSTERCONFIG_END_MARKER)}, // Wrong order
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.METRICS_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseRedpandaLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse redpanda metrics/configuration: no sections found. This can happen when the redpanda service is not running, or the logs where rotate"))
		})

	})

	Describe("Mock Service", func() {
		It("should implement all required interfaces", func() {
			mockService := redpanda_monitor.NewMockRedpandaMonitorService()

			// Test a few interfaces to make sure the mock works as expected
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Call AddRedpandaMonitorToS6Manager and check if called flag is set
			err := mockService.AddRedpandaMonitorToS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddRedpandaToS6ManagerCalled).To(BeTrue())

			// Generate config and verify it has expected content
			config, err := mockService.GenerateS6ConfigForRedpandaMonitor("redpanda")
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.GenerateS6ConfigForRedpandaMonitorCalled).To(BeTrue())
			Expect(config.ConfigFiles).To(HaveKey("run_redpanda_monitor.sh"))

			// Test setting service state
			mockService.SetServiceState(redpanda_monitor.ServiceStateFlags{
				IsRunning:       true,
				IsMetricsActive: true,
			})
			state := mockService.GetServiceState()
			Expect(state.IsRunning).To(BeTrue())
			Expect(state.IsMetricsActive).To(BeTrue())
		})
	})

	Describe("Can parse the metrics", func() {
		It("should return an error if no metrics are provided", func() {
			logs := []process_shared.LogEntry{}
			_, err := service.ParseRedpandaLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
		})
	})
	It("should parse the metrics", func() {

		m, err := redpanda_monitor.ParseMetrics(getMetricsReader())
		Expect(err).NotTo(HaveOccurred())
		mShould := redpanda_monitor.Metrics{
			Infrastructure: redpanda_monitor.InfrastructureMetrics{
				Storage: redpanda_monitor.StorageMetrics{
					FreeBytes:      255598518272,
					TotalBytes:     494384795648,
					FreeSpaceAlert: false,
				},
			},
			Cluster: redpanda_monitor.ClusterMetrics{
				Topics:            0,
				UnavailableTopics: 0,
			},
			Throughput: redpanda_monitor.ThroughputMetrics{
				BytesIn:  0,
				BytesOut: 0,
			},
			Topic: redpanda_monitor.TopicMetrics{
				TopicPartitionMap: map[string]int64{},
			},
		}
		Expect(m).To(Equal(mShould))
	})

	It("should parse the test_metrics", func() {
		// 1. Load the test_metrics.txt file (from current dir)
		metricsData, err := os.ReadFile("test_metrics.txt")
		Expect(err).NotTo(HaveOccurred())

		// 2. Parse it line by line into s6service.LogEntry
		lines := strings.Split(string(metricsData), "\n")
		var logEntries []process_shared.LogEntry

		for _, line := range lines {
			if len(line) > 0 {
				// Remove timestamps at the beginning of the line
				parts := strings.SplitN(line, "  ", 2)
				if len(parts) == 2 {
					// Use the content part (after the timestamp)
					logEntries = append(logEntries, process_shared.LogEntry{Content: parts[1]})
				} else {
					// For lines without timestamps (like the marker lines)
					logEntries = append(logEntries, process_shared.LogEntry{Content: line})
				}
			}
		}

		// 3. Parse it into metrics
		redpandaMetricsConfig, err := service.ParseRedpandaLogs(ctx, logEntries, tick)
		Expect(err).NotTo(HaveOccurred())
		Expect(redpandaMetricsConfig).NotTo(BeNil())

		// 4. Verify the metrics are parsed correctly
		metricsResult := redpandaMetricsConfig.RedpandaMetrics

		// Verify storage metrics
		// Note: this value is different from the other test, as the metrics are different
		Expect(metricsResult.Metrics.Infrastructure.Storage.FreeBytes).To(Equal(int64(135588388864)))
		Expect(metricsResult.Metrics.Infrastructure.Storage.TotalBytes).To(Equal(int64(253322825728)))
		Expect(metricsResult.Metrics.Infrastructure.Storage.FreeSpaceAlert).To(BeFalse())

		// Verify cluster metrics
		Expect(metricsResult.Metrics.Cluster.Topics).To(Equal(int64(0)))
		Expect(metricsResult.Metrics.Cluster.UnavailableTopics).To(Equal(int64(0)))

		// Verify throughput metrics
		Expect(metricsResult.Metrics.Throughput.BytesIn).To(Equal(int64(0)))
		Expect(metricsResult.Metrics.Throughput.BytesOut).To(Equal(int64(0)))

		// Verify topic metrics
		Expect(metricsResult.Metrics.Topic.TopicPartitionMap).To(HaveLen(0))

		// Verify the cluster config
		config := redpandaMetricsConfig.ClusterConfig
		Expect(config.Topic.DefaultTopicCompressionAlgorithm).To(Equal("producer"))
		Expect(config.Topic.DefaultTopicRetentionBytes).To(Equal(int64(0)))
		Expect(config.Topic.DefaultTopicRetentionMs).To(Equal(int64(604800000)))
		Expect(config.Topic.DefaultTopicCleanupPolicy).To(Equal("delete"))
		Expect(config.Topic.DefaultTopicSegmentMs).To(Equal(int64(1209600000)))
	})

	Describe("RedpandaMetricsState", func() {
		It("should calculate throughput metrics correctly", func() {
			// Create a new metrics state
			state := redpanda_monitor.NewRedpandaMetricsState()

			// Create test metrics with known values
			metrics := redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesIn:  100,
					BytesOut: 50,
				},
			}

			// Initial update (tick 1)
			state.UpdateFromMetrics(metrics, 1)

			// Verify initial values - first update should set bytes per tick to the initial count
			Expect(state.Input.BytesPerTick).To(Equal(float64(100)))
			Expect(state.Output.BytesPerTick).To(Equal(float64(50)))
			Expect(state.IsActive).To(BeTrue())

			// Update with increased values (tick 2)
			metrics.Throughput.BytesIn = 200  // +100 from last tick
			metrics.Throughput.BytesOut = 150 // +100 from last tick
			state.UpdateFromMetrics(metrics, 2)

			// Verify average calculation over window
			// (200-100)/(2-1) = 100 bytes per tick for input
			// (150-50)/(2-1) = 100 bytes per tick for output
			Expect(state.Input.BytesPerTick).To(Equal(float64(100)))
			Expect(state.Output.BytesPerTick).To(Equal(float64(100)))
			Expect(state.IsActive).To(BeTrue())

			// Update with smaller increase (tick 3)
			metrics.Throughput.BytesIn = 250  // +50 from last tick
			metrics.Throughput.BytesOut = 200 // +50 from last tick
			state.UpdateFromMetrics(metrics, 3)

			// Verify average calculation over window
			// (250-100)/(3-1) = 75 bytes per tick for input
			// (200-50)/(3-1) = 75 bytes per tick for output
			Expect(state.Input.BytesPerTick).To(Equal(float64(75)))
			Expect(state.Output.BytesPerTick).To(Equal(float64(75)))

			// Simulate a counter reset (tick 4)
			metrics.Throughput.BytesIn = 50  // Less than previous, simulating a reset
			metrics.Throughput.BytesOut = 25 // Less than previous, simulating a reset
			state.UpdateFromMetrics(metrics, 4)

			// Verify window is reset after counter reset
			Expect(state.Input.BytesPerTick).To(Equal(float64(50)))
			Expect(state.Output.BytesPerTick).To(Equal(float64(25)))

			// Update after reset (tick 5)
			metrics.Throughput.BytesIn = 150  // +100 from last tick
			metrics.Throughput.BytesOut = 125 // +100 from last tick
			state.UpdateFromMetrics(metrics, 5)

			// Verify new average calculation after reset
			// (150-50)/(5-4) = 100 bytes per tick for input
			// (125-25)/(5-4) = 100 bytes per tick for output
			Expect(state.Input.BytesPerTick).To(Equal(float64(100)))
			Expect(state.Output.BytesPerTick).To(Equal(float64(100)))

			// Test inactive state (tick 6)
			metrics.Throughput.BytesIn = 150  // No change, 0 throughput
			metrics.Throughput.BytesOut = 125 // No change, 0 throughput
			state.UpdateFromMetrics(metrics, 6)

			// Verify inactive detection
			// (150-50)/(6-4) = 50 bytes per tick for input
			// (125-25)/(6-4) = 50 bytes per tick for output
			Expect(state.Input.BytesPerTick).To(Equal(float64(50)))
			Expect(state.IsActive).To(BeTrue()) // IsActive is true as long as BytesPerTick > 0
		})

		It("should handle edge cases correctly", func() {
			state := redpanda_monitor.NewRedpandaMetricsState()
			metrics := redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesIn:  100,
					BytesOut: 50,
				},
			}

			// Test same tick values - should not cause division by zero
			state.UpdateFromMetrics(metrics, 1)
			state.UpdateFromMetrics(metrics, 1)                    // Same tick
			Expect(state.Input.BytesPerTick).To(Equal(float64(0))) // When tick values are the same, BytesPerTick is set to 0

			// Test window size limit
			// First, create enough entries to fill the window and then some
			for i := uint64(2); i < redpanda_monitor.ThroughputWindowSize+10; i++ {
				metrics.Throughput.BytesIn = 100 + int64(i*10)
				state.UpdateFromMetrics(metrics, i)
			}

			// Verify the window size is capped
			Expect(len(state.Input.Window)).To(BeNumerically("<=", redpanda_monitor.ThroughputWindowSize))

			// Test extreme values
			state = redpanda_monitor.NewRedpandaMetricsState() // Reset state

			// Test with max int64 value
			maxInt64 := int64(9223372036854775807)
			metrics.Throughput.BytesIn = maxInt64
			state.UpdateFromMetrics(metrics, 1)
			Expect(state.Input.BytesPerTick).To(Equal(float64(maxInt64)))

			// Test with tick wraparound (while unlikely, it's an edge case)
			metrics.Throughput.BytesIn = maxInt64 - 100
			state.UpdateFromMetrics(metrics, 0) // Lower tick value

			// This should be treated as a reset since the tick value is lower
			Expect(len(state.Input.Window)).To(Equal(1))
			Expect(state.Input.BytesPerTick).To(Equal(float64(maxInt64 - 100)))
		})

		It("should correctly track activity status (input only)", func() {
			state := redpanda_monitor.NewRedpandaMetricsState()

			// New state should be inactive
			Expect(state.IsActive).To(BeFalse())

			// Manually set BytesPerTick and verify IsActive follows it
			state.UpdateFromMetrics(redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesIn: 0,
				},
			}, 1)
			Expect(state.IsActive).To(BeFalse())

			state.UpdateFromMetrics(redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesIn: 100,
				},
			}, 2)
			Expect(state.IsActive).To(BeTrue())

			state.UpdateFromMetrics(redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesIn: 0,
				},
			}, 3)
			Expect(state.IsActive).To(BeFalse())

			// Verify that BytesPerTick is calculated from metrics
			metrics := redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesIn: 200,
				},
			}
			state.UpdateFromMetrics(metrics, 4)
			// First update after reset (or beginning) sets BytesPerTick to count value
			Expect(state.Input.BytesPerTick).To(Equal(float64(200)))
			Expect(state.IsActive).To(BeTrue())
		})
		It("should correctly track activity status (output only)", func() {
			state := redpanda_monitor.NewRedpandaMetricsState()

			// New state should be inactive
			Expect(state.IsActive).To(BeFalse())

			// Manually set BytesPerTick and verify IsActive follows it
			state.UpdateFromMetrics(redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesOut: 0,
				},
			}, 1)
			Expect(state.IsActive).To(BeFalse())

			state.UpdateFromMetrics(redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesOut: 100,
				},
			}, 2)
			Expect(state.IsActive).To(BeTrue())

			state.UpdateFromMetrics(redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesOut: 0,
				},
			}, 3)
			Expect(state.IsActive).To(BeFalse())

			// Verify that BytesPerTick is calculated from metrics
			metrics := redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesOut: 200,
				},
			}
			state.UpdateFromMetrics(metrics, 4)
			// First update after reset (or beginning) sets BytesPerTick to count value
			Expect(state.Output.BytesPerTick).To(Equal(float64(200)))
			Expect(state.IsActive).To(BeTrue())
		})
		It("should correctly track activity status (input and output)", func() {
			state := redpanda_monitor.NewRedpandaMetricsState()

			// New state should be inactive
			Expect(state.IsActive).To(BeFalse())

			state.UpdateFromMetrics(redpanda_monitor.Metrics{
				Throughput: redpanda_monitor.ThroughputMetrics{
					BytesIn:  100,
					BytesOut: 100,
				},
			}, 1)
			Expect(state.IsActive).To(BeTrue())
		})
	})

	Describe("parseRedpandaIntegerlikeValue and parseValue", func() {
		Context("parseValue function", func() {
			It("should parse uint64 values correctly", func() {
				input := uint64(42)
				result, err := redpanda_monitor.ParseValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(uint64(42)))
			})

			It("should parse positive float64 values correctly", func() {
				input := float64(42.5)
				result, err := redpanda_monitor.ParseValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(uint64(42)))
			})

			It("should return error for negative float64 values", func() {
				input := float64(-42.5)
				_, err := redpanda_monitor.ParseValue(input)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("value is negative"))
			})

			It("should parse positive int values correctly", func() {
				input := int(42)
				result, err := redpanda_monitor.ParseValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(uint64(42)))
			})

			It("should return error for negative int values", func() {
				input := int(-42)
				_, err := redpanda_monitor.ParseValue(input)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("value is negative"))
			})

			It("should parse positive int64 values correctly", func() {
				input := int64(42)
				result, err := redpanda_monitor.ParseValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(uint64(42)))
			})

			It("should return error for negative int64 values", func() {
				input := int64(-42)
				_, err := redpanda_monitor.ParseValue(input)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("value is negative"))
			})

			It("should parse string uint values correctly", func() {
				input := "42"
				result, err := redpanda_monitor.ParseValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(uint64(42)))
			})

			It("should parse string float values correctly", func() {
				input := "42.5"
				result, err := redpanda_monitor.ParseValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(uint64(42)))
			})

			It("should return error for unparseable string values", func() {
				input := "not-a-number"
				_, err := redpanda_monitor.ParseValue(input)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse string value"))
			})

			It("should return error for negative string values", func() {
				input := "-42"
				_, err := redpanda_monitor.ParseValue(input)
				Expect(err).To(HaveOccurred())
				// First it will fail to parse as uint, then try as float and detect negative
				Expect(err.Error()).To(ContainSubstring("value is negative"))
			})

			It("should return error for nil values", func() {
				var input interface{} = nil
				_, err := redpanda_monitor.ParseValue(input)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("value is nil"))
			})

			It("should return error for unsupported types", func() {
				input := struct{ name string }{"test"}
				_, err := redpanda_monitor.ParseValue(input)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported value type"))
			})

			It("should handle very large numbers", func() {
				input := "18446744073709551615" // Max uint64 value
				result, err := redpanda_monitor.ParseValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(uint64(18446744073709551615)))
			})
		})

		Context("parseRedpandaIntegerlikeValue function", func() {
			It("should parse regular values correctly", func() {
				input := int64(42)
				result, err := redpanda_monitor.ParseRedpandaIntegerlikeValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(int64(42)))
			})

			It("should convert values larger than MaxInt64 to 0", func() {
				input := uint64(math.MaxInt64 + 1)
				result, err := redpanda_monitor.ParseRedpandaIntegerlikeValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(int64(0)))
			})

			It("should convert large string values to 0", func() {
				input := "18446744073709552000" // Larger than MaxInt64
				result, err := redpanda_monitor.ParseRedpandaIntegerlikeValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(int64(0)))
			})

			It("should convert nil values to 0", func() {
				var input interface{} = nil
				result, err := redpanda_monitor.ParseRedpandaIntegerlikeValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(int64(0)))
			})

			It("should convert negative values to 0", func() {
				input := int64(-1)
				result, err := redpanda_monitor.ParseRedpandaIntegerlikeValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(int64(0)))
			})

			It("should handle values at MaxInt64 boundary correctly", func() {
				input := int64(math.MaxInt64)
				result, err := redpanda_monitor.ParseRedpandaIntegerlikeValue(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(int64(math.MaxInt64)))
			})

			It("should return error for unsupported types", func() {
				input := struct{ name string }{"test"}
				_, err := redpanda_monitor.ParseRedpandaIntegerlikeValue(input)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported value type"))
			})
		})
	})

	// Test for concatContent function
	Describe("concatContent", func() {
		It("should concatenate log entries correctly", func() {
			logs := []process_shared.LogEntry{
				{Content: "Hello "},
				{Content: "World"},
				{Content: "!"},
			}
			result := redpanda_monitor.ConcatContent(logs)
			Expect(string(result)).To(Equal("Hello World!"))
		})

		It("should handle empty log entries", func() {
			logs := []process_shared.LogEntry{
				{Content: ""},
				{Content: ""},
				{Content: ""},
			}
			result := redpanda_monitor.ConcatContent(logs)
			Expect(string(result)).To(Equal(""))
		})

		It("should handle mixed content", func() {
			logs := []process_shared.LogEntry{
				{Content: "Line 1\n"},
				{Content: "Line 2\n"},
				{Content: "Line 3"},
			}
			result := redpanda_monitor.ConcatContent(logs)
			Expect(string(result)).To(Equal("Line 1\nLine 2\nLine 3"))
		})

		It("should handle binary data", func() {
			logs := []process_shared.LogEntry{
				{Content: string([]byte{0x01, 0x02})},
				{Content: string([]byte{0x03, 0x04})},
			}
			result := redpanda_monitor.ConcatContent(logs)
			Expect(result).To(Equal([]byte{0x01, 0x02, 0x03, 0x04}))
		})

		It("should handle a single log entry", func() {
			logs := []process_shared.LogEntry{
				{Content: "Single entry"},
			}
			result := redpanda_monitor.ConcatContent(logs)
			Expect(string(result)).To(Equal("Single entry"))
		})

		It("should handle empty logs slice", func() {
			var logs []process_shared.LogEntry
			result := redpanda_monitor.ConcatContent(logs)
			Expect(result).To(HaveLen(0))
		})
	})

	// Test for StripMarkers function
	Describe("StripMarkers", func() {
		It("should remove all marker strings from input", func() {
			// Test with all markers
			input := []byte(
				redpanda_monitor.BLOCK_START_MARKER +
					"data1" +
					redpanda_monitor.METRICS_END_MARKER +
					"data2" +
					redpanda_monitor.CLUSTERCONFIG_END_MARKER +
					"data3" +
					redpanda_monitor.BLOCK_END_MARKER)

			result := redpanda_monitor.StripMarkers(input)
			Expect(string(result)).To(Equal("data1data2data3"))
		})

		It("should handle input with no markers", func() {
			input := []byte("just some regular data")
			result := redpanda_monitor.StripMarkers(input)
			Expect(string(result)).To(Equal("just some regular data"))
		})

		It("should handle empty input", func() {
			input := []byte{}
			result := redpanda_monitor.StripMarkers(input)
			Expect(result).To(HaveLen(0))
		})

		It("should handle input with only markers", func() {
			input := []byte(
				redpanda_monitor.BLOCK_START_MARKER +
					redpanda_monitor.METRICS_END_MARKER +
					redpanda_monitor.CLUSTERCONFIG_END_MARKER +
					redpanda_monitor.BLOCK_END_MARKER)

			result := redpanda_monitor.StripMarkers(input)
			Expect(result).To(HaveLen(0))
		})

		It("should handle multiple occurrences of markers", func() {
			input := []byte(
				redpanda_monitor.BLOCK_START_MARKER +
					"data" +
					redpanda_monitor.BLOCK_START_MARKER +
					"more")

			result := redpanda_monitor.StripMarkers(input)
			Expect(string(result)).To(Equal("datamore"))
		})

		It("should handle markers with special regex characters", func() {
			// Add some regex special chars that would cause issues if this was using regex
			input := []byte("data[*+?^${}()|]" + redpanda_monitor.BLOCK_START_MARKER + "more")
			result := redpanda_monitor.StripMarkers(input)
			Expect(string(result)).To(Equal("data[*+?^${}()|]more"))
		})
	})
})
