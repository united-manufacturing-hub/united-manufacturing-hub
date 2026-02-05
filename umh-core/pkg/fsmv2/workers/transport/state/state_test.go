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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/state"
)

// makeSnapshot creates a test snapshot with the given parameters.
func makeSnapshot(shutdownRequested bool, desiredState string, jwtToken string, jwtExpiry time.Time, childrenHealthy, childrenUnhealthy int) fsmv2.Snapshot {
	desired := &snapshot.TransportDesiredState{
		BaseDesiredState: config.BaseDesiredState{
			State:             desiredState,
			ShutdownRequested: shutdownRequested,
		},
		InstanceUUID: "test-uuid",
		AuthToken:    "test-auth-token",
		RelayURL:     "https://relay.test.com",
		Timeout:      30 * time.Second,
	}

	observed := snapshot.TransportObservedState{
		CollectedAt:           time.Now(),
		JWTToken:              jwtToken,
		JWTExpiry:             jwtExpiry,
		TransportDesiredState: *desired,
		ChildrenHealthy:       childrenHealthy,
		ChildrenUnhealthy:     childrenUnhealthy,
	}

	return fsmv2.Snapshot{
		Observed: observed,
		Desired:  desired,
	}
}

var _ = Describe("TransportWorker States", func() {

	Describe("StoppedState", func() {
		var s *state.StoppedState

		BeforeEach(func() {
			s = &state.StoppedState{}
		})

		It("should compile and instantiate", func() {
			Expect(s).NotTo(BeNil())
		})

		It("should return Stopped for String()", func() {
			Expect(s.String()).To(Equal("Stopped"))
		})

		It("should return PhaseStopped for LifecyclePhase()", func() {
			Expect(s.LifecyclePhase()).To(Equal(config.PhaseStopped))
		})

		It("should signal removal when shutdown requested", func() {
			snap := makeSnapshot(true, config.DesiredStateRunning, "", time.Time{}, 0, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})

		It("should transition to Starting when desired=running", func() {
			snap := makeSnapshot(false, config.DesiredStateRunning, "", time.Time{}, 0, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StartingState{}))
		})

		It("should stay stopped when desired=stopped", func() {
			snap := makeSnapshot(false, config.DesiredStateStopped, "", time.Time{}, 0, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("StartingState", func() {
		var s *state.StartingState

		BeforeEach(func() {
			s = &state.StartingState{}
		})

		It("should compile and instantiate", func() {
			Expect(s).NotTo(BeNil())
		})

		It("should return Starting for String()", func() {
			Expect(s.String()).To(Equal("Starting"))
		})

		It("should return PhaseStarting for LifecyclePhase()", func() {
			Expect(s.LifecyclePhase()).To(Equal(config.PhaseStarting))
		})

		It("should transition to Stopping when shutdown requested", func() {
			snap := makeSnapshot(true, config.DesiredStateRunning, "", time.Time{}, 0, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
		})

		It("should emit AuthenticateAction when no valid token", func() {
			snap := makeSnapshot(false, config.DesiredStateRunning, "", time.Time{}, 0, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StartingState{}))
			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("authenticate"))
		})

		It("should transition to Running when token is valid", func() {
			validExpiry := time.Now().Add(1 * time.Hour) // Token expires in 1 hour
			snap := makeSnapshot(false, config.DesiredStateRunning, "valid-jwt-token", validExpiry, 0, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("RunningState", func() {
		var s *state.RunningState

		BeforeEach(func() {
			s = &state.RunningState{}
		})

		It("should compile and instantiate", func() {
			Expect(s).NotTo(BeNil())
		})

		It("should return Running for String()", func() {
			Expect(s.String()).To(Equal("Running"))
		})

		It("should return PhaseRunningHealthy for LifecyclePhase()", func() {
			Expect(s.LifecyclePhase()).To(Equal(config.PhaseRunningHealthy))
		})

		It("should transition to Stopping when shutdown requested", func() {
			validExpiry := time.Now().Add(1 * time.Hour)
			snap := makeSnapshot(true, config.DesiredStateRunning, "valid-token", validExpiry, 2, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
		})

		It("should transition to Degraded when children unhealthy", func() {
			validExpiry := time.Now().Add(1 * time.Hour)
			snap := makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 1, 1)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		})

		It("should transition to Starting when token expired", func() {
			expiredExpiry := time.Now().Add(-1 * time.Hour) // Token already expired
			snap := makeSnapshot(false, config.DesiredStateRunning, "expired-token", expiredExpiry, 2, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StartingState{}))
		})

		It("should stay Running when all healthy", func() {
			validExpiry := time.Now().Add(1 * time.Hour)
			snap := makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 2, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			Expect(result.Action).To(BeNil())
		})
	})

	Describe("DegradedState", func() {
		var s *state.DegradedState

		BeforeEach(func() {
			s = &state.DegradedState{}
		})

		It("should compile and instantiate", func() {
			Expect(s).NotTo(BeNil())
		})

		It("should return Degraded for String()", func() {
			Expect(s.String()).To(Equal("Degraded"))
		})

		It("should return PhaseRunningDegraded for LifecyclePhase()", func() {
			Expect(s.LifecyclePhase()).To(Equal(config.PhaseRunningDegraded))
		})

		It("should transition to Stopping when shutdown requested", func() {
			validExpiry := time.Now().Add(1 * time.Hour)
			snap := makeSnapshot(true, config.DesiredStateRunning, "valid-token", validExpiry, 1, 1)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
		})

		It("should transition to Starting when token expired", func() {
			expiredExpiry := time.Now().Add(-1 * time.Hour)
			snap := makeSnapshot(false, config.DesiredStateRunning, "expired-token", expiredExpiry, 1, 1)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StartingState{}))
		})

		It("should transition to Running when all children healthy", func() {
			validExpiry := time.Now().Add(1 * time.Hour)
			snap := makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 2, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
		})

		It("should stay Degraded when children still unhealthy", func() {
			validExpiry := time.Now().Add(1 * time.Hour)
			snap := makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 1, 1)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
		})
	})

	Describe("StoppingState", func() {
		var s *state.StoppingState

		BeforeEach(func() {
			s = &state.StoppingState{}
		})

		It("should compile and instantiate", func() {
			Expect(s).NotTo(BeNil())
		})

		It("should return Stopping for String()", func() {
			Expect(s.String()).To(Equal("Stopping"))
		})

		It("should return PhaseStopping for LifecyclePhase()", func() {
			Expect(s.LifecyclePhase()).To(Equal(config.PhaseStopping))
		})

		It("should transition to Stopped when all children stopped", func() {
			snap := makeSnapshot(true, config.DesiredStateStopped, "", time.Time{}, 0, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})

		It("should stay Stopping when children still running", func() {
			snap := makeSnapshot(true, config.DesiredStateStopped, "", time.Time{}, 1, 0)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
		})

		It("should stay Stopping when children unhealthy but not stopped", func() {
			snap := makeSnapshot(true, config.DesiredStateStopped, "", time.Time{}, 0, 1)
			result := s.Next(snap)

			Expect(result.Signal).To(Equal(fsmv2.SignalNone))
			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
		})
	})

	Describe("Architecture Compliance", func() {
		It("all states should have non-nil return values", func() {
			states := []fsmv2.State[any, any]{
				&state.StoppedState{},
				&state.StartingState{},
				&state.RunningState{},
				&state.DegradedState{},
				&state.StoppingState{},
			}

			snap := makeSnapshot(false, config.DesiredStateRunning, "", time.Time{}, 0, 0)

			for _, s := range states {
				result := s.Next(snap)
				Expect(result.State).NotTo(BeNil(), "State %s returned nil state", s.String())
				Expect(result.Reason).NotTo(BeEmpty(), "State %s returned empty reason", s.String())
			}
		})

		It("all states should have valid String() methods", func() {
			stateTests := []struct {
				state    fsmv2.State[any, any]
				expected string
			}{
				{&state.StoppedState{}, "Stopped"},
				{&state.StartingState{}, "Starting"},
				{&state.RunningState{}, "Running"},
				{&state.DegradedState{}, "Degraded"},
				{&state.StoppingState{}, "Stopping"},
			}

			for _, tt := range stateTests {
				Expect(tt.state.String()).To(Equal(tt.expected))
			}
		})

		It("all states should have valid LifecyclePhase() methods", func() {
			stateTests := []struct {
				state    fsmv2.State[any, any]
				expected config.LifecyclePhase
			}{
				{&state.StoppedState{}, config.PhaseStopped},
				{&state.StartingState{}, config.PhaseStarting},
				{&state.RunningState{}, config.PhaseRunningHealthy},
				{&state.DegradedState{}, config.PhaseRunningDegraded},
				{&state.StoppingState{}, config.PhaseStopping},
			}

			for _, tt := range stateTests {
				Expect(tt.state.LifecyclePhase()).To(Equal(tt.expected), "State %s has wrong lifecycle phase", tt.state.String())
			}
		})
	})
})
