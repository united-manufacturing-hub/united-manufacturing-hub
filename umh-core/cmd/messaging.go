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

package main

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/managementconsole"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

// runRouter starts the subscriber handler and the router. The router reads the
// messages users sent to this instance from inbound. Status messages from the
// subscriber handler and the router's action replies go to outbound, where the
// Management Console module reads them. It returns once they are started;
// the reply forwarding stops when ctx is cancelled.
func runRouter(
	ctx context.Context,
	inbound <-chan handoff.Message,
	outbound chan<- handoff.Message,
	configData *config.FullConfig,
	communicationState *communication_state.CommunicationState,
	logger *zap.SugaredLogger,
	status managementconsole.StatusFunc,
) {
	communicationState.InitialiseAndStartSubscriberHandler(
		5*time.Minute, // TTL: time until subscriber considered dead
		1*time.Minute, // Cull: cycle time to remove dead subscribers
		configData,
		communicationState.SystemSnapshotManager,
		communicationState.ConfigManager,
		outbound,
		status,
	)
	communicationState.InitializeRouterForFSMv2(inbound)

	sentry.SafeGoWithContext(ctx, func(ctx context.Context) {
		forwardReplies(ctx, communicationState.OutboundChannel, outbound)
	})

	logger.Info("Router started")
}

// forwardReplies moves the router's action replies from replies to outbound
// until ctx is cancelled or replies is closed.
//
// A reply waits while outbound is full rather than being dropped, because a
// dropped reply leaves the user's action without an answer. Status messages
// are dropped instead, because the next one replaces them. outbound stays
// full only while the Management Console module has no login. In that time
// the module receives no new actions, so only the actions already running
// wait, each in its own goroutine. The router keeps reading inbound.
func forwardReplies(ctx context.Context, replies <-chan *models.UMHMessage, outbound chan<- handoff.Message) {
	for {
		reply, open := receiveReply(ctx, replies)
		if !open {
			return
		}

		if reply == nil {
			continue
		}

		if !sendUntilCancelled(ctx, outbound, replyMessage(reply)) {
			return
		}
	}
}

// receiveReply returns open=false when ctx is cancelled or replies is closed.
func receiveReply(ctx context.Context, replies <-chan *models.UMHMessage) (reply *models.UMHMessage, open bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case reply, open := <-replies:
		return reply, open
	}
}

// sendUntilCancelled waits until outbound accepts message, and returns false
// when ctx is cancelled first.
func sendUntilCancelled(ctx context.Context, outbound chan<- handoff.Message, message handoff.Message) bool {
	select {
	case outbound <- message:
		return true
	case <-ctx.Done():
		return false
	}
}

// replyMessage turns an action reply into the message the Management Console
// module sends: the encoded content, the user it goes to, and the trace ID.
func replyMessage(reply *models.UMHMessage) handoff.Message {
	m := handoff.Message{Content: []byte(reply.Content), SenderEmail: reply.Email}
	if reply.Metadata != nil && reply.Metadata.TraceID != uuid.Nil {
		m.TraceID = reply.Metadata.TraceID.String()
	}

	return m
}

// isAuthenticatedUUID reports whether observed is the instance UUID returned by
// the relay at login, rather than empty or the random placeholder.
func isAuthenticatedUUID(observed, placeholder string) bool {
	return observed != "" && observed != placeholder
}

// applyAuthenticatedUUID hands the relay's instance UUID to the login
// response in the communication state and to Sentry.
func applyAuthenticatedUUID(instanceUUID string, communicationState *communication_state.CommunicationState) {
	communicationState.SetLoginResponseForFSMv2(instanceUUID)
	sentry.SetInstanceUUID(instanceUUID)
}
