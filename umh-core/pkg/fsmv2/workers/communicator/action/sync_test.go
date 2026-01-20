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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

var _ = Describe("SyncAction", func() {
	var (
		act           *action.SyncAction
		dependencies  *communicator.CommunicatorDependencies
		logger        *zap.SugaredLogger
		mockTransport *mockSyncTransport
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockTransport = &mockSyncTransport{}
		identity := fsmv2.Identity{ID: "test-id", WorkerType: "communicator"}
		dependencies = communicator.NewCommunicatorDependencies(mockTransport, logger, nil, identity)
		// Dependencies now passed to Execute(), not constructor
		act = action.NewSyncAction("test-jwt-token")
	})

	Describe("Message Storage", func() {
		It("should store pulled messages in dependencies after successful sync", func() {
			ctx := context.Background()
			expectedMessages := []*transport.UMHMessage{
				{Email: "test@example.com", InstanceUUID: "uuid-1", Content: "message-1"},
				{Email: "test@example.com", InstanceUUID: "uuid-2", Content: "message-2"},
			}
			mockTransport.pullResponse = expectedMessages

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Verify pulled messages are stored in dependencies
			storedMessages := dependencies.GetPulledMessages()
			Expect(storedMessages).To(HaveLen(2))
			Expect(storedMessages[0].Content).To(Equal("message-1"))
			Expect(storedMessages[1].Content).To(Equal("message-2"))
		})

		It("should store empty slice when no messages are pulled", func() {
			ctx := context.Background()
			mockTransport.pullResponse = []*transport.UMHMessage{}

			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())

			// Verify empty slice is stored
			storedMessages := dependencies.GetPulledMessages()
			Expect(storedMessages).To(BeEmpty())
		})

		It("should overwrite previous messages on subsequent pulls", func() {
			ctx := context.Background()

			// First pull with messages
			mockTransport.pullResponse = []*transport.UMHMessage{
				{Email: "test@example.com", InstanceUUID: "uuid-1", Content: "first"},
			}
			err := act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.GetPulledMessages()).To(HaveLen(1))
			Expect(dependencies.GetPulledMessages()[0].Content).To(Equal("first"))

			// Second pull with different messages
			mockTransport.pullResponse = []*transport.UMHMessage{
				{Email: "test@example.com", InstanceUUID: "uuid-2", Content: "second"},
				{Email: "test@example.com", InstanceUUID: "uuid-3", Content: "third"},
			}
			err = act.Execute(ctx, dependencies)
			Expect(err).NotTo(HaveOccurred())
			Expect(dependencies.GetPulledMessages()).To(HaveLen(2))
			Expect(dependencies.GetPulledMessages()[0].Content).To(Equal("second"))
		})
	})

	Describe("Idempotency (Invariant I10)", func() {
		It("should be idempotent when sync succeeds", func() {
			ctx := context.Background()
			for range 3 {
				err := act.Execute(ctx, dependencies)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(mockTransport.pullCallCount).To(Equal(3))
			Expect(mockTransport.pushCallCount).To(Equal(0))
		})

		It("should be idempotent when pushing messages", func() {
			ctx := context.Background()
			act.MessagesToBePushed = []*transport.UMHMessage{
				{Email: "test@example.com", InstanceUUID: "uuid", Content: "msg1"},
			}

			for range 3 {
				err := act.Execute(ctx, dependencies)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(mockTransport.pullCallCount).To(Equal(3))
			Expect(mockTransport.pushCallCount).To(Equal(3))
		})
	})
})

type mockSyncTransport struct {
	pullCallCount int
	pushCallCount int
	pullResponse  []*transport.UMHMessage
}

func (m *mockSyncTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}

func (m *mockSyncTransport) Pull(ctx context.Context, jwtToken string) ([]*transport.UMHMessage, error) {
	m.pullCallCount++

	return m.pullResponse, nil
}

func (m *mockSyncTransport) Push(ctx context.Context, jwtToken string, messages []*transport.UMHMessage) error {
	m.pushCallCount++

	return nil
}

func (m *mockSyncTransport) Close() {
}

func (m *mockSyncTransport) Reset() {
}
