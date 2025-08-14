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

// ────────────────────────────────────────────────────────────────────────────────
// ENG-2893  Faster metric parsing for Redpanda/Redpanda
// Context: Slack thread 2025-04-30 (this file ↔ ENG-2893, ENG-2884)
// ────────────────────────────────────────────────────────────────────────────────
//
// Benchmarks (Go 1.22, Ryzen 9-5900X, pkg/service/redpanda_monitor):
//	BenchmarkGzipDecode-24                             98608             11983 ns/op           49169 B/op         11 allocs/op
// 	BenchmarkHexDecode-24                            3110442             388.6 ns/op             416 B/op          1 allocs/op
// 	BenchmarkMetricsParsing-24                         27571             42960 ns/op           28392 B/op        781 allocs/op
// 	BenchmarkCompleteProcessing-24                     20348             59672 ns/op           75673 B/op        792 allocs/op
// 	BenchmarkParseRedpandaLogsWithPercentiles-24        10000            111753 ns/op            112572 p50ns            345167 p95ns            484639 p99ns          217350 B/op        911 allocs/op
//
// Findings
// --------
// • ParseMetricsFromBytes ≈ 73 % of end-to-end time (42 µs of 58 µs).
// • Total cost to handle one Redpanda metrics payload (gzip→hex→parse) ≈ 0.05 ms.
// • Hex decode is negligible; gzip decompression is minor; Prometheus text parse
//   is the clear hotspot.
//
// Options under evaluation
// ------------------------
// 1. Avoid parsing every tick for every Redpanda instance (parse only when needed).
// 2. Replace Prometheus text parser with minimal hand-rolled parser.
//
// Related tickets
//  • ENG-2893 – performance work.
//  • ENG-2884 – parsing errors / observed-state update.
//
// Sample metrics used for the benchmark are below (taken from test_metrics.txt).
// ────────────────────────────────────────────────────────────────────────────────

package redpanda_monitor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"io"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
)

var sampleMetrics = `# HELP redpanda_application_build Redpanda build information
# TYPE redpanda_application_build gauge
redpanda_application_build{redpanda_revision="b1dd9f54ab1fcd31110608ff214d0937bf30fdb1",redpanda_version="v24.3.8"} 1.000000
# HELP redpanda_application_fips_mode Identifies whether or not Redpanda is running in FIPS mode.
# TYPE redpanda_application_fips_mode gauge
redpanda_application_fips_mode{} 0.000000
# HELP redpanda_application_uptime_seconds_total Redpanda uptime in seconds
# TYPE redpanda_application_uptime_seconds_total gauge
redpanda_application_uptime_seconds_total{} 27.203389
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
redpanda_cluster_features_enterprise_license_expiry_sec{} 2591973.000000
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
redpanda_cpu_busy_seconds_total{shard="0"} 1.939297
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
# HELP redpanda_io_queue_total_write_ops Total write operations passed in the queue
# TYPE redpanda_io_queue_total_write_ops counter
redpanda_io_queue_total_write_ops{class="",iogroup="0",mountpoint="none",shard="0"} 31
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
redpanda_memory_allocated_memory{shard="0"} 545533952.000000
# HELP redpanda_memory_available_memory Total shard memory potentially available in bytes (free_memory plus reclaimable)
# TYPE redpanda_memory_available_memory gauge
redpanda_memory_available_memory{shard="0"} 1601982464.000000
# HELP redpanda_memory_available_memory_low_water_mark The low-water mark for available_memory from process start
# TYPE redpanda_memory_available_memory_low_water_mark gauge
redpanda_memory_available_memory_low_water_mark{shard="0"} 1601982464.000000
# HELP redpanda_memory_free_memory Free memory size in bytes
# TYPE redpanda_memory_free_memory gauge
redpanda_memory_free_memory{shard="0"} 1601949696.000000
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
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="admin",shard="0"} 0.174560
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="archival_upload",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="cache_background_reclaim",shard="0"} 0.000644
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="cluster",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="datalake",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="fetch",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="kafka",shard="0"} 0.000000
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="log_compaction",shard="0"} 0.216092
redpanda_scheduler_runtime_seconds_total{redpanda_scheduling_group="main",shard="0"} 1.467145
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
redpanda_storage_cache_disk_free_bytes{} 258907324416.000000
# HELP redpanda_storage_cache_disk_free_space_alert Status of low storage space alert. 0-OK, 1-Low Space 2-Degraded
# TYPE redpanda_storage_cache_disk_free_space_alert gauge
redpanda_storage_cache_disk_free_space_alert{} 0.000000
# HELP redpanda_storage_cache_disk_total_bytes Total size of attached storage, in bytes.
# TYPE redpanda_storage_cache_disk_total_bytes gauge
redpanda_storage_cache_disk_total_bytes{} 494384795648.000000
# HELP redpanda_storage_disk_free_bytes Disk storage bytes free.
# TYPE redpanda_storage_disk_free_bytes gauge
redpanda_storage_disk_free_bytes{} 258907324416.000000
# HELP redpanda_storage_disk_free_space_alert Status of low storage space alert. 0-OK, 1-Low Space 2-Degraded
# TYPE redpanda_storage_disk_free_space_alert gauge
redpanda_storage_disk_free_space_alert{} 0.000000
# HELP redpanda_storage_disk_total_bytes Total size of attached storage, in bytes.
# TYPE redpanda_storage_disk_total_bytes gauge
redpanda_storage_disk_total_bytes{} 494384795648.000000
`

