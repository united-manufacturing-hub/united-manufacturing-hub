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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions/providers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type GetMetricsAction struct {

	// ─── Utilities ──────────────────────────────────────────────────────────
	provider providers.MetricsProvider

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	actionLogger *zap.SugaredLogger

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetMetricsRequest

	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewGetMetricsAction creates a new GetMetricsAction with the provided parameters.
// Caller needs to invoke Parse and Validate before calling Execute.
func NewGetMetricsAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, systemSnapshotManager *fsm.SnapshotManager, logger *zap.SugaredLogger) *GetMetricsAction {
	return &GetMetricsAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		systemSnapshotManager: systemSnapshotManager,
		provider:              &providers.DefaultMetricsProvider{},
		actionLogger:          logger.With("action", "GetMetricsAction"),
	}
}

// For testing - allow injection of a custom provider
func NewGetMetricsActionWithProvider(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, systemSnapshotManager *fsm.SnapshotManager, logger *zap.SugaredLogger, provider providers.MetricsProvider) *GetMetricsAction {
	action := NewGetMetricsAction(userEmail, actionUUID, instanceUUID, outboundChannel, systemSnapshotManager, logger)
	action.provider = provider
	return action
}

// Parse extracts the business fields from the raw JSON payload.
// Shape errors are detected here, while semantic validation is done in Validate.
func (a *GetMetricsAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetMetricsRequest](payload)
	a.actionLogger.Infow("Payload parsed", "payload", a.payload)
	return err
}

// Validate performs semantic validation of the parsed payload.
// This verifies that the metric type is allowed and that the UUID is valid for DFC metrics.
func (a *GetMetricsAction) Validate() (err error) {
	a.actionLogger.Info("Validating the payload")

	allowedMetricTypes := []models.MetricResourceType{models.DFCMetricResourceType, models.RedpandaMetricResourceType, models.TopicBrowserMetricResourceType, models.StreamProcessorMetricResourceType, models.ProtocolConverterMetricResourceType}
	if !slices.Contains(allowedMetricTypes, a.payload.Type) {
		return errors.New("metric type must be set and must be one of the following: dfc, redpanda, topic-browser, stream-processor, protocol-converter")
	}

	switch a.payload.Type {
	case models.DFCMetricResourceType:
		if a.payload.UUID == "" {
			return errors.New("uuid must be set to retrieve metrics for a DFC")
		}
		_, err = uuid.Parse(a.payload.UUID)
		if err != nil {
			return fmt.Errorf("invalid UUID format: %v", err)
		}
	case models.StreamProcessorMetricResourceType:
		if a.payload.UUID == "" {
			return errors.New("uuid must be set to retrieve metrics for a Stream Processor")
		}
		_, err = uuid.Parse(a.payload.UUID)
		if err != nil {
			return fmt.Errorf("invalid UUID format: %v", err)
		}
	case models.ProtocolConverterMetricResourceType:
		if a.payload.UUID == "" {
			return errors.New("uuid must be set to retrieve metrics for a Protocol Converter")
		}
		_, err = uuid.Parse(a.payload.UUID)
		if err != nil {
			return fmt.Errorf("invalid UUID format: %v", err)
		}
	}

	return nil
}

// Execute retrieves the metrics from the correct source based on the metric type.
// It returns a response object with an array of metrics or an error if the retrieval fails.
func (a *GetMetricsAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing the action")

	metrics, err := a.provider.GetMetrics(a.payload, a.systemSnapshotManager.GetDeepCopySnapshot())
	if err != nil {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetMetrics)
		return nil, nil, err
	}

	return metrics, nil, nil
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
