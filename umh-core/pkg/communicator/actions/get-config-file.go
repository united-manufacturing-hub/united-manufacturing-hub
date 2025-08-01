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
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

type GetConfigFileAction struct {
	configManager config.ConfigManager

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger
	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewGetConfigFileAction creates a new GetConfigFileAction with the provided parameters.
func NewGetConfigFileAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, systemSnapshotManager *fsm.SnapshotManager, configManager config.ConfigManager) *GetConfigFileAction {
	return &GetConfigFileAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		systemSnapshotManager: systemSnapshotManager,
		configManager:         configManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
	}
}

// Parse extracts the business fields from the raw JSON payload.
// The GetConfigFile action doesn't require any payload, so this is a no-op.
func (a *GetConfigFileAction) Parse(payload interface{}) error {
	a.actionLogger.Info("Parsing GetConfigFile payload")
	// No payload to parse for this action
	return nil
}

// Validate performs semantic validation of the parsed payload.
// The GetConfigFile action doesn't require any payload, so this is a no-op.
func (a *GetConfigFileAction) Validate() error {
	a.actionLogger.Info("Validating GetConfigFile action")
	// No validation needed for this action
	return nil
}

// Execute takes care of retrieving the config file content.
func (a *GetConfigFileAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing GetConfigFile action")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), constants.GetOrSetConfigFileTimeout)
	defer cancel()

	// Use the default config path from the config manager
	configPath := config.DefaultConfigPath

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		fmt.Sprintf("Reading config file from %s", configPath), a.outboundChannel, models.GetConfigFile)

	// Get the config file content as string using the new method
	content, err := a.configManager.GetConfigAsString(ctx)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to read config file: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errMsg, a.outboundChannel, models.GetConfigFile)
		return nil, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// the GetConfigAsString call above updates the cache mod time in the config manager
	// so we can just get it without updating it (which would be a blocking operation)
	lastModifiedTime := a.configManager.GetCacheModTimeWithoutUpdate()

	// Return the file content as a string
	response := models.GetConfigFileResponse{
		Content:          content,
		LastModifiedTime: lastModifiedTime.Format(time.RFC3339),
	}

	return response, nil, nil
}

func (a *GetConfigFileAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetConfigFileAction) getUuid() uuid.UUID {
	return a.actionUUID
}
