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

package actions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type GetMetricsAction struct {
	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetMetricsRequest

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger
}

// NewGetMetricsAction creates a new GetMetricsAction with the provided parameters.
// Caller needs to invoke Parse and Validate before calling Execute.
func NewGetMetricsAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, systemSnapshotManager *fsm.SnapshotManager) *GetMetricsAction {
	return &GetMetricsAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          zap.S().With("action", "GetMetricsAction"),
	}
}

// Parse extracts the business fields from the raw JSON payload.
// Shape errors are detected here, while semantic validation is done in Validate.
func (a *GetMetricsAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetMetricsRequest](payload)
	a.actionLogger.Info("Payload parsed: %v", a.payload)
	return err
}

// Validate performs semantic validation of the parsed payload.
// This verifies that the metric type is allowed and that the UUID is valid for DFC metrics.
func (a *GetMetricsAction) Validate() (err error) {
	a.actionLogger.Info("Validating the payload")

	allowedMetricTypes := []models.MetricResourceType{models.DFCMetricResourceType, models.RedpandaMetricResourceType}
	if !slices.Contains(allowedMetricTypes, a.payload.Type) {
		return errors.New("metric type must be set and must be one of the following: dfc, redpanda")
	}

	if a.payload.Type == models.DFCMetricResourceType {
		if a.payload.UUID == "" {
			return errors.New("uuid must be set to retrieve metrics for a DFC")
		}

		_, err = uuid.Parse(a.payload.UUID)
		if err != nil {
			return fmt.Errorf("invalid UUID format: %v", err)
		}
	}

	return nil
}

// getDFCMetrics retrieves metrics from the DFC instance and converts them
// to a standardized format matching the Get-Metrics API response structure.
// The metrics are organized by component types that match Benthos' naming conventions.
// See: https://docs.redpanda.com/redpanda-connect/components/metrics/about/#metric-names
func getDFCMetrics(uuid string, systemSnapshotManager *fsm.SnapshotManager) (models.GetMetricsResponse, error) {
	res := models.GetMetricsResponse{Metrics: []models.Metric{}}

	dfcInstance, err := fsm.FindDfcInstanceByUUID(systemSnapshotManager.GetDeepCopySnapshot(), uuid)
	if err != nil {
		return res, err
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
func getRedpandaMetrics(systemSnapshot *fsm.SnapshotManager) (models.GetMetricsResponse, error) {
	res := models.GetMetricsResponse{Metrics: []models.Metric{}}

	redpandaInst, ok := fsm.FindInstance(systemSnapshot.GetDeepCopySnapshot(), constants.RedpandaManagerName, constants.RedpandaInstanceName)
	if !ok || redpandaInst == nil {
		return res, fmt.Errorf("redpanda instance not found")
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

// Execute retrieves the metrics from the correct source based on the metric type.
// It returns a response object with an array of metrics or an error if the retrieval fails.
func (a *GetMetricsAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing the action")

	metricType := a.payload.Type

	switch metricType {
	case models.DFCMetricResourceType:
		metrics, err := getDFCMetrics(a.payload.UUID, a.systemSnapshotManager)
		if err != nil {
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetMetrics)
			return nil, nil, err
		}

		return metrics, nil, nil
	case models.RedpandaMetricResourceType:
		metrics, err := getRedpandaMetrics(a.systemSnapshotManager)
		if err != nil {
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetMetrics)
			return nil, nil, err
		}

		return metrics, nil, nil
	default:
		err := errors.New("unknown metric type")
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetMetrics)
		return nil, nil, err
	}
}

func (a *GetMetricsAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetMetricsAction) getUuid() uuid.UUID {
	return a.actionUUID
}

func (a *GetMetricsAction) GetParsedPayload() models.GetMetricsRequest {
	return a.payload
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
