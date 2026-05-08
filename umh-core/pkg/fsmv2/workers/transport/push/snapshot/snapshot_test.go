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

package snapshot_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
)

var _ = Describe("PushObservedState", func() {
	Describe("GetTimestamp", func() {
		It("should return the CollectedAt timestamp", func() {
			now := time.Now()
			observed := snapshot.PushObservedState{
				CollectedAt: now,
			}

			Expect(observed.GetTimestamp()).To(Equal(now))
		})
	})

	Describe("GetObservedDesiredState", func() {
		It("should return a pointer to the embedded desired state", func() {
			observed := snapshot.PushObservedState{
				CollectedAt: time.Now(),
				State:       "running",
			}

			desired := observed.GetObservedDesiredState()
			Expect(desired).NotTo(BeNil())
		})
	})

	Describe("SetState", func() {
		It("should return an updated observed state with the new state", func() {
			observed := snapshot.PushObservedState{
				CollectedAt: time.Now(),
				State:       "stopped",
			}

			result := observed.SetState("running")
			updated, ok := result.(snapshot.PushObservedState)
			Expect(ok).To(BeTrue())
			Expect(updated.State).To(Equal("running"))
		})
	})

	Describe("SetParentMappedState", func() {
		It("should return an updated observed state with the new parent mapped state", func() {
			observed := snapshot.PushObservedState{}

			result := observed.SetParentMappedState("running")
			updated, ok := result.(snapshot.PushObservedState)
			Expect(ok).To(BeTrue())
			Expect(updated.ParentMappedState).To(Equal("running"))
		})
	})

	Describe("SetShutdownRequested", func() {
		It("should return an updated observed state with shutdown requested", func() {
			observed := snapshot.PushObservedState{}

			result := observed.SetShutdownRequested(true)
			updated, ok := result.(snapshot.PushObservedState)
			Expect(ok).To(BeTrue())
			Expect(updated.IsShutdownRequested()).To(BeTrue())
		})
	})
})

var _ = Describe("PushDesiredState", func() {
	Describe("ShouldBeRunning", func() {
		It("should return true when ParentMappedState is running and shutdown is not requested", func() {
			desired := &snapshot.PushDesiredState{
				ParentMappedState: "running",
			}

			Expect(desired.ShouldBeRunning()).To(BeTrue())
		})

		It("should return false when ShutdownRequested is true", func() {
			desired := &snapshot.PushDesiredState{
				ParentMappedState: "running",
			}
			desired.SetShutdownRequested(true)

			Expect(desired.ShouldBeRunning()).To(BeFalse())
		})

		It("should return false when ParentMappedState is stopped", func() {
			desired := &snapshot.PushDesiredState{
				ParentMappedState: "stopped",
			}

			Expect(desired.ShouldBeRunning()).To(BeFalse())
		})
	})
})

var _ = Describe("PushObservedState.IsStopRequired", func() {
	DescribeTable("should correctly determine stop requirement",
		func(shutdownRequested bool, parentMappedState string, want bool) {
			obs := snapshot.PushObservedState{
				PushDesiredState: snapshot.PushDesiredState{
					ParentMappedState: parentMappedState,
				},
			}
			obs.PushDesiredState.SetShutdownRequested(shutdownRequested)

			Expect(obs.IsStopRequired()).To(Equal(want))
		},
		Entry("returns true when shutdown requested", true, "running", true),
		Entry("returns true when parent mapped state is stopped", false, "stopped", true),
		Entry("returns true when parent mapped state is empty", false, "", true),
		Entry("returns false when running and not shutdown requested", false, "running", false),
		Entry("returns true when both shutdown requested and parent stopped", true, "stopped", true),
	)
})
