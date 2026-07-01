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
// GetHistorian reads the current `historian:` section from config.yaml and
// returns it to the caller.
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

// GetHistorianAction implements the Action interface for reading the historian
// configuration. All fields are immutable after construction.
type GetHistorianAction struct {
	configManager   config.ConfigManager
	outboundChannel chan *models.UMHMessage
	actionLogger    *zap.SugaredLogger

	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewGetHistorianAction returns an un-parsed action instance.
func NewGetHistorianAction(
	userEmail string,
	actionUUID uuid.UUID,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	configManager config.ConfigManager,
) *GetHistorianAction {
	return &GetHistorianAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface. GetHistorian carries no payload.
func (a *GetHistorianAction) Parse(_ interface{}) error {
	return nil
}

// Validate implements the Action interface. Nothing to validate for a read.
func (a *GetHistorianAction) Validate() error {
	return nil
}

// getUserEmail implements the Action interface.
func (a *GetHistorianAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface.
func (a *GetHistorianAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// Execute implements the Action interface by reading and returning the historian config.
func (a *GetHistorianAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing GetHistorian action")

	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	cfg, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to read configuration: %v", err)
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, models.ErrConfigFileInvalid, nil, a.outboundChannel, models.GetHistorian, nil)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	if cfg.Historian == nil {
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			"Historian is not configured", models.ErrValidationFailed, nil, a.outboundChannel, models.GetHistorian, nil)

		return nil, nil, nil
	}

	response := *cfg.Historian

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull,
		response, a.outboundChannel, models.GetHistorian)

	return response, nil, nil
}
