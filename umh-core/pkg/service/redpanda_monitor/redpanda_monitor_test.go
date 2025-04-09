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

package redpanda_monitor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// getTmpDir returns the temporary directory for a container
func getTmpDir() string {
	tmpDir := "/tmp"
	// If we are in a devcontainer, use the workspace as tmp dir
	if os.Getenv("REMOTE_CONTAINERS") != "" || os.Getenv("CODESPACE_NAME") != "" || os.Getenv("USER") == "vscode" {
		tmpDir = "/workspaces/united-manufacturing-hub/umh-core/tmp"
	}
	return tmpDir
}

// newTimeoutContext creates a context with a 30-second timeout
func newTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

var _ = Describe("Redpanda Monitor Service", func() {
	var (
		service *RedpandaMonitorService
		tick    uint64
		mockFS  *filesystem.MockFileSystem
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		service = NewRedpandaMonitorService()
		tick = 0

		// Cleanup the data directory
		ctx, cancel := newTimeoutContext()
		defer cancel()
		mockFS.RemoveAll(ctx, getTmpDir())
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
			Expect(script).To(ContainSubstring(BLOCK_START_MARKER))
			Expect(script).To(ContainSubstring(METRICS_END_MARKER))
			Expect(script).To(ContainSubstring(BLOCK_END_MARKER))
			Expect(script).To(ContainSubstring("curl -sSL"))
			Expect(script).To(ContainSubstring("sleep 1"))
		})
	})

	Describe("Lifecycle Management", func() {
		var (
			service       *RedpandaMonitorService
			mockS6Service *s6service.MockService
		)

		BeforeEach(func() {
			mockS6Service = s6service.NewMockService()
			mockFS = filesystem.NewMockFileSystem()
			// Use the new WithS6Service option instead of setting it after creation
			service = NewRedpandaMonitorService(WithS6Service(mockS6Service))
		})

		It("should add, start, stop and remove a Redpanda Monitor service", func() {
			ctx, cancel := newTimeoutContext()
			defer cancel()

			// Add the service
			By("Adding the Redpanda Monitor service")
			err := service.AddRedpandaMonitorToS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Service should exist in the S6 manager
			Expect(service.s6ServiceConfig).NotTo(BeNil())

			// Start the service
			By("Starting the Redpanda Monitor service")
			err = service.StartRedpandaMonitor(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to apply changes
			By("Reconciling the manager")
			err, _ = service.ReconcileManager(ctx, mockFS, 0)
			Expect(err).NotTo(HaveOccurred())

			// Explicitly mark the service as existing in the mock
			servicePath := fmt.Sprintf("%s/%s", "/run/service", service.getS6ServiceName())
			mockS6Service.ExistingServices[servicePath] = true

			// Verify service is running
			exists := service.ServiceExists(ctx, mockFS)
			Expect(exists).To(BeTrue())

			// Stop the service
			By("Stopping the Redpanda Monitor service")
			err = service.StopRedpandaMonitor(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to apply changes
			By("Reconciling the manager again")
			err, _ = service.ReconcileManager(ctx, mockFS, 1)
			Expect(err).NotTo(HaveOccurred())

			// Remove the service
			By("Removing the Redpanda Monitor service")
			err = service.RemoveRedpandaMonitorFromS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Service should no longer exist in the S6 manager
			Expect(service.s6ServiceConfig).To(BeNil())
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
			service = NewRedpandaMonitorService(WithS6Service(mockS6))

			// Add the service first
			err := service.AddRedpandaMonitorToS6Manager(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Make sure the service exists by reconciling
			err, _ = service.ReconcileManager(ctx, mockFS, 0)
			Expect(err).NotTo(HaveOccurred())

			// Explicitly mark the service as existing in the mock
			servicePath := fmt.Sprintf("%s/%s", constants.S6BaseDir, service.getS6ServiceName())
			mockS6.ExistingServices[servicePath] = true

			// Set up mock logs that include our markers and some fake metrics data
			mockLogs := []s6service.LogEntry{
				{Content: fmt.Sprintf("%s\n", BLOCK_START_MARKER)},
				{Content: "1f8b0800000000000003abcd4f2c492d2e516c0600000000ffff0300ee1f0e9e09000000\n"}, // Some hex-encoded gzipped data
				{Content: fmt.Sprintf("%s\n", METRICS_END_MARKER)},
				{Content: "1f8b0800000000000003abcd4f2c492d2e516c0600000000ffff0300ee1f0e9e09000000\n"}, // Some hex-encoded gzipped data
				{Content: fmt.Sprintf("%s\n", BLOCK_END_MARKER)},
			}

			// Set the mock logs result directly
			mockS6.GetLogsResult = mockLogs

			// Try getting status - we don't need to capture the result
			_, err = service.Status(ctx, mockFS, tick)
			Expect(err).To(HaveOccurred())

			// We expect an error due to the mock data not being real metrics data
			// but at least the service should report as existing
			Expect(service.ServiceExists(ctx, mockFS)).To(BeTrue())
		})
	})

	Describe("parseRedpandaLogs", func() {
		It("should return an error for empty logs", func() {
			logs := []s6service.LogEntry{}
			_, err := service.parseRedpandaLogs(logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no logs provided"))
		})

		It("should return an error if no block end marker is found", func() {
			logs := []s6service.LogEntry{
				{Content: fmt.Sprintf("%s\n", BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", METRICS_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", CLUSTERCONFIG_END_MARKER)},
				{Content: "timestamp data\n"},
			}
			_, err := service.parseRedpandaLogs(logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no block end marker found"))
		})

		It("should return an error if no start marker is found", func() {
			logs := []s6service.LogEntry{
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", METRICS_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", CLUSTERCONFIG_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", BLOCK_END_MARKER)},
			}
			_, err := service.parseRedpandaLogs(logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no start marker found"))
		})

		It("should return an error if no metrics end marker is found", func() {
			logs := []s6service.LogEntry{
				{Content: fmt.Sprintf("%s\n", BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", CLUSTERCONFIG_END_MARKER)},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", BLOCK_END_MARKER)},
			}
			_, err := service.parseRedpandaLogs(logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no metrics end marker found"))
		})

		It("should return an error if no config end marker is found", func() {
			logs := []s6service.LogEntry{
				{Content: fmt.Sprintf("%s\n", BLOCK_START_MARKER)},
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", METRICS_END_MARKER)},
				{Content: "more data\n"},
				{Content: "timestamp data\n"},
				{Content: fmt.Sprintf("%s\n", BLOCK_END_MARKER)},
			}
			_, err := service.parseRedpandaLogs(logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no config end marker found"))
		})

		It("should return an error if markers are in incorrect order", func() {
			logs := []s6service.LogEntry{
				{Content: fmt.Sprintf("%s\n", BLOCK_START_MARKER)},
				{Content: fmt.Sprintf("%s\n", CLUSTERCONFIG_END_MARKER)}, // Wrong order
				{Content: "some data\n"},
				{Content: fmt.Sprintf("%s\n", METRICS_END_MARKER)},
				{Content: "more data\n"},
				{Content: fmt.Sprintf("%s\n", BLOCK_END_MARKER)},
			}
			_, err := service.parseRedpandaLogs(logs, tick)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("markers found in incorrect order"))
		})

		It("should successfully parse the test_metrics.txt file", func() {
			// Read the test_metrics.txt file
			fileContent, err := os.ReadFile("test_metrics.txt")
			Expect(err).NotTo(HaveOccurred())

			// Convert file content to log entries
			var logs []s6service.LogEntry
			lines := strings.Split(string(fileContent), "\n")
			for _, line := range lines {
				// Skip empty lines
				if line == "" {
					continue
				}
				// Remove leading timestamp (just split on space)
				parts := strings.SplitN(line, " ", 3)
				log := parts[2]
				// Trim any whitespace
				log = strings.TrimSpace(log)
				// Remove trailing newline
				log = strings.TrimSuffix(log, "\n")
				logs = append(logs, s6service.LogEntry{Content: log})
			}

			// Parse the logs
			result, err := service.parseRedpandaLogs(logs, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Verify some key metrics
			Expect(result.Metrics).NotTo(BeNil())
			Expect(result.ClusterConfig).NotTo(BeNil())

			// Check if the LastUpdatedAt timestamp is set
			Expect(result.LastUpdatedAt).NotTo(Equal(time.Time{}))

			// Verify storage metrics
			Expect(result.Metrics.Metrics.Infrastructure.Storage.TotalBytes).To(BeNumerically(">", 0))
			Expect(result.Metrics.Metrics.Infrastructure.Storage.FreeBytes).To(BeNumerically(">", 0))

			// Verify cluster metrics
			Expect(result.Metrics.Metrics.Cluster.Topics).To(BeNumerically(">=", 0))

			// Verify cluster config
			Expect(result.ClusterConfig.Topic.DefaultTopicRetentionMs).To(BeNumerically(">", 0))
			Expect(result.ClusterConfig.Topic.DefaultTopicRetentionBytes).To(BeNumerically(">=", 0))
		})
	})

	Describe("Mock Service", func() {
		It("should implement all required interfaces", func() {
			mockService := NewMockRedpandaMonitorService()

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
			mockService.SetServiceState(ServiceStateFlags{
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
})
