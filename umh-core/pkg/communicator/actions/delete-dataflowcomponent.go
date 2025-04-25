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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// DeleteDataflowComponentAction implements the Action interface for deleting
// dataflow components from the UMH instance.
type DeleteDataflowComponentAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager
	systemSnapshot  *fsm.SystemSnapshot
	componentUUID   uuid.UUID
	actionLogger    *zap.SugaredLogger
}

// NewDeleteDataflowComponentAction creates a new DeleteDataflowComponentAction with the provided parameters.
// This constructor is primarily used for testing to enable dependency injection, though it can be used
// in production code as well. It initializes the action with the necessary fields but doesn't
// populate the component UUID field which must be done via Parse.
func NewDeleteDataflowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *DeleteDataflowComponentAction {
	return &DeleteDataflowComponentAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting component UUID from the payload.
// It parses the UUID string into a valid UUID object for later use.
func (a *DeleteDataflowComponentAction) Parse(payload interface{}) error {
	// Parse the payload to get the UUID
	parsedPayload, err := ParseActionPayload[models.DeleteDFCPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %v", err)
	}

	// Validate UUID is provided
	if parsedPayload.UUID == "" {
		return errors.New("missing required field UUID")
	}

	// Parse string UUID into UUID object
	componentUUID, err := uuid.Parse(parsedPayload.UUID)
	if err != nil {
		return fmt.Errorf("invalid UUID format: %v", err)
	}

	a.componentUUID = componentUUID
	a.actionLogger.Debugf("Parsed DeleteDataFlowComponent action payload: UUID=%s", a.componentUUID)

	return nil
}

// Validate implements the Action interface. For this action, validation is minimal
// as the only requirement is a valid UUID, which is already checked during parsing.
func (a *DeleteDataflowComponentAction) Validate() error {
	// UUID validation is already done in Parse, so there's not much additional validation needed
	if a.componentUUID == uuid.Nil {
		return errors.New("component UUID is missing or invalid")
	}

	return nil
}

// Execute implements the Action interface by performing the actual deletion of the dataflow component.
// It follows the standard pattern for actions:
// 1. Sends ActionConfirmed to indicate the action is starting
// 2. Attempts to delete the component by UUID
// 3. Sends ActionFinishedWithFailure if any error occurs
// 4. Returns a success message (not sending ActionFinishedSuccessfull as that's done by the caller)
func (a *DeleteDataflowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeleteDataflowComponent action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting DeleteDataflowComponent", a.outboundChannel, models.DeleteDataFlowComponent)

	// Delete the component from configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	err := a.configManager.AtomicDeleteDataflowcomponent(ctx, a.componentUUID)
	if err != nil {
		errorMsg := fmt.Sprintf("failed to delete dataflow component: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.DeleteDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// wait for the component to be removed
	if a.systemSnapshot != nil { // skipping this for the unit tests
		err = a.waitForComponentToBeRemoved()
		if err != nil {
			errorMsg := fmt.Sprintf("failed to wait for dataflowcomponent to be removed: %v", err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.DeleteDataFlowComponent)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	}

	// return success message, but do not send it as this is done by the caller
	successMsg := fmt.Sprintf("Successfully deleted data flow component with UUID: %s", a.componentUUID)

	return successMsg, nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *DeleteDataflowComponentAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *DeleteDataflowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetComponentUUID returns the UUID of the component to be deleted - exposed primarily for testing purposes.
func (a *DeleteDataflowComponentAction) GetComponentUUID() uuid.UUID {
	return a.componentUUID
}

func (a *DeleteDataflowComponentAction) waitForComponentToBeRemoved() error {
	//check the system snapshot and waits for the instance to be removed
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	for {
		select {
		case <-timeout:
			return fmt.Errorf("dataflowcomponent %s was not removed in time", a.componentUUID)
		case <-ticker.C:
			if dataflowcomponentManager, exists := a.systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
				instances := dataflowcomponentManager.GetInstances()
				for _, instance := range instances {
					if dataflowcomponentconfig.GenerateUUIDFromName(instance.ID) == a.componentUUID {
						// component is still there, so we need to wait longer
						continue
					}
					return nil
				}
			}
		}
	}
}
