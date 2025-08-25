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
	"bytes"
	"context"
	"fmt"
	"time"

	"errors"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

type SetConfigFileAction struct {
	configManager config.ConfigManager

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger

	// ─── Request payload ───────────────────────────────────────────────────
	payload models.SetConfigFilePayload

	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewSetConfigFileAction creates a new SetConfigFileAction with the provided parameters.
func NewSetConfigFileAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, systemSnapshotManager *fsm.SnapshotManager, configManager config.ConfigManager) *SetConfigFileAction {
	return &SetConfigFileAction{
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
func (a *SetConfigFileAction) Parse(payload interface{}) error {
	a.actionLogger.Info("Parsing SetConfigFile payload")

	// Extract SetConfigFilePayload from the interface{}
	payloadStruct, err := ParseActionPayload[models.SetConfigFilePayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload as SetConfigFilePayload: %w", err)
	}

	a.payload = payloadStruct

	return nil
}

// Validate performs semantic validation of the parsed payload.
func (a *SetConfigFileAction) Validate() error {
	a.actionLogger.Info("Validating SetConfigFile action")

	// Ensure content is not empty
	if a.payload.Content == "" {
		return errors.New("config file content cannot be empty")
	}

	// Validate YAML format by trying to parse it with yaml.v3, but allow unknown fields
	// This will permit YAML anchors while still checking basic YAML syntax
	var yamlContent interface{}

	dec := yaml.NewDecoder(bytes.NewReader([]byte(a.payload.Content)))
	dec.KnownFields(false) // Allow unknown fields (including anchors)

	if err := dec.Decode(&yamlContent); err != nil {
		return fmt.Errorf("invalid YAML content: %w", err)
	}

	// Ensure LastModifiedTime is not zero
	if a.payload.LastModifiedTime == "" {
		return errors.New("last modified time cannot be zero")
	}

	return nil
}

// Execute takes care of updating the config file content.
func (a *SetConfigFileAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing SetConfigFile action")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), constants.GetOrSetConfigFileTimeout)
	defer cancel()

	// Use the default config path from the config manager
	configPath := config.DefaultConfigPath

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Updating config file at "+configPath, a.outboundChannel, models.SetConfigFile)

	// Write the new content to the file with atomic concurrent modification check
	err := a.configManager.WriteYAMLConfigFromString(ctx, a.payload.Content, a.payload.LastModifiedTime)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to write config file: %v", err)
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errMsg, models.ErrRetryConfigWriteFailed, nil, a.outboundChannel, models.SetConfigFile, nil)

		return nil, nil, fmt.Errorf("failed to write config file: %w", err)
	}

	// the WriteYAMLConfigFromString call above does not update the cache mod time in the config manager
	newLastModifiedTime, err := a.configManager.UpdateAndGetCacheModTime(ctx)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to update cache mod time: %v", err)
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errMsg, models.ErrGetCacheModTimeFailed, nil, a.outboundChannel, models.SetConfigFile, nil)

		return nil, nil, fmt.Errorf("failed to update cache mod time: %w", err)
	}

	newLastModifiedTimeString := newLastModifiedTime.Format(time.RFC3339)

	// Return the new last modified time
	response := models.SetConfigFileResponse{
		Content:          a.payload.Content,
		LastModifiedTime: newLastModifiedTimeString,
		Success:          true,
	}

	return response, nil, nil
}

func (a *SetConfigFileAction) getUserEmail() string {
	return a.userEmail
}

func (a *SetConfigFileAction) getUuid() uuid.UUID {
	return a.actionUUID
}
