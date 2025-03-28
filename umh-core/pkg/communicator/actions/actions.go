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
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

type Action interface {
	// Parse parses the ActionMessagePayload into the corresponding action type.
	Parse(interface{}) error
	// Validate validates the action payload, returns an error if something is wrong
	Validate() error
	// Execute executes the action, returns the result as an interface and an error if something went wrong
	// It must not send the final action reply, as it is done by the caller.
	Execute() (interface{}, map[string]interface{}, error)
	// getUserEmail returns the user email of the action
	getUserEmail() string
	// getUuid returns the UUID of the action
	getUuid() uuid.UUID
}

func HandleActionMessage(instanceUUID uuid.UUID, payload models.ActionMessagePayload, sender string, outboundChannel chan *models.UMHMessage, releaseChannel config.ReleaseChannel, dog watchdog.Iface, traceID uuid.UUID) {
	// Start a new transaction for this action
	zap.S().Infof("Handling action message: Type: %s, Payload: %v", payload.ActionType, payload.ActionPayload)

	var action Action
	switch payload.ActionType {
	case models.DummyAction:
		action = &DummyAction{
			userEmail:       sender,
			actionUUID:      payload.ActionUUID,
			instanceUUID:    instanceUUID,
			outboundChannel: outboundChannel,
		}
	default:
		zap.S().Errorf("Unknown action type: %s", payload.ActionType)
		SendActionReply(instanceUUID, sender, payload.ActionUUID, models.ActionFinishedWithFailure, "Unknown action type", outboundChannel, payload.ActionType)
		return
	}

	// Parse the action payload
	err := action.Parse(payload.ActionPayload)
	if err != nil {
		zap.S().Errorf("Error parsing action payload: %s", err)
		return
	}

	// Validate the action payload
	err = action.Validate()
	if err != nil {
		zap.S().Errorf("Error validating action payload: %s", err)
		return
	}

	// Execute the action
	result, metadata, err := action.Execute()
	if err != nil {
		zap.S().Errorf("Error executing action: %s", err)
		return
	}

	// Send the action result to the outbound channel
	outboundChannel <- &models.UMHMessage{
		Content: string(safejson.MustMarshal(models.UMHMessageContent{
			MessageType: models.ActionReply,
			Payload: models.ActionReplyMessagePayload{
				ActionReplyState:   models.ActionFinishedSuccessfull,
				ActionReplyPayload: result,
				ActionUUID:         payload.ActionUUID,
				ActionContext:      metadata,
			},
		})),
		Email:        sender,
		InstanceUUID: instanceUUID,
		Metadata: &models.MessageMetadata{
			TraceID: traceID,
		},
	}
}

// SendActionReply sends an action reply with the given state and payload
// and returns false if an error occurred
// Deprecated: Use SendActionReplyV2 instead. This function accepts payload of type interface{} which is discouraged for further usage.
func SendActionReply(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, action models.ActionType) bool {
	return SendActionReplyWithAdditionalContext(instanceUUID, userEmail, actionUUID, arstate, payload, outboundChannel, action, nil)
}

// SendActionReplyWithAdditionalContext is the same as SendActionReply but with additional context
func SendActionReplyWithAdditionalContext(instanceUUID uuid.UUID, userEmail string, actionUUID uuid.UUID, arstate models.ActionReplyState, payload interface{}, outboundChannel chan *models.UMHMessage, action models.ActionType, actionContext map[string]interface{}) bool {
	// zap.S().Debugf("SendingActionReply [InstanceUUID: %s, UserEmail: %s, ActionUUID: %s, ActionReplyState: %s, Payload: %v]", instanceUUID, userEmail, actionUUID, arstate, payload)

	err := sendActionReplyInternal(instanceUUID, userEmail, actionUUID, arstate, payload, outboundChannel, actionContext)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Error generating action reply: %s", err)
		return false
	}
	return true
}

// sendActionReplyInternal sends an action reply with the given state and payload
// This function is only meant to be called within the actions.go file !
// Use SendActionReply instead (or SendActionReplyWithAdditionalContext if you need to pass additional context)
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
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Error generating umh message: %v", err)
		return err
	}
	outboundChannel <- &umhMessage

	return nil
}

// generateUMHMessage generates a UMHMessage with the given user email, message type and payload.
// There is no check for matching message type and payload, so make sure that the payload is
// compatible with the message type.
//
// The message content gets encrypted before it is added to the UMHMessage
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
