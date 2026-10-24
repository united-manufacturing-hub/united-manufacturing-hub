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
// Management Console module reads them. This function blocks until ctx is
// cancelled.
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

	go forwardReplies(ctx, communicationState.OutboundChannel, outbound)

	logger.Info("Router started, waiting for context cancellation")

	// Block until the caller's context is cancelled (e.g. main ctx cancelled).
	<-ctx.Done()

	logger.Info("Router context cancelled")
}

// forwardReplies moves the router's action replies from replies to outbound
// until ctx is cancelled. A reply waits while outbound is full, because a
// dropped reply leaves the user's action without an answer.
func forwardReplies(ctx context.Context, replies <-chan *models.UMHMessage, outbound chan<- handoff.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case reply, ok := <-replies:
			if !ok {
				return
			}

			if reply == nil {
				continue
			}

			select {
			case outbound <- replyMessage(reply):
			case <-ctx.Done():
				return
			}
		}
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
func applyAuthenticatedUUID(uuid string, cs *communication_state.CommunicationState) {
	cs.SetLoginResponseForFSMv2(uuid)
	sentry.SetInstanceUUID(uuid)
}
