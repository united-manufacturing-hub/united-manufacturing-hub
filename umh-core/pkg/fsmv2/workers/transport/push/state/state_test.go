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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/state"
)

func makeSnapshot(
	parentMappedState string,
	shutdownRequested bool,
	consecutiveErrors int,
	hasTransport bool,
	hasValidToken bool,
) fsmv2.Snapshot {
	return makeSnapshotFull(parentMappedState, shutdownRequested, consecutiveErrors, hasTransport, hasValidToken, 0)
}

func makeSnapshotFull(
	parentMappedState string,
	shutdownRequested bool,
	consecutiveErrors int,
	hasTransport bool,
	hasValidToken bool,
	pendingMessageCount int,
) fsmv2.Snapshot {
	return makeSnapshotWithBackoff(parentMappedState, shutdownRequested, consecutiveErrors, hasTransport, hasValidToken, pendingMessageCount, 0, 0, time.Time{}, time.Time{})
}

func makeSnapshotWithBackoff(
	parentMappedState string,
	shutdownRequested bool,
	consecutiveErrors int,
	hasTransport bool,
	hasValidToken bool,
	pendingMessageCount int,
	lastErrorType httpTransport.ErrorType,
	lastRetryAfter time.Duration,
	degradedEnteredAt time.Time,
	lastErrorAt time.Time,
) fsmv2.Snapshot {
	desired := &snapshot.PushDesiredState{
		ParentMappedState: parentMappedState,
		BaseDesiredState: config.BaseDesiredState{
			ShutdownRequested: shutdownRequested,
		},
	}

	observed := snapshot.PushObservedState{
		CollectedAt: time.Now(),
		PushDesiredState: snapshot.PushDesiredState{
			ParentMappedState: parentMappedState,
			BaseDesiredState: config.BaseDesiredState{
				ShutdownRequested: shutdownRequested,
			},
		},
		ConsecutiveErrors:   consecutiveErrors,
		PendingMessageCount: pendingMessageCount,
		HasTransport:        hasTransport,
		HasValidToken:       hasValidToken,
		LastErrorType:       lastErrorType,
		LastRetryAfter:      lastRetryAfter,
		DegradedEnteredAt:   degradedEnteredAt,
		LastErrorAt:         lastErrorAt,
	}

	return fsmv2.Snapshot{
		Observed: observed,
		Desired:  desired,
		Identity: deps.Identity{
			ID:         "test-push-worker",
			Name:       "push",
			WorkerType: "push",
		},
	}
}

var _ = Describe("StoppedState", func() {
	var s *state.StoppedState

	BeforeEach(func() {
		s = &state.StoppedState{}
	})

	It("should return LifecyclePhase PhaseStopped", func() {
		Expect(s.LifecyclePhase()).To(Equal(config.PhaseStopped))
	})

	It("should signal NeedsRemoval on shutdown", func() {
		snap := makeSnapshot(config.DesiredStateRunning, true, 0, false, false)
		result := s.Next(snap)
		Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
	})

	It("should transition to Running when ShouldBeRunning", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 0, false, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
	})

	It("should stay Stopped when not ShouldBeRunning", func() {
		snap := makeSnapshot(config.DesiredStateStopped, false, 0, false, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
	})

	It("should return a valid String()", func() {
		Expect(s.String()).To(Equal("Stopped"))
	})
})

var _ = Describe("RunningState", func() {
	var s *state.RunningState

	BeforeEach(func() {
		s = &state.RunningState{}
	})

	It("should return LifecyclePhase PhaseRunningHealthy", func() {
		Expect(s.LifecyclePhase()).To(Equal(config.PhaseRunningHealthy))
	})

	It("should transition to Stopping on IsStopRequired (shutdown)", func() {
		snap := makeSnapshot(config.DesiredStateRunning, true, 0, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
	})

	It("should transition to Stopping on IsStopRequired (parent stopped)", func() {
		snap := makeSnapshot(config.DesiredStateStopped, false, 0, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
	})

	It("should transition to Degraded on 3+ consecutive errors", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 3, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
	})

	It("should stay Running and emit PushAction when less than 3 errors", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 2, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("push"))
	})

	It("should stay Running and emit PushAction with 0 errors", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 0, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("push"))
	})

	It("should stay Running with nil action when waiting for transport", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 0, false, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).To(BeNil())
	})

	It("should stay Running with nil action when waiting for token", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 0, true, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).To(BeNil())
	})

	It("should transition to Degraded when pending messages exceed threshold", func() {
		snap := makeSnapshotFull(config.DesiredStateRunning, false, 0, true, true, 100)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Reason).To(ContainSubstring("pending"))
	})

	It("should stay Running when pending messages below threshold", func() {
		snap := makeSnapshotFull(config.DesiredStateRunning, false, 0, true, true, 99)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Action).NotTo(BeNil())
	})

	It("should return a valid String()", func() {
		Expect(s.String()).To(Equal("Running"))
	})
})

