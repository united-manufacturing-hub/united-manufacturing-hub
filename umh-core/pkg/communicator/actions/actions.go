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
	"fmt"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

// Action is the interface that all action types must implement.
// It defines the core lifecycle methods for parsing, validating, and executing actions.
type Action interface {
	// Parse parses the ActionMessagePayload into the corresponding action type.
	// It should extract and validate all required fields from the raw payload.
	Parse(interface{}) error

	// Validate validates the action payload, returns an error if something is wrong.
	// This should perform deeper validation than Parse, checking business rules and constraints.
	Validate() error

	// Execute executes the action, returns the result as an interface and an error if something went wrong.
	// It must send ActionConfirmed and ActionExecuting messages for progress updates.
	// It must send ActionFinishedWithFailure messages if an error occurs.
	// It must not send the final successfull action reply, as it is done by the caller.
	Execute() (interface{}, map[string]interface{}, error)

	// getUserEmail returns the user email of the action
	getUserEmail() string

	// getUuid returns the UUID of the action
	getUuid() uuid.UUID
}

// HandleActionMessage is the main entry point for processing action messages.
// It identifies the action type, creates the appropriate action implementation,
// and processes it through the Parse->Validate->Execute flow.
//
// After execution, it handles sending the success reply if the action completed successfully.
// Error handling for each step is done within this function.
func HandleActionMessage(instanceUUID uuid.UUID, payload models.ActionMessagePayload, sender string, outboundChannel chan *models.UMHMessage, releaseChannel config.ReleaseChannel, dog watchdog.Iface, traceID uuid.UUID, systemSnapshotManager *fsm.SnapshotManager, configManager config.ConfigManager) {
	log := logger.For(logger.ComponentCommunicator)

	// Start a new transaction for this action
	log.Debugf("Handling action message: Type: %s, Payload: %v", payload.ActionType, payload.ActionPayload)

	var action Action
	switch payload.ActionType {

	case models.EditInstance:
		action = &EditInstanceAction{
			userEmail:       sender,
			actionUUID:      payload.ActionUUID,
			instanceUUID:    instanceUUID,
			outboundChannel: outboundChannel,
			configManager:   configManager,
			actionLogger:    log,
		}
	case models.DeployDataFlowComponent:
		action = &DeployDataflowComponentAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			systemSnapshotManager: systemSnapshotManager,
			actionLogger:          log,
		}

	case models.DeleteDataFlowComponent:
		action = &DeleteDataflowComponentAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			systemSnapshotManager: systemSnapshotManager,
			actionLogger:          log,
		}

	case models.GetDataFlowComponent:
		action = &GetDataFlowComponentAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			systemSnapshotManager: systemSnapshotManager,
			actionLogger:          log,
		}
	case models.EditDataFlowComponent:
		action = &EditDataflowComponentAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
		}

	default:
		log.Errorf("Unknown action type: %s", payload.ActionType)
		SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Unknown action type", outboundChannel, payload.ActionType)
		return
	}

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Parsing action payload", outboundChannel, payload.ActionType)
	// Parse the action payload
	err := action.Parse(payload.ActionPayload)
	if err != nil {
		log.Errorf("Error parsing action payload: %s", err)
		return
	}

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Validating action payload", outboundChannel, payload.ActionType)
	// Validate the action payload
	err = action.Validate()
	if err != nil {
		log.Errorf("Error validating action payload: %s", err)
		return
	}

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Executing action", outboundChannel, payload.ActionType)
	// Execute the action
	result, metadata, err := action.Execute()
	if err != nil {
		log.Errorf("Error executing action: %s", err)
		return
	}

	log.Debugf("Action executed, sending reply: %v", result)

	SendActionReplyWithAdditionalContext(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedSuccessfull, result, outboundChannel, payload.ActionType, metadata)
}

