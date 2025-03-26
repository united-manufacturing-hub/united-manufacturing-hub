package actions

import (
	"database/sql"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models/mgmtconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/safejson"
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

func HandleActionMessage(instanceUUID uuid.UUID, payload models.ActionMessagePayload, sender string, outboundChannel chan *models.UMHMessage, releaseChannel mgmtconfig.ReleaseChannel, dog watchdog.Iface, traceID uuid.UUID, database *sql.DB) {
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
