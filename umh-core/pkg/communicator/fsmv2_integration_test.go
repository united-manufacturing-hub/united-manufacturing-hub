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

package communicator_test

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

func TestFSMv2Integration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FSMv2 Bridge Integration Suite")
}

// MockWorkerState simulates the FSMv2 worker's state.
// In production, this would be the actual FSMv2 supervisor maintaining
// ObservedState (updated by SyncAction) and Dependencies (read by next SyncAction).
type MockWorkerState struct {
	observed snapshot.CommunicatorObservedState
	outbound []*transport.UMHMessage
}

func newMockWorkerState() *MockWorkerState {
	return &MockWorkerState{
		observed: snapshot.CommunicatorObservedState{
			CollectedAt:   time.Now(),
			Authenticated: true,
		},
	}
}

func (m *MockWorkerState) GetObservedState() snapshot.CommunicatorObservedState {
	return m.observed
}

func (m *MockWorkerState) SetObservedState(state snapshot.CommunicatorObservedState) {
	m.observed = state
}

func (m *MockWorkerState) SetMessagesToBePushed(messages []*transport.UMHMessage) {
	m.outbound = messages
}

func (m *MockWorkerState) GetMessagesToBePushed() []*transport.UMHMessage {
	return m.outbound
}

// testLogger creates a no-op logger for tests.
var integrationTestLogger = zap.NewNop().Sugar()

