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

// Package router dispatches the messages that umh-core receives from the
// Management Console.
//
// Actions travel from the Management Console to umh-core, and status travels
// back. The connection itself belongs to the Management Console module, which
// the official image links in when it is built. In the open-source build no
// module is linked, and no message arrives.
//
// An action takes this path:
//
//  1. The Management Console module receives the message, checks it, and
//     hands it over as a handoff.Message holding the encoded bytes.
//  2. The Router built by NewRouterForFSMv2 decodes the bytes and refuses a
//     message that names no sender. Router.handleAction passes the action to
//     actions.HandleActionMessage.
//  3. Each action type has its own handler in pkg/communicator/actions. A
//     handler that changes something writes config.yaml through the config
//     manager. The Management Console never writes benthos config directly.
//  4. The FSMs reconcile against the new config.yaml, render the benthos
//     config and start benthos-umh under S6.
//
// Status takes the reverse path. The subscriber package builds and encodes the
// status message, and the Management Console module sends it.
package router

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"

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
	inboundChannel        <-chan handoff.Message
	outboundChannel       chan *models.UMHMessage
	clientConnections     map[string]*ClientConnection
	subHandler            *subscriber.Handler
	systemSnapshotManager *fsm.SnapshotManager
	actionLogger          *zap.SugaredLogger
	routerLogger          *zap.SugaredLogger
	releaseChannel        config.ReleaseChannel
	clientConnectionsLock sync.RWMutex
	instanceUUIDMu        sync.RWMutex
	instanceUUID          uuid.UUID
}

type ClientConnection struct {
	FrontendToCompanionChannel chan []byte
	CompanionToFrontendChannel chan []byte
}

// NewRouter creates a Router for the legacy (non-FSMv2) path.
// Deprecated: Use NewRouterForFSMv2 for FSMv2 mode with gatekeeper.
func NewRouter(dog watchdog.Iface,
	inboundChannel chan *models.UMHMessage,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	releaseChannel config.ReleaseChannel,
	subHandler *subscriber.Handler,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	logger *zap.SugaredLogger,
) *Router {
	// Legacy path: spawn adapter goroutine that hands over the encoded bytes.
	verifiedChan := make(chan handoff.Message, cap(inboundChannel))
	go func() {
		for msg := range inboundChannel {
			traceID := ""
			if msg.Metadata != nil {
				traceID = msg.Metadata.TraceID.String()
			}

			verifiedChan <- handoff.Message{
				Content:     []byte(msg.Content),
				SenderEmail: msg.Email,
				TraceID:     traceID,
			}
		}
		close(verifiedChan)
	}()

	return &Router{
		dog:                   dog,
		inboundChannel:        verifiedChan,
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
	}
}

// NewRouterForFSMv2 creates a Router for FSMv2 mode where the producers put
// encoded message bytes on the inbound channel.
func NewRouterForFSMv2(dog watchdog.Iface,
	inboundChannel <-chan handoff.Message,
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
	}
}

// SetInstanceUUID updates the Router's instance UUID.
// This is required for FSMv2 because the Router is created BEFORE authentication
// with a placeholder UUID. When authentication completes asynchronously via
// AuthenticateAction, the real UUID must be updated here so action replies
// contain the correct instanceUUID.
// Called by SetLoginResponseForFSMv2 in communication_state.go.
// Thread-safe: uses mutex for concurrent access protection.
func (r *Router) SetInstanceUUID(newUUID uuid.UUID) {
	r.instanceUUIDMu.Lock()
	defer r.instanceUUIDMu.Unlock()

	r.instanceUUID = newUUID
}

// GetInstanceUUID returns the Router's instance UUID.
// Thread-safe: uses mutex for concurrent access protection.
func (r *Router) GetInstanceUUID() uuid.UUID {
	r.instanceUUIDMu.RLock()
	defer r.instanceUUIDMu.RUnlock()

	return r.instanceUUID
}

func (r *Router) Start() {
	go r.router()
}

func (r *Router) router() {
	watcherUUID := r.dog.RegisterHeartbeat("router", 5, 600, true)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-r.inboundChannel:
			if !ok {
				r.routerLogger.Infow("inbound channel closed, stopping router")

				return
			}

			r.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_OK)

			messageContent, err := encoding.DecodeMessageFromUserToUMHInstance(string(message.Content))
			if err != nil {
				r.routerLogger.Warnw("Failed to decode message", "error", err)

				continue
			}

			if message.SenderEmail == "" {
				r.routerLogger.Warnw("Refusing message because it names no sender")

				continue
			}

			switch messageContent.MessageType {
			case models.Subscribe:
				r.handleSub(messageContent, message, watcherUUID)
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
func (r *Router) handleSub(messageContent models.UMHMessageContent, envelope handoff.Message, watcherUUID uuid.UUID) {
	if r.subHandler == nil {
		r.dog.ReportHeartbeatStatus(watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		r.routerLogger.Warnf("Subscribe handler not yet initialized")

		return
	}

	var subscribePayload models.SubscribeMessagePayload

	switch payload := messageContent.Payload.(type) {
	case models.SubscribeMessagePayload:
		subscribePayload = payload
	case map[string]any:
		if err := maptostruct.MapToStruct(payload, &subscribePayload); err != nil {
			r.routerLogger.Warnf("Failed to convert payload into SubscribeMessagePayload: %v", err)
		}
	}

	r.subHandler.AddOrRefreshSubscriber(envelope.SenderEmail, subscribePayload.Resubscribed)
}

func (r *Router) handleAction(messageContent models.UMHMessageContent, envelope handoff.Message, watcherUUID uuid.UUID) {
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

	traceID := uuid.Nil
	if envelope.TraceID != "" {
		parsed, err := uuid.Parse(envelope.TraceID)
		if err == nil {
			traceID = parsed
		}
	}

	go actions.HandleActionMessage(
		r.GetInstanceUUID(),
		actionPayload,
		envelope.SenderEmail,
		r.outboundChannel,
		r.releaseChannel,
		r.dog,
		traceID,
		r.systemSnapshotManager,
		r.configManager,
	)
}
