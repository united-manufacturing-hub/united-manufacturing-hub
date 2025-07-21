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

package providers

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// MetricsProvider defines a simple interface for retrieving metrics
type MetricsProvider interface {
	// GetMetrics retrieves metrics from the appropriate source based on the payload type.
	// It returns a response object with an array of metrics or an error if the retrieval fails.
	GetMetrics(payload models.GetMetricsRequest, snapshot fsm.SystemSnapshot) (models.GetMetricsResponse, error)
}

// DefaultMetricsProvider implements the MetricsProvider interface
type DefaultMetricsProvider struct{}

// GetMetrics delegates to the appropriate helper function based on resource type
func (p *DefaultMetricsProvider) GetMetrics(payload models.GetMetricsRequest, snapshot fsm.SystemSnapshot) (models.GetMetricsResponse, error) {
	switch payload.Type {
	case models.DFCMetricResourceType:
		return getDFCMetrics(payload.UUID, snapshot)
	case models.RedpandaMetricResourceType:
		return getRedpandaMetrics(snapshot)
	case models.TopicBrowserMetricResourceType:
		return getTopicBrowserMetrics(snapshot)
	case models.StreamProcessorMetricResourceType:
		return getStreamProcessorMetrics(payload.UUID, snapshot)
	default:
		return models.GetMetricsResponse{}, fmt.Errorf("unsupported metric type: %s", payload.Type)
	}
}

