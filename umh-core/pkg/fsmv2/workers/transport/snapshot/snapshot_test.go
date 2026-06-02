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
	})

	Describe("IsTokenExpired", func() {
		It("should return false when expiry is zero", func() {
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
		status := snapshot.TransportStatus{
			AuthSession: types.AuthSession{
				Token:        "jwt",
				InstanceUUID: "be-uuid",
				Expiry:       time.Now().Add(time.Hour),
			},
		}
		specs, err := snapshot.RenderChildren(snapshot.TransportDesiredState{}, status, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(specs).To(HaveLen(2))
		for _, sp := range specs {
			var carrier types.ChildAuthUserSpec
			Expect(yaml.Unmarshal([]byte(sp.UserSpec.Config), &carrier)).To(Succeed())
			Expect(carrier.AuthSession.Token).To(Equal("jwt"))
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
