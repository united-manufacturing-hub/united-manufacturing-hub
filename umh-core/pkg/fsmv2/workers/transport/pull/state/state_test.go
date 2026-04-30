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
	pull_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/state"
)

func makeSnapshot(
	shutdownRequested bool,
	consecutiveErrors int,
	hasTransport bool,
	hasValidToken bool,
) fsmv2.Snapshot {
	return makeSnapshotFull(shutdownRequested, consecutiveErrors, hasTransport, hasValidToken, 0)
}

func makeSnapshotFull(
	shutdownRequested bool,
	consecutiveErrors int,
	hasTransport bool,
	hasValidToken bool,
	pendingMessageCount int,
) fsmv2.Snapshot {
	return makeSnapshotWithBackoff(shutdownRequested, consecutiveErrors, hasTransport, hasValidToken, pendingMessageCount, 0, 0, time.Time{}, time.Time{})
}

func makeSnapshotWithBackoff(
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
	desired := &fsmv2.WrappedDesiredState[pull_pkg.PullConfig]{
		BaseDesiredState: config.BaseDesiredState{
			ShutdownRequested: shutdownRequested,
		},
	}

	observed := fsmv2.Observation[pull_pkg.PullStatus]{
		CollectedAt: time.Now(),
		Status: pull_pkg.PullStatus{
			ConsecutiveErrors:   consecutiveErrors,
			PendingMessageCount: pendingMessageCount,
			HasTransport:        hasTransport,
			HasValidToken:       hasValidToken,
			LastErrorType:       lastErrorType,
			LastRetryAfter:      lastRetryAfter,
			DegradedEnteredAt:   degradedEnteredAt,
			LastErrorAt:         lastErrorAt,
		},
	}

	return fsmv2.Snapshot{
		Observed: observed,
		Desired:  desired,
		Identity: deps.Identity{
			ID:         "test-pull-worker",
			Name:       "pull",
			WorkerType: "pull",
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
		snap := makeSnapshot(true, 0, false, false)
		result := s.Next(snap)
		Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
	})

	It("should transition to Running when not shutdown (parent enables child via ShutdownRequested=false)", func() {
		snap := makeSnapshot(false, 0, false, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
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

	It("should transition to Stopping on ShouldStop (shutdown requested)", func() {
		snap := makeSnapshot(true, 0, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
	})

	It("should transition to Degraded on 3+ consecutive errors", func() {
		snap := makeSnapshot(false, 3, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
	})

	It("should stay Running and emit PullAction when less than 3 errors", func() {
		snap := makeSnapshot(false, 2, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("pull"))
	})

	It("should stay Running and emit PullAction with 0 errors", func() {
		snap := makeSnapshot(false, 0, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("pull"))
	})

	It("should stay Running with nil action when waiting for transport", func() {
		snap := makeSnapshot(false, 0, false, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).To(BeNil())
	})

	It("should stay Running with nil action when waiting for token", func() {
		snap := makeSnapshot(false, 0, true, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).To(BeNil())
	})

	It("should transition to Degraded when pending messages exceed threshold", func() {
		snap := makeSnapshotFull(false, 0, true, true, 100)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Reason).To(ContainSubstring("pending"))
	})

	It("should stay Running when pending messages below threshold", func() {
		snap := makeSnapshotFull(false, 0, true, true, 99)
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

	It("should transition to Stopping on ShouldStop", func() {
		snap := makeSnapshot(true, 5, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
	})

	It("should recover to Running on 0 errors and low pending count", func() {
		snap := makeSnapshotFull(false, 0, true, true, 0)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
	})

	It("should recover to Running on 0 errors when pending is below threshold", func() {
		snap := makeSnapshotFull(false, 0, true, true, 99)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
	})

	It("should NOT recover to Running when errors are 0 but pending >= threshold (oscillation prevention)", func() {
		snap := makeSnapshotFull(false, 0, true, true, 100)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
	})

	It("should stay Degraded and emit PullAction with non-zero errors", func() {
		snap := makeSnapshot(false, 5, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("pull"))
	})

	It("should stay Degraded and emit PullAction with non-zero errors but low pending count", func() {
		snap := makeSnapshotWithBackoff(
			false, 2, true, true, 10,
			httpTransport.ErrorTypeNetwork, 0,
			time.Now().Add(-30*time.Second),
			time.Time{},
		)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("pull"))
	})

	It("should stay Degraded with nil action when waiting for transport or token", func() {
		snap := makeSnapshot(false, 5, false, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Action).To(BeNil())
	})

	It("should wait for backoff before retrying pull in degraded", func() {
		snap := makeSnapshotWithBackoff(
			false, 3, true, true, 5,
			httpTransport.ErrorTypeNetwork, 0,
			time.Now(), // degraded just entered
			time.Time{},
		)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Action).To(BeNil())
		Expect(result.Reason).To(ContainSubstring("backoff"))
	})

	It("should dispatch pull when backoff has expired", func() {
		snap := makeSnapshotWithBackoff(
			false, 1, true, true, 2,
			httpTransport.ErrorTypeNetwork, 0,
			time.Now().Add(-10*time.Second), // degraded 10s ago, backoff for 1 error = 2s
			time.Time{},
		)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		Expect(result.Action).NotTo(BeNil())
		Expect(result.Action.Name()).To(Equal("pull"))
	})

	It("should respect Retry-After header in degraded", func() {
		snap := makeSnapshotWithBackoff(
			false, 1, true, true, 1,
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
			false, 5, true, true, 3,
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

	It("should transition to Stopped on shutdown requested (stop signal)", func() {
		snap := makeSnapshot(true, 0, false, false)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
	})

	It("should transition to Stopped unconditionally (ENG-4608)", func() {
		snap := makeSnapshot(false, 0, true, true)
		result := s.Next(snap)
		Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
		Expect(result.Signal).To(Equal(fsmv2.SignalNone))
		Expect(result.Reason).To(ContainSubstring("stop complete"))
	})

	// ENG-4608: Scenarios where the stop signal disappears while in Stopping.
	// StoppingState must always progress to Stopped — it must never get stuck.
	// StoppedState handles recovery back to Running when conditions change.
	Describe("stop signal reverted during shutdown", func() {
		It("should recover when parent transitions back to Running (token re-auth)", func() {
			// Scenario: transport parent briefly enters Starting for JWT refresh.
			// Children get ShutdownRequested=true (via ChildSpec.Enabled=false), enter Stopping.
			// Parent re-authenticates, returns to Running, sets ShutdownRequested=false.
			// Children now have ShutdownRequested=false but are in Stopping — they progress.

			// Tick N: parent in Starting → child told to stop
			snapParentStopped := makeSnapshot(true, 0, true, true)
			result := s.Next(snapParentStopped)
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}),
				"Stopping always progresses to Stopped")

			// Tick N+1: parent back in Running → child should recover
			stopped := result.State.(*state.StoppedState)
			snapParentRunning := makeSnapshot(false, 0, true, true)
			result = stopped.Next(snapParentRunning)
			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}),
				"StoppedState recovers to Running when parent is healthy")
		})

		It("should recover when shutdown is cancelled mid-stop", func() {
			// Scenario: shutdown requested, worker enters Stopping.
			// Shutdown is then cancelled (e.g., operator decision, rolling restart aborted).

			// Tick N: shutdown requested
			snapShutdown := makeSnapshot(true, 0, true, true)
			result := s.Next(snapShutdown)
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}),
				"Stopping always progresses to Stopped")

			// Tick N+1: shutdown cancelled, parent still Running
			stopped := result.State.(*state.StoppedState)
			snapNoShutdown := makeSnapshot(false, 0, true, true)
			result = stopped.Next(snapNoShutdown)
			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}),
				"StoppedState recovers to Running when shutdown is cancelled")
		})

		It("should handle parent flapping between Running and Starting", func() {
			// Scenario: parent oscillates (e.g., repeated auth failures/retries).
			// Each cycle: Running → Starting → Running. Children must not get stuck.

			for i := 0; i < 3; i++ {
				// Parent enters Starting → child enters Stopping → Stopped
				stopping := &state.StoppingState{}
				snapStopped := makeSnapshot(true, 0, true, true)
				result := stopping.Next(snapStopped)
				Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}),
					"Cycle %d: Stopping must progress to Stopped", i)

				// Parent returns to Running → child recovers
				stopped := result.State.(*state.StoppedState)
				snapRunning := makeSnapshot(false, 0, true, true)
				result = stopped.Next(snapRunning)
				Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}),
					"Cycle %d: Stopped must recover to Running", i)
			}
		})
	})

	It("should return a valid String()", func() {
		Expect(s.String()).To(Equal("Stopping"))
	})
})