// getDFCMetrics retrieves metrics from the DFC instance and converts them
// to a standardized format matching the Get-Metrics API response structure.
// The metrics are organized by component types that match Benthos' naming conventions.
// See: https://docs.redpanda.com/redpanda-connect/components/metrics/about/#metric-names
func getDFCMetrics(uuid string, snapshot fsm.SystemSnapshot) (models.GetMetricsResponse, error) {
	res := models.GetMetricsResponse{Metrics: []models.Metric{}}

	dfcInstance, err := fsm.FindDfcInstanceByUUID(snapshot, uuid)
	if err != nil {
		return res, fmt.Errorf("failed to find the %s instance: %w", models.DFCMetricResourceType, err)
	}

	observedState, ok := dfcInstance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
	if !ok || observedState == nil {
		err = fmt.Errorf("DFC instance %s has no observed state", uuid)
		return res, err
	}

	metrics := observedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics

	// Process Input metrics
	inputMetrics := metrics.Input
	addMetrics(&res, DFCMetricComponentTypeInput, DFCInputPath,
		MetricEntry{Name: "connection_failed", Value: inputMetrics.ConnectionFailed, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_lost", Value: inputMetrics.ConnectionLost, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_up", Value: inputMetrics.ConnectionUp, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "received", Value: inputMetrics.Received, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p50", Value: inputMetrics.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p90", Value: inputMetrics.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p99", Value: inputMetrics.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_sum", Value: inputMetrics.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_count", Value: inputMetrics.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
	)

	// Process Output metrics
	outputMetrics := metrics.Output
	addMetrics(&res, DFCMetricComponentTypeOutput, DFCOutputPath,
		MetricEntry{Name: "batch_sent", Value: outputMetrics.BatchSent, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_failed", Value: outputMetrics.ConnectionFailed, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_lost", Value: outputMetrics.ConnectionLost, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_up", Value: outputMetrics.ConnectionUp, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "error", Value: outputMetrics.Error, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "sent", Value: outputMetrics.Sent, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p50", Value: outputMetrics.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p90", Value: outputMetrics.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p99", Value: outputMetrics.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_sum", Value: outputMetrics.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_count", Value: outputMetrics.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
	)

	// Process processor metrics
	for path, proc := range metrics.Process.Processors {
		addMetrics(&res, DFCMetricComponentTypeProcessor, path,
			MetricEntry{Name: "label", Value: proc.Label, ValueType: models.MetricValueTypeString},
			MetricEntry{Name: "received", Value: proc.Received, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "batch_received", Value: proc.BatchReceived, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "sent", Value: proc.Sent, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "batch_sent", Value: proc.BatchSent, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "error", Value: proc.Error, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p50", Value: proc.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p90", Value: proc.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p99", Value: proc.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_sum", Value: proc.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_count", Value: proc.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
		)
	}

	return res, nil
}

// getRedpandaMetrics retrieves metrics from the Redpanda instance and converts them
// to a standardized format matching the Get-Metrics API response structure.
// The metrics are organized by component types that match Redpanda's naming conventions.
// See: https://docs.redpanda.com/current/reference/public-metrics-reference
func getRedpandaMetrics(snapshot fsm.SystemSnapshot) (models.GetMetricsResponse, error) {
	res := models.GetMetricsResponse{Metrics: []models.Metric{}}

	redpandaInst, ok := fsm.FindInstance(snapshot, constants.RedpandaManagerName, constants.RedpandaInstanceName)
	if !ok || redpandaInst == nil {
		return res, fmt.Errorf("failed to find the %s instance", models.RedpandaMetricResourceType)
	}

	observedState, ok := redpandaInst.LastObservedState.(*redpanda.RedpandaObservedStateSnapshot)
	if !ok || observedState == nil {
		return res, fmt.Errorf("redpanda instance %s has no observed state", redpandaInst.ID)
	}

	metrics := observedState.ServiceInfoSnapshot.RedpandaStatus.RedpandaMetrics.Metrics

	// Process storage metrics
	storageMetrics := metrics.Infrastructure.Storage
	addMetrics(&res, RedpandaMetricComponentTypeStorage, RedpandaStoragePath,
		MetricEntry{Name: "disk_free_bytes", Value: storageMetrics.FreeBytes, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "disk_total_bytes", Value: storageMetrics.TotalBytes, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "disk_free_space_alert", Value: storageMetrics.FreeSpaceAlert, ValueType: models.MetricValueTypeBoolean},
	)

	// Process cluster metrics
	clusterMetrics := metrics.Cluster
	addMetrics(&res, RedpandaMetricComponentTypeCluster, RedpandaClusterPath,
		MetricEntry{Name: "topics", Value: clusterMetrics.Topics, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "unavailable_partitions", Value: clusterMetrics.UnavailableTopics, ValueType: models.MetricValueTypeNumber},
	)

	// Process kafka metrics (throughput)
	kafkaMetrics := metrics.Throughput
	addMetrics(&res, RedpandaMetricComponentTypeKafka, RedpandaKafkaPath,
		MetricEntry{Name: "request_bytes_in", Value: kafkaMetrics.BytesIn, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "request_bytes_out", Value: kafkaMetrics.BytesOut, ValueType: models.MetricValueTypeNumber},
	)

	// Process topic metrics
	for topicName, partitionCount := range metrics.Topic.TopicPartitionMap {
		topicPath := fmt.Sprintf("%s.%s", RedpandaTopicPath, topicName)
		addMetrics(&res, RedpandaMetricComponentTypeTopic, topicPath,
			MetricEntry{Name: "partitions", Value: partitionCount, ValueType: models.MetricValueTypeNumber},
		)
	}

	return res, nil
}

// getTopicBrowserMetrics retrieves metrics from the topic browser instance and converts them
// to a standardized format matching the Get-Metrics API response structure.
func getTopicBrowserMetrics(snapshot fsm.SystemSnapshot) (models.GetMetricsResponse, error) {
	res := models.GetMetricsResponse{Metrics: []models.Metric{}}

	// Create empty observed state for now
	inst, ok := fsm.FindInstance(snapshot, constants.TopicBrowserManagerName, constants.TopicBrowserInstanceName)
	if !ok || inst == nil {
		return res, fmt.Errorf("failed to find the %s instance", models.TopicBrowserMetricResourceType)
	}
	observedState, ok := inst.LastObservedState.(*topicbrowser.ObservedStateSnapshot)
	if !ok || observedState == nil {
		return res, fmt.Errorf("topic browser instance %s has no observed state", inst.ID)
	}

	// Extract benthos metrics from the observed state
	metrics := observedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics

	// Process Input metrics (same structure as DFC since topic browser uses benthos)
	inputMetrics := metrics.Input
	addMetrics(&res, TopicBrowserMetricComponentTypeInput, TopicBrowserInputPath,
		MetricEntry{Name: "connection_failed", Value: inputMetrics.ConnectionFailed, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_lost", Value: inputMetrics.ConnectionLost, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_up", Value: inputMetrics.ConnectionUp, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "received", Value: inputMetrics.Received, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p50", Value: inputMetrics.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p90", Value: inputMetrics.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p99", Value: inputMetrics.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_sum", Value: inputMetrics.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_count", Value: inputMetrics.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
	)

	// Process Output metrics
	outputMetrics := metrics.Output
	addMetrics(&res, TopicBrowserMetricComponentTypeOutput, TopicBrowserOutputPath,
		MetricEntry{Name: "batch_sent", Value: outputMetrics.BatchSent, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_failed", Value: outputMetrics.ConnectionFailed, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_lost", Value: outputMetrics.ConnectionLost, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_up", Value: outputMetrics.ConnectionUp, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "error", Value: outputMetrics.Error, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "sent", Value: outputMetrics.Sent, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p50", Value: outputMetrics.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p90", Value: outputMetrics.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p99", Value: outputMetrics.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_sum", Value: outputMetrics.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_count", Value: outputMetrics.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
	)

	// Process processor metrics
	for path, proc := range metrics.Process.Processors {
		addMetrics(&res, TopicBrowserMetricComponentTypeProcessor, path,
			MetricEntry{Name: "label", Value: proc.Label, ValueType: models.MetricValueTypeString},
			MetricEntry{Name: "received", Value: proc.Received, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "batch_received", Value: proc.BatchReceived, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "sent", Value: proc.Sent, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "batch_sent", Value: proc.BatchSent, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "error", Value: proc.Error, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p50", Value: proc.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p90", Value: proc.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p99", Value: proc.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_sum", Value: proc.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_count", Value: proc.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
		)
	}

	return res, nil
}

const (
	// DFC paths
	DFCInputPath  = "root.input"
	DFCOutputPath = "root.output"

	// DFC metric component types
	DFCMetricComponentTypeInput     = "input"
	DFCMetricComponentTypeOutput    = "output"
	DFCMetricComponentTypeProcessor = "processor"

	// Redpanda paths
	RedpandaStoragePath = "redpanda.storage"
	RedpandaClusterPath = "redpanda.cluster"
	RedpandaKafkaPath   = "redpanda.kafka"
	RedpandaTopicPath   = "redpanda.topic"

	// Redpanda metric component types
	RedpandaMetricComponentTypeStorage = "storage"
	RedpandaMetricComponentTypeCluster = "cluster"
	RedpandaMetricComponentTypeKafka   = "kafka"
	RedpandaMetricComponentTypeTopic   = "topic"

	// Topic Browser paths
	TopicBrowserInputPath  = "root.input"
	TopicBrowserOutputPath = "root.output"

	// Topic Browser metric component types
	TopicBrowserMetricComponentTypeInput     = "input"
	TopicBrowserMetricComponentTypeOutput    = "output"
	TopicBrowserMetricComponentTypeProcessor = "processor"

	// Stream Processor paths
	StreamProcessorInputPath  = "root.input"
	StreamProcessorOutputPath = "root.output"

	// Stream Processor metric component types
	StreamProcessorMetricComponentTypeInput     = "input"
	StreamProcessorMetricComponentTypeOutput    = "output"
	StreamProcessorMetricComponentTypeProcessor = "processor"
)

type MetricEntry struct {
	Name      string
	Value     any
	ValueType models.MetricValueType
}

func addMetrics(res *models.GetMetricsResponse, componentType string, path string, entries ...MetricEntry) {
	for _, entry := range entries {
		res.Metrics = append(res.Metrics, models.Metric{ValueType: entry.ValueType, Value: entry.Value, ComponentType: componentType, Path: path, Name: entry.Name})
	}
}

// getStreamProcessorMetrics retrieves metrics from the stream processor instance and converts them
// to a standardized format matching the Get-Metrics API response structure.
func getStreamProcessorMetrics(uuid string, snapshot fsm.SystemSnapshot) (models.GetMetricsResponse, error) {
	res := models.GetMetricsResponse{Metrics: []models.Metric{}}

	// Create empty observed state for now
	inst, ok := fsm.FindManager(snapshot, constants.StreamProcessorManagerName)
	if !ok || inst == nil {
		return res, fmt.Errorf("failed to find the %s instance", models.StreamProcessorMetricResourceType)
	}
	streamProcessorInstances := inst.GetInstances()
	var observedState *streamprocessor.ObservedStateSnapshot
	found := false
	for _, instance := range streamProcessorInstances {
		if dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String() == uuid {
			var ok bool
			observedState, ok = instance.LastObservedState.(*streamprocessor.ObservedStateSnapshot)
			if !ok || observedState == nil {
				return res, fmt.Errorf("stream processor instance %s has no observed state", instance.ID)
			}
			found = true
			break
		}
	}
	if !found {
		return res, fmt.Errorf("stream processor instance %s not found", uuid)
	}

	if observedState == nil {
		return res, fmt.Errorf("stream processor instance has nil observed state")
	}

	// Extract benthos metrics from the observed state
	metrics := observedState.ServiceInfo.DFCObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics

	// Process Input metrics (same structure as DFC since topic browser uses benthos)
	inputMetrics := metrics.Input
	addMetrics(&res, StreamProcessorMetricComponentTypeInput, StreamProcessorInputPath,
		MetricEntry{Name: "connection_failed", Value: inputMetrics.ConnectionFailed, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_lost", Value: inputMetrics.ConnectionLost, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_up", Value: inputMetrics.ConnectionUp, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "received", Value: inputMetrics.Received, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p50", Value: inputMetrics.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p90", Value: inputMetrics.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p99", Value: inputMetrics.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_sum", Value: inputMetrics.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_count", Value: inputMetrics.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
	)

	// Process Output metrics
	outputMetrics := metrics.Output
	addMetrics(&res, StreamProcessorMetricComponentTypeOutput, StreamProcessorOutputPath,
		MetricEntry{Name: "batch_sent", Value: outputMetrics.BatchSent, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_failed", Value: outputMetrics.ConnectionFailed, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_lost", Value: outputMetrics.ConnectionLost, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "connection_up", Value: outputMetrics.ConnectionUp, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "error", Value: outputMetrics.Error, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "sent", Value: outputMetrics.Sent, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p50", Value: outputMetrics.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p90", Value: outputMetrics.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_p99", Value: outputMetrics.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_sum", Value: outputMetrics.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
		MetricEntry{Name: "latency_ns_count", Value: outputMetrics.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
	)

	// Process processor metrics
	for path, proc := range metrics.Process.Processors {
		addMetrics(&res, StreamProcessorMetricComponentTypeProcessor, path,
			MetricEntry{Name: "label", Value: proc.Label, ValueType: models.MetricValueTypeString},
			MetricEntry{Name: "received", Value: proc.Received, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "batch_received", Value: proc.BatchReceived, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "sent", Value: proc.Sent, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "batch_sent", Value: proc.BatchSent, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "error", Value: proc.Error, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p50", Value: proc.LatencyNS.P50, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p90", Value: proc.LatencyNS.P90, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_p99", Value: proc.LatencyNS.P99, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_sum", Value: proc.LatencyNS.Sum, ValueType: models.MetricValueTypeNumber},
			MetricEntry{Name: "latency_ns_count", Value: proc.LatencyNS.Count, ValueType: models.MetricValueTypeNumber},
		)
	}

	return res, nil
}
