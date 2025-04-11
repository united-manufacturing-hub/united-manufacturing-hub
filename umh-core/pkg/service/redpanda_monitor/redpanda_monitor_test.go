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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
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

// Helper function to set up mock logs for service status checks
func setupMockS6Logs(mockS6Service *s6service.MockService) {

	mockLogs := []s6service.LogEntry{
		{Content: redpanda_monitor.BLOCK_START_MARKER},
	}

	// Add metrics line by line
	for _, line := range strings.Split(metrics, "\n") {
		mockLogs = append(mockLogs, s6service.LogEntry{Content: line})
	}

	// Add metrics end marker
	mockLogs = append(mockLogs, s6service.LogEntry{Content: redpanda_monitor.METRICS_END_MARKER})

	// Add config line by line
	for _, line := range strings.Split(config, "\n") {
		mockLogs = append(mockLogs, s6service.LogEntry{Content: line})
	}

	// Add config end marker
	mockLogs = append(mockLogs, s6service.LogEntry{Content: redpanda_monitor.CLUSTERCONFIG_END_MARKER})

	// Add timestamp line (unix nanoseconds since epoch)
	mockLogs = append(mockLogs, s6service.LogEntry{Content: fmt.Sprintf("%d", time.Now().UnixNano())})

	// Add block end marker
	mockLogs = append(mockLogs, s6service.LogEntry{Content: redpanda_monitor.BLOCK_END_MARKER})

	mockS6Service.GetLogsResult = mockLogs
}

// Helper function to check service state
func checkServiceState(ctx context.Context, service *redpanda_monitor.RedpandaMonitorService, mockFS *filesystem.MockFileSystem, expectedState string) (redpanda_monitor.ServiceInfo, error) {
	serviceInfo, err := service.Status(ctx, mockFS, 0)
	if err != nil {
		return serviceInfo, err
	}

	return serviceInfo, nil
}

