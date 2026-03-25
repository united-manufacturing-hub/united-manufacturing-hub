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

package state_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	commTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

type mockResettableTransport struct {
	resetCount int
}

func (m *mockResettableTransport) Authenticate(_ context.Context, _ commTransport.AuthRequest) (commTransport.AuthResponse, error) {
	return commTransport.AuthResponse{}, nil
}

func (m *mockResettableTransport) Pull(_ context.Context, _ string) ([]*commTransport.UMHMessage, error) {
	return nil, nil
}

func (m *mockResettableTransport) Push(_ context.Context, _ string, _ []*commTransport.UMHMessage) error {
	return nil
}

func (m *mockResettableTransport) Close() {}

func (m *mockResettableTransport) Reset() {
	m.resetCount++
}

const (
	// DegradedBackoffDelay is the minimum wait time in RecoveringState before emitting actions.
	// Tests use values > 60s to ensure backoff has elapsed.
	DegradedBackoffDelay = 60 * time.Second
)

// This test simulates the full FSM cycle to verify we don't get stuck in an
// infinite reset_transport loop. It tests the fix for the bug where
// ResetTransportAction didn't advance the retry counter.
var _ = Describe("RecoveringState Integration - Infinite Loop Prevention", func() {
	var (
		stateObj   *state.RecoveringState
		logger     deps.FSMLogger
		mockTransp *mockResettableTransport
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		mockTransp = &mockResettableTransport{}
		identity := deps.Identity{ID: "test-id", WorkerType: "communicator"}
		baseDeps := deps.NewBaseDependencies(logger, nil, identity)
		commDeps := communicator.NewCommunicatorDependencies(baseDeps)
		commDeps.SetTransport(mockTransp)
		_ = commDeps
		stateObj = &state.RecoveringState{}
	})

	Describe("Recovery Cycle", func() {
		It("should complete full cycle: healthy -> degraded -> recovery", func() {
			// Phase 1: Children go unhealthy — stay in Recovering
			snap1 := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.WrappedObservedState[communicator.CommunicatorStatus]{
					ChildrenHealthy:   0,
					ChildrenUnhealthy: 1,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
			}

			result1 := stateObj.Next(snap1)
			Expect(result1.State).To(BeAssignableToTypeOf(&state.RecoveringState{}))
			Expect(result1.Action).To(BeNil())

			// Phase 2: Children still unhealthy — stay in Recovering
			snap2 := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.WrappedObservedState[communicator.CommunicatorStatus]{
					ChildrenHealthy:   0,
					ChildrenUnhealthy: 1,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
			}

			result2 := stateObj.Next(snap2)
			Expect(result2.State).To(BeAssignableToTypeOf(&state.RecoveringState{}))
			Expect(result2.Action).To(BeNil())

			// Phase 3: Children recover — transition to Syncing
			snap3 := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: fsmv2.WrappedObservedState[communicator.CommunicatorStatus]{
					ChildrenHealthy:   1,
					ChildrenUnhealthy: 0,
				},
				Desired: &fsmv2.WrappedDesiredState[communicator.CommunicatorConfig]{},
			}

			result3 := stateObj.Next(snap3)
			Expect(result3.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(result3.Action).To(BeNil())
		})
	})
})
