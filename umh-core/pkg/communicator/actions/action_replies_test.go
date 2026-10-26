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

package actions_test

import (
	"sync"

	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// queuedReply is an action reply as the user receives it.
type queuedReply struct {
	State     string
	Message   string
	ErrorCode string
}

// replyRecorder collects the messages an action sends on its outbound channel. Each spec
// creates its own, so a consumer still running from an earlier spec cannot append to it.
type replyRecorder struct {
	messages []*models.UMHMessage
	mu       sync.Mutex
}

// newReplyRecorder consumes outbound in the background and records every message on it.
func newReplyRecorder(outbound chan *models.UMHMessage) *replyRecorder {
	recorder := &replyRecorder{}

	go actions.ConsumeOutboundMessages(outbound, &recorder.messages, &recorder.mu, true)

	return recorder
}

// replies decodes the action replies recorded so far, in order.
func (r *replyRecorder) replies() []queuedReply {
	r.mu.Lock()
	defer r.mu.Unlock()

	return decodeReplies(r.messages)
}

// decodeReplies decodes the action replies among messages, in order.
func decodeReplies(messages []*models.UMHMessage) []queuedReply {
	var replies []queuedReply

	for _, message := range messages {
		decoded, err := encoding.DecodeMessageFromUMHInstanceToUser(message.Content)
		Expect(err).NotTo(HaveOccurred())

		payload, isMap := decoded.Payload.(map[string]interface{})
		if !isMap {
			continue
		}

		var reply queuedReply
		reply.State, _ = payload["actionReplyState"].(string)
		reply.Message, _ = payload["actionReplyPayload"].(string)

		if payloadV2, hasV2 := payload["actionReplyPayloadV2"].(map[string]interface{}); hasV2 {
			reply.ErrorCode, _ = payloadV2["errorCode"].(string)
		}

		replies = append(replies, reply)
	}

	return replies
}

// drainReplies decodes and returns the action replies queued in outbound, in order.
func drainReplies(outbound chan *models.UMHMessage) []queuedReply {
	var messages []*models.UMHMessage
	for len(outbound) > 0 {
		messages = append(messages, <-outbound)
	}

	return decodeReplies(messages)
}

// failureReply returns the ActionFinishedWithFailure reply among replies, or the zero
// reply when there is none.
func failureReply(replies []queuedReply) queuedReply {
	for _, reply := range replies {
		if reply.State == string(models.ActionFinishedWithFailure) {
			return reply
		}
	}

	return queuedReply{}
}
