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
)

var _ = Describe("ResetTransportAction", func() {
	var (
		act          *action.ResetTransportAction
		dependencies *communicator.CommunicatorDependencies
		logger       *zap.SugaredLogger
		mockTransp   *mockResettableTransport
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockTransp = &mockResettableTransport{}
		identity := deps.Identity{ID: "test-id", WorkerType: "communicator"}
		dependencies = communicator.NewCommunicatorDependencies(mockTransp, logger, nil, identity)
		act = action.NewResetTransportAction()
	})

	Describe("Name", func() {
		It("should return action name", func() {
			Expect(act.Name()).To(Equal("reset_transport"))
		})
	})

	Describe("Execute", func() {
		It("should call Reset on the transport", func() {
			ctx := context.Background()

			err := act.Execute(ctx, dependencies)

			Expect(err).NotTo(HaveOccurred())
			Expect(mockTransp.resetCallCount).To(Equal(1))
		})

		It("should be idempotent - safe to call multiple times", func() {
			ctx := context.Background()

			for range 3 {
				err := act.Execute(ctx, dependencies)
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(mockTransp.resetCallCount).To(Equal(3))
		})

		// Note: We removed the "nil transport" test case because transport is now
		// GUARANTEED to be non-nil when ResetTransportAction executes:
		// - ResetTransportAction is ONLY called from DegradedState
		// - DegradedState is only reachable AFTER SyncingState
		// - SyncingState is only reachable AFTER TryingToAuthenticateState
		// - TryingToAuthenticateState runs AuthenticateAction which creates the transport
		// Therefore, testing nil transport is testing an impossible scenario.
	})
})

// mockResettableTransport is a mock transport with Reset() method tracking.
type mockResettableTransport struct {
	mockTransport
	resetCallCount int
}

func (m *mockResettableTransport) Reset() {
	m.resetCallCount++
}
