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
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
)

type GetLogsAction struct {
	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetLogsRequest

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger
}

// NewGetLogsAction creates a new GetLogsAction with the provided parameters.
// This constructor is primarily used for testing to enable dependency injection.
// Caller needs to invoke Parse and Validate before calling Execute.
func NewGetLogsAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, systemSnapshotManager *fsm.SnapshotManager) *GetLogsAction {
	return &GetLogsAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
	}
}

// Parse extracts the business fields from the raw JSON payload.
// Shape errors are detected here, while semantic validation is done in Validate.
func (a *GetLogsAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetLogsRequest](payload)
	a.actionLogger.Info("Payload parsed: %v", a.payload)
	return err
}

// Validate performs semantic validation of the parsed payload.
// This includes checking that the provided start time is a valid timestamp,
// and that the log type is one of the allowed types.
// The UUID is necessary for DFC logs to identify the correct instance.
func (a *GetLogsAction) Validate() (err error) {
	a.actionLogger.Info("Validating the payload")

	if a.payload.StartTime <= 0 {
		return errors.New("start time must be greater than 0")
	}

	if a.payload.StartTime > time.Now().UnixMilli() {
		return errors.New("start time must be in the past")
	}

	allowedLogTypes := []models.LogType{models.AgentLogType, models.DFCLogType, models.RedpandaLogType, models.TagBrowserLogType}
	if !slices.Contains(allowedLogTypes, a.payload.Type) {
		return errors.New("log type must be set and must be one of the following: agent, dfc, redpanda, tag-browser")
	}

	if a.payload.Type == models.DFCLogType {
		if a.payload.UUID == "" {
			return errors.New("uuid must be set to retrieve logs for a DFC")
		}

		_, err = uuid.Parse(a.payload.UUID)
		if err != nil {
			return fmt.Errorf("invalid UUID format: %v", err)
		}
	}

	return nil
}

// mapS6LogsToSlice maps the S6 logs to a slice of strings.
// It filters out logs that are before the provided start time.
func mapS6LogsToSlice(s6Logs []s6.LogEntry, startTimeUTC time.Time) []string {
	logs := []string{}

	for _, log := range s6Logs {
		if log.Timestamp.Before(startTimeUTC) {
			continue
		}

		logs = append(logs, log.Content)
	}

	return logs
}

func logsRetrievalError(err error, logType models.LogType) error {
	return fmt.Errorf("failed to retrieve logs for %s: %v", logType, err)
}

// Execute takes care of retrieving the logs from the correct source based on the log type.
// It returns a response object with an array of logs from the provided start time up to the current time.
func (a *GetLogsAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing GetLogs action")

	// Request time is in unix ms, but log entries contain UTC timestamps
	reqStartTime := time.UnixMilli(a.payload.StartTime).UTC()
	logType := a.payload.Type

	res := models.GetLogsResponse{Logs: []string{}}
	systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

	switch logType {
	case models.RedpandaLogType:
		redpandaInst, ok := fsm.FindInstance(systemSnapshot, constants.RedpandaManagerName, constants.RedpandaInstanceName)
		if !ok || redpandaInst == nil {
			err := logsRetrievalError(fmt.Errorf("redpanda instance not found"), logType)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetLogs)
			return nil, nil, err
		}

		observedState, ok := redpandaInst.LastObservedState.(*redpanda.RedpandaObservedStateSnapshot)
		if !ok || observedState == nil {
			err := logsRetrievalError(fmt.Errorf("invalid observed state type for redpanda instance %s", redpandaInst.ID), logType)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetLogs)
			return nil, nil, err
		}

		res.Logs = mapS6LogsToSlice(observedState.ServiceInfoSnapshot.RedpandaStatus.Logs, reqStartTime)
	case models.AgentLogType:
		agentInstance, ok := fsm.FindInstance(systemSnapshot, constants.AgentManagerName, constants.AgentInstanceName)
		if !ok || agentInstance == nil {
			err := logsRetrievalError(fmt.Errorf("agent instance not found"), logType)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetLogs)
			return nil, nil, err
		}

		observedState, ok := agentInstance.LastObservedState.(*agent_monitor.AgentObservedStateSnapshot)
		if !ok || observedState == nil {
			err := logsRetrievalError(fmt.Errorf("invalid observed state type for agent instance %s", agentInstance.ID), logType)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetLogs)
			return nil, nil, err
		}

		res.Logs = mapS6LogsToSlice(observedState.ServiceInfoSnapshot.AgentLogs, reqStartTime)
	case models.DFCLogType:
		dfcInstance, err := fsm.FindDfcInstanceByUUID(systemSnapshot, a.payload.UUID)
		if err != nil || dfcInstance == nil {
			err := logsRetrievalError(err, logType)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetLogs)
			return nil, nil, err
		}

		observedState, ok := dfcInstance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
		if !ok || observedState == nil {
			err := logsRetrievalError(fmt.Errorf("invalid observed state type for DFC instance %s", dfcInstance.ID), logType)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetLogs)
			return nil, nil, err
		}

		res.Logs = mapS6LogsToSlice(observedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs, reqStartTime)
	case models.TagBrowserLogType:
		// TODO: Implement tag browser logs
		err := errors.New("tag-browser logs are not implemented yet")
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, err.Error(), a.outboundChannel, models.GetLogs)
		return nil, nil, err
	}

	return res, nil, nil
}

func (a *GetLogsAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetLogsAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// for testing
func (a *GetLogsAction) GetPayload() models.GetLogsRequest {
	return a.payload
}
