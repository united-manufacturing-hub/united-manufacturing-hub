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

package supervisor_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

var _ = Describe("Snapshot Immutability (I9)", func() {
	Describe("Go pass-by-value semantics", func() {
		Context("when snapshot is passed to function", func() {
			It("should receive a copy, not reference to original", func() {
				originalSnapshot := fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: &mockObservedState{ID: "test", CollectedAt: time.Now()},
					Desired:  &mockDesiredState{},
				}

				copiedSnapshot := receiveSnapshotByValue(originalSnapshot)

				copiedSnapshot.Observed = nil
				copiedSnapshot.Identity.Name = "Modified"

				Expect(originalSnapshot.Observed).ToNot(BeNil())
				Expect(originalSnapshot.Identity.Name).To(Equal("Test Worker"))
			})
		})
	})

	Describe("State.Next() immutability", func() {
		Context("when state tries to mutate snapshot", func() {
			It("should not affect original snapshot passed to state", func() {
				mutatingState := &snapshotMutatingState{}
				originalSnapshot := fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: &mockObservedState{ID: "test", CollectedAt: time.Now()},
					Desired:  &mockDesiredState{},
				}

				// Call state.Next() with the snapshot
				_ = mutatingState.Next(originalSnapshot)

				// Verify original snapshot wasn't mutated
				// Even though state.Next() tried to set Observed to nil,
				// the original should remain unchanged due to pass-by-value semantics
				Expect(originalSnapshot.Observed).ToNot(BeNil())
			})

			It("should demonstrate mutating state returns new snapshot without affecting original", func() {
				mutatingState := &snapshotMutatingState{}
				originalSnapshot := fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: &mockObservedState{ID: "test", CollectedAt: time.Now()},
					Desired:  &mockDesiredState{},
				}

				// Even after multiple invocations, original remains unchanged
				for range 5 {
					_ = mutatingState.Next(originalSnapshot)
					Expect(originalSnapshot.Observed).ToNot(BeNil())
				}
			})
		})

		Context("when state mutates Identity field", func() {
			It("should not affect supervisor's identity", func() {
				identityMutatingState := &identityMutatingState{}
				originalSnapshot := fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: &mockObservedState{ID: "test", CollectedAt: time.Now()},
					Desired:  &mockDesiredState{},
				}

				originalID := originalSnapshot.Identity.ID
				originalName := originalSnapshot.Identity.Name

				// Call state.Next() which tries to mutate identity
				_ = identityMutatingState.Next(originalSnapshot)

				// Verify original identity wasn't changed
				Expect(originalSnapshot.Identity.ID).To(Equal(originalID))
				Expect(originalSnapshot.Identity.Name).To(Equal(originalName))
			})
		})

		Context("when state tries to modify all snapshot fields", func() {
			It("should not affect any field in original snapshot", func() {
				aggressiveMutatingState := &aggressiveMutatingState{}
				originalTimestamp := time.Now()
				originalSnapshot := fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: &mockObservedState{ID: "test", CollectedAt: originalTimestamp},
					Desired:  &mockDesiredState{},
				}

				// Store original values
				originalID := originalSnapshot.Identity.ID
				originalName := originalSnapshot.Identity.Name
				originalObserved := originalSnapshot.Observed
				originalDesired := originalSnapshot.Desired

				// Call state.Next() which tries to mutate everything
				_ = aggressiveMutatingState.Next(originalSnapshot)

				// Verify nothing was mutated in the original
				Expect(originalSnapshot.Identity.ID).To(Equal(originalID))
				Expect(originalSnapshot.Identity.Name).To(Equal(originalName))
				Expect(originalSnapshot.Observed).To(Equal(originalObserved))
				Expect(originalSnapshot.Desired).To(Equal(originalDesired))
			})
		})
	})
})

func receiveSnapshotByValue(snapshot fsmv2.Snapshot) fsmv2.Snapshot {
	return snapshot
}

type snapshotMutatingState struct{}

func (s *snapshotMutatingState) Next(snapshot any) fsmv2.NextResult[any, any] {
	snap := snapshot.(fsmv2.Snapshot)
	snap.Observed = nil

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "testing snapshot immutability")
}

func (s *snapshotMutatingState) String() string { return "SnapshotMutatingState" }

type identityMutatingState struct{}

func (s *identityMutatingState) Next(snapshot any) fsmv2.NextResult[any, any] {
	snap := snapshot.(fsmv2.Snapshot)
	snap.Identity.Name = "Modified"

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "testing identity immutability")
}

func (s *identityMutatingState) String() string { return "IdentityMutatingState" }

type aggressiveMutatingState struct{}

func (s *aggressiveMutatingState) Next(snapshot any) fsmv2.NextResult[any, any] {
	snap := snapshot.(fsmv2.Snapshot)
	snap.Observed = nil
	snap.Identity.Name = "Modified"
	snap.Desired = nil

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "testing complete snapshot immutability")
}

func (s *aggressiveMutatingState) String() string { return "AggressiveMutatingState" }