var clusterConfig = `{"iceberg_rest_catalog_crl_file":null,"iceberg_rest_catalog_request_timeout_ms":10000,"iceberg_rest_catalog_token":null,"iceberg_rest_catalog_client_secret":null,"iceberg_rest_catalog_client_id":null,"iceberg_rest_catalog_endpoint":null,"iceberg_catalog_type":"object_storage","datalake_coordinator_snapshot_max_delay_secs":900,"iceberg_catalog_commit_interval_ms":60000,"iceberg_delete":true,"iceberg_enabled":false,"tls_min_version":"v1.2","unsafe_enable_consumer_offsets_delete_retention":false,"virtual_cluster_min_producer_ids":18446744073709551615,"http_authentication":["BASIC"],"oidc_keys_refresh_interval":3600,"oidc_principal_mapping":"$.sub","oidc_discovery_url":"https://auth.prd.cloud.redpanda.com/.well-known/openid-configuration","debug_bundle_auto_removal_seconds":null,"rpk_path":"/usr/bin/rpk","debug_bundle_storage_dir":null,"cpu_profiler_sample_period_ms":100,"cpu_profiler_enabled":false,"kafka_memory_share_for_fetch":0.5,"max_in_flight_schema_registry_requests_per_shard":500,"schema_registry_protobuf_renderer_v2":false,"enable_schema_id_validation":"none","legacy_unsafe_log_warning_interval_sec":300,"legacy_permit_unsafe_log_operation":true,"node_isolation_heartbeat_timeout":3000,"kafka_throughput_controlled_api_keys":["produce","fetch"],"kafka_quota_balancer_min_shard_throughput_ratio":0.01,"kafka_quota_balancer_window_ms":5000,"kafka_throughput_limit_node_out_bps":null,"kafka_throughput_limit_node_in_bps":null,"controller_log_accummulation_rps_capacity_move_operations":null,"rps_limit_move_operations":1000,"controller_log_accummulation_rps_capacity_node_management_operations":null,"controller_log_accummulation_rps_capacity_acls_and_users_operations":null,"enable_controller_log_rate_limiting":false,"cloud_storage_upload_ctrl_d_coeff":0.0,"metrics_reporter_report_interval":86400000,"enable_transactions":true,"enable_metrics_reporter":true,"storage_strict_data_init":false,"cloud_storage_disable_tls":false,"health_monitor_max_metadata_age":10000,"health_manager_tick_interval":180000,"internal_topic_replication_factor":3,"cloud_storage_crl_file":null,"kafka_qdc_latency_alpha":0.002,"core_balancing_continuous":false,"fetch_max_bytes":57671680,"core_balancing_on_core_count_change":true,"alive_timeout_ms":5000,"default_leaders_preference":"none","leader_balancer_mute_timeout":300000,"log_cleanup_policy":"delete","cloud_storage_upload_ctrl_min_shares":100,"partition_autobalancing_topic_aware":true,"cloud_storage_cluster_metadata_num_consumer_groups_per_upload":1000,"partition_autobalancing_node_availability_timeout_sec":900,"lz4_decompress_reusable_buffers_disabled":false,"auto_create_topics_enabled":true,"partition_autobalancing_min_size_threshold":null,"zstd_decompress_workspace_bytes":8388608,"kafka_qdc_depth_update_ms":7000,"cloud_storage_bucket":null,"kafka_qdc_max_depth":100,"kafka_qdc_idle_depth":10,"topic_partitions_per_shard":1000,"kafka_qdc_enable":false,"cloud_storage_inventory_self_managed_report_config":false,"controller_log_accummulation_rps_capacity_configuration_operations":null,"cloud_storage_inventory_reports_prefix":"redpanda_scrubber_inventory","cloud_storage_cache_trim_threshold_percent_size":null,"rps_limit_topic_operations":1000,"cloud_storage_cache_num_buckets":0,"tls_enable_renegotiation":false,"cloud_storage_cache_size":0,"cloud_storage_chunk_eviction_strategy":"eager","cloud_storage_disable_chunk_reads":false,"kafka_schema_id_validation_cache_capacity":128,"cloud_storage_min_chunks_per_segment_threshold":5,"cloud_storage_cache_chunk_size":16777216,"join_retry_timeout_ms":5000,"cloud_storage_throughput_limit_percent":50,"metrics_reporter_tick_interval":60000,"kafka_max_bytes_per_fetch":67108864,"cloud_storage_disable_upload_consistency_checks":false,"cloud_storage_max_segment_readers_per_shard":null,"controller_snapshot_max_age_sec":60,"cloud_storage_cache_max_objects":100000,"cloud_storage_cache_trim_threshold_percent_objects":null,"cloud_storage_cache_size_percent":20.0,"initial_retention_local_target_ms_default":null,"space_management_enable_override":false,"retention_local_trim_overage_coeff":2.0,"sasl_kerberos_principal":"redpanda","retention_local_strict_override":true,"retention_local_target_ms_default":86400000,"cloud_storage_upload_ctrl_p_coeff":-2.0,"cloud_storage_azure_adls_port":null,"cloud_storage_azure_shared_key":null,"topic_fds_per_partition":5,"cloud_storage_cache_check_interval":5000,"log_compaction_use_sliding_window":true,"retention_local_trim_interval":30000,"cloud_storage_partial_scrub_interval_ms":3600000,"cloud_storage_azure_container":null,"raft_recovery_throttle_disable_dynamic_mode":false,"cloud_storage_hydration_timeout_ms":600000,"raft_replica_max_flush_delay_ms":100,"cloud_storage_segment_upload_timeout_ms":30000,"cloud_storage_disable_metadata_consistency_checks":true,"iceberg_rest_catalog_prefix":null,"cloud_storage_enable_segment_merging":true,"cloud_storage_topic_purge_grace_period_ms":30000,"raft_recovery_default_read_size":524288,"cloud_storage_materialized_manifest_ttl_ms":10000,"raft_enable_lw_heartbeat":true,"cloud_storage_manifest_cache_size":1048576,"cloud_storage_spillover_manifest_size":65536,"append_chunk_size":16384,"cloud_storage_credentials_host":null,"cloud_storage_spillover_manifest_max_segments":null,"cloud_storage_max_concurrent_hydrations_per_shard":null,"cloud_storage_backend":"unknown","initial_retention_local_target_bytes_default":null,"cloud_storage_max_throughput_per_shard":1073741824,"cloud_storage_segment_size_target":null,"cloud_storage_recovery_topic_validation_mode":"check_manifest_existence","cloud_storage_recovery_temporary_retention_bytes_default":1073741824,"disable_batch_cache":false,"enable_rack_awareness":false,"cloud_storage_enable_compacted_topic_reupload":true,"enable_cluster_metadata_upload_loop":true,"raft_io_timeout_ms":10000,"disable_cluster_recovery_loop_for_tests":false,"cloud_storage_disable_upload_loop_for_tests":false,"partition_autobalancing_concurrent_moves":50,"cloud_storage_scrubbing_interval_jitter_ms":600000,"cloud_storage_enable_scrubbing":false,"cloud_storage_background_jobs_quota":5000,"storage_min_free_bytes":5368709120,"rps_limit_configuration_operations":1000,"cloud_storage_idle_threshold_rps":10.0,"transaction_coordinator_cleanup_policy":"delete","cloud_storage_idle_timeout_ms":10000,"leader_balancer_transfer_limit_per_shard":512,"node_management_operation_timeout_ms":5000,"cloud_storage_housekeeping_interval_ms":300000,"cloud_storage_metadata_sync_timeout_ms":10000,"cloud_storage_inventory_based_scrub_enabled":false,"cloud_storage_credentials_source":"config_file","cloud_storage_readreplica_manifest_sync_timeout_ms":30000,"cloud_storage_garbage_collect_timeout_ms":30000,"cloud_storage_initial_backoff_ms":100,"cloud_storage_upload_loop_initial_backoff_ms":100,"use_fetch_scheduler_group":true,"cloud_storage_roles_operation_timeout_ms":30000,"node_status_interval":100,"cloud_storage_azure_managed_identity_id":null,"group_max_session_timeout_ms":300000,"cloud_storage_url_style":null,"schema_registry_normalize_on_startup":false,"cloud_storage_api_endpoint":null,"cloud_storage_region":null,"fetch_session_eviction_timeout_ms":60000,"cloud_storage_secret_key":null,"cloud_storage_access_key":null,"topic_memory_per_partition":4194304,"cloud_storage_disable_archiver_manager":true,"cloud_storage_enable_remote_write":false,"kafka_connections_max_per_ip":null,"cloud_storage_chunk_prefetch":0,"raft_heartbeat_interval_ms":150,"audit_excluded_principals":[],"kafka_qdc_window_size_ms":1500,"retention_local_target_capacity_percent":80.0,"audit_excluded_topics":[],"audit_client_max_buffer_size":16777216,"audit_log_replication_factor":null,"controller_log_accummulation_rps_capacity_topic_operations":null,"log_message_timestamp_type":"CreateTime","audit_log_num_partitions":12,"audit_enabled":false,"kafka_enable_describe_log_dirs_remote_storage":true,"cloud_storage_azure_hierarchical_namespace_enabled":null,"kafka_rpc_server_tcp_send_buf":null,"kafka_rpc_server_tcp_recv_buf":null,"aggregate_metrics":false,"kafka_client_group_byte_rate_quota":[],"kafka_connections_max_overrides":[],"cloud_storage_full_scrub_interval_ms":43200000,"legacy_group_offset_retention_enabled":false,"kafka_connections_max":null,"rpc_server_listen_backlog":null,"compaction_ctrl_backlog_size":null,"compaction_ctrl_max_shares":1000,"cloud_storage_cluster_metadata_retries":5,"compaction_ctrl_i_coeff":0.0,"topic_partitions_reserve_shard0":0,"kafka_batch_max_bytes":1048576,"kafka_quota_balancer_node_period_ms":0,"cloud_storage_segment_size_min":null,"rpc_server_tcp_send_buf":null,"kafka_enable_partition_reassignment":true,"enable_mpx_extensions":false,"tx_timeout_delay_ms":1000,"raft_heartbeat_disconnect_failures":3,"sasl_mechanisms":["SCRAM"],"compaction_ctrl_p_coeff":-12.5,"cloud_storage_enabled":false,"kafka_mtls_principal_mapping_rules":null,"cloud_storage_inventory_id":"redpanda_scrubber_inventory","cloud_storage_azure_storage_account":null,"sasl_kerberos_keytab":"/var/lib/redpanda/redpanda.keytab","group_offset_retention_sec":604800,"cloud_storage_upload_loop_max_backoff_ms":10000,"kafka_nodelete_topics":["_redpanda.audit_log","__consumer_offsets","_schemas"],"oidc_clock_skew_tolerance":0,"node_status_reconnect_max_backoff_ms":15000,"memory_enable_memory_sampling":true,"sasl_kerberos_config":"/etc/krb5.conf","data_transforms_per_core_memory_reservation":20971520,"log_retention_ms":604800000,"id_allocator_log_capacity":100,"debug_load_slice_warning_depth":null,"storage_ignore_cstore_hints":false,"group_initial_rebalance_delay":3000,"storage_compaction_index_memory":134217728,"cloud_storage_recovery_topic_validation_depth":10,"disk_reservation_percent":25.0,"storage_max_concurrent_replay":1024,"reclaim_batch_cache_min_free":67108864,"cloud_storage_upload_ctrl_update_interval_ms":60000,"alter_topic_cfg_timeout_ms":5000,"max_in_flight_pandaproxy_requests_per_shard":500,"kafka_tcp_keepalive_probes":3,"data_transforms_enabled":false,"default_window_sec":1000,"segment_fallocation_step":33554432,"partition_autobalancing_mode":"node_add","storage_reserve_min_segments":2,"storage_read_readahead_count":10,"cloud_storage_disable_read_replica_loop_for_tests":false,"enable_sasl":false,"kvstore_max_segment_size":16777216,"retention_bytes":null,"release_cache_on_segment_roll":false,"memory_abort_on_alloc_failure":true,"cloud_storage_disable_remote_labels_for_tests":false,"kvstore_flush_interval":10,"raft_enable_longest_log_detection":true,"minimum_topic_replications":1,"enable_pid_file":true,"reclaim_stable_window":10000,"metrics_reporter_url":"https://m.rp.vectorized.io/v2","reclaim_min_size":131072,"cloud_storage_trust_file":null,"storage_target_replay_bytes":10737418240,"id_allocator_batch_size":1000,"raft_replicate_batch_window_size":1048576,"cloud_storage_api_endpoint_port":443,"recovery_append_timeout_ms":5000,"compaction_ctrl_d_coeff":0.2,"replicate_append_timeout_ms":3000,"reclaim_growth_window":3000,"cloud_storage_max_segments_pending_deletion_per_partition":5000,"partition_autobalancing_max_disk_usage_percent":80,"kafka_group_recovery_timeout_ms":30000,"partition_autobalancing_tick_moves_drop_threshold":0.2,"default_topic_partitions":1,"max_compacted_log_segment_size":5368709120,"iceberg_rest_catalog_trust_file":null,"create_topic_timeout_ms":2000,"max_kafka_throttle_delay_ms":30000,"kafka_throughput_control":[],"raft_smp_max_non_local_requests":null,"cloud_storage_manifest_max_upload_interval_sec":60,"transaction_max_timeout_ms":900000,"write_caching_default":"false","abort_timed_out_transactions_interval_ms":10000,"transaction_coordinator_log_segment_size":1073741824,"storage_read_buffer_size":131072,"cloud_storage_hydrated_chunks_per_segment_ratio":0.7,"tombstone_retention_ms":null,"cloud_storage_inventory_report_check_interval_ms":21600000,"compaction_ctrl_update_interval_ms":30000,"kafka_request_max_bytes":104857600,"default_topic_replications":1,"retention_local_strict":false,"tm_sync_timeout_ms":10000,"log_disable_housekeeping_for_tests":false,"cloud_storage_attempt_cluster_restore_on_bootstrap":false,"data_transforms_write_buffer_memory_percentage":45,"abort_index_segment_size":50000,"kafka_memory_batch_size_estimate_for_fetch":1048576,"kafka_client_group_fetch_byte_rate_quota":[],"storage_compaction_key_map_memory":134217728,"enable_idempotence":true,"oidc_token_audience":"redpanda","audit_queue_drain_interval_ms":500,"raft_max_concurrent_append_requests_per_follower":16,"kafka_connection_rate_limit_overrides":[],"log_compaction_interval_ms":10000,"kafka_connection_rate_limit":null,"partition_autobalancing_tick_interval_ms":30000,"group_min_session_timeout_ms":6000,"space_management_max_segment_concurrency":10,"metadata_status_wait_timeout_ms":2000,"kafka_qdc_max_latency_ms":80,"cloud_storage_max_connection_idle_time_ms":5000,"kafka_qdc_window_count":12,"usage_disk_persistance_interval_sec":300,"storage_compaction_key_map_memory_limit_percent":12.0,"kafka_tcp_keepalive_timeout":120,"segment_appender_flush_timeout_ms":1000,"data_transforms_binary_max_size":10485760,"log_message_timestamp_alert_after_ms":7200000,"superusers":[],"raft_learner_recovery_rate":104857600,"log_message_timestamp_alert_before_ms":null,"storage_space_alert_free_threshold_percent":5,"fetch_pid_target_utilization_fraction":0.2,"rpc_server_compress_replies":false,"iceberg_catalog_base_location":"redpanda-iceberg-catalog","target_quota_byte_rate":0,"fetch_pid_p_coeff":100.0,"fetch_pid_d_coeff":0.0,"fetch_read_strategy":"non_polling","rm_sync_timeout_ms":10000,"kafka_quota_balancer_min_shard_throughput_bps":256,"fetch_pid_i_coeff":0.01,"controller_backend_housekeeping_interval_ms":1000,"data_transforms_commit_interval_ms":3000,"metadata_dissemination_retries":30,"default_num_windows":10,"target_fetch_quota_byte_rate":null,"metadata_dissemination_retry_delay_ms":320,"cloud_storage_cache_trim_walk_concurrency":1,"group_offset_retention_check_ms":600000,"readers_cache_target_max_size":200,"log_segment_size_min":1048576,"cloud_storage_upload_ctrl_max_shares":1000,"cluster_id":"e5c27f6a-45dc-4427-a0fc-5b9183b6b28d","log_segment_size_max":null,"group_new_member_join_timeout":30000,"transaction_coordinator_partitions":50,"members_backend_retry_ms":5000,"core_balancing_debounce_timeout":10000,"cloud_storage_cluster_metadata_upload_timeout_ms":60000,"kafka_rpc_server_stream_recv_buf":null,"rps_limit_node_management_operations":1000,"readers_cache_eviction_timeout_ms":30000,"fetch_pid_max_debounce_ms":100,"usage_window_width_interval_sec":3600,"max_concurrent_producer_ids":18446744073709551615,"cloud_storage_enable_remote_read":false,"raft_timeout_now_timeout_ms":1000,"usage_num_windows":24,"quota_manager_gc_sec":30000,"storage_ignore_timestamps_in_future_sec":null,"raft_recovery_concurrency_per_shard":64,"kafka_qdc_depth_alpha":0.8,"kafka_admin_topic_api_rate":null,"cloud_storage_segment_max_upload_interval_sec":3600,"kafka_throughput_throttling_v2":true,"kafka_sasl_max_reauth_ms":null,"log_compression_type":"producer","fetch_reads_debounce_timeout":1,"storage_space_alert_free_threshold_bytes":0,"log_segment_ms":3600000,"features_auto_enable":true,"rpc_server_tcp_recv_buf":null,"wait_for_leader_timeout_ms":5000,"cloud_storage_cluster_metadata_upload_interval_ms":3600000,"transactional_id_expiration_ms":604800000,"data_transforms_logging_line_max_bytes":1024,"data_transforms_per_function_memory_limit":2097152,"data_transforms_logging_flush_interval_ms":500,"sasl_kerberos_principal_mapping":["DEFAULT"],"raft_heartbeat_timeout_ms":3000,"retention_local_target_capacity_bytes":null,"partition_manager_shutdown_watchdog_timeout":30000,"cloud_storage_graceful_transfer_timeout_ms":5000,"cloud_storage_manifest_upload_timeout_ms":10000,"leader_balancer_idle_timeout":120000,"kafka_enable_authorization":null,"data_transforms_logging_buffer_capacity_bytes":512000,"raft_replica_max_pending_flush_bytes":262144,"audit_enabled_event_types":["management","authenticate","admin"],"disable_public_metrics":false,"data_transforms_read_buffer_memory_percentage":45,"kafka_throughput_replenish_threshold":null,"transaction_coordinator_delete_retention_ms":604800000,"compaction_ctrl_min_shares":10,"election_timeout_ms":1500,"data_transforms_runtime_limit_ms":3000,"pp_sr_smp_max_non_local_requests":null,"enable_leader_balancer":true,"raft_transfer_leader_recovery_timeout_ms":10000,"reclaim_max_size":4194304,"cloud_storage_inventory_max_hash_size_during_parse":67108864,"raft_max_recovery_memory":null,"space_management_max_log_concurrency":20,"rps_limit_acls_and_users_operations":1000,"kafka_noproduce_topics":[],"space_management_enable":true,"group_topic_partitions":16,"log_segment_size":134217728,"max_transactions_per_coordinator":18446744073709551615,"audit_queue_max_buffer_size_per_shard":1048576,"log_segment_size_jitter_percent":5,"rpc_client_connections_per_peer":128,"cloud_storage_max_connections":20,"log_segment_ms_max":31536000000,"admin_api_require_auth":false,"log_segment_ms_min":600000,"metadata_dissemination_interval_ms":3000,"enable_usage":false,"kafka_qdc_min_depth":1,"compacted_log_segment_size":268435456,"cloud_storage_azure_adls_endpoint":null,"disable_metrics":false,"kafka_tcp_keepalive_probe_interval_seconds":60,"retention_local_target_bytes_default":null}`

