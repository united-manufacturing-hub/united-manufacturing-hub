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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// Parity test for parent-state child gating after the ChildStartStates
// deletion (L5a): the Enabled flag on rendered ChildSpecs is now the only
// gate. Children stay enabled (idle-but-resident) for the parent's entire
// auth trajectory — Starting, AuthFailed, re-auth from Running/Degraded —
// because push/pull actions are gated separately by HasTransport &&
// HasValidToken. This prevents child churn during auth windows. Only the
// stop trajectory (Stopping, Stopped) emits Enabled=false, keeping children
// resident but idle.

// expectChildren asserts that a state emitted both push and pull specs with
// the expected Enabled value.
func expectChildren(specs []config.ChildSpec, enabled bool) {
	ExpectWithOffset(1, specs).To(HaveLen(2))

	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
		ExpectWithOffset(1, spec.Enabled).To(Equal(enabled),
			"child %q: expected Enabled=%t", spec.Name, enabled)
	}

	ExpectWithOffset(1, names).To(ConsistOf("push", "pull"))
}

// makeDisabledSnapshot creates a snapshot with Disabled=true so
// StoppedState's IsDisabled branch is reachable.
func makeDisabledSnapshot() fsmv2.Snapshot {
	desired := &fsmv2.WrappedDesiredState[snapshot.TransportDesiredState]{
		State: config.DesiredStateStopped,
		BaseDesiredState: config.BaseDesiredState{
			Disabled: true,
		},
		Config: snapshot.TransportDesiredState{
			State:        config.DesiredStateStopped,
			InstanceUUID: "test-uuid",
			AuthToken:    "test-auth-token",
			RelayURL:     "https://relay.test.com",
			Timeout:      30 * time.Second,
		},
	}
	obs := fsmv2.Observation[snapshot.TransportStatus]{}

	return fsmv2.Snapshot{Observed: obs, Desired: desired}
}

var _ = Describe("Transport parent-state child gating (Enabled flag parity)", func() {
	validExpiry := time.Now().Add(1 * time.Hour)
	expiredAt := time.Now().Add(-1 * time.Hour)

	Describe("StoppedState", func() {
		s := &state.StoppedState{}

		It("emits disabled children when shutdown is requested", func() {
			result := s.Next(makeSnapshot(true, config.DesiredStateRunning, "", time.Time{}, 0, 0))

			Expect(result.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
			expectChildren(result.Children, false)
		})

		It("emits disabled children when administratively disabled", func() {
			result := s.Next(makeDisabledSnapshot())

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			expectChildren(result.Children, false)
		})

		It("emits enabled children one tick early when transitioning to Starting", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "", time.Time{}, 0, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StartingState{}))
			expectChildren(result.Children, true)
		})

		It("emits disabled children while staying stopped", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateStopped, "", time.Time{}, 0, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			expectChildren(result.Children, false)
		})
	})

	Describe("StartingState", func() {
		s := &state.StartingState{}

		It("emits disabled children when shutdown is requested", func() {
			result := s.Next(makeSnapshot(true, config.DesiredStateRunning, "", time.Time{}, 0, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
			expectChildren(result.Children, false)
		})

		It("keeps children enabled while dispatching auth", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "", time.Time{}, 0, 0))

			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("authenticate"))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled during auth backoff", func() {
			result := s.Next(makeSnapshotWithBackoff(
				false, config.DesiredStateRunning, "", time.Time{}, 0, 0,
				3, types.ErrorTypeNetwork,
				time.Now(),
				0,
			))

			Expect(result.Reason).To(ContainSubstring("auth backoff"))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled when entering AuthFailed on a permanent error", func() {
			result := s.Next(makeAuthFailedStartingSnapshot(
				"test-auth-token", "https://relay.test.com", "test-uuid",
				"test-auth-token", "https://relay.test.com", "test-uuid",
				1, types.ErrorTypeInvalidToken))

			Expect(result.State).To(BeAssignableToTypeOf(&state.AuthFailedState{}))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled when transitioning to Running on valid token", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 0, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			expectChildren(result.Children, true)
		})
	})

	Describe("RunningState", func() {
		s := &state.RunningState{}

		It("emits disabled children when shutdown is requested", func() {
			result := s.Next(makeSnapshot(true, config.DesiredStateRunning, "valid-token", validExpiry, 2, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
			expectChildren(result.Children, false)
		})

		It("keeps children enabled when re-authenticating on token expiry", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "expired-token", expiredAt, 2, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StartingState{}))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled when entering Degraded", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 1, 1))

			Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled while all healthy", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 2, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			expectChildren(result.Children, true)
		})
	})

	Describe("DegradedState", func() {
		s := &state.DegradedState{}

		It("emits disabled children when shutdown is requested", func() {
			result := s.Next(makeSnapshot(true, config.DesiredStateRunning, "valid-token", validExpiry, 1, 1))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
			expectChildren(result.Children, false)
		})

		It("keeps children enabled when re-authenticating on token expiry", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "expired-token", expiredAt, 1, 1))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StartingState{}))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled while dispatching transport reset", func() {
			result := s.Next(makeSnapshotFull(
				false, config.DesiredStateRunning, "valid-token", validExpiry, 1, 1,
				5, types.ErrorTypeNetwork))

			Expect(result.Action).NotTo(BeNil())
			Expect(result.Action.Name()).To(Equal("reset_transport"))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled when recovering to Running", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 2, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.RunningState{}))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled while staying degraded", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateRunning, "valid-token", validExpiry, 1, 1))

			Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}))
			expectChildren(result.Children, true)
		})
	})

	Describe("AuthFailedState", func() {
		s := &state.AuthFailedState{}

		It("emits disabled children when shutdown is requested", func() {
			result := s.Next(makeAuthFailedSnapshot(
				"test-auth-token", "https://relay.test.com", "test-uuid", true))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppingState{}))
			expectChildren(result.Children, false)
		})

		It("keeps children enabled when retrying auth after a config change", func() {
			result := s.Next(makeAuthFailedStartingSnapshot(
				"new-auth-token", "https://relay.test.com", "test-uuid",
				"old-auth-token", "https://relay.test.com", "test-uuid",
				3, types.ErrorTypeInvalidToken))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StartingState{}))
			expectChildren(result.Children, true)
		})

		It("keeps children enabled while waiting for a config change", func() {
			result := s.Next(makeAuthFailedSnapshot(
				"test-auth-token", "https://relay.test.com", "test-uuid", false))

			Expect(result.State).To(BeAssignableToTypeOf(&state.AuthFailedState{}))
			expectChildren(result.Children, true)
		})
	})

	Describe("StoppingState", func() {
		s := &state.StoppingState{}

		It("emits disabled children on its unconditional transition to Stopped", func() {
			result := s.Next(makeSnapshot(false, config.DesiredStateStopped, "", time.Time{}, 0, 0))

			Expect(result.State).To(BeAssignableToTypeOf(&state.StoppedState{}))
			expectChildren(result.Children, false)
		})
	})
})
