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

package fsmv2_bridge_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/fsmv2_bridge"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func TestBridge(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FSMv2 Bridge Suite")
}

// mockStateGetter implements fsmv2_bridge.StateGetter for testing.
type mockStateGetter struct {
	observedState snapshot.CommunicatorObservedState
}

func newMockStateGetter() *mockStateGetter {
	return &mockStateGetter{
		observedState: snapshot.CommunicatorObservedState{
			CollectedAt:       time.Now(),
			Authenticated:     true,
			ConsecutiveErrors: 0,
		},
	}
}

func (m *mockStateGetter) GetObservedState() snapshot.CommunicatorObservedState {
	return m.observedState
}

func (m *mockStateGetter) SetObservedState(state snapshot.CommunicatorObservedState) {
	m.observedState = state
}

// mockStateSetter implements fsmv2_bridge.StateSetter for testing.
type mockStateSetter struct {
	messages []*transport.UMHMessage
}

func newMockStateSetter() *mockStateSetter {
	return &mockStateSetter{}
}

func (m *mockStateSetter) SetMessagesToBePushed(messages []*transport.UMHMessage) {
	m.messages = messages
}

// testLogger creates a no-op logger for tests.
var testLogger = zap.NewNop().Sugar()

var _ = Describe("FSMv2 Router Bridge", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		bridge       *fsmv2_bridge.Bridge
		inboundChan  chan *models.UMHMessage
		outboundChan chan *models.UMHMessage
		stateGetter  *mockStateGetter
		stateSetter  *mockStateSetter
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		inboundChan = make(chan *models.UMHMessage, 100)
		outboundChan = make(chan *models.UMHMessage, 100)
		stateGetter = newMockStateGetter()
		stateSetter = newMockStateSetter()
		bridge = fsmv2_bridge.NewBridge(
			stateGetter,
			stateSetter,
			inboundChan,
			outboundChan,
			testLogger,
		)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("PollAndForward", func() {
		It("polls MessagesReceived from FSMv2 and forwards to inboundChannel", func() {
			// Arrange: FSMv2 worker has messages in ObservedState
			stateGetter.SetObservedState(snapshot.CommunicatorObservedState{
				CollectedAt:   time.Now(),
				Authenticated: true,
				MessagesReceived: []transport.UMHMessage{
					{Content: `{"action": "get-status"}`},
				},
			})

			// Act: Bridge polls and forwards
			err := bridge.PollAndForward(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert: Message appears in inboundChannel
			Eventually(func() int {
				return len(inboundChan)
			}).WithTimeout(time.Second).Should(Equal(1))

			msg := <-inboundChan
			Expect(msg.Content).To(ContainSubstring("get-status"))
		})

		It("handles full inboundChannel without blocking (Bug #2 fix)", func() {
			// Arrange: Fill channel to capacity
			for range 100 {
				inboundChan <- &models.UMHMessage{}
			}
			stateGetter.SetObservedState(snapshot.CommunicatorObservedState{
				CollectedAt:   time.Now(),
				Authenticated: true,
				MessagesReceived: []transport.UMHMessage{
					{Content: "overflow-test"},
				},
			})

			// Act: Should complete without blocking
			done := make(chan error, 1)
			go func() {
				done <- bridge.PollAndForward(ctx)
			}()

			// Assert: Completes within timeout (non-blocking)
			// Using 1s timeout - realistic for non-blocking operation
			Eventually(done).WithTimeout(time.Second).Should(Receive(BeNil()))
		})

		It("returns dropped message count for monitoring", func() {
			// Arrange: Fill channel to capacity
			for range 100 {
				inboundChan <- &models.UMHMessage{}
			}
			stateGetter.SetObservedState(snapshot.CommunicatorObservedState{
				CollectedAt:   time.Now(),
				Authenticated: true,
				MessagesReceived: []transport.UMHMessage{
					{Content: "msg-1"},
					{Content: "msg-2"},
					{Content: "msg-3"},
				},
			})

			// Act
			forwarded, dropped, err := bridge.PollAndForwardWithStats(ctx)

			// Assert: Stats are returned for monitoring
			Expect(err).NotTo(HaveOccurred())
			Expect(forwarded).To(Equal(0))
			Expect(dropped).To(Equal(3))
		})

		It("correctly converts transport.UMHMessage to models.UMHMessage with UUID", func() {
			// Arrange: FSMv2 worker has messages with InstanceUUID as string
			testUUID := uuid.New()
			stateGetter.SetObservedState(snapshot.CommunicatorObservedState{
				CollectedAt:   time.Now(),
				Authenticated: true,
				MessagesReceived: []transport.UMHMessage{
					{
						Content:      `{"action": "test"}`,
						InstanceUUID: testUUID.String(),
						Email:        "test@example.com",
					},
				},
			})

			// Act: Bridge polls and forwards
			err := bridge.PollAndForward(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert: Message is converted correctly with uuid.UUID type
			msg := <-inboundChan
			Expect(msg.Content).To(Equal(`{"action": "test"}`))
			Expect(msg.InstanceUUID).To(Equal(testUUID))
			Expect(msg.Email).To(Equal("test@example.com"))
		})
	})

	Describe("CollectAndWrite", func() {
		It("drains outboundChannel and writes to FSMv2 Dependencies", func() {
			// Arrange: Router has responses in outboundChannel
			outboundChan <- &models.UMHMessage{Content: "response-1"}
			outboundChan <- &models.UMHMessage{Content: "response-2"}

			// Act: Bridge collects and writes to FSMv2
			err := bridge.CollectAndWrite(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert: Channel is drained
			Expect(outboundChan).To(BeEmpty())
		})

		It("uses non-blocking drain from outboundChannel", func() {
			// Arrange: Empty channel

			// Act & Assert: Should complete immediately (within 1s)
			done := make(chan error, 1)
			go func() {
				done <- bridge.CollectAndWrite(ctx)
			}()

			Eventually(done).WithTimeout(time.Second).Should(Receive(BeNil()))
		})

		It("correctly converts models.UMHMessage to transport.UMHMessage with UUID string", func() {
			// Arrange: Router has responses with uuid.UUID type
			testUUID := uuid.New()
			outboundChan <- &models.UMHMessage{
				Content:      "response-with-uuid",
				InstanceUUID: testUUID,
				Email:        "test@example.com",
			}

			// Act: Bridge collects and writes to FSMv2
			err := bridge.CollectAndWrite(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert: Message is converted correctly with string UUID
			Expect(stateSetter.messages).To(HaveLen(1))
			Expect(stateSetter.messages[0].Content).To(Equal("response-with-uuid"))
			Expect(stateSetter.messages[0].InstanceUUID).To(Equal(testUUID.String()))
			Expect(stateSetter.messages[0].Email).To(Equal("test@example.com"))
		})
	})
})