var _ = Describe("Redpanda Monitor Service", func() {
	var (
		service *redpanda_monitor.RedpandaMonitorService
		tick    uint64
		mockFS  *filesystem.MockFileSystem
		ctx     context.Context
		cancel  context.CancelFunc
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		service = redpanda_monitor.NewRedpandaMonitorService()
		tick = 0

		// Cleanup the data directory
		ctx, cancel = newTimeoutContext()
		mockFS.RemoveAll(ctx, getTmpDir())
	})
	AfterEach(func() {
		cancel()
	})

	Describe("GenerateS6ConfigForRedpandaMonitor", func() {
		It("should generate valid S6 configuration", func() {
			s6Config, err := service.GenerateS6ConfigForRedpandaMonitor()
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

			_, err := service.Status(ctx, mockFS, tick)
			Expect(err).To(HaveOccurred())
		})

		It("should return service info when service exists", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Mock the S6 service to return some logs
			mockS6 := s6service.NewMockService()

			// Create a new service with the mock S6 service
			service = redpanda_monitor.NewRedpandaMonitorService(redpanda_monitor.WithS6Service(mockS6))

			// Add the service first
			err := service.AddRedpandaMonitorToS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Make sure the service exists by reconciling
			err, _ = service.ReconcileManager(ctx, mockFS, 0)
			Expect(err).NotTo(HaveOccurred())

			// Explicitly mark the service as existing in the mock
			servicePath := fmt.Sprintf("%s/%s", constants.S6BaseDir, service.GetS6ServiceName())
			mockS6.ExistingServices[servicePath] = true

			// Set up mock logs that include our markers and some fake metrics data
			mockLogs := []s6service.LogEntry{
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_START_MARKER)},
				{Content: "1f8b0800000000000003abcd4f2c492d2e516c0600000000ffff0300ee1f0e9e09000000\n"}, // Some hex-encoded gzipped data
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.METRICS_END_MARKER)},
				{Content: "1f8b0800000000000003abcd4f2c492d2e516c0600000000ffff0300ee1f0e9e09000000\n"}, // Some hex-encoded gzipped data
				{Content: fmt.Sprintf("%s\n", redpanda_monitor.BLOCK_END_MARKER)},
			}

			// Set the mock logs result directly
			mockS6.GetLogsResult = mockLogs

			// Try getting status - we don't need to capture the result
			_, err = service.Status(ctx, mockFS, tick)
			Expect(err).To(HaveOccurred())
			// Check that this is a "failed to parse metrics" error
			Expect(err.Error()).To(ContainSubstring("failed to parse metrics"))

			// We expect an error due to the mock data not being real metrics data
			// but at least the service should report as existing
			Expect(service.ServiceExists(ctx, mockFS)).To(BeTrue())
		})
	})

	Describe("ParseRedpandaLogs", func() {
		It("should return an error for empty logs", func() {
			logs := []s6service.LogEntry{}
			_, err := service.ParseRedpandaLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no logs provided"))
		})

		It("should return an error if no block end marker is found", func() {
			logs := []s6service.LogEntry{
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
			logs := []s6service.LogEntry{
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
			logs := []s6service.LogEntry{
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
			logs := []s6service.LogEntry{
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
			logs := []s6service.LogEntry{
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
			config, err := mockService.GenerateS6ConfigForRedpandaMonitor()
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.GenerateS6ConfigForRedpandaMonitorCalled).To(BeTrue())
			Expect(config.ConfigFiles).To(HaveKey("run_redpanda_monitor.sh"))

			// Test setting service state
			mockService.SetServiceState(redpanda_monitor.ServiceStateFlags{
				IsRunning:       true,
				IsConfigLoaded:  true,
				IsMetricsActive: true,
			})
			state := mockService.GetServiceState()
			Expect(state.IsRunning).To(BeTrue())
			Expect(state.IsConfigLoaded).To(BeTrue())
			Expect(state.IsMetricsActive).To(BeTrue())
		})
	})

	Describe("Can parse the metrics", func() {
		It("should return an error if no metrics are provided", func() {
			logs := []s6service.LogEntry{}
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
		var logEntries []s6service.LogEntry

		for _, line := range lines {
			if len(line) > 0 {
				// Remove timestamps at the beginning of the line
				parts := strings.SplitN(line, "  ", 2)
				if len(parts) == 2 {
					// Use the content part (after the timestamp)
					logEntries = append(logEntries, s6service.LogEntry{Content: parts[1]})
				} else {
					// For lines without timestamps (like the marker lines)
					logEntries = append(logEntries, s6service.LogEntry{Content: line})
				}
			}
		}

		// 3. Parse it into metrics
		redpandaMetricsConfig, err := service.ParseRedpandaLogs(ctx, logEntries, tick)
		Expect(err).NotTo(HaveOccurred())
		Expect(redpandaMetricsConfig).NotTo(BeNil())

		// 4. Verify the metrics are parsed correctly
		metricsResult := redpandaMetricsConfig.Metrics.Metrics

		// Verify storage metrics
		// Note: this value is different from the other test, as the metrics are different
		Expect(metricsResult.Infrastructure.Storage.FreeBytes).To(Equal(int64(258896789504)))
		Expect(metricsResult.Infrastructure.Storage.TotalBytes).To(Equal(int64(494384795648)))
		Expect(metricsResult.Infrastructure.Storage.FreeSpaceAlert).To(BeFalse())

		// Verify cluster metrics
		Expect(metricsResult.Cluster.Topics).To(Equal(int64(0)))
		Expect(metricsResult.Cluster.UnavailableTopics).To(Equal(int64(0)))

		// Verify throughput metrics
		Expect(metricsResult.Throughput.BytesIn).To(Equal(int64(0)))
		Expect(metricsResult.Throughput.BytesOut).To(Equal(int64(0)))

		// Verify topic metrics
		Expect(metricsResult.Topic.TopicPartitionMap).To(HaveLen(0))
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
})
