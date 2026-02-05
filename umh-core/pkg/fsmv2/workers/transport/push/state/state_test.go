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
		ConsecutiveErrors: consecutiveErrors,
		HasTransport:      hasTransport,
		HasValidToken:     hasValidToken,
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

	It("should return a valid String()", func() {
		Expect(s.String()).To(Equal("Stopping"))
	})
})
