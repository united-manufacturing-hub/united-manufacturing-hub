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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

const (
	// DegradedBackoffDelay is the minimum wait time in DegradedState before emitting actions.
	// Tests use values > 60s to ensure backoff has elapsed.
	DegradedBackoffDelay = 60 * time.Second
)

// This test simulates the full FSM cycle to verify we don't get stuck in an
// infinite reset_transport loop. It tests the fix for the bug where
// ResetTransportAction didn't advance the retry counter.
var _ = Describe("DegradedState Integration - Infinite Loop Prevention", func() {
	var (
		stateObj     *state.DegradedState
		dependencies *communicator.CommunicatorDependencies
		logger       *zap.SugaredLogger
		mockTransp   *mockResettableTransport
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		mockTransp = &mockResettableTransport{}
		identity := deps.Identity{ID: "test-id", WorkerType: "communicator"}
		dependencies = communicator.NewCommunicatorDependencies(mockTransp, logger, nil, identity)
		stateObj = &state.DegradedState{}
	})

	Describe("Reset Transport Loop Prevention", func() {
		// This test documents the exact bug that was found and verifies the fix.
		// Bug: ResetTransportAction didn't call Attempt(), so counter stayed at threshold,
		// causing ShouldResetTransport() to return true indefinitely.
		It("should NOT cause infinite reset_transport loop at reset threshold", func() {
			// Simulate network errors to reach reset threshold (backoff.TransportResetThreshold = 5)
			for range backoff.TransportResetThreshold {
				dependencies.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			}

			Expect(dependencies.GetConsecutiveErrors()).To(Equal(backoff.TransportResetThreshold),
				"Should have exactly TransportResetThreshold errors")

			// First evaluation: Should emit ResetTransportAction at threshold
			snap1 := buildSnapshot(dependencies, httpTransport.ErrorTypeNetwork)
			result1 := stateObj.Next(snap1)

			Expect(result1.State).To(BeAssignableToTypeOf(&state.DegradedState{}),
				"Should stay in DegradedState")
			Expect(result1.Action).NotTo(BeNil(),
				"Should emit an action")
			Expect(result1.Action.Name()).To(Equal("reset_transport"),
				"At threshold errors, should emit reset_transport")

			// Execute the ResetTransportAction - this is where the fix is
			resetAction := result1.Action.(*action.ResetTransportAction)
			err := resetAction.Execute(context.Background(), dependencies)
			Expect(err).NotTo(HaveOccurred())

			// BUG FIX VERIFICATION: Counter should now be threshold+1, not still at threshold
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(backoff.TransportResetThreshold+1),
				"After ResetTransportAction, counter should advance past threshold to break modulo-N trigger")

			// Second evaluation: Should emit SyncAction (NOT another reset_transport!)
			snap2 := buildSnapshot(dependencies, httpTransport.ErrorTypeNetwork)
			result2 := stateObj.Next(snap2)

			Expect(result2.State).To(BeAssignableToTypeOf(&state.DegradedState{}),
				"Should stay in DegradedState")
			Expect(result2.Action).NotTo(BeNil(),
				"Should emit an action")
			Expect(result2.Action.Name()).To(Equal("sync"),
				"At threshold+1 errors, should emit sync (not reset_transport) - THIS BREAKS THE INFINITE LOOP")
		})

		It("should emit reset_transport again at 2x threshold after proper progression", func() {
			threshold := backoff.TransportResetThreshold
			doubleThreshold := threshold * 2

			// Start at threshold errors
			for range threshold {
				dependencies.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			}

			// First reset at threshold errors
			snap1 := buildSnapshot(dependencies, httpTransport.ErrorTypeNetwork)
			result1 := stateObj.Next(snap1)
			Expect(result1.Action.Name()).To(Equal("reset_transport"),
				"At threshold, should emit reset_transport")

			// Execute reset - advances counter to threshold+1
			resetAction1 := result1.Action.(*action.ResetTransportAction)
			_ = resetAction1.Execute(context.Background(), dependencies)
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(threshold+1),
				"After reset, counter should be threshold+1")

			// Simulate sync failures from threshold+1 to 2*threshold-1
			for i := threshold + 2; i < doubleThreshold; i++ {
				dependencies.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
				Expect(dependencies.GetConsecutiveErrors()).To(Equal(i),
					"Counter should be %d", i)

				snap := buildSnapshot(dependencies, httpTransport.ErrorTypeNetwork)
				result := stateObj.Next(snap)
				Expect(result.Action.Name()).To(Equal("sync"),
					"At %d errors, should emit sync (not reset_transport)", i)
			}

			// Error at 2*threshold - should trigger another reset
			dependencies.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(doubleThreshold),
				"Counter should be at 2x threshold")

			snap2x := buildSnapshot(dependencies, httpTransport.ErrorTypeNetwork)
			result2x := stateObj.Next(snap2x)
			Expect(result2x.Action.Name()).To(Equal("reset_transport"),
				"At 2x threshold errors, should emit reset_transport again")
		})

		It("should recover to Syncing when sync succeeds", func() {
			// Start in degraded with threshold+1 errors
			errorCount := backoff.TransportResetThreshold + 1
			for range errorCount {
				dependencies.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			}

			Expect(dependencies.GetConsecutiveErrors()).To(Equal(errorCount),
				"Should have threshold+1 errors")

			// Simulate successful sync - this resets the counter
			dependencies.RecordSuccess()

			Expect(dependencies.GetConsecutiveErrors()).To(Equal(0),
				"RecordSuccess() should reset counter to 0")

			// Build snapshot with recovered state
			snap := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: dependencies.GetConsecutiveErrors(),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result := stateObj.Next(snap)

			Expect(result.State).To(BeAssignableToTypeOf(&state.SyncingState{}),
				"Should transition to SyncingState when errors are cleared")
			Expect(result.Action).To(BeNil())
		})

		It("should handle rapid consecutive resets correctly", func() {
			// This tests that even if somehow we get multiple resets in sequence,
			// each one advances the counter properly
			threshold := backoff.TransportResetThreshold

			// Start at threshold errors
			for range threshold {
				dependencies.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			}

			// First reset
			resetAction := action.NewResetTransportAction()
			_ = resetAction.Execute(context.Background(), dependencies)
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(threshold+1),
				"After first reset, counter should be threshold+1")

			// Force counter back to 5 manually (simulating a bug where something resets it)
			// This shouldn't happen in real code, but let's verify robustness
			// Actually, we can't easily do this without accessing internals...

			// Instead, verify that calling Attempt() multiple times is safe
			tracker := dependencies.RetryTracker()
			tracker.Attempt()
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(threshold+2),
				"After second Attempt(), counter should be threshold+2")

			tracker.Attempt()
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(threshold+3),
				"After third Attempt(), counter should be threshold+3")

			// At threshold+3 errors, should still emit sync (not reset_transport)
			snap := buildSnapshot(dependencies, httpTransport.ErrorTypeNetwork)
			result := stateObj.Next(snap)
			Expect(result.Action.Name()).To(Equal("sync"),
				"At non-threshold error count, should emit sync")
		})
	})

	Describe("Full Recovery Cycle", func() {
		It("should complete full cycle: healthy -> degraded -> reset -> sync retry -> recovery", func() {
			threshold := backoff.TransportResetThreshold

			// Phase 1: Healthy state (simulated by having 0 errors initially)
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(0),
				"Should start with 0 errors")

			// Phase 2: Network failures accumulate to threshold
			for range threshold {
				dependencies.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			}

			// Phase 3: At threshold errors, reset_transport fires
			snap := buildSnapshot(dependencies, httpTransport.ErrorTypeNetwork)
			result := stateObj.Next(snap)
			Expect(result.Action.Name()).To(Equal("reset_transport"),
				"At threshold, should emit reset_transport")

			// Execute reset
			resetAction := result.Action.(*action.ResetTransportAction)
			_ = resetAction.Execute(context.Background(), dependencies)

			// Phase 4: Counter at threshold+1, sync fires
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(threshold+1),
				"After reset, counter should be threshold+1")
			snap2 := buildSnapshot(dependencies, httpTransport.ErrorTypeNetwork)
			result2 := stateObj.Next(snap2)
			Expect(result2.Action.Name()).To(Equal("sync"),
				"At threshold+1, should emit sync")

			// Phase 5: Sync succeeds (network comes back)
			dependencies.RecordSuccess()

			// Phase 6: Recovery - should transition to Syncing
			snap3 := fsmv2.Snapshot{
				Identity: deps.Identity{ID: "test", Name: "test", WorkerType: "communicator"},
				Observed: snapshot.CommunicatorObservedState{
					Authenticated:     true,
					JWTExpiry:         time.Now().Add(time.Hour),
					ConsecutiveErrors: dependencies.GetConsecutiveErrors(),
				},
				Desired: &snapshot.CommunicatorDesiredState{},
			}

			result3 := stateObj.Next(snap3)
			Expect(result3.State).To(BeAssignableToTypeOf(&state.SyncingState{}))
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(0))
		})
	})

	Describe("Retry Tracker Synchronization", func() {
		It("should keep RetryTracker and GetConsecutiveErrors in sync", func() {
			// RecordError should sync both
			dependencies.RecordError()
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(1))
			Expect(dependencies.RetryTracker().ConsecutiveErrors()).To(Equal(1))

			// RecordTypedError should sync both
			dependencies.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(2))
			Expect(dependencies.RetryTracker().ConsecutiveErrors()).To(Equal(2))

			// Attempt() should advance the tracker (and GetConsecutiveErrors reads from tracker)
			dependencies.RetryTracker().Attempt()
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(3))
			Expect(dependencies.RetryTracker().ConsecutiveErrors()).To(Equal(3))

			// RecordSuccess should reset both
			dependencies.RecordSuccess()
			Expect(dependencies.GetConsecutiveErrors()).To(Equal(0))
			Expect(dependencies.RetryTracker().ConsecutiveErrors()).To(Equal(0))
		})
	})
})