// Returns a gzip compressed string of the sample metrics
// Also returns the same data but hex encoded.
func prepareDataForBenchmark(rawData string) (string, []byte) { //nolint:unparam // second return value may be used in future benchmarks
	var buf bytes.Buffer

	gzipWriter := gzip.NewWriter(&buf)
	_, _ = gzipWriter.Write([]byte(rawData))

	err := gzipWriter.Close()
	if err != nil {
		panic(err)
	}

	bufBytes := buf.Bytes()

	return hex.EncodeToString(bufBytes), bufBytes
}

// Prepare sample log entries for ParseRedpandaLogs benchmark.
func prepareSampleLogEntries() []s6_shared.LogEntry {
	entries := []s6_shared.LogEntry{}

	// Add BLOCK_START_MARKER
	entries = append(entries, s6_shared.LogEntry{Content: BLOCK_START_MARKER})

	// Add metrics data
	metricsGZIPHex, _ := prepareDataForBenchmark(sampleMetrics)
	entries = append(entries, s6_shared.LogEntry{Content: metricsGZIPHex})

	// Add METRICS_END_MARKER
	entries = append(entries, s6_shared.LogEntry{Content: METRICS_END_MARKER})

	// Add cluster config data
	clusterConfigGZIPHex, _ := prepareDataForBenchmark(clusterConfig)
	entries = append(entries, s6_shared.LogEntry{Content: clusterConfigGZIPHex})

	// Add CLUSTERCONFIG_END_MARKER
	entries = append(entries, s6_shared.LogEntry{Content: CLUSTERCONFIG_END_MARKER})

	// Add timestamp (unix timestamp in nanoseconds)
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	entries = append(entries, s6_shared.LogEntry{Content: timestamp})

	// Add BLOCK_END_MARKER
	entries = append(entries, s6_shared.LogEntry{Content: BLOCK_END_MARKER})

	return entries
}