var _ = Describe("FSMv2 Bridge Integration", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		bridge       *fsmv2_bridge.Bridge
		inboundChan  chan *models.UMHMessage
		outboundChan chan *models.UMHMessage
		workerState  *MockWorkerState
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		inboundChan = make(chan *models.UMHMessage, 100)
		outboundChan = make(chan *models.UMHMessage, 100)
		workerState = newMockWorkerState()
		bridge = fsmv2_bridge.NewBridge(
			workerState,
			workerState,
			inboundChan,
			outboundChan,
			integrationTestLogger,
		)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Full Message Flow Integration", func() {
		// This test verifies the complete message flow:
		// 1. FSMv2 pulls message -> ObservedState.MessagesReceived
		// 2. Bridge.PollAndForward() reads ObservedState -> inboundChannel
		// 3. Router processes inbound messages -> outboundChannel (simulated here)
		// 4. Bridge.CollectAndWrite() drains outboundChannel -> Dependencies.MessagesToBePushed
		// 5. FSMv2 pushes MessagesToBePushed (verified by checking workerState.outbound)

		It("forwards pulled messages from FSMv2 to Router inbound channel", func() {
			// Arrange: Simulate FSMv2 SyncAction having pulled messages from backend
			// These messages appear in ObservedState.MessagesReceived
			testUUID := uuid.New()
			workerState.SetObservedState(snapshot.CommunicatorObservedState{
				CollectedAt:   time.Now(),
				Authenticated: true,
				MessagesReceived: []transport.UMHMessage{
					{
						Content:      `{"messageType":"action","payload":{"actionType":"get-status"}}`,
						InstanceUUID: testUUID.String(),
						Email:        "user@example.com",
					},
				},
			})

			// Act: Bridge polls from FSMv2 ObservedState and forwards to inboundChannel
			err := bridge.PollAndForward(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert: Message appears in inboundChannel (ready for Router)
			Eventually(func() int {
				return len(inboundChan)
			}).WithTimeout(time.Second).Should(Equal(1))

			msg := <-inboundChan
			Expect(msg.Content).To(ContainSubstring("get-status"))
			Expect(msg.InstanceUUID).To(Equal(testUUID))
			Expect(msg.Email).To(Equal("user@example.com"))
		})

		It("collects Router responses and writes to FSMv2 for push", func() {
			// Arrange: Simulate Router having processed messages and placed responses
			// in outboundChannel (this is what Router.handleAction does via actions.HandleActionMessage)
			testUUID := uuid.New()
			outboundChan <- &models.UMHMessage{
				Content:      `{"status":"ok","version":"1.0.0"}`,
				InstanceUUID: testUUID,
				Email:        "user@example.com",
			}
			outboundChan <- &models.UMHMessage{
				Content:      `{"bridges":[]}`,
				InstanceUUID: testUUID,
				Email:        "user@example.com",
			}

			// Act: Bridge collects from outboundChannel and writes to FSMv2 Dependencies
			err := bridge.CollectAndWrite(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert: Messages appear in FSMv2 Dependencies for next SyncAction to push
			messages := workerState.GetMessagesToBePushed()
			Expect(messages).To(HaveLen(2))
			Expect(messages[0].Content).To(ContainSubstring("status"))
			Expect(messages[0].InstanceUUID).To(Equal(testUUID.String()))
			Expect(messages[1].Content).To(ContainSubstring("bridges"))
		})

		It("handles full integration cycle: pull -> Router -> push", func() {
			// This test simulates the complete cycle that happens in production:
			// 1. FSMv2 SyncAction pulls messages from backend -> ObservedState.MessagesReceived
			// 2. Bridge.PollAndForward() reads ObservedState -> inboundChannel
			// 3. Router reads inboundChannel, processes, writes to outboundChannel
			// 4. Bridge.CollectAndWrite() drains outboundChannel -> Dependencies.MessagesToBePushed
			// 5. FSMv2 SyncAction pushes Dependencies.MessagesToBePushed to backend

			// Step 1: Simulate FSMv2 having pulled a message
			testUUID := uuid.New()
			workerState.SetObservedState(snapshot.CommunicatorObservedState{
				CollectedAt:   time.Now(),
				Authenticated: true,
				MessagesReceived: []transport.UMHMessage{
					{
						Content:      `{"messageType":"action","payload":{"actionType":"get-version"}}`,
						InstanceUUID: testUUID.String(),
						Email:        "admin@umh.app",
					},
				},
			})

			// Step 2: Bridge forwards to Router's inbound channel
			err := bridge.PollAndForward(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify message reached inbound channel
			var inboundMsg *models.UMHMessage
			Eventually(func() bool {
				select {
				case inboundMsg = <-inboundChan:
					return true
				default:
					return false
				}
			}).WithTimeout(time.Second).Should(BeTrue())
			Expect(inboundMsg.Content).To(ContainSubstring("get-version"))

			// Step 3: Simulate Router processing and producing response
			// In production, Router.handleAction would call actions.HandleActionMessage
			// which writes responses to outboundChannel
			responseMsg := &models.UMHMessage{
				Content:      `{"version":"2.0.0","build":"abc123"}`,
				InstanceUUID: testUUID,
				Email:        "admin@umh.app",
			}
			outboundChan <- responseMsg

			// Step 4: Bridge collects responses for FSMv2 to push
			err = bridge.CollectAndWrite(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Step 5: Verify messages are ready for FSMv2 to push
			pushMessages := workerState.GetMessagesToBePushed()
			Expect(pushMessages).To(HaveLen(1))
			Expect(pushMessages[0].Content).To(ContainSubstring("version"))
			Expect(pushMessages[0].Content).To(ContainSubstring("2.0.0"))
			Expect(pushMessages[0].InstanceUUID).To(Equal(testUUID.String()))
			Expect(pushMessages[0].Email).To(Equal("admin@umh.app"))
		})

		It("handles multiple messages in a single cycle", func() {
			// Test that Bridge correctly handles batches of messages
			testUUID := uuid.New()

			// Multiple inbound messages (e.g., multiple users sending actions)
			workerState.SetObservedState(snapshot.CommunicatorObservedState{
				CollectedAt:   time.Now(),
				Authenticated: true,
				MessagesReceived: []transport.UMHMessage{
					{Content: `{"action":"get-status"}`, InstanceUUID: testUUID.String(), Email: "user1@example.com"},
					{Content: `{"action":"get-bridges"}`, InstanceUUID: testUUID.String(), Email: "user2@example.com"},
					{Content: `{"action":"get-dataflows"}`, InstanceUUID: testUUID.String(), Email: "user3@example.com"},
				},
			})

			// Forward all messages
			forwarded, dropped, err := bridge.PollAndForwardWithStats(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(forwarded).To(Equal(3))
			Expect(dropped).To(Equal(0))

			// Verify all messages reached inbound channel
			Expect(inboundChan).To(HaveLen(3))

			// Simulate Router producing responses for each
			for i := range 3 {
				outboundChan <- &models.UMHMessage{
					Content:      `{"response":` + string(rune('0'+i)) + `}`,
					InstanceUUID: testUUID,
					Email:        "user@example.com",
				}
			}

			// Collect all responses
			err = bridge.CollectAndWrite(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Verify all responses are queued for push
			pushMessages := workerState.GetMessagesToBePushed()
			Expect(pushMessages).To(HaveLen(3))
		})

		It("preserves message integrity through the full cycle", func() {
			// Verify that message content, UUIDs, and emails are preserved exactly
			originalUUID := uuid.New()
			originalContent := `{"complex":{"nested":"data"},"array":[1,2,3]}`
			originalEmail := "complex+email@sub.domain.example.com"

			workerState.SetObservedState(snapshot.CommunicatorObservedState{
				CollectedAt:   time.Now(),
				Authenticated: true,
				MessagesReceived: []transport.UMHMessage{
					{
						Content:      originalContent,
						InstanceUUID: originalUUID.String(),
						Email:        originalEmail,
					},
				},
			})

			// Forward to inbound
			err := bridge.PollAndForward(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Check inbound message integrity
			inboundMsg := <-inboundChan
			Expect(inboundMsg.Content).To(Equal(originalContent))
			Expect(inboundMsg.InstanceUUID).To(Equal(originalUUID))
			Expect(inboundMsg.Email).To(Equal(originalEmail))

			// Simulate Router echoing back (for testing purposes)
			outboundChan <- inboundMsg

			// Collect to outbound
			err = bridge.CollectAndWrite(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Check outbound message integrity
			pushMessages := workerState.GetMessagesToBePushed()
			Expect(pushMessages).To(HaveLen(1))
			Expect(pushMessages[0].Content).To(Equal(originalContent))
			Expect(pushMessages[0].InstanceUUID).To(Equal(originalUUID.String()))
			Expect(pushMessages[0].Email).To(Equal(originalEmail))
		})
	})
})
