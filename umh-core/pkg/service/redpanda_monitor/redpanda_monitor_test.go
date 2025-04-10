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
	"context"
	"fmt"
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
			Expect(err.Error()).To(ContainSubstring("no block end marker found"))
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
			Expect(err.Error()).To(ContainSubstring("no start marker found"))
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
			Expect(err.Error()).To(ContainSubstring("no metrics end marker found"))
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
			Expect(err.Error()).To(ContainSubstring("no config end marker found"))
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
			Expect(err.Error()).To(ContainSubstring("markers found in incorrect order"))
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

		const metrics = `# HELP redpanda_application_build Redpanda build information
# TYPE redpanda_application_build gauge
redpanda_application_build{redpanda_revision="b1dd9f54ab1fcd31110608ff214d0937bf30fdb1",redpanda_version="v24.3.8"} 1.000000
# HELP redpanda_application_fips_mode Identifies whether or not Redpanda is running in FIPS mode.
# TYPE redpanda_application_fips_mode gauge
redpanda_application_fips_mode{} 0.000000
# HELP redpanda_application_uptime_seconds_total Redpanda uptime in seconds
# TYPE redpanda_application_uptime_seconds_total gauge
redpanda_application_uptime_seconds_total{} 37.610635
# HELP redpanda_authorization_result Total number of authorization results by type
# TYPE redpanda_authorization_result counter
redpanda_authorization_result{type="empty"} 0
redpanda_authorization_result{type="deny"} 0
redpanda_authorization_result{type="allow"} 0
# HELP redpanda_cluster_brokers Number of configured brokers in the cluster
# TYPE redpanda_cluster_brokers gauge
redpanda_cluster_brokers{} 1.000000
# HELP redpanda_cluster_controller_log_limit_requests_available_rps Controller log rate limiting. Available rps for group
# TYPE redpanda_cluster_controller_log_limit_requests_available_rps gauge
redpanda_cluster_controller_log_limit_requests_available_rps{redpanda_cmd_group="node_management_operations"} 1000.000000
redpanda_cluster_controller_log_limit_requests_available_rps{redpanda_cmd_group="move_operations"} 1000.000000
redpanda_cluster_controller_log_limit_requests_available_rps{redpanda_cmd_group="configuration_operations"} 1000.000000
redpanda_cluster_controller_log_limit_requests_available_rps{redpanda_cmd_group="topic_operations"} 1000.000000
redpanda_cluster_controller_log_limit_requests_available_rps{redpanda_cmd_group="acls_and_users_operations"} 1000.000000
# HELP redpanda_cluster_controller_log_limit_requests_dropped Controller log rate limiting. Amount of requests that are dropped due to exceeding limit in group
# TYPE redpanda_cluster_controller_log_limit_requests_dropped counter
redpanda_cluster_controller_log_limit_requests_dropped{redpanda_cmd_group="node_management_operations"} 0
redpanda_cluster_controller_log_limit_requests_dropped{redpanda_cmd_group="move_operations"} 0
redpanda_cluster_controller_log_limit_requests_dropped{redpanda_cmd_group="configuration_operations"} 0
redpanda_cluster_controller_log_limit_requests_dropped{redpanda_cmd_group="topic_operations"} 0
redpanda_cluster_controller_log_limit_requests_dropped{redpanda_cmd_group="acls_and_users_operations"} 0
# HELP redpanda_cluster_features_enterprise_license_expiry_sec Number of seconds remaining until the Enterprise license expires
# TYPE redpanda_cluster_features_enterprise_license_expiry_sec gauge
redpanda_cluster_features_enterprise_license_expiry_sec{} 2591962.000000
# HELP redpanda_cluster_members_backend_queued_node_operations Number of queued node operations
# TYPE redpanda_cluster_members_backend_queued_node_operations gauge
redpanda_cluster_members_backend_queued_node_operations{shard="0"} 0.000000
# HELP redpanda_cluster_non_homogenous_fips_mode Number of nodes that have a non-homogenous FIPS mode value
# TYPE redpanda_cluster_non_homogenous_fips_mode gauge
redpanda_cluster_non_homogenous_fips_mode{} 0.000000
# HELP redpanda_cluster_partition_moving_from_node Amount of partitions that are moving from node
# TYPE redpanda_cluster_partition_moving_from_node gauge
redpanda_cluster_partition_moving_from_node{} 0.000000
# HELP redpanda_cluster_partition_moving_to_node Amount of partitions that are moving to node
# TYPE redpanda_cluster_partition_moving_to_node gauge
redpanda_cluster_partition_moving_to_node{} 0.000000
# HELP redpanda_cluster_partition_node_cancelling_movements Amount of cancelling partition movements for node
# TYPE redpanda_cluster_partition_node_cancelling_movements gauge
redpanda_cluster_partition_node_cancelling_movements{} 0.000000
# HELP redpanda_cluster_partition_num_with_broken_rack_constraint Number of partitions that don't satisfy the rack awareness constraint
# TYPE redpanda_cluster_partition_num_with_broken_rack_constraint gauge
redpanda_cluster_partition_num_with_broken_rack_constraint{} 0.000000
# HELP redpanda_cluster_partitions Number of partitions in the cluster (replicas not included)
# TYPE redpanda_cluster_partitions gauge
redpanda_cluster_partitions{} 0.000000
# HELP redpanda_cluster_topics Number of topics in the cluster
# TYPE redpanda_cluster_topics gauge
redpanda_cluster_topics{} 0.000000
# HELP redpanda_cluster_unavailable_partitions Number of partitions that lack quorum among replicants
# TYPE redpanda_cluster_unavailable_partitions gauge
redpanda_cluster_unavailable_partitions{} 0.000000
# HELP redpanda_cpu_busy_seconds_total Total CPU busy time in seconds
# TYPE redpanda_cpu_busy_seconds_total gauge
redpanda_cpu_busy_seconds_total{shard="0"} 0.742441
# HELP redpanda_debug_bundle_failed_generation_count Running count of failed debug bundle generations
# TYPE redpanda_debug_bundle_failed_generation_count counter
redpanda_debug_bundle_failed_generation_count{shard="0"} 0
# HELP redpanda_debug_bundle_last_failed_bundle_timestamp_seconds Timestamp of last failed debug bundle generation (seconds since epoch)
# TYPE redpanda_debug_bundle_last_failed_bundle_timestamp_seconds gauge
redpanda_debug_bundle_last_failed_bundle_timestamp_seconds{shard="0"} 0.000000
# HELP redpanda_debug_bundle_last_successful_bundle_timestamp_seconds Timestamp of last successful debug bundle generation (seconds since epoch)
# TYPE redpanda_debug_bundle_last_successful_bundle_timestamp_seconds gauge
redpanda_debug_bundle_last_successful_bundle_timestamp_seconds{shard="0"} 0.000000
# HELP redpanda_debug_bundle_successful_generation_count Running count of successful debug bundle generations
# TYPE redpanda_debug_bundle_successful_generation_count counter
redpanda_debug_bundle_successful_generation_count{shard="0"} 0
# HELP redpanda_io_queue_total_read_ops Total read operations passed in the queue
# TYPE redpanda_io_queue_total_read_ops counter
redpanda_io_queue_total_read_ops{class="",iogroup="0",mountpoint="none",shard="0"} 1
redpanda_io_queue_total_read_ops{class="compaction",iogroup="0",mountpoint="none",shard="0"} 0
# HELP redpanda_io_queue_total_write_ops Total write operations passed in the queue
# TYPE redpanda_io_queue_total_write_ops counter
redpanda_io_queue_total_write_ops{class="",iogroup="0",mountpoint="none",shard="0"} 30
redpanda_io_queue_total_write_ops{class="compaction",iogroup="0",mountpoint="none",shard="0"} 0
# HELP redpanda_kafka_handler_latency_seconds Latency histogram of kafka requests
# TYPE redpanda_kafka_handler_latency_seconds histogram
redpanda_kafka_handler_latency_seconds_sum{handler="produce"} 0
redpanda_kafka_handler_latency_seconds_count{handler="produce"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.000255"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.000511"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.001023"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.002047"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.004095"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.008191"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.016383"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.032767"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.065535"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.131071"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.262143"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="0.524287"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="1.048575"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="2.097151"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="4.194303"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="8.388607"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="16.777215"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="33.554431"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="produce",le="+Inf"} 0
redpanda_kafka_handler_latency_seconds_sum{handler="fetch"} 0
redpanda_kafka_handler_latency_seconds_count{handler="fetch"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.000255"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.000511"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.001023"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.002047"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.004095"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.008191"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.016383"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.032767"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.065535"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.131071"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.262143"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="0.524287"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="1.048575"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="2.097151"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="4.194303"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="8.388607"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="16.777215"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="33.554431"} 0
redpanda_kafka_handler_latency_seconds_bucket{handler="fetch",le="+Inf"} 0
# HELP redpanda_kafka_max_offset Latest readable offset of the partition (i.e. high watermark)
# TYPE redpanda_kafka_max_offset gauge
redpanda_kafka_max_offset{redpanda_namespace="redpanda",redpanda_partition="0",redpanda_topic="controller"} 7.000000
# HELP redpanda_kafka_quotas_client_quota_throttle_time Client quota throttling delay per rule and quota type (in seconds)
# TYPE redpanda_kafka_quotas_client_quota_throttle_time histogram
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_sum{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_count{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.001000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.003000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.007000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.015000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.031000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.063000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.127000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.255000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="0.511000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="1.023000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="2.047000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="4.095000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="8.191000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="16.383000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throttle_time_bucket{le="+Inf",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
# HELP redpanda_kafka_quotas_client_quota_throughput Client quota throughput per rule and quota type
# TYPE redpanda_kafka_quotas_client_quota_throughput histogram
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="not_applicable",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="not_applicable",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="not_applicable",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="cluster_client_prefix",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="kafka_client_id",redpanda_quota_type="produce_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="partition_mutation_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_sum{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_count{redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="7.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="15.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="31.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="63.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="127.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="255.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="511.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1023.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2047.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4095.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8191.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16383.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="32767.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="65535.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="131071.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="262143.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="524287.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="1048575.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="2097151.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="4194303.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="8388607.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="16777215.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="33554431.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="67108863.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="134217727.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="268435455.000000",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
redpanda_kafka_quotas_client_quota_throughput_bucket{le="+Inf",redpanda_quota_rule="cluster_client_default",redpanda_quota_type="fetch_quota"} 0
# HELP redpanda_kafka_records_fetched_total Total number of records fetched
# TYPE redpanda_kafka_records_fetched_total counter
redpanda_kafka_records_fetched_total{redpanda_namespace="redpanda",redpanda_topic="controller"} 0
# HELP redpanda_kafka_records_produced_total Total number of records produced
# TYPE redpanda_kafka_records_produced_total counter
redpanda_kafka_records_produced_total{redpanda_namespace="redpanda",redpanda_topic="controller"} 0
# HELP redpanda_kafka_request_bytes_total Total number of bytes produced per topic
# TYPE redpanda_kafka_request_bytes_total counter
redpanda_kafka_request_bytes_total{redpanda_namespace="redpanda",redpanda_request="produce",redpanda_topic="controller"} 0
redpanda_kafka_request_bytes_total{redpanda_namespace="redpanda",redpanda_request="follower_consume",redpanda_topic="controller"} 0
redpanda_kafka_request_bytes_total{redpanda_namespace="redpanda",redpanda_request="consume",redpanda_topic="controller"} 0
# HELP redpanda_kafka_request_latency_seconds Internal latency of kafka produce requests
# TYPE redpanda_kafka_request_latency_seconds histogram
redpanda_kafka_request_latency_seconds_sum{redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_count{redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.000255",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.000511",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.001023",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.002047",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.004095",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.008191",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.016383",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.032767",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.065535",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.131071",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.262143",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.524287",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="1.048575",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="2.097151",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="4.194303",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="8.388607",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="16.777215",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="33.554431",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_bucket{le="+Inf",redpanda_request="produce"} 0
redpanda_kafka_request_latency_seconds_sum{redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_count{redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.000255",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.000511",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.001023",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.002047",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.004095",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.008191",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.016383",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.032767",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.065535",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.131071",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.262143",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="0.524287",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="1.048575",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="2.097151",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="4.194303",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="8.388607",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="16.777215",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="33.554431",redpanda_request="consume"} 0
redpanda_kafka_request_latency_seconds_bucket{le="+Inf",redpanda_request="consume"} 0
# HELP redpanda_kafka_rpc_sasl_session_expiration_total Total number of SASL session expirations
# TYPE redpanda_kafka_rpc_sasl_session_expiration_total counter
redpanda_kafka_rpc_sasl_session_expiration_total{} 0
# HELP redpanda_kafka_rpc_sasl_session_reauth_attempts_total Total number of SASL reauthentication attempts
# TYPE redpanda_kafka_rpc_sasl_session_reauth_attempts_total counter
redpanda_kafka_rpc_sasl_session_reauth_attempts_total{} 0
# HELP redpanda_kafka_rpc_sasl_session_revoked_total Total number of SASL sessions revoked
# TYPE redpanda_kafka_rpc_sasl_session_revoked_total counter
redpanda_kafka_rpc_sasl_session_revoked_total{} 0
# HELP redpanda_kafka_under_replicated_replicas Number of under replicated replicas (i.e. replicas that are live, but not at the latest offest)
# TYPE redpanda_kafka_under_replicated_replicas gauge
redpanda_kafka_under_replicated_replicas{redpanda_namespace="redpanda",redpanda_partition="0",redpanda_topic="controller"} 0.000000
# HELP redpanda_memory_allocated_memory Allocated memory size in bytes
# TYPE redpanda_memory_allocated_memory gauge
redpanda_memory_allocated_memory{shard="0"} 545374208.000000
# HELP redpanda_memory_available_memory Total shard memory potentially available in bytes (free_memory plus reclaimable)
# TYPE redpanda_memory_available_memory gauge
redpanda_memory_available_memory{shard="0"} 1602142208.000000
# HELP redpanda_memory_available_memory_low_water_mark The low-water mark for available_memory from process start
# TYPE redpanda_memory_available_memory_low_water_mark gauge
redpanda_memory_available_memory_low_water_mark{shard="0"} 1602142208.000000
# HELP redpanda_memory_free_memory Free memory size in bytes
# TYPE redpanda_memory_free_memory gauge
redpanda_memory_free_memory{shard="0"} 1602109440.000000
# HELP redpanda_node_status_rpcs_received Number of node status RPCs received by this node
# TYPE redpanda_node_status_rpcs_received gauge
redpanda_node_status_rpcs_received{} 0.000000
# HELP redpanda_node_status_rpcs_sent Number of node status RPCs sent by this node
# TYPE redpanda_node_status_rpcs_sent gauge
redpanda_node_status_rpcs_sent{} 0.000000
# HELP redpanda_node_status_rpcs_timed_out Number of timed out node status RPCs from this node
# TYPE redpanda_node_status_rpcs_timed_out gauge
redpanda_node_status_rpcs_timed_out{} 0.000000
# HELP redpanda_raft_leadership_changes Number of won leader elections across all partitions in given topic
# TYPE redpanda_raft_leadership_changes counter
redpanda_raft_leadership_changes{redpanda_namespace="redpanda",redpanda_topic="controller"} 1
# HELP redpanda_raft_learners_gap_bytes Total numbers of bytes that must be delivered to learners
# TYPE redpanda_raft_learners_gap_bytes gauge
redpanda_raft_learners_gap_bytes{shard="0"} 0.000000
# HELP redpanda_raft_recovery_offsets_pending Sum of offsets that partitions on this node need to recover.
# TYPE redpanda_raft_recovery_offsets_pending gauge
redpanda_raft_recovery_offsets_pending{} 0.000000
# HELP redpanda_raft_recovery_partition_movement_available_bandwidth Bandwidth available for partition movement. bytes/sec
# TYPE redpanda_raft_recovery_partition_movement_available_bandwidth gauge
redpanda_raft_recovery_partition_movement_available_bandwidth{shard="0"} 104857600.000000
# HELP redpanda_raft_recovery_partition_movement_consumed_bandwidth Bandwidth consumed for partition movement. bytes/sec
# TYPE redpanda_raft_recovery_partition_movement_consumed_bandwidth gauge
redpanda_raft_recovery_partition_movement_consumed_bandwidth{shard="0"} 0.000000
# HELP redpanda_raft_recovery_partitions_active Number of partition replicas are currently recovering on this node.
# TYPE redpanda_raft_recovery_partitions_active gauge
redpanda_raft_recovery_partitions_active{} 0.000000
# HELP redpanda_raft_recovery_partitions_to_recover Number of partition replicas that have to recover for this node.
# TYPE redpanda_raft_recovery_partitions_to_recover gauge
redpanda_raft_recovery_partitions_to_recover{} 0.000000
# HELP redpanda_rest_proxy_inflight_requests_memory_usage_ratio Memory usage ratio of in-flight requests in the rest_proxy
# TYPE redpanda_rest_proxy_inflight_requests_memory_usage_ratio gauge
redpanda_rest_proxy_inflight_requests_memory_usage_ratio{shard="0"} 0.000000
# HELP redpanda_rest_proxy_inflight_requests_usage_ratio Usage ratio of in-flight requests in the rest_proxy
# TYPE redpanda_rest_proxy_inflight_requests_usage_ratio gauge
redpanda_rest_proxy_inflight_requests_usage_ratio{shard="0"} 0.000000
# HELP redpanda_rest_proxy_queued_requests_memory_blocked Number of requests queued in rest_proxy, due to memory limitations
# TYPE redpanda_rest_proxy_queued_requests_memory_blocked gauge
redpanda_rest_proxy_queued_requests_memory_blocked{shard="0"} 0.000000
# HELP redpanda_rest_proxy_request_errors_total Total number of rest_proxy server errors
# TYPE redpanda_rest_proxy_request_errors_total counter
redpanda_rest_proxy_request_errors_total{redpanda_status="5xx"} 0
redpanda_rest_proxy_request_errors_total{redpanda_status="4xx"} 0
redpanda_rest_proxy_request_errors_total{redpanda_status="3xx"} 0
# HELP redpanda_rest_proxy_request_latency_seconds Internal latency of request for rest_proxy
# TYPE redpanda_rest_proxy_request_latency_seconds histogram
redpanda_rest_proxy_request_latency_seconds_sum{} 0
redpanda_rest_proxy_request_latency_seconds_count{} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.000255"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.000511"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.001023"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.002047"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.004095"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.008191"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.016383"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.032767"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.065535"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.131071"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.262143"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="0.524287"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="1.048575"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="2.097151"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="4.194303"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="8.388607"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="16.777215"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="33.554431"} 0
redpanda_rest_proxy_request_latency_seconds_bucket{le="+Inf"} 0
# HELP redpanda_rpc_active_connections Count of currently active connections
# TYPE redpanda_rpc_active_connections gauge
redpanda_rpc_active_connections{redpanda_server="kafka"} 0.000000
redpanda_rpc_active_connections{redpanda_server="internal"} 0.000000
# HELP redpanda_rpc_received_bytes internal: Number of bytes received from the clients in valid requests
# TYPE redpanda_rpc_received_bytes counter
redpanda_rpc_received_bytes{redpanda_server="kafka"} 0
redpanda_rpc_received_bytes{redpanda_server="internal"} 0
# HELP redpanda_rpc_request_errors_total Number of rpc errors
# TYPE redpanda_rpc_request_errors_total counter
redpanda_rpc_request_errors_total{redpanda_server="kafka"} 0
redpanda_rpc_request_errors_total{redpanda_server="internal"} 0
# HELP redpanda_rpc_request_latency_seconds RPC latency
# TYPE redpanda_rpc_request_latency_seconds histogram
redpanda_rpc_request_latency_seconds_sum{redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_count{redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.000255",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.000511",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.001023",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.002047",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.004095",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.008191",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.016383",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.032767",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.065535",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.131071",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.262143",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.524287",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="1.048575",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="2.097151",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="4.194303",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="8.388607",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="16.777215",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="33.554431",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_bucket{le="+Inf",redpanda_server="kafka"} 0
redpanda_rpc_request_latency_seconds_sum{redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_count{redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.000255",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.000511",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.001023",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.002047",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.004095",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.008191",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.016383",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.032767",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.065535",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.131071",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.262143",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="0.524287",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="1.048575",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="2.097151",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="4.194303",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="8.388607",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="16.777215",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="33.554431",redpanda_server="internal"} 0
redpanda_rpc_request_latency_seconds_bucket{le="+Inf",redpanda_server="internal"} 0
# HELP redpanda_rpc_sent_bytes internal: Number of bytes sent to clients
# TYPE redpanda_rpc_sent_bytes counter
redpanda_rpc_sent_bytes{redpanda_server="kafka"} 0
redpanda_rpc_sent_bytes{redpanda_server="internal"} 0
# HELP redpanda_scheduler_runtime_seconds_total Accumulated runtime of task queue associated with this scheduling group
# TYPE redpanda_scheduler_runtime_seconds_total counter
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="admin",shard="0"} 0.152766
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="archival_upload",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="cache_background_reclaim",shard="0"} 0.000006
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="cluster",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="datalake",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="fetch",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="kafka",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="log_compaction",shard="0"} 0.228820
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="main",shard="0"} 0.293392
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="node_status",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="raft",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="raft_learner_recovery",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="self_test",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="transforms",shard="0"} 0.000000
# HELP redpanda_schema_registry_cache_schema_count The number of schemas in the store
# TYPE redpanda_schema_registry_cache_schema_count gauge
redpanda_schema_registry_cache_schema_count{} 0.000000
# HELP redpanda_schema_registry_cache_schema_memory_bytes The memory usage of schemas in the store
# TYPE redpanda_schema_registry_cache_schema_memory_bytes gauge
redpanda_schema_registry_cache_schema_memory_bytes{} 0.000000
# HELP redpanda_schema_registry_cache_subject_count The number of subjects in the store
# TYPE redpanda_schema_registry_cache_subject_count gauge
redpanda_schema_registry_cache_subject_count{deleted="true"} 0.000000
redpanda_schema_registry_cache_subject_count{deleted="false"} 0.000000
# HELP redpanda_schema_registry_inflight_requests_memory_usage_ratio Memory usage ratio of in-flight requests in the schema_registry
# TYPE redpanda_schema_registry_inflight_requests_memory_usage_ratio gauge
redpanda_schema_registry_inflight_requests_memory_usage_ratio{shard="0"} 0.000000
# HELP redpanda_schema_registry_inflight_requests_usage_ratio Usage ratio of in-flight requests in the schema_registry
# TYPE redpanda_schema_registry_inflight_requests_usage_ratio gauge
redpanda_schema_registry_inflight_requests_usage_ratio{shard="0"} 0.000000
# HELP redpanda_schema_registry_queued_requests_memory_blocked Number of requests queued in schema_registry, due to memory limitations
# TYPE redpanda_schema_registry_queued_requests_memory_blocked gauge
redpanda_schema_registry_queued_requests_memory_blocked{shard="0"} 0.000000
# HELP redpanda_schema_registry_request_errors_total Total number of schema_registry server errors
# TYPE redpanda_schema_registry_request_errors_total counter
redpanda_schema_registry_request_errors_total{redpanda_status="5xx"} 0
redpanda_schema_registry_request_errors_total{redpanda_status="4xx"} 0
redpanda_schema_registry_request_errors_total{redpanda_status="3xx"} 0
# HELP redpanda_schema_registry_request_latency_seconds Internal latency of request for schema_registry
# TYPE redpanda_schema_registry_request_latency_seconds histogram
redpanda_schema_registry_request_latency_seconds_sum{} 0
redpanda_schema_registry_request_latency_seconds_count{} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.000255"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.000511"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.001023"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.002047"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.004095"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.008191"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.016383"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.032767"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.065535"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.131071"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.262143"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="0.524287"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="1.048575"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="2.097151"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="4.194303"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="8.388607"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="16.777215"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="33.554431"} 0
redpanda_schema_registry_request_latency_seconds_bucket{le="+Inf"} 0
# HELP redpanda_security_audit_errors_total Running count of errors in creating/publishing audit event log entries
# TYPE redpanda_security_audit_errors_total counter
redpanda_security_audit_errors_total{} 0
# HELP redpanda_security_audit_last_event_timestamp_seconds Timestamp of last successful publish on the audit log (seconds since epoch)
# TYPE redpanda_security_audit_last_event_timestamp_seconds counter
redpanda_security_audit_last_event_timestamp_seconds{} 0
# HELP redpanda_storage_cache_disk_free_bytes Disk storage bytes free.
# TYPE redpanda_storage_cache_disk_free_bytes gauge
redpanda_storage_cache_disk_free_bytes{} 255598518272.000000
# HELP redpanda_storage_cache_disk_free_space_alert Status of low storage space alert. 0-OK, 1-Low Space 2-Degraded
# TYPE redpanda_storage_cache_disk_free_space_alert gauge
redpanda_storage_cache_disk_free_space_alert{} 0.000000
# HELP redpanda_storage_cache_disk_total_bytes Total size of attached storage, in bytes.
# TYPE redpanda_storage_cache_disk_total_bytes gauge
redpanda_storage_cache_disk_total_bytes{} 494384795648.000000
# HELP redpanda_storage_disk_free_bytes Disk storage bytes free.
# TYPE redpanda_storage_disk_free_bytes gauge
redpanda_storage_disk_free_bytes{} 255598518272.000000
# HELP redpanda_storage_disk_free_space_alert Status of low storage space alert. 0-OK, 1-Low Space 2-Degraded
# TYPE redpanda_storage_disk_free_space_alert gauge
redpanda_storage_disk_free_space_alert{} 0.000000
# HELP redpanda_storage_disk_total_bytes Total size of attached storage, in bytes.
# TYPE redpanda_storage_disk_total_bytes gauge
redpanda_storage_disk_total_bytes{} 494384795648.000000
`
		metricsReader := strings.NewReader(metrics)
		m, err := redpanda_monitor.ParseMetrics(metricsReader)
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
})
