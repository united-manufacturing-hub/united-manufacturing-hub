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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
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

func (a *GetMetricsAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetMetricsRequest](payload)
	a.actionLogger.Info("Payload parsed: %v", a.payload)
	return err
}

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

const (
	// DFC metric component types
	DFCMetricComponentTypeInput     = "input"
	DFCMetricComponentTypeOutput    = "output"
	DFCMetricComponentTypeProcessor = "processor"

	// Redpanda metric component types
	RedpandaMetricComponentTypeStorage = "storage"
	RedpandaMetricComponentTypeCluster = "cluster"
	RedpandaMetricComponentTypeKafka   = "kafka"
)

func getDFCMetrics(uuid string, systemSnapshotManager *fsm.SnapshotManager) (*models.GetMetricsResponse, error) {
	dfcInstance, err := fsm.FindDfcInstanceByUUID(systemSnapshotManager.GetDeepCopySnapshot(), uuid)
	if err != nil {
		return nil, err
	}

	// Safety check to ensure LastObservedState is not nil
	if dfcInstance.LastObservedState == nil {
		err = fmt.Errorf("DFC instance %s has no observed state", uuid)
		return nil, err
	}

	metrics := dfcInstance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot).ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics
	flattenedMetrics := []models.Metric{}

	// Process Input metrics
	inputPath := "root.input"
	flattenedMetrics = append(flattenedMetrics,
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.ConnectionFailed, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "connection_failed"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.ConnectionLost, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "connection_lost"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.ConnectionUp, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "connection_up"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.Received, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "received"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.P50, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_p50"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.P90, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_p90"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.P99, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_p99"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.Sum, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_sum"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Input.LatencyNS.Count, ComponentType: DFCMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_count"},
	)

	// Process Output metrics
	outputPath := "root.output"
	flattenedMetrics = append(flattenedMetrics,
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.BatchSent, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "batch_sent"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.ConnectionFailed, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "connection_failed"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.ConnectionLost, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "connection_lost"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.ConnectionUp, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "connection_up"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.Error, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "error"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.Sent, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "sent"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.P50, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_p50"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.P90, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_p90"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.P99, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_p99"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.Sum, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_sum"},
		models.Metric{ValueType: models.MetricValueTypeNumber, Value: metrics.Output.LatencyNS.Count, ComponentType: DFCMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_count"},
	)

	// Process processor metrics
	for path, proc := range metrics.Process.Processors {
		flattenedMetrics = append(flattenedMetrics,
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

	return &models.GetMetricsResponse{Metrics: flattenedMetrics}, nil
}

// func getRedpandaMetrics() (models.GetMetricsResponse, error) {
// }

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
	// case models.RedpandaMetricResourceType:
	// 	metrics, err := getRedpandaMetrics()
	// 	if err != nil {
	// 		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetMetrics)
	// 		return nil, nil, err
	// 	}

	// 	return metrics, nil, nil
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
