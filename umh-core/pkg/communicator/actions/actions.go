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

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
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
	Parse(ctx context.Context, payload interface{}) error

	// Validate validates the action payload, returns an error if something is wrong.
	// This should perform deeper validation than Parse, checking business rules and constraints.
	Validate(ctx context.Context) error

	// Execute executes the action, returns the result as an interface and an error if something went wrong.
	// It must send ActionConfirmed and ActionExecuting messages for progress updates.
	// It must send ActionFinishedWithFailure messages if an error occurs.
	// It must not send the final successful action reply, as it is done by the caller.
	Execute(ctx context.Context) (interface{}, map[string]interface{}, error)

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

	// Create context for the entire action execution flow (3x [Parse, Validate, Execute])
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout*3)
	defer cancel()

	// Start a new transaction for this action
	log.Debugf("Handling action message: Type: %s, Payload: %v", payload.ActionType, payload.ActionPayload)

	var action Action

	switch payload.ActionType {
	case models.EditInstance:
		action = &EditInstanceAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
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
	case models.GetLogs:
		action = &GetLogsAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
		}
	case models.GetConfigFile:
		action = &GetConfigFileAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			systemSnapshotManager: systemSnapshotManager,
			configManager:         configManager,
			actionLogger:          log,
		}
	case models.SetConfigFile:
		action = &SetConfigFileAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			systemSnapshotManager: systemSnapshotManager,
			configManager:         configManager,
			actionLogger:          log,
		}

	case models.GetDataFlowComponentMetrics: //nolint:staticcheck // Deprecated but kept for back compat
		action = &GetDataflowcomponentMetricsAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
		}

	case models.DeployProtocolConverter:
		action = &DeployProtocolConverterAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			actionLogger:          log,
			systemSnapshotManager: systemSnapshotManager,
		}
	case models.EditProtocolConverter:
		action = &EditProtocolConverterAction{
			userEmail:             sender,
			actionUUID:            payload.ActionUUID,
			instanceUUID:          instanceUUID,
			outboundChannel:       outboundChannel,
			configManager:         configManager,
			systemSnapshotManager: systemSnapshotManager,
			actionLogger:          log,
		}
	case models.GetProtocolConverter:
		action = NewGetProtocolConverterAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)

	case models.GetMetrics:
		action = NewGetMetricsAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, systemSnapshotManager, log)
	case models.DeleteProtocolConverter:
		action = NewDeleteProtocolConverterAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	case models.AddDataModel:
		action = NewAddDataModelAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager)
	case models.DeleteDataModel:
		action = NewDeleteDataModelAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager)
	case models.EditDataModel:
		action = NewEditDataModelAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager)
	case models.GetDataModel:
		action = NewGetDataModelAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager)
	case models.DeleteStreamProcessor:
		action = NewDeleteStreamProcessorAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	case models.EditStreamProcessor:
		action = NewEditStreamProcessorAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	case models.DeployStreamProcessor:
		action = NewDeployStreamProcessorAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)
	case models.GetStreamProcessor:
		action = NewGetStreamProcessorAction(sender, payload.ActionUUID, instanceUUID, outboundChannel, configManager, systemSnapshotManager)

	// Explicitly handle unsupported action types
	case models.UnknownAction, models.DummyAction, models.TestBenthosInput, models.TestNetworkConnection,
		models.DeployConnection, models.EditConnection, models.DeleteConnection,
		models.GetConnectionNotes, models.DeployOPCUADatasource, models.GetDatasourceBasic,
		models.EditOPCUADatasource, models.DeleteDatasource, models.UpgradeCompanion, models.EditMqttBroker,
		models.GetAuditLog, models.EditInstanceLocation, models.GetDataFlowComponentLog,
		models.GetKubernetesEvents, models.GetOPCUATags, models.RollbackDataFlowComponent,
		models.AllowAppSecretAccess, models.GetConfiguration, models.UpdateConfiguration,
		models.DeployOPCUAConnection: //nolint:staticcheck // deprecated but must handle for exhaustive check
		log.Errorf("Unsupported action type: %s", payload.ActionType)
		SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Action type not implemented", outboundChannel, payload.ActionType)

		return

	default:
		log.Errorf("Unknown action type: %s", payload.ActionType)
		SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Unknown action type", outboundChannel, payload.ActionType)

		return
	}

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Parsing action payload", outboundChannel, payload.ActionType)
	// Parse the action payload
	err := action.Parse(ctx, payload.ActionPayload)
	if err != nil {
		// If parsing fails, send a structured error reply using SendActionReplyV2 with ErrRetryParseFailed
		// this will allow the UI to retry the action
		SendActionReplyV2(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Failed to parse action payload: "+err.Error(), models.ErrParseFailed, nil, outboundChannel, payload.ActionType, nil)
		log.Errorf("Error parsing action payload: %s", err)

		return
	}

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Validating action payload", outboundChannel, payload.ActionType)
	// Validate the action payload
	err = action.Validate(ctx)
	if err != nil {
		// If validation fails, send a structured error reply using SendActionReplyV2 with ErrEditValidationFailed
		SendActionReplyV2(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Failed to validate action payload: "+err.Error(), models.ErrValidationFailed, nil, outboundChannel, payload.ActionType, nil)
		log.Errorf("Error validating action payload: %s", err)

		return
	}

	SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionExecuting, "Executing action", outboundChannel, payload.ActionType)
	// Execute the action
	result, metadata, err := action.Execute(ctx)
	if err != nil {
		log.Errorf("Error executing action: %s", err)

		return
	}

	// Normally, the action.go logs the execution result that is sent back to the frontend. For the get-logs action,
	// this introduced a problem: in the frontend, we auto-refresh the logs of the companion. The get-logs action
	// reply is then the whole current log of the companion. If we log this action reply, we double the amount of
	// log lines on every get-logs call. This exponential growth of logs leads after a short time to problems.
	// To avoid this, we do not log the get-logs result.
	// This behavior is signaled to the frontend via the "log-logs-suppression" supported feature flag.
	if payload.ActionType != models.GetLogs {
		log.Debugf("Action executed, sending reply: %v", result)
	}

	SendActionReplyWithAdditionalContext(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedSuccessfull, result, outboundChannel, payload.ActionType, metadata)
}

