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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

var _ = Describe("SyncAction Backpressure", func() {
	var (
		act              *action.SyncAction
		dependencies     *communicator.CommunicatorDependencies
		logger           *zap.SugaredLogger
		mockTransport    *mockBackpressureTransport
		channelProvider  *mockBackpressureChannelProvider
		inboundChan      chan *transport.UMHMessage
		outboundChan     chan *transport.UMHMessage
	)

	BeforeEach(func() {
		// Clear any existing provider from suite setup
		communicator.ClearChannelProvider()

		logger = zap.NewNop().Sugar()
		mockTransport = &mockBackpressureTransport{}

		// Create channels with capacity 100 for testing
		inboundChan = make(chan *transport.UMHMessage, 100)
		outboundChan = make(chan *transport.UMHMessage, 100)

		channelProvider = &mockBackpressureChannelProvider{
			inbound:  inboundChan,
			outbound: outboundChan,
		}
		communicator.SetChannelProvider(channelProvider)

		identity := deps.Identity{ID: "test-id", WorkerType: "communicator"}
		dependencies = communicator.NewCommunicatorDependencies(mockTransport, logger, nil, identity)
		act = action.NewSyncAction("test-jwt-token")
	})

	AfterEach(func() {
		communicator.ClearChannelProvider()
	})

	Describe("Backpressure Detection", func() {
		It("should skip pull when available capacity is below threshold", func() {
			// Setup: Fill channel to leave only 40 available (< 50 threshold)
			// Channel capacity is 100, fill 60 messages
			for range 60 {
				inboundChan <- &transport.UMHMessage{Content: "fill"}
			}

			ctx := context.Background()
			mockTransport.pullResponse = []*transport.UMHMessage{
				{Content: "should-not-be-pulled"},
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Pull should NOT have been called due to backpressure
			Expect(mockTransport.pullCallCount).To(Equal(0))

			// Dependencies should indicate backpressure
			Expect(dependencies.IsBackpressured()).To(BeTrue())
		})

		It("should pull when available capacity is above threshold", func() {
			// Setup: Channel has 80 available (> 50 threshold)
			// Fill only 20 messages
			for range 20 {
				inboundChan <- &transport.UMHMessage{Content: "fill"}
			}

			ctx := context.Background()
			mockTransport.pullResponse = []*transport.UMHMessage{
				{Content: "pulled-message"},
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Pull SHOULD have been called
			Expect(mockTransport.pullCallCount).To(Equal(1))

			// No backpressure
			Expect(dependencies.IsBackpressured()).To(BeFalse())
		})

		It("should NOT trigger backpressure when available equals threshold exactly", func() {
			// Setup: Fill channel to leave exactly 50 available (== ExpectedBatchSize)
			// Channel capacity is 100, fill 50 messages to leave exactly 50 available.
			// This documents the `<` vs `<=` boundary behavior:
			// backpressure triggers when available < ExpectedBatchSize, NOT <=
			for range 50 {
				inboundChan <- &transport.UMHMessage{Content: "fill"}
			}

			ctx := context.Background()
			mockTransport.pullResponse = []*transport.UMHMessage{
				{Content: "pulled-at-boundary"},
			}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Pull SHOULD be called when available == threshold (50 is NOT < 50)
			Expect(mockTransport.pullCallCount).To(Equal(1))

			// Should NOT be backpressured at exact boundary
			Expect(dependencies.IsBackpressured()).To(BeFalse())
		})

		It("should remain backpressured until low water mark is reached (hysteresis)", func() {
			// Setup: Enter backpressure state first
			for range 60 {
				inboundChan <- &transport.UMHMessage{Content: "fill"}
			}

			ctx := context.Background()
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.IsBackpressured()).To(BeTrue())
			Expect(mockTransport.pullCallCount).To(Equal(0))

			// Drain some messages to get 60 available (still below low water mark of 100)
			for range 20 {
				<-inboundChan
			}
			// Now: 40 filled, 60 available

			// Execute again - should STILL be backpressured (60 < 100 low water mark)
			err = act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.IsBackpressured()).To(BeTrue())
			Expect(mockTransport.pullCallCount).To(Equal(0))
		})

		It("should exit backpressure at low water mark", func() {
			// Setup: Enter backpressure state first
			for range 60 {
				inboundChan <- &transport.UMHMessage{Content: "fill"}
			}

			ctx := context.Background()
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.IsBackpressured()).To(BeTrue())

			// Drain all messages to get 100 available (>= low water mark of 100)
			for len(inboundChan) > 0 {
				<-inboundChan
			}

			mockTransport.pullResponse = []*transport.UMHMessage{
				{Content: "resumed-pull"},
			}

			// Execute again - should resume pulling
			err = act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.IsBackpressured()).To(BeFalse())
			Expect(mockTransport.pullCallCount).To(Equal(1))
		})
	})

	Describe("Backpressure Error Handling", func() {
		It("should NOT increment consecutiveErrors when backpressured", func() {
			// Setup: Fill channel to trigger backpressure
			for range 60 {
				inboundChan <- &transport.UMHMessage{Content: "fill"}
			}

			initialErrors := dependencies.GetConsecutiveErrors()

			ctx := context.Background()
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Backpressure is flow control, NOT a transport error
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(initialErrors))
			Expect(dependencies.IsBackpressured()).To(BeTrue())
		})

		It("should return nil error when backpressured (not a failure)", func() {
			// Setup: Fill channel to trigger backpressure
			for range 60 {
				inboundChan <- &transport.UMHMessage{Content: "fill"}
			}

			ctx := context.Background()
			err := act.Execute(ctx, dependencies)

			// Backpressure is not an error - it's normal flow control
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Backpressure Push Behavior", func() {
		It("should still push outbound messages during backpressure", func() {
			// Setup: Fill inbound channel to trigger backpressure
			for range 60 {
				inboundChan <- &transport.UMHMessage{Content: "fill"}
			}

			// Queue outbound messages
			outboundChan <- &transport.UMHMessage{Content: "outbound-1"}
			outboundChan <- &transport.UMHMessage{Content: "outbound-2"}

			ctx := context.Background()
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Pull should NOT have been called (backpressure)
			Expect(mockTransport.pullCallCount).To(Equal(0))

			// Push SHOULD have been called (outbound not affected by inbound backpressure)
			Expect(mockTransport.pushCallCount).To(Equal(1))
			Expect(mockTransport.lastPushedMessages).To(HaveLen(2))
		})
	})
})