var _ = Describe("DegradedState", func() {
	var s *state.DegradedState

	BeforeEach(func() {
		s = &state.DegradedState{}
	})

	It("should return LifecyclePhase PhaseRunningDegraded", func() {
		Expect(s.LifecyclePhase()).To(Equal(config.PhaseRunningDegraded))
	})

	It("should transition to Stopping on IsStopRequired", func() {
		snap := makeSnapshot(config.DesiredStateRunning, true, 5, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
	})

	It("should recover to Running on 0 errors", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 0, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
	})

	It("should stay Degraded and emit PushAction with non-zero errors", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 5, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("push"))
	})

	It("should stay Degraded with nil action when waiting for transport or token", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 5, false, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).To(BeNil())
	})

	It("should wait for backoff before retrying push in degraded", func() {
		snap := makeSnapshotWithBackoff(
			config.DesiredStateRunning, false, 3, true, true, 5,
			httpTransport.ErrorTypeNetwork, 0,
			time.Now(), // degraded just entered
			time.Time{},
		)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Action).To(BeNil())
		Expect(result.Reason).To(ContainSubstring("backoff"))
	})

	It("should dispatch push when backoff has expired", func() {
		snap := makeSnapshotWithBackoff(
			config.DesiredStateRunning, false, 1, true, true, 2,
			httpTransport.ErrorTypeNetwork, 0,
			time.Now().Add(-10*time.Second), // degraded 10s ago, backoff for 1 error = 2s
			time.Time{},
		)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("push"))
	})

	It("should respect Retry-After header in degraded", func() {
		snap := makeSnapshotWithBackoff(
			config.DesiredStateRunning, false, 1, true, true, 1,
			httpTransport.ErrorTypeServerError, 60*time.Second,
			time.Time{},
			time.Now(), // error just occurred, Retry-After = 60s
		)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Action).To(BeNil())
		Expect(result.Reason).To(ContainSubstring("backoff"))
	})

	It("should use DegradedEnteredAt for exponential backoff timing", func() {
		snap := makeSnapshotWithBackoff(
			config.DesiredStateRunning, false, 5, true, true, 3,
			httpTransport.ErrorTypeNetwork, 0,
			time.Now().Add(-1*time.Second), // degraded 1s ago, backoff for 5 errors = 32s
			time.Time{},
		)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Action).To(BeNil())
		Expect(result.Reason).To(ContainSubstring("backoff"))
	})

	It("should return a valid String()", func() {
		Expect(s.String()).To(Equal("Degraded"))
	})
})

var _ = Describe("StoppingState", func() {
	var s *state.StoppingState

	BeforeEach(func() {
		s = &state.StoppingState{}
	})

	It("should return LifecyclePhase PhaseStopping", func() {
		Expect(s.LifecyclePhase()).To(Equal(config.PhaseStopping))
	})

	It("should transition to Stopped when stop is required", func() {
		snap := makeSnapshot(config.DesiredStateStopped, false, 0, false, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
	})

	It("should transition to Stopped on shutdown requested", func() {
		snap := makeSnapshot(config.DesiredStateRunning, true, 0, false, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
	})

	It("should transition to Stopped unconditionally (ENG-4608)", func() {
		snap := makeSnapshot(config.DesiredStateRunning, false, 0, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Reason).To(ContainSubstring("stop complete"))
	})

	Describe("stop signal reverted during shutdown", func() {
		It("should recover when parent transitions back to Running (token re-auth)", func() {
			snapParentStopped := makeSnapshot(config.DesiredStateStopped, false, 0, true, true)
			result := s.Next(snapParentStopped)
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))

			stopped := result.State.(*state.StoppedState)
			snapParentRunning := makeSnapshot(config.DesiredStateRunning, false, 0, true, true)
			result = stopped.Next(snapParentRunning)
			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		})

		It("should recover when shutdown is cancelled mid-stop", func() {
			snapShutdown := makeSnapshot(config.DesiredStateRunning, true, 0, true, true)
			result := s.Next(snapShutdown)
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))

			stopped := result.State.(*state.StoppedState)
			snapNoShutdown := makeSnapshot(config.DesiredStateRunning, false, 0, true, true)
			result = stopped.Next(snapNoShutdown)
			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		})

		It("should handle parent flapping between Running and Starting", func() {
			for i := 0; i < 3; i++ {
				stopping := &state.StoppingState{}
				snapStopped := makeSnapshot(config.DesiredStateStopped, false, 0, true, true)
				result := stopping.Next(snapStopped)
				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))

				stopped := result.State.(*state.StoppedState)
				snapRunning := makeSnapshot(config.DesiredStateRunning, false, 0, true, true)
				result = stopped.Next(snapRunning)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			}
		})
	})

	It("should return a valid String()", func() {
		Expect(s.String()).To(Equal("Stopping"))
	})
})
