// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Snapshot Immutability (I9)", func() {
	Describe("Go pass-by-value semantics", func() {
		Context("when snapshot is passed to function", func() {
			It("should receive a copy, not reference to original", func() {
				originalSnapshot := fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: &mockObservedState{timestamp: time.Now()},
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
			It("should not affect supervisor's snapshot", func() {
				mutatingState := &snapshotMutatingState{}

				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Observed: &mockObservedState{timestamp: time.Now()},
						Desired:  &mockDesiredState{},
					},
				}

				s := newSupervisorWithWorker(&mockWorker{initialState: mutatingState}, store, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				loadedSnapshot, err := store.LoadSnapshot(context.Background(), "test", "test-worker")
				Expect(err).ToNot(HaveOccurred())
				Expect(loadedSnapshot.Observed).ToNot(BeNil())
				Expect(loadedSnapshot.Identity.Name).To(Equal("Test Worker"))
			})

			It("should demonstrate multiple mutations don't accumulate", func() {
				mutatingState := &snapshotMutatingState{}

				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Observed: &mockObservedState{timestamp: time.Now()},
						Desired:  &mockDesiredState{},
					},
				}

				s := newSupervisorWithWorker(&mockWorker{initialState: mutatingState}, store, supervisor.CollectorHealthConfig{})

				for i := 0; i < 5; i++ {
					err := s.Tick(context.Background())
					Expect(err).ToNot(HaveOccurred())
				}

				loadedSnapshot, err := store.LoadSnapshot(context.Background(), "test", "test-worker")
				Expect(err).ToNot(HaveOccurred())
				Expect(loadedSnapshot.Observed).ToNot(BeNil())
				Expect(loadedSnapshot.Identity.Name).To(Equal("Test Worker"))
			})
		})

		Context("when state mutates Identity field", func() {
			It("should not affect supervisor's identity", func() {
				identityMutatingState := &identityMutatingState{}

				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Observed: &mockObservedState{timestamp: time.Now()},
						Desired:  &mockDesiredState{},
					},
				}

				s := newSupervisorWithWorker(&mockWorker{initialState: identityMutatingState}, store, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				loadedSnapshot, err := store.LoadSnapshot(context.Background(), "test", "test-worker")
				Expect(err).ToNot(HaveOccurred())
				Expect(loadedSnapshot.Identity.ID).To(Equal("test-worker"))
				Expect(loadedSnapshot.Identity.Name).To(Equal("Test Worker"))
			})
		})

		Context("when state tries to modify all snapshot fields", func() {
			It("should not affect any field in supervisor's snapshot", func() {
				aggressiveMutatingState := &aggressiveMutatingState{}

				originalTimestamp := time.Now()
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Observed: &mockObservedState{timestamp: originalTimestamp},
						Desired:  &mockDesiredState{},
					},
				}

				s := newSupervisorWithWorker(&mockWorker{initialState: aggressiveMutatingState}, store, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				loadedSnapshot, err := store.LoadSnapshot(context.Background(), "test", "test-worker")
				Expect(err).ToNot(HaveOccurred())
				Expect(loadedSnapshot.Identity.ID).To(Equal("test-worker"))
				Expect(loadedSnapshot.Identity.Name).To(Equal("Test Worker"))
				Expect(loadedSnapshot.Observed).ToNot(BeNil())
				Expect(loadedSnapshot.Desired).ToNot(BeNil())
				Expect(loadedSnapshot.Observed.GetTimestamp()).To(Equal(originalTimestamp))
			})
		})
	})
})

func receiveSnapshotByValue(snapshot fsmv2.Snapshot) fsmv2.Snapshot {
	return snapshot
}

type snapshotMutatingState struct{}

func (s *snapshotMutatingState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	snapshot.Observed = nil

	return s, fsmv2.SignalNone, nil
}

func (s *snapshotMutatingState) String() string { return "SnapshotMutatingState" }
func (s *snapshotMutatingState) Reason() string { return "testing snapshot immutability" }

type identityMutatingState struct{}

func (s *identityMutatingState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	snapshot.Identity.ID = "hacked-id"
	snapshot.Identity.Name = "Hacked Name"

	return s, fsmv2.SignalNone, nil
}

func (s *identityMutatingState) String() string { return "IdentityMutatingState" }
func (s *identityMutatingState) Reason() string { return "testing identity immutability" }

type aggressiveMutatingState struct{}

func (s *aggressiveMutatingState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	snapshot.Identity.ID = "totally-hacked"
	snapshot.Identity.Name = "Totally Hacked"
	snapshot.Observed = &mockObservedState{timestamp: time.Unix(0, 0)}
	snapshot.Desired = &mockDesiredState{shutdown: true}

	return s, fsmv2.SignalNone, nil
}

func (s *aggressiveMutatingState) String() string { return "AggressiveMutatingState" }
func (s *aggressiveMutatingState) Reason() string {
	return "testing complete snapshot immutability"
}
