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
//
//
// TODO: why is there a fsmv2 adapter and a fsmv2 bridge, this doesnt make sense

// Package fsmv2_bridge connects the FSMv2 communicator worker with the
// legacy Router's channel-based messaging.
//
// IMPORTANT: This package is in pkg/communicator (Router's domain), NOT in
// pkg/fsmv2. The FSMv2 worker only manages ObservedState/DesiredState.
// This bridge handles the channel translation.
//
// BUG #2 (ENG-3600): Old communicator blocked when channel full, freezing
// NotifySubscribers goroutine. This bridge uses select with default case.
package fsmv2_bridge

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// StateGetter allows reading ObservedState from FSMv2 worker.
// Implemented by the FSMv2 supervisor or a state cache.
type StateGetter interface {
	GetObservedState() snapshot.CommunicatorObservedState
}

// StateSetter allows writing to FSMv2 Dependencies for next SyncAction.
// Implemented by the FSMv2 CommunicatorDependencies object.
type StateSetter interface {
	SetMessagesToBePushed(messages []*transport.UMHMessage)
}

// Bridge connects FSMv2 communicator's state-based messaging with
// the legacy Router's channel-based messaging.
//
// Flow:
//  1. FSMv2 SyncAction pulls messages → ObservedState.MessagesReceived
//  2. Bridge.PollAndForward() reads ObservedState → inboundChannel → Router
//  3. Router processes → 50+ Actions → outboundChannel
//  4. Bridge.CollectAndWrite() drains outboundChannel → Dependencies.MessagesToBePushed
//  5. FSMv2 SyncAction pushes MessagesToBePushed
//
// TODO: shouldnt this here not use stateGetter and setter and instead just the channel?
type Bridge struct {
	stateGetter  StateGetter
	stateSetter  StateSetter
	inboundChan  chan *models.UMHMessage
	outboundChan chan *models.UMHMessage
	logger       *zap.SugaredLogger
}

func NewBridge(
	stateGetter StateGetter,
	stateSetter StateSetter,
	inboundChan chan *models.UMHMessage,
	outboundChan chan *models.UMHMessage,
	logger *zap.SugaredLogger,
) *Bridge {
	return &Bridge{
		stateGetter:  stateGetter,
		stateSetter:  stateSetter,
		inboundChan:  inboundChan,
		outboundChan: outboundChan,
		logger:       logger,
	}
}

// PollAndForward reads MessagesReceived from FSMv2 ObservedState and
// forwards them to inboundChannel for Router processing.
//
// Uses non-blocking send to prevent channel overflow blocking (Bug #2 fix).
// Returns nil even if messages are dropped due to full channel (logged).
func (b *Bridge) PollAndForward(ctx context.Context) error {
	_, _, err := b.PollAndForwardWithStats(ctx)

	return err
}

// PollAndForwardWithStats is like PollAndForward but returns stats for monitoring.
// Returns (forwarded count, dropped count, error).
//
// BUG #2 (ENG-3600): Unlike old communicator which blocked forever, this
// drops messages immediately when channel is full. Stats allow monitoring
// the drop rate to detect issues.
func (b *Bridge) PollAndForwardWithStats(ctx context.Context) (int, int, error) {
	observed := b.stateGetter.GetObservedState()
	messages := observed.MessagesReceived

	if len(messages) == 0 {
		return 0, 0, nil
	}

	forwarded := 0
	dropped := 0

	for _, msg := range messages {
		// Convert transport.UMHMessage to models.UMHMessage
		// transport.UMHMessage.InstanceUUID is string
		// models.UMHMessage.InstanceUUID is uuid.UUID
		var instanceUUID uuid.UUID
		if msg.InstanceUUID != "" {
			parsed, err := uuid.Parse(msg.InstanceUUID)
			if err != nil {
				if b.logger != nil {
					b.logger.Warnw("failed to parse InstanceUUID, using nil UUID",
						"instanceUUID", msg.InstanceUUID,
						"error", err)
				}

				instanceUUID = uuid.Nil
			} else {
				instanceUUID = parsed
			}
		}

		modelMsg := &models.UMHMessage{
			Content:      msg.Content,
			InstanceUUID: instanceUUID,
			Email:        msg.Email,
		}

		select {
		case b.inboundChan <- modelMsg:
			forwarded++
		case <-ctx.Done():
			return forwarded, dropped, ctx.Err()
		default:
			// Channel full - log but don't block (Bug #2 fix)
			dropped++
		}
	}

	if dropped > 0 && b.logger != nil {
		b.logger.Warnw("dropped messages due to full inbound channel",
			"forwarded", forwarded,
			"dropped", dropped)
	}

	return forwarded, dropped, nil
}

// CollectAndWrite drains outboundChannel and writes messages to FSMv2
// Dependencies for the next SyncAction to push.
//
// Uses non-blocking drain to prevent deadlock.
func (b *Bridge) CollectAndWrite(ctx context.Context) error {
	var messages []*transport.UMHMessage

	for {
		select {
		case msg := <-b.outboundChan:
			if msg != nil {
				// Convert models.UMHMessage to transport.UMHMessage
				// models.UMHMessage.InstanceUUID is uuid.UUID
				// transport.UMHMessage.InstanceUUID is string
				messages = append(messages, &transport.UMHMessage{
					Content:      msg.Content,
					InstanceUUID: msg.InstanceUUID.String(),
					Email:        msg.Email,
				})
			}
		case <-ctx.Done():
			// Write what we have before returning
			if len(messages) > 0 {
				b.stateSetter.SetMessagesToBePushed(messages)
			}

			return ctx.Err()
		default:
			// Channel empty - write and return
			if len(messages) > 0 {
				b.stateSetter.SetMessagesToBePushed(messages)
			}

			return nil
		}
	}
}
