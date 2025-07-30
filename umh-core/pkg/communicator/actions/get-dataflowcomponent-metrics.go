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

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// Deprecated: Use GetMetricsAction instead. Kept for backward compatibility.
type GetDataflowcomponentMetricsAction struct {

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger
	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail string

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetDataflowcomponentMetricsRequest //nolint:staticcheck // Deprecated but kept for back compat

	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

func NewGetDataflowcomponentMetricsAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, systemSnapshotManager *fsm.SnapshotManager) *GetDataflowcomponentMetricsAction {
	return &GetDataflowcomponentMetricsAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          zap.S().With("action", "GetDataflowcomponentMetricsAction"),
	}
}

func (a *GetDataflowcomponentMetricsAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetDataflowcomponentMetricsRequest](payload) //nolint:staticcheck // Deprecated but kept for back compat
	a.actionLogger.Info("Payload parsed: %v", a.payload)
	return err
}

func (a *GetDataflowcomponentMetricsAction) Validate() (err error) {
	a.actionLogger.Info("Validating the payload")

	if a.payload.UUID == "" {
		return errors.New("uuid must be set to retrieve metrics for a dataflowcomponent")
	}

	_, err = uuid.Parse(a.payload.UUID)
	if err != nil {
		return fmt.Errorf("invalid UUID format: %v", err)
	}

	return nil
}

func (a *GetDataflowcomponentMetricsAction) Execute() (interface{}, map[string]interface{}, error) {
	dfcInstance, err := fsm.FindDfcInstanceByUUID(a.systemSnapshotManager.GetDeepCopySnapshot(), a.payload.UUID)
	if err != nil {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, "failed to find DFC instance", a.outboundChannel, models.GetDataFlowComponentMetrics) //nolint:staticcheck // Deprecated but kept for back compat
		return nil, nil, err
	}

	// Safety check to ensure LastObservedState is not nil
	if dfcInstance.LastObservedState == nil {
		err = fmt.Errorf("DFC instance %s has no observed state", a.payload.UUID)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetDataFlowComponentMetrics) //nolint:staticcheck // Deprecated but kept for back compat
		return nil, nil, err
	}

	metrics := dfcInstance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot).ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics

	// Convert metrics to flattened DfcMetrics format
	dfcMetrics := DfcMetrics{
		Metrics: []DfcMetric{},
	}

	// Process Input metrics
	inputPath := "root.input"
	dfcMetrics.Metrics = append(dfcMetrics.Metrics,
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.ConnectionFailed, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "connection_failed"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.ConnectionLost, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "connection_lost"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.ConnectionUp, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "connection_up"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.Received, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "received"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.LatencyNS.P50, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_p50"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.LatencyNS.P90, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_p90"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.LatencyNS.P99, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_p99"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.LatencyNS.Sum, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_sum"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Input.LatencyNS.Count, ComponentType: DfcMetricComponentTypeInput, Path: inputPath, Name: "latency_ns_count"},
	)

	// Process Output metrics
	outputPath := "root.output"
	dfcMetrics.Metrics = append(dfcMetrics.Metrics,
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.BatchSent, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "batch_sent"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.ConnectionFailed, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "connection_failed"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.ConnectionLost, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "connection_lost"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.ConnectionUp, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "connection_up"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.Error, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "error"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.Sent, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "sent"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.LatencyNS.P50, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_p50"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.LatencyNS.P90, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_p90"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.LatencyNS.P99, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_p99"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.LatencyNS.Sum, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_sum"},
		DfcMetric{ValueType: DfcMetricTypeNumber, Value: metrics.Output.LatencyNS.Count, ComponentType: DfcMetricComponentTypeOutput, Path: outputPath, Name: "latency_ns_count"},
	)

	// Process processor metrics
	for path, proc := range metrics.Process.Processors {
		dfcMetrics.Metrics = append(dfcMetrics.Metrics,
			DfcMetric{ValueType: DfcMetricTypeString, Value: proc.Label, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "label"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.Received, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "received"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.BatchReceived, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "batch_received"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.Sent, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "sent"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.BatchSent, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "batch_sent"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.Error, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "error"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.LatencyNS.P50, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "latency_ns_p50"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.LatencyNS.P90, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "latency_ns_p90"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.LatencyNS.P99, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "latency_ns_p99"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.LatencyNS.Sum, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "latency_ns_sum"},
			DfcMetric{ValueType: DfcMetricTypeNumber, Value: proc.LatencyNS.Count, ComponentType: DfcMetricComponentTypeProcessor, Path: path, Name: "latency_ns_count"},
		)
	}

	return dfcMetrics, nil, nil
}

func (a *GetDataflowcomponentMetricsAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetDataflowcomponentMetricsAction) getUuid() uuid.UUID {
	return a.actionUUID
}

func (a *GetDataflowcomponentMetricsAction) GetParsedPayload() models.GetDataflowcomponentMetricsRequest { //nolint:staticcheck // Deprecated but kept for back compat
	return a.payload
}

// Deprecated: Use models.GetMetricsResponse instead. Kept for backward compatibility.
type DfcMetrics struct {
	Metrics []DfcMetric `json:"metrics"`
}

// Deprecated: Use models.Metric instead. Kept for backward compatibility.
type DfcMetric struct {
	ValueType     DfcMetricType          `json:"value_type"`
	Value         any                    `json:"value"`
	ComponentType DfcMetricComponentType `json:"component_type"`
	Path          string                 `json:"path"`
	Name          string                 `json:"name"`
}

// Deprecated: Use models.MetricValueType instead. Kept for backward compatibility.
type DfcMetricType string

const (
	DfcMetricTypeNumber DfcMetricType = "number"
	DfcMetricTypeString DfcMetricType = "string"
)

// DfcMetricComponentType represents the Redpanda Connect component type that emitted the metric.
// Each component type uses a specific prefix in its metric names (e.g., input_received, processor_error, output_sent).
//
// Deprecated: It's now a regular string defined in models.Metric.ComponentType.
type DfcMetricComponentType string

const (
	DfcMetricComponentTypeInput     DfcMetricComponentType = "input"
	DfcMetricComponentTypeOutput    DfcMetricComponentType = "output"
	DfcMetricComponentTypeProcessor DfcMetricComponentType = "processor"
)
