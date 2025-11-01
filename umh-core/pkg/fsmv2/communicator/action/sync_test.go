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

package action_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

func VerifyActionIdempotency(action interface {
	Execute(context.Context) error
}, iterations int, verifyState func()) {
	ctx := context.Background()

	for i := 0; i < iterations; i++ {
		err := action.Execute(ctx)
		Expect(err).ToNot(HaveOccurred(), "Action should succeed on iteration %d", i+1)
	}

	verifyState()
}

var _ = Describe("SyncAction", func() {
	var (
		action       *communicator.SyncAction
		ctx          context.Context
		transport    *MockTransport
		inboundChan  chan *transportpkg.UMHMessage
		outboundChan chan *transportpkg.UMHMessage
	)

	BeforeEach(func() {
		ctx = context.Background()
		transport = NewMockTransport()
		inboundChan = make(chan *transportpkg.UMHMessage, 10)
		outboundChan = make(chan *transportpkg.UMHMessage, 10)
		action = communicator.NewSyncAction(transport, inboundChan, outboundChan)
	})

	Describe("Name", func() {
		It("should return action name", func() {
			Expect(action.Name()).To(Equal("Sync"))
		})
	})

	Describe("Execute", func() {
		Context("pull operation", func() {
			It("should pull messages from transport", func() {
				// Arrange: Transport has 2 messages to pull
				messages := []*transportpkg.UMHMessage{
					{InstanceUUID: "test-1", Content: "msg1"},
					{InstanceUUID: "test-2", Content: "msg2"},
				}
				transport.SetPullMessages(messages)

				// Act
				err := action.Execute(ctx)

				// Assert: No error and pull was called
				Expect(err).NotTo(HaveOccurred())
				Expect(transport.PullCallCount()).To(Equal(1))
			})

			It("should push pulled messages to inbound channel", func() {
				// Arrange
				messages := []*transportpkg.UMHMessage{
					{InstanceUUID: "test-1", Content: "msg1"},
					{InstanceUUID: "test-2", Content: "msg2"},
				}
				transport.SetPullMessages(messages)

				// Act
				err := action.Execute(ctx)

				// Assert: Messages appear in inbound channel
				Expect(err).NotTo(HaveOccurred())
				Expect(inboundChan).To(HaveLen(2))

				msg1 := <-inboundChan
				msg2 := <-inboundChan
				Expect(msg1.Content).To(Equal("msg1"))
				Expect(msg2.Content).To(Equal("msg2"))
			})

			It("should return error when pull fails", func() {
				// Arrange
				transport.SetPullError(errors.New("network error"))

				// Act
				err := action.Execute(ctx)

				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pull failed"))
				Expect(err.Error()).To(ContainSubstring("network error"))
			})

			It("should handle context cancellation during pull", func() {
				// Arrange
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()
				transport.SetPullError(context.Canceled)

				// Act
				err := action.Execute(cancelCtx)

				// Assert
				Expect(err).To(MatchError(context.Canceled))
			})

			It("should continue when inbound channel is full", func() {
				// Arrange: Fill inbound channel
				for i := 0; i < 10; i++ {
					inboundChan <- &transportpkg.UMHMessage{Content: "blocking"}
				}

				// Transport has messages but channel is full
				messages := []*transportpkg.UMHMessage{
					{InstanceUUID: "test-1", Content: "msg1"},
				}
				transport.SetPullMessages(messages)

				// Act: Should not block, should log warning and continue
				err := action.Execute(ctx)

				// Assert: No error, operation completes
				Expect(err).NotTo(HaveOccurred())
				Expect(transport.PullCallCount()).To(Equal(1))
			})
		})

		Context("push operation", func() {
			It("should drain outbound channel", func() {
				// Arrange: Messages waiting in outbound channel
				outboundChan <- &transportpkg.UMHMessage{Content: "msg1"}
				outboundChan <- &transportpkg.UMHMessage{Content: "msg2"}

				// Act
				err := action.Execute(ctx)

				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(outboundChan).To(HaveLen(0))
			})

			It("should push drained messages to transport", func() {
				// Arrange
				outboundChan <- &transportpkg.UMHMessage{Content: "msg1"}
				outboundChan <- &transportpkg.UMHMessage{Content: "msg2"}

				// Act
				err := action.Execute(ctx)

				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(transport.PushCallCount()).To(Equal(1))

				pushed := transport.GetPushedMessages()
				Expect(pushed).To(HaveLen(2))
				Expect(pushed[0].Content).To(Equal("msg1"))
				Expect(pushed[1].Content).To(Equal("msg2"))
			})

			It("should return error when push fails", func() {
				// Arrange
				outboundChan <- &transportpkg.UMHMessage{Content: "msg1"}
				transport.SetPushError(errors.New("backend error"))

				// Act
				err := action.Execute(ctx)

				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("push failed"))
				Expect(err.Error()).To(ContainSubstring("backend error"))
			})

			It("should not push when outbound channel is empty", func() {
				// Arrange: Empty outbound channel

				// Act
				err := action.Execute(ctx)

				// Assert: No push call
				Expect(err).NotTo(HaveOccurred())
				Expect(transport.PushCallCount()).To(Equal(0))
			})

			It("should batch up to 10 messages", func() {
				// Arrange: Recreate outbound channel with capacity for 15 messages
				outboundChan = make(chan *transportpkg.UMHMessage, 15)
				action = communicator.NewSyncAction(transport, inboundChan, outboundChan)

				// Arrange: 15 messages in outbound
				for i := 0; i < 15; i++ {
					outboundChan <- &transportpkg.UMHMessage{Content: fmt.Sprintf("msg%d", i)}
				}

				// Act
				err := action.Execute(ctx)

				// Assert: First batch of 10 sent
				Expect(err).NotTo(HaveOccurred())
				Expect(transport.PushCallCount()).To(Equal(1))

				pushed := transport.GetPushedMessages()
				Expect(pushed).To(HaveLen(10))

				// Remaining 5 still in channel
				Expect(outboundChan).To(HaveLen(5))
			})
		})

		Context("bidirectional sync", func() {
			It("should handle pull and push in same execution", func() {
				// Arrange: Messages in both directions
				transport.SetPullMessages([]*transportpkg.UMHMessage{
					{Content: "incoming"},
				})
				outboundChan <- &transportpkg.UMHMessage{Content: "outgoing"}

				// Act
				err := action.Execute(ctx)

				// Assert: Both operations succeed
				Expect(err).NotTo(HaveOccurred())
				Expect(transport.PullCallCount()).To(Equal(1))
				Expect(transport.PushCallCount()).To(Equal(1))
				Expect(inboundChan).To(HaveLen(1))
				Expect(outboundChan).To(HaveLen(0))
			})
		})
	})

	Describe("Idempotency", func() {
		Context("when executed multiple times", func() {
			It("should be idempotent", func() {
				// Arrange: Setup messages
				transport.SetPullMessages([]*transportpkg.UMHMessage{
					{Content: "msg1"},
				})

				// Act: Execute 3 times
				VerifyActionIdempotency(action, 3, func() {
					// Assert: Pull called 3 times (once per execute)
					Expect(transport.PullCallCount()).To(Equal(3))
				})
			})
		})

		Context("when operations fail", func() {
			It("should allow retries after pull failure", func() {
				// Arrange
				transport.SetPullError(errors.New("temporary error"))

				// Act: First attempt fails
				err := action.Execute(ctx)
				Expect(err).To(HaveOccurred())

				// Fix error and retry
				transport.SetPullError(nil)
				transport.SetPullMessages([]*transportpkg.UMHMessage{
					{Content: "retry-msg"},
				})

				// Act: Second attempt succeeds
				err = action.Execute(ctx)

				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(inboundChan).To(HaveLen(1))
			})
		})
	})
})
