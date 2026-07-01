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
// "Deleting" a historian removes the `historian:` section from config.yaml
// entirely. The action is a no-op (succeeds silently) when no historian is
// configured, to keep the frontend delete flow idempotent.
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

// DeleteHistorianAction implements the Action interface for removing the historian
// section from config.yaml. All fields are immutable after construction.
type DeleteHistorianAction struct {
	configManager   config.ConfigManager
	outboundChannel chan *models.UMHMessage
	actionLogger    *zap.SugaredLogger

	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewDeleteHistorianAction returns an un-parsed action instance.
func NewDeleteHistorianAction(
	userEmail string,
	actionUUID uuid.UUID,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	configManager config.ConfigManager,
) *DeleteHistorianAction {
	return &DeleteHistorianAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface. DeleteHistorian carries no payload.
func (a *DeleteHistorianAction) Parse(_ interface{}) error {
	return nil
}

// Validate implements the Action interface. Nothing to validate for a delete.
func (a *DeleteHistorianAction) Validate() error {
	return nil
}

// getUserEmail implements the Action interface.
func (a *DeleteHistorianAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface.
func (a *DeleteHistorianAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// Execute implements the Action interface by removing the historian config.
func (a *DeleteHistorianAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeleteHistorian action")

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting deletion of Historian", a.outboundChannel, models.DeleteHistorian)

	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Removing Historian configuration...", a.outboundChannel, models.DeleteHistorian)

	if err := a.configManager.AtomicDeleteHistorian(ctx); err != nil {
		errorMsg := fmt.Sprintf("Failed to delete Historian configuration: %v", err)
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, models.ErrRetryConfigWriteFailed, nil, a.outboundChannel, models.DeleteHistorian, nil)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull,
		"Historian deleted successfully", a.outboundChannel, models.DeleteHistorian)

	return nil, nil, nil
}