// SendActionReply sends an action reply with the given state and payload.
// It is a convenience wrapper around SendActionReplyWithAdditionalContext that doesn't include additional context.
// It returns false if an error occurred during message generation or sending.
func SendActionReply(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, action models.ActionType) bool {
	// TODO: The 'action' parameter will be used in the future for action-specific logic or logging
	return SendActionReplyWithAdditionalContext(instanceUUID, userEmail, actionUUID, arstate, payload, outboundChannel, action, nil)
}

// SendActionReplyWithAdditionalContext sends an action reply with added context metadata.
// It is used for all user-facing communication about action progress and results.
// The actionContext parameter allows passing additional structured data with the reply.
//
// This is the primary method for sending action status messages to users, and is
// used for confirmation, progress updates, success, and failure notifications.
func SendActionReplyWithAdditionalContext(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, action models.ActionType, actionContext map[string]interface{}) bool {
	// TODO: The 'action' parameter will be used in the future for action-specific logic or logging
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
	var (
		err        error
		umhMessage models.UMHMessage
	)

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

// SendActionReplyV2 sends an action reply with the given state and payload which is a map[string]interface{}
// SendActionReplyV2 should be used only for ActionFailure messages for backwards compatibility. This will be changed in the future.
// This function is preferred over SendActionReply as it is more flexible and allows for more complex payloads
// The return type is a bool and returns false if an error occurred.
func SendActionReplyV2(
	instanceUUID uuid.UUID,
	userEmail string,
	actionUUID uuid.UUID,
	arstate models.ActionReplyState,
	message string,
	errorCode string,
	payloadV2 map[string]interface{},
	outboundChannel chan *models.UMHMessage,
	action models.ActionType,
	actionContext map[string]interface{},
) bool {
	// TODO: The 'action' parameter will be used in the future for action-specific logic or logging
	return sendActionReplyWithAdditionalContextV2(instanceUUID, userEmail, actionUUID, arstate, message, errorCode, payloadV2, outboundChannel, action, actionContext)
}

func sendActionReplyWithAdditionalContextV2(
	instanceUUID uuid.UUID,
	userEmail string,
	actionUUID uuid.UUID,
	arstate models.ActionReplyState,
	message string,
	errorCode string,
	payloadV2 map[string]interface{},
	outboundChannel chan *models.UMHMessage,
	action models.ActionType,
	actionContext map[string]interface{},
) bool {
	// TODO: The 'action' parameter will be used in the future for action-specific logic or logging
	err := sendActionReplyInternalV2(instanceUUID, userEmail, actionUUID, arstate, message, errorCode, payloadV2, outboundChannel, actionContext)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentCommunicator), "Error generating action reply: %w", err)

		return false
	}

	return true
}

func sendActionReplyInternalV2(
	instanceUUID uuid.UUID,
	userEmail string,
	actionUUID uuid.UUID,
	arstate models.ActionReplyState,
	message string,
	errorCode string,
	payloadV2 map[string]interface{},
	outboundChannel chan *models.UMHMessage,
	actionContext map[string]interface{},
) error {
	payloadResponse := ConstructActionReplyV2Response(message, errorCode, arstate, actionUUID.String(), payloadV2, actionContext)

	umhMessageV2, err := generateUMHMessage(instanceUUID, userEmail, models.ActionReply, payloadResponse)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, logger.For(logger.ComponentCommunicator), "Error generating umh message: %w", err)

		return err
	}

	outboundChannel <- &umhMessageV2

	return nil
}

// ConstructActionReplyV2Response creates a new ActionReplyResponseSchemaJson.
func ConstructActionReplyV2Response(
	message string,
	errorCode string,
	actionReplyState models.ActionReplyState,
	actionUUID string,
	payloadV2 map[string]interface{},
	actionContext models.ActionReplyResponseSchemaJsonActionContext,
) models.ActionReplyResponseSchemaJson {
	//  For backwards compatibility, we need to support the old payload format
	//  This will be removed in the future
	var payload interface{}
	if message != "" {
		payload = message
	}

	// if payload is still nil, we use the first message from payloadV2
	if payload == nil {
		// Get first message from map regardless of key
		payload = GetFirstMessageFromMap(payloadV2)
	}

	return models.ActionReplyResponseSchemaJson{
		ActionContext:      actionContext,
		ActionReplyPayload: payload,
		ActionReplyPayloadV2: &models.ActionReplyResponseSchemaJsonActionReplyPayloadV2{
			Message:   message,
			ErrorCode: &errorCode,
			Payload:   payloadV2,
		},
		ActionReplyState: models.ActionReplyResponseSchemaJsonActionReplyState(actionReplyState),
		ActionUUID:       actionUUID,
	}
}

func GetFirstMessageFromMap(msg map[string]interface{}) interface{} {
	for _, v := range msg {
		return v
	}

	return nil
}
