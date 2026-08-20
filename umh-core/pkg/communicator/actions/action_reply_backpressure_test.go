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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// The outbound channel is the queue the agent uses to hand messages to the
// Management Console. Two kinds of message share it: status messages, produced
// once per second per subscriber, and action replies, produced while an action
// runs. It has a fixed capacity, so it can be full.
//
// A caller that cannot enqueue has two options: give up, or wait. Status
// messages give up -- pkg/communicator/pkg/subscriber/subscribers.go uses a
// select with a default branch and logs a drop. Action replies wait, without a
// bound, because sendActionReplyInternal ends in a bare channel send.
//
// Waiting without a bound parks the goroutine that runs the action. The browser
// applies its own 30-second deadline (DEFAULT_ACTION_TIMEOUT in
// frontend/src/lib/utils/fetcher.ts) and then reports "No response from the UMH
// Instance", which names the wrong party: the instance produced the reply and
// could not queue it.
//
// These specs fix the boundary at "the caller returns". They deliberately say
// nothing about whether a reply that cannot be queued should be dropped, retried
// or delivered late, because that choice is still open.
const replyEnqueueBudget = 2 * time.Second

var _ = Describe("Action reply enqueue under backpressure", func() {
	var (
		userEmail    string
		actionUUID   uuid.UUID
		instanceUUID uuid.UUID
	)

	BeforeEach(func() {
		userEmail = "operator@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
	})

	// queuedReply reports whether the channel holds a generated reply, as opposed
	// to the filler this spec used to occupy the capacity. The filler carries no
	// Content; a real reply always carries encrypted Content.
	queuedReply := func(outbound chan *models.UMHMessage) bool {
		for {
			select {
			case msg := <-outbound:
				if msg != nil && msg.Content != "" {
					return true
				}
			default:
				return false
			}
		}
	}

	// sendAsync calls SendActionReply on its own goroutine and reports how the
	// call ended. A nil result means the call had not returned within the
	// budget. The goroutine is intentionally left running: a blocked channel
	// send cannot be cancelled, and leaking it is what the production defect
	// does too.
	sendAsync := func(outbound chan *models.UMHMessage) *bool {
		done := make(chan bool, 1)

		go func() {
			done <- actions.SendActionReply(
				instanceUUID,
				userEmail,
				actionUUID,
				models.ActionFinishedSuccessfull,
				"config file contents",
				outbound,
				models.GetConfigFile,
			)
		}()

		select {
		case ok := <-done:
			return &ok
		case <-time.After(replyEnqueueBudget):
			return nil
		}
	}

	// This is the control. It shares every line of machinery with the spec
	// below and differs in one variable: the channel has room. It must pass
	// both before and after the defect is fixed. If it ever fails, the failure
	// below is not evidence about backpressure -- message generation or
	// encryption broke instead.
	It("delivers the reply when the channel has room", func() {
		outbound := make(chan *models.UMHMessage, 4)

		result := sendAsync(outbound)

		Expect(result).NotTo(BeNil(), "SendActionReply must return when the channel has room")
		Expect(*result).To(BeTrue(), "SendActionReply reports success by returning true")
		Expect(outbound).To(HaveLen(1), "the reply must be on the channel")

		queued := <-outbound
		Expect(queued.Email).To(Equal(userEmail), "the reply must be addressed to the requesting user")
		Expect(queued.Content).NotTo(BeEmpty(), "the reply must carry encrypted content")
	})

	It("returns instead of parking the caller when the channel is full", func() {
		outbound := make(chan *models.UMHMessage, 2)

		// Fill to capacity. Asserting the fill is what stops this spec from
		// passing vacuously against a channel that had room all along.
		for range cap(outbound) {
			outbound <- &models.UMHMessage{InstanceUUID: instanceUUID, Email: userEmail}
		}
		Expect(outbound).To(HaveLen(cap(outbound)), "the channel must be full before the reply is sent")

		result := sendAsync(outbound)

		Expect(result).NotTo(BeNil(),
			"SendActionReply parked the goroutine on a full outbound channel; "+
				"an action cannot report its own result, and the browser blames the instance")

		// Returning is necessary but not sufficient. An implementation that
		// silently discards the reply and returns true also returns, and it leaves
		// the user with exactly the same failure while removing the Sentry report.
		// Measured: such an implementation passes the assertion above on its own.
		//
		// So: reporting success is only allowed if the reply is actually queued.
		// This says nothing about which of drop-with-error, bounded-wait or a
		// priority lane is the right design -- all three satisfy it. It only
		// forbids claiming success for a reply that went nowhere.
		if *result {
			Expect(queuedReply(outbound)).To(BeTrue(),
				"SendActionReply reported success for a reply it never queued")
		}
	})

	It("returns on a full channel even when no consumer ever drains it", func() {
		outbound := make(chan *models.UMHMessage, 1)
		outbound <- &models.UMHMessage{InstanceUUID: instanceUUID, Email: userEmail}

		// HandleActionMessage emits four replies for one action: parsing,
		// validating, executing, then the result. Each one is a separate
		// enqueue, so the first is enough to strand an action that has not yet
		// started work. Sending several here checks that no single call
		// succeeds by consuming room a previous call happened to free.
		for i := range 3 {
			result := sendAsync(outbound)
			Expect(result).NotTo(BeNil(),
				"reply %d of 3 parked the caller on a full outbound channel", i+1)
		}
	})
})
