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

// Package actions contains implementations of the Action interface that create
// and manage historian configurations in the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// "Deploying" a historian writes (or overwrites) the top-level `historian:`
// section in config.yaml with the supplied TimescaleDB/Postgres connection
// settings. The historian section is a singleton — there is at most one per
// instance. If a historian is already configured, use the edit-historian action
// instead to preserve the audit trail of intent.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// DeployHistorianAction implements the Action interface for writing the historian
// configuration into config.yaml. All fields are immutable after construction.
type DeployHistorianAction struct {
	configManager   config.ConfigManager
	outboundChannel chan *models.UMHMessage
	actionLogger    *zap.SugaredLogger

	userEmail    string
	payload      config.HistorianConfig
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewDeployHistorianAction returns an un-parsed action instance.
func NewDeployHistorianAction(
	userEmail string,
	actionUUID uuid.UUID,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	configManager config.ConfigManager,
) *DeployHistorianAction {
	return &DeployHistorianAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface.
func (a *DeployHistorianAction) Parse(payload interface{}) error {
	parsed, err := ParseActionPayload[config.HistorianConfig](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsed

	a.actionLogger.Debugf("Parsed DeployHistorian action payload: host=%s port=%d database=%s",
		a.payload.Host, a.payload.Port, a.payload.Database)

	return nil
}

// Validate implements the Action interface.
func (a *DeployHistorianAction) Validate() error {
	return a.payload.Validate()
}

// getUserEmail implements the Action interface.
func (a *DeployHistorianAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface.
func (a *DeployHistorianAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// Execute implements the Action interface by writing the historian config.
func (a *DeployHistorianAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeployHistorian action")

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting deployment of Historian", a.outboundChannel, models.DeployHistorian)

	cfg := a.payload.WithDefaults()

	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Writing Historian configuration...", a.outboundChannel, models.DeployHistorian)

	if err := a.configManager.AtomicSetHistorian(ctx, cfg); err != nil {
		errorMsg := fmt.Sprintf("Failed to write Historian configuration: %v", err)
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, models.ErrRetryConfigWriteFailed, nil, a.outboundChannel, models.DeployHistorian, nil)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// The terminal ActionFinishedSuccessfull reply is sent by the caller (see actions.go).
	return cfg, nil, nil
}
