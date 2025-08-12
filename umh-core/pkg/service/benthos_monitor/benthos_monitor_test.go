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

package benthos_monitor_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_orig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
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

const curlError = "curl: (7) Failed to connect to localhost port 9123 after 0 ms: Could not connect to server"

var _ = Describe("Benthos Monitor Service", func() {
	var (
		service      *benthos_monitor.BenthosMonitorService
		tick         uint64
		mockServices *serviceregistry.Registry
		ctx          context.Context
		cancel       context.CancelFunc
		serviceName  = "myservice"
	)

	BeforeEach(func() {
		mockServices = serviceregistry.NewMockRegistry()
		service = benthos_monitor.NewBenthosMonitorService(serviceName, benthos_monitor.WithS6Service(s6service.NewMockService()))
		tick = 0

		// Cleanup the data directory
		ctx, cancel = newTimeoutContext()
		err := mockServices.GetFileSystem().RemoveAll(ctx, getTmpDir())
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		cancel()
	})

	Describe("GenerateS6ConfigForBenthosMonitor", func() {
		It("should generate valid S6 configuration", func() {
			s6Config, err := service.GenerateS6ConfigForBenthosMonitor(serviceName, 8080)
			Expect(err).NotTo(HaveOccurred())

			// Verify the config contains the expected command and script
			Expect(s6Config.Command).To(HaveLen(2))
			Expect(s6Config.Command[0]).To(Equal("/bin/sh"))
			Expect(s6Config.ConfigFiles).To(HaveKey("run_benthos_monitor.sh"))

			// Verify the script content contains the necessary markers
			script := s6Config.ConfigFiles["run_benthos_monitor.sh"]
			Expect(script).To(ContainSubstring(benthos_monitor.BLOCK_START_MARKER))
			Expect(script).To(ContainSubstring(benthos_monitor.PING_END_MARKER))
			Expect(script).To(ContainSubstring(benthos_monitor.READY_END))
			Expect(script).To(ContainSubstring(benthos_monitor.VERSION_END))
			Expect(script).To(ContainSubstring(benthos_monitor.METRICS_END_MARKER))
			Expect(script).To(ContainSubstring(benthos_monitor.BLOCK_END_MARKER))
			Expect(script).To(ContainSubstring("curl -sSL"))
			Expect(script).To(ContainSubstring("sleep 1"))
		})
	})

	Describe("Service Status", func() {
		It("should return an error if service does not exist", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			_, err := service.Status(ctx, mockServices, tick)
			Expect(err).To(HaveOccurred())
		})

		It("should return service info when service exists", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Mock the S6 service to return some logs
			mockS6 := s6service.NewMockService()

			// Create a new service with the mock S6 service
			service = benthos_monitor.NewBenthosMonitorService(serviceName, benthos_monitor.WithS6Service(mockS6))

			// Add the service first
			err := service.AddBenthosMonitorToS6Manager(ctx, 8080)
			Expect(err).NotTo(HaveOccurred())

			// Make sure the service exists by reconciling
			err, _ = service.ReconcileManager(ctx, mockServices, 0)
			Expect(err).NotTo(HaveOccurred())

			// Explicitly mark the service as existing in the mock
			servicePath := fmt.Sprintf("%s/%s", constants.S6BaseDir, service.GetS6ServiceName())
			mockS6.ExistingServices[servicePath] = true

			// Set up mock logs that include our markers and some fake metrics data
			mockLogs := []s6_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_START_MARKER)},
				{Content: pingResponse + "\n"}, // Some hex-encoded gzipped data from the /ping endpoint
				{Content: fmt.Sprintf("%s\n", benthos_monitor.PING_END_MARKER)},
				{Content: readyResponse + "\n"}, // Some hex-encoded gzipped data from the /ready endpoint
				{Content: fmt.Sprintf("%s\n", benthos_monitor.READY_END)},
				{Content: versionResponse + "\n"}, // Some hex-encoded gzipped data from the /version endpoint
				{Content: fmt.Sprintf("%s\n", benthos_monitor.VERSION_END)},
				{Content: metricsResponse + "XXXXXXXXXX\n"}, // Some hex-encoded gzipped data from the /metrics endpoint, + some random data that should result in a failure
				{Content: fmt.Sprintf("%s\n", benthos_monitor.METRICS_END_MARKER)},
				{Content: "1745502164\n"}, // Some  timestamp
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_END_MARKER)},
			}
			// Set the mock logs result directly
			mockS6.GetLogsResult = mockLogs

			// Try getting status - we don't need to capture the result
			_, err = service.Status(ctx, mockServices, tick)
			Expect(err).To(HaveOccurred())
			// Check that this is a "failed to parse metrics" error
			Expect(err.Error()).To(ContainSubstring("failed to parse metrics"))

			// We expect an error due to the mock data not being real metrics data
			// but at least the service should report as existing
			Expect(service.ServiceExists(ctx, mockServices)).To(BeTrue())
		})
	})

	Describe("ParseBenthosLogs", func() {
		It("should return an error for empty logs", func() {
			logs := []s6_shared.LogEntry{}
			_, err := service.ParseBenthosLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no logs provided"))
		})

		It("should return an error if no block end marker is found", func() {
			logs := []s6_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.PING_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.READY_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.VERSION_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.METRICS_END_MARKER)},
				{Content: "timestamp data\n"},
				// but no block end marker
			}
			_, err := service.ParseBenthosLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotate"))
		})

		It("should return an error if no start marker is found", func() {
			logs := []s6_shared.LogEntry{
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.PING_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.READY_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.VERSION_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.METRICS_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseBenthosLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotate"))
		})

		It("should return an error if no ping end marker is found", func() {
			logs := []s6_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.READY_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.VERSION_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.METRICS_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseBenthosLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotate"))
		})

		It("should return an error if no ready end marker is found", func() {
			logs := []s6_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.PING_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.VERSION_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.METRICS_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseBenthosLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotate"))
		})

		It("should return an error if no version end marker is found", func() {
			logs := []s6_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.PING_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.READY_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.METRICS_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseBenthosLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotate"))
		})

		It("should return an error if no metrics end marker is found", func() {
			logs := []s6_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.PING_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.READY_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.VERSION_END)},
				{Content: "more data\n"},
				// {Content: fmt.Sprintf("%s\n", benthos_monitor.METRICS_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseBenthosLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotate"))
		})

		It("should return an error if markers are in incorrect order", func() {
			logs := []s6_shared.LogEntry{
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.METRICS_END_MARKER)}, // Wrong order
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.READY_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.VERSION_END)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.PING_END_MARKER)}, // wrong order
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", benthos_monitor.BLOCK_END_MARKER)},
			}
			_, err := service.ParseBenthosLogs(ctx, logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not parse benthos metrics/configuration: no sections found. This can happen when the benthos service is not running, or the logs where rotate"))
		})

	})

	Describe("Mock Service", func() {
		It("should implement all required interfaces", func() {
			mockService := benthos_monitor.NewMockBenthosMonitorService()

			// Test a few interfaces to make sure the mock works as expected
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Call AddRedpandaMonitorToS6Manager and check if called flag is set
			err := mockService.AddBenthosMonitorToS6Manager(ctx, 8080)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddBenthosToS6ManagerCalled).To(BeTrue())

			// Generate config and verify it has expected content
			config, err := mockService.GenerateS6ConfigForBenthosMonitor(serviceName, 8080)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.GenerateS6ConfigForBenthosMonitorCalled).To(BeTrue())
			Expect(config.ConfigFiles).To(HaveKey("run_benthos_monitor.sh"))

			// Test setting service state
			mockService.SetServiceState(benthos_monitor.ServiceStateFlags{
				IsRunning:       true,
				IsMetricsActive: true,
			})
			state := mockService.GetServiceState()
			Expect(state.IsRunning).To(BeTrue())
			Expect(state.IsMetricsActive).To(BeTrue())
		})
	})

	Describe("ProcessPingData", func() {
		It("should return an error if no ping data is provided", func() {
			_, err := service.ProcessPingData(nil)
			Expect(err).To(HaveOccurred())
		})

		It("should parse the static ping data", func() {
			isLive, err := service.ProcessPingData([]byte(pingResponse))
			Expect(err).NotTo(HaveOccurred())
			Expect(isLive).To(BeTrue())
		})

		It("should return an error if the ping data is a curl error", func() {
			_, err := service.ProcessPingData([]byte(curlError))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ProcessReadyData", func() {
		It("should return an error if no ready data is provided", func() {
			_, _, err := service.ProcessReadyData(nil)
			Expect(err).To(HaveOccurred())
		})

		It("should parse the static ping data", func() {
			isReady, readyResp, err := service.ProcessReadyData([]byte(readyResponse))
			Expect(err).NotTo(HaveOccurred())
			Expect(isReady).To(BeTrue())
			Expect(readyResp.Error).To(BeEmpty())
		})

		It("should return an error if the ping data is a curl error", func() {
			isReady, _, err := service.ProcessReadyData([]byte(curlError))
			Expect(err).To(HaveOccurred())
			Expect(isReady).To(BeFalse())
		})
	})

	Describe("ProcessVersionData", func() {
		It("should return an error if no version data is provided", func() {
			_, err := service.ProcessVersionData(nil)
			Expect(err).To(HaveOccurred())
		})

		It("should parse the static ping data", func() {
			versionResp, err := service.ProcessVersionData([]byte(versionResponse))
			Expect(err).NotTo(HaveOccurred())
			Expect(versionResp.Version).To(Equal(""))
			Expect(versionResp.Built).To(Equal("2025-02-24T12:50:06Z"))
		})

		It("should return an error if the ping data is a curl error", func() {
			_, err := service.ProcessVersionData([]byte(curlError))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ProcessMetricsData", func() {
		It("should return an error if no metrics data is provided", func() {
			_, err := service.ProcessMetricsData(nil, tick)
			Expect(err).To(HaveOccurred())
		})

		It("should parse the static ping data", func() {
			metrics, err := service.ProcessMetricsData([]byte(metricsResponse), tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(metrics.Metrics.Input.ConnectionUp).To(Equal(int64(1)))
			Expect(metrics.Metrics.Input.Received).To(Equal(int64(7)))
			Expect(metrics.Metrics.Output.ConnectionUp).To(Equal(int64(1)))
			Expect(metrics.Metrics.Output.Sent).To(Equal(int64(7)))
		})

		It("should return an error if the ping data is a curl error", func() {
			_, err := service.ProcessMetricsData([]byte(curlError), tick)
			Expect(err).To(HaveOccurred())
		})
	})

	It("should parse the test_metrics", func() {
		// 1. Load the test_metrics.txt file (from current dir)
		metricsData, err := os.ReadFile("test_metrics.txt")
		Expect(err).NotTo(HaveOccurred())

		// 2. Parse it line by line into s6_shared.LogEntry
		lines := strings.Split(string(metricsData), "\n")
		var logEntries []s6_shared.LogEntry

		for _, line := range lines {
			if len(line) > 0 {
				// Remove timestamps at the beginning of the line
				parts := strings.SplitN(line, "  ", 2)
				if len(parts) == 2 {
					// Use the content part (after the timestamp)
					logEntries = append(logEntries, s6_shared.LogEntry{Content: parts[1]})
				} else {
					// For lines without timestamps (like the marker lines)
					logEntries = append(logEntries, s6_shared.LogEntry{Content: line})
				}
			}
		}

		// 3. Parse it into metrics
		benthosMetricsConfig, err := service.ParseBenthosLogs(ctx, logEntries, tick)
		Expect(err).NotTo(HaveOccurred())
		Expect(benthosMetricsConfig).NotTo(BeNil())

		// Here we simply check that it took the last element in the log entries
		Expect(benthosMetricsConfig.LastUpdatedAt.Unix()).To(Equal(int64(1745502180)))
	})

	Describe("TailInt", func() {
		It("should parse a simple integer at the end of the line", func() {
			val, err := benthos_monitor.TailInt([]byte("foo 50000"))
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(int64(50000)))
		})

		It("should parse a float in scientific notation", func() {
			line := []byte(`input_received{label="",path="root.input"} 1.074682e+06`)
			val, err := benthos_monitor.TailInt(line)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(int64(1074682)))
		})
	})
})
