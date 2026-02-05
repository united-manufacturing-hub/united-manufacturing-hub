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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

var _ = Describe("TransportObservedState", func() {
	Describe("GetTimestamp", func() {
		It("should return the CollectedAt timestamp", func() {
			now := time.Now()
			observed := snapshot.TransportObservedState{
				CollectedAt: now,
			}

			Expect(observed.GetTimestamp()).To(Equal(now))
		})
	})

	Describe("GetObservedDesiredState", func() {
		It("should return a non-nil desired state", func() {
			observed := snapshot.TransportObservedState{
				CollectedAt: time.Now(),
				State:       "running_healthy",
			}

			desired := observed.GetObservedDesiredState()
			Expect(desired).NotTo(BeNil())
		})

		It("should return the embedded desired state", func() {
			observed := snapshot.TransportObservedState{
				CollectedAt: time.Now(),
				TransportDesiredState: snapshot.TransportDesiredState{
					InstanceUUID: "test-uuid",
					AuthToken:    "test-token",
					RelayURL:     "https://example.com",
					Timeout:      30 * time.Second,
				},
			}

			desired := observed.GetObservedDesiredState()
			Expect(desired).NotTo(BeNil())

			transportDesired, ok := desired.(*snapshot.TransportDesiredState)
			Expect(ok).To(BeTrue())
			Expect(transportDesired.InstanceUUID).To(Equal("test-uuid"))
			Expect(transportDesired.AuthToken).To(Equal("test-token"))
			Expect(transportDesired.RelayURL).To(Equal("https://example.com"))
			Expect(transportDesired.Timeout).To(Equal(30 * time.Second))
		})
	})

	Describe("SetState", func() {
		It("should set the state and return a new observed state", func() {
			observed := snapshot.TransportObservedState{
				CollectedAt: time.Now(),
				State:       "stopped",
			}

			newObserved := observed.SetState("running_healthy")
			Expect(newObserved).NotTo(BeNil())

			transportObserved, ok := newObserved.(snapshot.TransportObservedState)
			Expect(ok).To(BeTrue())
			Expect(transportObserved.State).To(Equal("running_healthy"))
		})
	})

	Describe("SetShutdownRequested", func() {
		It("should set shutdown requested and return a new observed state", func() {
			observed := snapshot.TransportObservedState{
				CollectedAt: time.Now(),
			}

			newObserved := observed.SetShutdownRequested(true)
			Expect(newObserved).NotTo(BeNil())

			transportObserved, ok := newObserved.(snapshot.TransportObservedState)
			Expect(ok).To(BeTrue())
			Expect(transportObserved.IsShutdownRequested()).To(BeTrue())
		})
	})

	Describe("HasValidToken", func() {
		It("should return false when JWT token is empty", func() {
			observed := snapshot.TransportObservedState{
				JWTToken:  "",
				JWTExpiry: time.Now().Add(1 * time.Hour),
			}
			Expect(observed.HasValidToken()).To(BeFalse())
		})

		It("should return false when token is present but expired", func() {
			observed := snapshot.TransportObservedState{
				JWTToken:  "valid-token",
				JWTExpiry: time.Now().Add(-1 * time.Hour),
			}
			Expect(observed.HasValidToken()).To(BeFalse())
		})

		It("should return true when token is present and not expired", func() {
			observed := snapshot.TransportObservedState{
				JWTToken:  "valid-token",
				JWTExpiry: time.Now().Add(1 * time.Hour),
			}
			Expect(observed.HasValidToken()).To(BeTrue())
		})
	})

	Describe("IsTokenExpired", func() {
		It("should return false when expiry is zero", func() {
			observed := snapshot.TransportObservedState{
				JWTToken: "some-token",
			}
			Expect(observed.IsTokenExpired()).To(BeFalse())
		})

		It("should return true when token expires within refresh buffer (10 min)", func() {
			observed := snapshot.TransportObservedState{
				JWTToken:  "valid-token",
				JWTExpiry: time.Now().Add(5 * time.Minute),
			}
			Expect(observed.IsTokenExpired()).To(BeTrue())
		})

		It("should return false when token expires beyond refresh buffer", func() {
			observed := snapshot.TransportObservedState{
				JWTToken:  "valid-token",
				JWTExpiry: time.Now().Add(15 * time.Minute),
			}
			Expect(observed.IsTokenExpired()).To(BeFalse())
		})

		It("should return true when token already expired", func() {
			observed := snapshot.TransportObservedState{
				JWTToken:  "expired-token",
				JWTExpiry: time.Now().Add(-1 * time.Hour),
			}
			Expect(observed.IsTokenExpired()).To(BeTrue())
		})
	})

	Describe("Interface compliance", func() {
		It("should implement fsmv2.ObservedState interface", func() {
			var _ fsmv2.ObservedState = snapshot.TransportObservedState{}
		})
	})
})

var _ = Describe("TransportDesiredState", func() {
	Describe("ShutdownRequested", func() {
		DescribeTable("should correctly report shutdown status",
			func(shutdown bool, want bool) {
				desired := &snapshot.TransportDesiredState{}
				desired.SetShutdownRequested(shutdown)

				Expect(desired.IsShutdownRequested()).To(Equal(want))
			},
			Entry("not requested", false, false),
			Entry("requested", true, true),
		)
	})

	Describe("GetState", func() {
		It("should return the state value", func() {
			desired := &snapshot.TransportDesiredState{}
			desired.State = "running"

			Expect(desired.GetState()).To(Equal("running"))
		})

		It("should return running by default when state is empty", func() {
			desired := &snapshot.TransportDesiredState{}

			Expect(desired.GetState()).To(Equal("running"))
		})
	})

	Describe("Interface compliance", func() {
		It("should implement fsmv2.DesiredState interface", func() {
			var _ fsmv2.DesiredState = &snapshot.TransportDesiredState{}
		})
	})

	Describe("GetChildrenSpecs", func() {
		It("should return nil when no children are configured", func() {
			desired := &snapshot.TransportDesiredState{}
			Expect(desired.GetChildrenSpecs()).To(BeNil())
		})

		It("should return children specs when configured", func() {
			desired := &snapshot.TransportDesiredState{
				ChildrenSpecs: []config.ChildSpec{
					{
						Name:             "push",
						WorkerType:       "push",
						ChildStartStates: []string{"Running", "Degraded"},
					},
				},
			}
			specs := desired.GetChildrenSpecs()
			Expect(specs).To(HaveLen(1))
			Expect(specs[0].Name).To(Equal("push"))
			Expect(specs[0].WorkerType).To(Equal("push"))
			Expect(specs[0].ChildStartStates).To(ConsistOf("Running", "Degraded"))
		})

		It("should implement config.ChildSpecProvider interface", func() {
			var _ config.ChildSpecProvider = &snapshot.TransportDesiredState{}
		})
	})
})

var _ = Describe("TransportSnapshot", func() {
	Describe("Structure", func() {
		It("should have all required fields", func() {
			snap := snapshot.TransportSnapshot{
				Desired:  &snapshot.TransportDesiredState{},
				Observed: snapshot.TransportObservedState{},
			}

			Expect(snap.Desired).NotTo(BeNil())
		})
	})
})
