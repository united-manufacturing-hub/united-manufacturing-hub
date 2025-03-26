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

package router

import (
	"database/sql"
	"sync"
	"time"

	//"github.com/united-manufacturing-hub/ManagementConsole/shared/models/mgmtconfig"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/actions"

	//"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models/mgmtconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/maptostruct"
	"go.uber.org/zap"
)

type Router struct {
	dog                   watchdog.Iface
	database              *sql.DB
	inboundChannel        chan *models.UMHMessageWithAdditionalInfo
	outboundChannel       chan *models.UMHMessage
	instanceUUID          uuid.UUID
	releaseChannel        mgmtconfig.ReleaseChannel
	clientConnections     map[string]*ClientConnection
	clientConnectionsLock sync.RWMutex
}

type ClientConnection struct {
	FrontendToCompanionChannel chan []byte
	CompanionToFrontendChannel chan []byte
}

func NewRouter(dog watchdog.Iface,
	inboundChannel chan *models.UMHMessageWithAdditionalInfo,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	releaseChannel mgmtconfig.ReleaseChannel,
	database *sql.DB,
) *Router {
	return &Router{
		dog:                   dog,
		database:              database,
		inboundChannel:        inboundChannel,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		releaseChannel:        releaseChannel,
		clientConnections:     make(map[string]*ClientConnection),
		clientConnectionsLock: sync.RWMutex{},
	}
}

func (r *Router) Start() {
	go r.router()
}

func (r *Router) router() {
	watcherUUID := r.dog.RegisterHeartbeat("router", 5, 600, true)
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case message := <-r.inboundChannel:
			r.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
			// Decode message
			messageContent, err := encoding.DecodeMessageFromUserToUMHInstance(message.Content)
			if err != nil {
				zap.S().Warnf("Failed to decrypt message: %s", err.Error())
				continue
			}
			switch messageContent.MessageType {
			// case models.Subscribe:
			// 	r.handleSub(message, watcherUUID)
			case models.Action:
				r.handleAction(messageContent, message, watcherUUID)
			default:
				zap.S().Warnf("Unexpected message type: %s", messageContent.MessageType)
				continue
			}
		case <-ticker.C:
			r.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
		}
	}
}

// func (r *Router) handleSub(message *models.UMHMessageWithAdditionalInfo, watcherUUID uuid.UUID) {
// 	if r.subHandler == nil {
// 		r.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
// 		zap.S().Warnf("Subscribe handler not yet initialized")
// 		return
// 	}
// 	r.subHandler.AddSubscriber(message.Email, message.Certificate)
// }

func (r *Router) handleAction(messageContent models.UMHMessageContent, message *models.UMHMessageWithAdditionalInfo, watcherUUID uuid.UUID) {
	var actionPayload models.ActionMessagePayload

	payloadMap, ok := messageContent.Payload.(map[string]interface{})
	if !ok {
		zap.S().Warnf("Warning: Could not assert payload to map[string]interface{}. Actual type: %T, Value: %v", messageContent.Payload, messageContent.Payload)
		return
	}

	if err := maptostruct.MapToStruct(payloadMap, &actionPayload); err != nil {
		zap.S().Warnf("Failed to convert payload into ActionMessagePayload: %v", err)
		return
	}

	traceId := uuid.Nil
	if message.Metadata != nil {
		traceId = message.Metadata.TraceID
	}
	go actions.HandleActionMessage(
		r.instanceUUID,
		actionPayload,
		message.Email,
		r.outboundChannel,
		r.releaseChannel,
		r.dog,
		traceId,
		r.database,
	)
}