// mockBackpressureTransport tracks pull/push calls for backpressure tests.
type mockBackpressureTransport struct {
	pullCallCount      int
	pushCallCount      int
	pullResponse       []*transport.UMHMessage
	lastPushedMessages []*transport.UMHMessage
}

func (m *mockBackpressureTransport) Authenticate(_ context.Context, _ transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}

func (m *mockBackpressureTransport) Pull(_ context.Context, _ string) ([]*transport.UMHMessage, error) {
	m.pullCallCount++

	return m.pullResponse, nil
}

func (m *mockBackpressureTransport) Push(_ context.Context, _ string, messages []*transport.UMHMessage) error {
	m.pushCallCount++
	m.lastPushedMessages = messages

	return nil
}

func (m *mockBackpressureTransport) Close() {}

func (m *mockBackpressureTransport) Reset() {}

// mockBackpressureChannelProvider implements communicator.ChannelProvider with GetInboundStats.
type mockBackpressureChannelProvider struct {
	inbound  chan *transport.UMHMessage
	outbound chan *transport.UMHMessage
}

func (m *mockBackpressureChannelProvider) GetChannels(_ string) (
	chan<- *transport.UMHMessage,
	<-chan *transport.UMHMessage,
) {
	return m.inbound, m.outbound
}

// GetInboundStats returns capacity and current length of the inbound channel.
func (m *mockBackpressureChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return cap(m.inbound), len(m.inbound)
}