// buildSnapshot creates a snapshot with the current dependencies state for DegradedState evaluation.
// Uses GetWorkerID()/GetWorkerType() from BaseDependencies (no production code changes needed).
func buildSnapshot(d *communicator.CommunicatorDependencies, lastErrorType httpTransport.ErrorType) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Identity: deps.Identity{
			ID:         d.GetWorkerID(),
			WorkerType: d.GetWorkerType(),
		},
		Observed: snapshot.CommunicatorObservedState{
			Authenticated:     false,
			ConsecutiveErrors: d.GetConsecutiveErrors(),
			LastErrorType:     lastErrorType,
			// Use time > DegradedBackoffDelay (60s) to ensure backoff has elapsed
			DegradedEnteredAt: time.Now().Add(-DegradedBackoffDelay - 5*time.Second),
		},
		Desired: &snapshot.CommunicatorDesiredState{},
	}
}

// mockResettableTransport implements the Transport interface with Reset() tracking.
type mockResettableTransport struct {
	resetCallCount int
}

func (m *mockResettableTransport) Reset() {
	m.resetCallCount++
}

func (m *mockResettableTransport) Authenticate(_ context.Context, _ transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}

func (m *mockResettableTransport) Pull(_ context.Context, _ string) ([]*transport.UMHMessage, error) {
	return nil, nil
}

func (m *mockResettableTransport) Push(_ context.Context, _ string, _ []*transport.UMHMessage) error {
	return nil
}

func (m *mockResettableTransport) Close() {
	// No-op for mock
}

// Ensure mockResettableTransport implements transport.Transport at compile time
var _ transport.Transport = (*mockResettableTransport)(nil)
