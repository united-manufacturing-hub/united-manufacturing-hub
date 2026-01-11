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

	PIt("should execute sync operation", func() {
		// Tests pending: Need registry pattern updates
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
}

func (m *mockSyncTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}

func (m *mockSyncTransport) Pull(ctx context.Context, jwtToken string) ([]*transport.UMHMessage, error) {
	m.pullCallCount++

	return nil, nil
}

func (m *mockSyncTransport) Push(ctx context.Context, jwtToken string, messages []*transport.UMHMessage) error {
	m.pushCallCount++

	return nil
}

func (m *mockSyncTransport) Close() {
}
