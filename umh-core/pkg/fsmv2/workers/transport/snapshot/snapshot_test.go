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
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

var _ = Describe("TransportStatus", func() {
	Describe("HasValidToken", func() {
		It("should return false when JWT token is empty", func() {
			status := snapshot.TransportStatus{
				AuthSession: types.AuthSession{
					Token:  "",
					Expiry: time.Now().Add(1 * time.Hour),
				},
			}
			Expect(status.HasValidToken()).To(BeFalse())
		})

		It("should return false when token is present but expired", func() {
			status := snapshot.TransportStatus{
				AuthSession: types.AuthSession{
					Token:  "valid-token",
					Expiry: time.Now().Add(-1 * time.Hour),
				},
			}
			Expect(status.HasValidToken()).To(BeFalse())
		})

		It("should return true when token is present and not expired", func() {
			status := snapshot.TransportStatus{
				AuthSession: types.AuthSession{
					Token:  "valid-token",
					Expiry: time.Now().Add(1 * time.Hour),
				},
			}
			Expect(status.HasValidToken()).To(BeTrue())
		})

		It("should return false when AuthSession is zero-value", func() {
			status := snapshot.TransportStatus{}
			Expect(status.HasValidToken()).To(BeFalse())
		})

		It("should return true for token+zero-expiry (IsTokenExpired=false for pre-auth window)", func() {
			// Zero Expiry with a non-empty token: IsTokenExpired returns false,
			// so HasValidToken returns true. Children's IsUsable(1m) returns false
			// for the same session, so they stay idle until the parent populates Expiry.
			status := snapshot.TransportStatus{
				AuthSession: types.AuthSession{Token: "tok"},
			}
			Expect(status.HasValidToken()).To(BeTrue())
			// Contrast: the child-side IsUsable gate correctly rejects it.
			Expect(status.AuthSession.IsUsable(time.Minute)).To(BeFalse())
		})
	})

	Describe("IsTokenExpired", func() {
		It("should return false when expiry is zero (deliberate: parent must not re-auth before first token)", func() {
			// Zero Expiry with a non-empty token → NOT expired.
			// Contrast: AuthSession.IsUsable(1m) treats the same value as unusable.
			// See the IsTokenExpired doc comment for the rationale.
			status := snapshot.TransportStatus{
				AuthSession: types.AuthSession{Token: "some-token"},
			}
			Expect(status.IsTokenExpired()).To(BeFalse())
		})

		It("should return true when token expires within refresh buffer (10 min)", func() {
			status := snapshot.TransportStatus{
				AuthSession: types.AuthSession{
					Token:  "valid-token",
					Expiry: time.Now().Add(5 * time.Minute),
				},
			}
			Expect(status.IsTokenExpired()).To(BeTrue())
		})

		It("should return false when token expires beyond refresh buffer", func() {
			status := snapshot.TransportStatus{
				AuthSession: types.AuthSession{
					Token:  "valid-token",
					Expiry: time.Now().Add(15 * time.Minute),
				},
			}
			Expect(status.IsTokenExpired()).To(BeFalse())
		})

		It("should return true when token already expired", func() {
			status := snapshot.TransportStatus{
				AuthSession: types.AuthSession{
					Token:  "expired-token",
					Expiry: time.Now().Add(-1 * time.Hour),
				},
			}
			Expect(status.IsTokenExpired()).To(BeTrue())
		})
	})

	Describe("FailedAuthConfig", func() {
		It("should return true from IsEmpty when no failed auth config is recorded", func() {
			fac := snapshot.FailedAuthConfig{}
			Expect(fac.IsEmpty()).To(BeTrue())
		})

		It("should return false from IsEmpty when a failed auth config is recorded", func() {
			fac := snapshot.FailedAuthConfig{
				AuthToken:    "token",
				RelayURL:     "https://relay.example.com",
				InstanceUUID: "uuid",
			}
			Expect(fac.IsEmpty()).To(BeFalse())
		})
	})
})

var _ = Describe("RenderChildren", func() {
	It("stamps AuthSession onto both push and pull children", func() {
		expiry := time.Now().Add(time.Hour)
		status := snapshot.TransportStatus{
			AuthSession: types.AuthSession{
				Token:        "jwt",
				InstanceUUID: "be-uuid",
				Expiry:       expiry,
			},
		}
		specs, err := snapshot.RenderChildren(snapshot.TransportDesiredState{}, status, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(HaveLen(2))
		for _, sp := range specs {
			var carrier types.ChildAuthUserSpec
			Expect(yaml.Unmarshal([]byte(sp.UserSpec.Config), &carrier)).To(Succeed())
			Expect(carrier.AuthSession.Token).To(Equal("jwt"))
			Expect(carrier.AuthSession.InstanceUUID).To(Equal("be-uuid"))
			Expect(carrier.AuthSession.Expiry).To(BeTemporally("==", expiry))
		}
	})

	It("propagates a refreshed token when parent AuthSession changes (BS1 refresh propagation)", func() {
		// Verifies that RenderChildren stamps the NEW token when the parent's
		// TransportStatus.AuthSession changes. The supervisor re-applies the
		// changed UserSpec to resident children every reconcile tick, so a
		// token refresh reaches long-lived push/pull children without respawn.

		status1 := snapshot.TransportStatus{
			AuthSession: types.AuthSession{
				Token:        "old-jwt",
				InstanceUUID: "be-uuid",
				Expiry:       time.Now().Add(time.Hour),
			},
		}
		status2 := snapshot.TransportStatus{
			AuthSession: types.AuthSession{
				Token:        "new-jwt",
				InstanceUUID: "be-uuid",
				Expiry:       time.Now().Add(2 * time.Hour),
			},
		}

		specs1, err := snapshot.RenderChildren(snapshot.TransportDesiredState{}, status1, true)
		Expect(err).ToNot(HaveOccurred())

		specs2, err := snapshot.RenderChildren(snapshot.TransportDesiredState{}, status2, true)
		Expect(err).ToNot(HaveOccurred())

		Expect(specs1).To(HaveLen(2))
		Expect(specs2).To(HaveLen(2))

		for i := range 2 {
			var c1, c2 types.ChildAuthUserSpec
			Expect(yaml.Unmarshal([]byte(specs1[i].UserSpec.Config), &c1)).To(Succeed())
			Expect(yaml.Unmarshal([]byte(specs2[i].UserSpec.Config), &c2)).To(Succeed())

			Expect(c1.AuthSession.Token).To(Equal("old-jwt"),
				"first render must carry the old token")
			Expect(c2.AuthSession.Token).To(Equal("new-jwt"),
				"second render must carry the refreshed token")
			Expect(c2.AuthSession.Token).NotTo(Equal(c1.AuthSession.Token),
				"tokens must differ: refresh must propagate")
		}
	})
})

var _ = Describe("TransportDesiredState", func() {
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
						Name:       "push",
						WorkerType: "push",
						Enabled:    true,
					},
				},
			}
			specs := desired.GetChildrenSpecs()
			Expect(specs).To(HaveLen(1))
			Expect(specs[0].Name).To(Equal("push"))
			Expect(specs[0].WorkerType).To(Equal("push"))
			Expect(specs[0].Enabled).To(BeTrue())
		})

		It("should implement config.ChildSpecProvider interface", func() {
			var _ config.ChildSpecProvider = &snapshot.TransportDesiredState{}
		})
	})
})
