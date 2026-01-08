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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/permissions"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/maptostruct"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

type Router struct {
	dog                   watchdog.Iface
	configManager         config.ConfigManager
	inboundChannel        chan *models.UMHMessageWithAdditionalInfo
	outboundChannel       chan *models.UMHMessage
	clientConnections     map[string]*ClientConnection
	subHandler            *subscriber.Handler
	systemSnapshotManager *fsm.SnapshotManager
	actionLogger          *zap.SugaredLogger
	routerLogger          *zap.SugaredLogger
	releaseChannel        config.ReleaseChannel
	clientConnectionsLock sync.RWMutex
	instanceUUID          uuid.UUID
	validator             permissions.Validator
}

type ClientConnection struct {
	FrontendToCompanionChannel chan []byte
	CompanionToFrontendChannel chan []byte
}

func NewRouter(dog watchdog.Iface,
	inboundChannel chan *models.UMHMessageWithAdditionalInfo,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	releaseChannel config.ReleaseChannel,
	subHandler *subscriber.Handler,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	logger *zap.SugaredLogger,
) *Router {
	return &Router{
		dog:                   dog,
		inboundChannel:        inboundChannel,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		releaseChannel:        releaseChannel,
		clientConnections:     make(map[string]*ClientConnection),
		clientConnectionsLock: sync.RWMutex{},
		subHandler:            subHandler,
		systemSnapshotManager: systemSnapshotManager,
		configManager:         configManager,
		actionLogger:          logger,
		routerLogger:          logger,
		validator:             permissions.NewValidator(),
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
				r.routerLogger.Warnf("Failed to decrypt message: %s", err.Error())

				continue
			}

			switch messageContent.MessageType {
			case models.Subscribe:
				r.handleSub(message, messageContent, watcherUUID)
			case models.Action:
				r.handleAction(messageContent, message, watcherUUID)
			default:
				r.routerLogger.Warnf("Unexpected message type: %s", messageContent.MessageType)

				continue
			}
		case <-ticker.C:
			r.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
		}
	}
}

// handleSub handles the subscribe message
// if the payload contains a "resubscribed" field, it will just "unexpire" the subscriber (reset the TTL)
// otherwise it will add the subscriber to the registry
// this is an optimization to avoid sending a "new subscriber" message, containing the cached uns data with at least
// one event for every topic, to the frontend when the user is already subscribed
// we should avoid unnecessary new subscriber message generation because of its high memory and cpu usage.
func (r *Router) handleSub(message *models.UMHMessageWithAdditionalInfo, messageContent models.UMHMessageContent, watcherUUID uuid.UUID) {
	if r.subHandler == nil {
		r.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		r.routerLogger.Warnf("Subscribe handler not yet initialized")

		return
	}

	var subscribePayload models.SubscribeMessagePayload
	if messageContent.Payload != nil {
		// Try direct type assertion first
		if payload, ok := messageContent.Payload.(models.SubscribeMessagePayload); ok {
			subscribePayload = payload
		}
	}

	r.subHandler.AddOrRefreshSubscriber(
		message.Email,
		subscribePayload.Resubscribed,
		message.Certificate,
		message.RootCA,
		message.IntermediateCerts,
		r.validator,
	)
}

func (r *Router) handleAction(messageContent models.UMHMessageContent, message *models.UMHMessageWithAdditionalInfo, watcherUUID uuid.UUID) {
	var actionPayload models.ActionMessagePayload

	payloadMap, ok := messageContent.Payload.(map[string]interface{})
	if !ok {
		r.routerLogger.Warnf("Warning: Could not assert payload to map[string]interface{}. Actual type: %T, Value: %v", messageContent.Payload, messageContent.Payload)

		return
	}

	if err := maptostruct.MapToStruct(payloadMap, &actionPayload); err != nil {
		r.routerLogger.Warnf("Failed to convert payload into ActionMessagePayload: %v", err)

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
		r.systemSnapshotManager,
		r.configManager,
		message.Certificate,
		message.RootCA,
		message.IntermediateCerts,
		r.validator,
	)
}
