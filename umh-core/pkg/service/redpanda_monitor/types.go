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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// Metrics contains information about the metrics of the Redpanda service
type Metrics struct {
	Infrastructure InfrastructureMetrics
	Cluster        ClusterMetrics
	Throughput     ThroughputMetrics
	Topic          TopicMetrics
}

// InfrastructureMetrics contains information about the infrastructure metrics of the Redpanda service
type InfrastructureMetrics struct {
	Storage StorageMetrics
	Uptime  UptimeMetrics
}

// StorageMetrics contains information about the storage metrics of the Redpanda service
type StorageMetrics struct {
	// redpanda_storage_disk_free_bytes
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_storage_disk_free_bytes
	FreeBytes int64
	// redpanda_storage_disk_total_bytes
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_storage_disk_total_bytes
	TotalBytes int64
	// redpanda_storage_disk_free_space_alert (0 == false, everything else == true)
	// type: gauge
	FreeSpaceAlert bool
}

// UptimeMetrics contains information about the uptime metrics of the Redpanda service
type UptimeMetrics struct {
	// redpanda_uptime_seconds_total
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_uptime_seconds_total
	Uptime int64
}

// ClusterMetrics contains information about the cluster metrics of the Redpanda service
type ClusterMetrics struct {
	// redpanda_cluster_topics
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_cluster_topics
	Topics int64
	// redpanda_cluster_unavailable_partitions
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_cluster_unavailable_partitions
	UnavailableTopics int64
}

// ThroughputMetrics contains information about the throughput metrics of the Redpanda service
type ThroughputMetrics struct {
	// redpanda_kafka_request_bytes_total over all redpanda_namespace and redpanda_topic labels using redpanda_request=("produce")
	// type: counter
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_kafka_request_bytes_total
	BytesIn int64
	// redpanda_kafka_request_bytes_total over all redpanda_namespace and redpanda_topic labels using redpanda_request=("consume")
	// type: counter
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_kafka_request_bytes_total
	BytesOut int64
}

// TopicMetrics contains information about the topic metrics of the Redpanda service
type TopicMetrics struct {
	// redpanda_kafka_partitions
	// type: gauge
	// Docs: https://docs.redpanda.com/current/reference/public-metrics-reference/#redpanda_kafka_partitions
	TopicPartitionMap map[string]int64
}

type RedpandaMetrics struct {
	// Metrics contains information about the metrics of the Redpanda service
	Metrics Metrics
	// MetricsState contains information about the metrics of the Redpanda service
	MetricsState *RedpandaMetricsState
}

// ServiceInfo contains information about a redpanda service
type ServiceInfo struct {
	// S6ObservedState contains information about the S6 service
	S6ObservedState s6fsm.S6ObservedState
	// S6FSMState contains the current state of the S6 FSM
	S6FSMState string
	// RedpandaStatus contains information about the status of the redpanda service
	RedpandaStatus RedpandaMonitorStatus
}

// RedpandaMonitorStatus contains status information about the redpanda service
type RedpandaMonitorStatus struct {
	// LastScan contains the result of the last scan
	LastScan *RedpandaMetrics
	// IsRunning indicates whether the redpanda service is running
	IsRunning bool
	// Logs contains the logs of the redpanda service
	Logs []s6service.LogEntry
}

type IRedpandaMonitorService interface {
	GenerateS6ConfigForRedpandaMonitor() (s6serviceconfig.S6ServiceConfig, error)
	Status(ctx context.Context, filesystemService filesystem.Service, tick uint64) (ServiceInfo, error)
	AddRedpandaToS6Manager(ctx context.Context) error
	RemoveRedpandaFromS6Manager(ctx context.Context) error
	StartRedpanda(ctx context.Context) error
	StopRedpanda(ctx context.Context) error
	ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (error, bool)
	ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool
}