// BenchmarkGzipDecode benchmarks just the gzip decoding operation.
func BenchmarkGzipDecode(b *testing.B) {
	encodedData, _ := prepareDataForBenchmark(sampleMetrics)

	// Hex decode
	decodedData, _ := hex.DecodeString(encodedData)

	b.ResetTimer()

	for range b.N {
		reader, err := gzip.NewReader(bytes.NewReader(decodedData))
		if err != nil {
			b.Fatal(err)
		}

		_, err = io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}

		err = reader.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHexDecode benchmarks just the hex decoding operation.
func BenchmarkHexDecode(b *testing.B) {
	encodedAndHexedData, _ := prepareDataForBenchmark(sampleMetrics)

	b.ResetTimer()

	for range b.N {
		_, err := hex.DecodeString(encodedAndHexedData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMetricsParsing benchmarks the metrics parsing operation.
func BenchmarkMetricsParsing(b *testing.B) {
	b.ResetTimer()

	for range b.N {
		_, err := ParseMetrics(bytes.NewReader([]byte(sampleMetrics)))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompleteProcessing benchmarks the entire pipeline: hex decode -> gzip decode -> parse metrics.
func BenchmarkCompleteProcessing(b *testing.B) {
	encodedAndHexedData, _ := prepareDataForBenchmark(sampleMetrics)

	b.ResetTimer()

	for range b.N {
		// Step 1: Hex decode
		decodedMetricsDataBytes, _ := hex.DecodeString(encodedAndHexedData)

		// Step 2: Gzip decompress
		gzipReader, err := gzip.NewReader(bytes.NewReader(decodedMetricsDataBytes))
		if err != nil {
			b.Fatal(err)
		}

		// Step 3: Parse metrics
		_, err = ParseMetrics(gzipReader)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseRedpandaLogs benchmarks the ParseRedpandaLogs function.
func BenchmarkParseRedpandaLogsWithPercentiles(b *testing.B) {
	// Create service and test data
	service := NewRedpandaMonitorService("test-redpanda")
	logs := prepareSampleLogEntries()
	ctx := context.Background()

	// Collect execution times
	times := make([]time.Duration, b.N)

	b.ResetTimer()
	b.StopTimer()

	for iteration := range b.N {
		start := time.Now()

		b.StartTimer()

		_, err := service.ParseRedpandaLogs(ctx, logs, uint64(iteration)) //nolint:gosec // G115: Safe conversion, benchmark iteration counter is positive

		b.StopTimer()

		if err != nil {
			b.Fatal(err)
		}

		times[iteration] = time.Since(start)
	}

	// Sort times for percentile calculation
	sort.Slice(times, func(i, j int) bool {
		return times[i] < times[j]
	})

	// Calculate and report percentiles
	p50 := times[int(float64(len(times))*0.50)]
	p95 := times[int(float64(len(times))*0.95)]
	p99 := times[int(float64(len(times))*0.99)]

	b.ReportMetric(float64(p50.Nanoseconds()), "p50ns")
	b.ReportMetric(float64(p95.Nanoseconds()), "p95ns")
	b.ReportMetric(float64(p99.Nanoseconds()), "p99ns")
}