// SendActionReply sends an action reply with the given state and payload.
// It is a convenience wrapper around SendActionReplyWithAdditionalContext that doesn't include additional context.
// It returns false if an error occurred during message generation or sending.
//
// Deprecated: Use SendActionReplyV2 instead. This function accepts payload of type interface{} which is discouraged for further usage.
func SendActionReply(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, action models.ActionType) bool {
	return SendActionReplyWithAdditionalContext(instanceUUID, userEmail, actionUUID, arstate, payload, outboundChannel, action, nil)
}

// SendActionReplyWithAdditionalContext sends an action reply with added context metadata.
// It is used for all user-facing communication about action progress and results.
// The actionContext parameter allows passing additional structured data with the reply.
//
// This is the primary method for sending action status messages to users, and is
// used for confirmation, progress updates, success, and failure notifications.
func SendActionReplyWithAdditionalContext(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, action models.ActionType, actionContext map[string]interface{}) bool {
	err := sendActionReplyInternal(instanceUUID, userEmail, actionUUID, arstate, payload, outboundChannel, actionContext)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentCommunicator), "Error generating action reply: %w", err)
		return false
	}
	return true
}

// sendActionReplyInternal is the internal implementation for SendActionReply.
// It handles the actual process of creating and sending UMH messages.
// This function is only meant to be called within the actions.go file!
// Use SendActionReply instead (or SendActionReplyWithAdditionalContext if you need to pass additional context).
func sendActionReplyInternal(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, actionContext map[string]interface{}) error {
	var err error
	var umhMessage models.UMHMessage
	if actionContext == nil {
		umhMessage, err = generateUMHMessage(instanceUUID, userEmail, models.ActionReply, models.ActionReplyMessagePayload{
			ActionUUID:         actionUUID,
			ActionReplyState:   arstate,
			ActionReplyPayload: payload,
		})
	} else {
		umhMessage, err = generateUMHMessage(instanceUUID, userEmail, models.ActionReply, models.ActionReplyMessagePayload{
			ActionUUID:         actionUUID,
			ActionReplyState:   arstate,
			ActionReplyPayload: payload,
			ActionContext:      actionContext,
		})
	}
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentCommunicator), "Error generating umh message: %v", err)
		return err
	}
	outboundChannel <- &umhMessage

	return nil
}

// generateUMHMessage creates a UMHMessage with the specified parameters.
// It handles the encryption of message content before adding it to the UMHMessage.
//
// There is no check for matching message type and payload, so ensure the payload
// is compatible with the message type. The content is encrypted using the
// encoding package before being added to the message.
func generateUMHMessage(instanceUUID uuid.UUID, userEmail string, messageType models.MessageType, payload any) (umhMessage models.UMHMessage, err error) {
	messageContent := models.UMHMessageContent{
		MessageType: messageType,
		Payload:     payload,
	}

	encryptedContent, err := encoding.EncodeMessageFromUMHInstanceToUser(messageContent)
	if err != nil {
		return
	}

	umhMessage = models.UMHMessage{
		Email:        userEmail,
		Content:      encryptedContent,
		InstanceUUID: instanceUUID,
	}

	return
}

// ParseActionPayload is a generic helper function that converts raw payload data into a typed struct.
// It handles the conversion from interface{} -> map -> JSON -> typed struct safely.
//
// This function is particularly useful for parsing nested structures within action payloads,
// and provides consistent error handling for payload parsing.
//
// Example usage:
//
//	myPayload, err := ParseActionPayload[MyCustomStruct](actionPayload)
func ParseActionPayload[T any](actionPayload interface{}) (T, error) {
	var payload T

	rawMap, ok := actionPayload.(map[string]interface{})
	if !ok {
		return payload, fmt.Errorf("could not assert ActionPayload to map[string]interface{}. Actual type: %T, Value: %v", actionPayload, actionPayload)
	}

	// Marshal the raw payload into JSON bytes
	jsonData, err := safejson.Marshal(rawMap)
	if err != nil {
		return payload, fmt.Errorf("error marshaling raw payload: %w", err)
	}

	// Unmarshal the JSON bytes into the specified type
	err = safejson.Unmarshal(jsonData, &payload)
	if err != nil {
		return payload, fmt.Errorf("error unmarshaling into target type: %w", err)
	}

	return payload, nil
}
