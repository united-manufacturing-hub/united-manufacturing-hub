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
	res.Metrics = append(res.Metrics,
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.ConnectionFailed, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "connection_failed"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.ConnectionLost, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "connection_lost"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.ConnectionUp, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "connection_up"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.Received, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "received"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.P50, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "latency_ns_p50"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.P90, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "latency_ns_p90"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.P99, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "latency_ns_p99"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.Sum, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "latency_ns_sum"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.Count, ComponentType: DFCMetricComponentTypeInput, Path: DFCInputPath, Name: "latency_ns_count"},
	)

	// Process Output metrics
	res.Metrics = append(res.Metrics,
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.BatchSent, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "batch_sent"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.ConnectionFailed, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "connection_failed"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.ConnectionLost, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "connection_lost"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.ConnectionUp, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "connection_up"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.Error, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "error"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.Sent, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "sent"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.P50, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "latency_ns_p50"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.P90, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "latency_ns_p90"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.P99, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "latency_ns_p99"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.Sum, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "latency_ns_sum"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.Count, ComponentType: DFCMetricComponentTypeOutput, Path: DFCOutputPath, Name: "latency_ns_count"},
	)

	// Process processor metrics
	for path, proc := range metrics.Process.Processors {
		res.Metrics = append(res.Metrics,
			models.Metric{ValueType: models.MetricValueTypeString, Value: proc.Label, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "label"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.Received, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "received"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.BatchReceived, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "batch_received"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.Sent, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "sent"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.BatchSent, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "batch_sent"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.Error, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "error"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.LatencyNS.P50, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "latency_ns_p50"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.LatencyNS.P90, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "latency_ns_p90"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.LatencyNS.P99, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "latency_ns_p99"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.LatencyNS.Sum, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "latency_ns_sum"},
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: proc.LatencyNS.Count, ComponentType: DFCMetricComponentTypeProcessor, Path: path, Name: "latency_ns_count"},
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
	res.Metrics = append(res.Metrics,
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: storageMetrics.FreeBytes, ComponentType: RedpandaMetricComponentTypeStorage, Path: RedpandaStoragePath, Name: "disk_free_bytes"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: storageMetrics.TotalBytes, ComponentType: RedpandaMetricComponentTypeStorage, Path: RedpandaStoragePath, Name: "disk_total_bytes"},
		models.Metric{ValueType: models.MetricValueTypeBoolean, Value: storageMetrics.FreeSpaceAlert, ComponentType: RedpandaMetricComponentTypeStorage, Path: RedpandaStoragePath, Name: "disk_free_space_alert"},
	)

	// Process cluster metrics
	clusterMetrics := metrics.Cluster
	res.Metrics = append(res.Metrics,
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: clusterMetrics.Topics, ComponentType: RedpandaMetricComponentTypeCluster, Path: RedpandaClusterPath, Name: "topics"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: clusterMetrics.UnavailableTopics, ComponentType: RedpandaMetricComponentTypeCluster, Path: RedpandaClusterPath, Name: "unavailable_partitions"},
	)

	// Process kafka metrics (throughput)
	res.Metrics = append(res.Metrics,
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Throughput.BytesIn, ComponentType: RedpandaMetricComponentTypeKafka, Path: RedpandaKafkaPath, Name: "request_bytes_in"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Throughput.BytesOut, ComponentType: RedpandaMetricComponentTypeKafka, Path: RedpandaKafkaPath, Name: "request_bytes_out"},
	)

	// Process topic metrics
	for topicName, partitionCount := range metrics.Topic.TopicPartitionMap {
		topicPath := fmt.Sprintf("%s.%s", RedpandaTopicPath, topicName)
		res.Metrics = append(res.Metrics,
			models.Metric{ValueType: models.MetricValueTypeNumber, Value: partitionCount, ComponentType: RedpandaMetricComponentTypeTopic, Path: topicPath, Name: "partitions"},
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
