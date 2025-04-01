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
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"go.uber.org/zap"
)

// editInstanceConfigManager extends config.ConfigManager with WriteConfig method
type editInstanceConfigManager interface {
	config.ConfigManager
	WriteConfig(ctx context.Context, config config.FullConfig) error
}

// EditInstanceAction implements the Action interface for editing an instance's properties
type EditInstanceAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	location        *EditInstanceLocation
	configManager   editInstanceConfigManager
}

// NewEditInstanceAction creates a new EditInstanceAction with default config manager
func NewEditInstanceAction(userEmail string, actionUUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage) *EditInstanceAction {
	return &EditInstanceAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   config.NewFileConfigManager(),
	}
}

// WithConfigManager allows setting a custom config manager for testing
func (a *EditInstanceAction) WithConfigManager(manager editInstanceConfigManager) *EditInstanceAction {
	a.configManager = manager
	return a
}

// EditInstanceLocation holds the location information for the instance
type EditInstanceLocation struct {
	Enterprise string  `json:"enterprise"`
	Site       *string `json:"site,omitempty"`
	Area       *string `json:"area,omitempty"`
	Line       *string `json:"line,omitempty"`
	WorkCell   *string `json:"workCell,omitempty"`
}

// Parse implements the Action interface
func (a *EditInstanceAction) Parse(payload interface{}) error {
	zap.S().Debug("Parsing EditInstance action payload")

	// Convert the payload to a map
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		return errors.New("invalid payload format, expected map")
	}

	// Check if we have a location field
	locationData, ok := payloadMap["location"]
	if !ok {
		// No location provided, which is fine since it's optional
		return nil
	}

	// Parse location data
	locationMap, ok := locationData.(map[string]interface{})
	if !ok {
		return errors.New("invalid location format, expected map")
	}

	// Extract enterprise (required)
	enterprise, ok := locationMap["enterprise"].(string)
	if !ok || enterprise == "" {
		return errors.New("missing or invalid enterprise in location")
	}

	// Create location object
	location := &EditInstanceLocation{
		Enterprise: enterprise,
	}

	// Extract optional fields
	if site, ok := locationMap["site"].(string); ok && site != "" {
		location.Site = &site
	}
	if area, ok := locationMap["area"].(string); ok && area != "" {
		location.Area = &area
	}
	if line, ok := locationMap["line"].(string); ok && line != "" {
		location.Line = &line
	}
	if workCell, ok := locationMap["workCell"].(string); ok && workCell != "" {
		location.WorkCell = &workCell
	}

	a.location = location
	return nil
}

// Validate implements the Action interface
func (a *EditInstanceAction) Validate() error {
	// If location is provided, validate that enterprise is not empty
	if a.location != nil && a.location.Enterprise == "" {
		return errors.New("enterprise cannot be empty when location is provided")
	}

	return nil
}

// Execute implements the Action interface
func (a *EditInstanceAction) Execute() (interface{}, map[string]interface{}, error) {
	zap.S().Info("Executing EditInstance action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting EditInstance action", a.outboundChannel, models.EditInstance)

	// If we don't have any location to update, return early
	if a.location == nil {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "No location changes requested", a.outboundChannel, models.EditInstance)
		return "No changes were made to the instance", nil, nil
	}

	// Send progress update
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Updating instance location", a.outboundChannel, models.EditInstance)

	// Update the location in the configuration
	err := a.updateLocation()
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to update instance location: %s", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.EditInstance)
		return nil, nil, fmt.Errorf("failed to update instance location: %w", err)
	}

	// Create success message based on the provided location
	message := "Successfully updated instance location"

	// Send the success message
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull, "Location update completed", a.outboundChannel, models.EditInstance)

	return message, nil, nil
}

// updateLocation updates the location in the configuration file
func (a *EditInstanceAction) updateLocation() error {
	// Create a context with timeout for config operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get the current configuration using the configManager
	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get current configuration: %w", err)
	}

	// Initialize the location map if it doesn't exist
	if currentConfig.Agent.Location == nil {
		currentConfig.Agent.Location = make(map[int]string)
	}

	// Update the location in the configuration
	// Location is a hierarchical structure represented as map[int]string
	// 0: Enterprise, 1: Site, 2: Area, 3: Line, 4: WorkCell
	currentConfig.Agent.Location[0] = a.location.Enterprise

	// Update optional fields if they exist
	if a.location.Site != nil {
		currentConfig.Agent.Location[1] = *a.location.Site
	}
	if a.location.Area != nil {
		currentConfig.Agent.Location[2] = *a.location.Area
	}
	if a.location.Line != nil {
		currentConfig.Agent.Location[3] = *a.location.Line
	}
	if a.location.WorkCell != nil {
		currentConfig.Agent.Location[4] = *a.location.WorkCell
	}

	// Write the updated configuration back to disk
	err = a.configManager.WriteConfig(ctx, currentConfig)
	if err != nil {
		return fmt.Errorf("failed to write updated configuration: %w", err)
	}

	return nil
}

// getUserEmail implements the Action interface
func (a *EditInstanceAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface
func (a *EditInstanceAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// Methods added for testing purposes

// GetLocation returns the location - for testing
func (a *EditInstanceAction) GetLocation() *EditInstanceLocation {
	return a.location
}

// SetLocation sets the location - for testing
func (a *EditInstanceAction) SetLocation(location *EditInstanceLocation) {
	a.location = location
}
