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

type GetDataflowcomponentMetricsAction struct {
	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetDataflowcomponentMetricsRequest

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger
}

func NewGetDataflowcomponentMetricsAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, systemSnapshotManager *fsm.SnapshotManager) *GetDataflowcomponentMetricsAction {
	return &GetDataflowcomponentMetricsAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		systemSnapshotManager: systemSnapshotManager,
	}
}

func (a *GetDataflowcomponentMetricsAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetDataflowcomponentMetricsRequest](payload)
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
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, "failed to find DFC instance", a.outboundChannel, models.GetDataFlowComponentMetrics)
		return nil, nil, err
	}

	metrics := dfcInstance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot).ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics

	return metrics, nil, nil
}

func (a *GetDataflowcomponentMetricsAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetDataflowcomponentMetricsAction) getUuid() uuid.UUID {
	return a.actionUUID
}

func (a *GetDataflowcomponentMetricsAction) GetParsedPayload() models.GetDataflowcomponentMetricsRequest {
	return a.payload
}
