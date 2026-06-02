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

package types_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

var _ = Describe("AuthSession.IsUsable", func() {
	// IsUsable(buffer) returns false when: token empty, expiry zero, already expired, or
	// expiry is within the buffer window. Returns true only when the token is non-empty and
	// expires strictly beyond now+buffer.
	//
	// Use generous offsets (minutes/hours, not seconds) to avoid CI flakes; we never assert
	// an exact boundary instant — only that clearly-inside and clearly-outside cases behave correctly.

	DescribeTable("child buffer (1m)",
		func(sess types.AuthSession, want bool) {
			Expect(sess.IsUsable(time.Minute)).To(Equal(want))
		},
		Entry("empty token → false", types.AuthSession{}, false),
		Entry("zero expiry with token → false",
			types.AuthSession{Token: "tok"}, false),
		Entry("token expired (1h ago) → false",
			types.AuthSession{Token: "tok", Expiry: time.Now().Add(-time.Hour)}, false),
		Entry("expiry within buffer (30s from now, buffer=1m) → false",
			types.AuthSession{Token: "tok", Expiry: time.Now().Add(30 * time.Second)}, false),
		Entry("expiry well beyond buffer (2h from now, buffer=1m) → true",
			types.AuthSession{Token: "tok", Expiry: time.Now().Add(2 * time.Hour)}, true),
	)

	DescribeTable("parent buffer (10m)",
		func(sess types.AuthSession, want bool) {
			Expect(sess.IsUsable(10 * time.Minute)).To(Equal(want))
		},
		Entry("empty token → false", types.AuthSession{}, false),
		Entry("zero expiry with token → false",
			types.AuthSession{Token: "tok"}, false),
		Entry("token expired (1h ago) → false",
			types.AuthSession{Token: "tok", Expiry: time.Now().Add(-time.Hour)}, false),
		Entry("expiry within buffer (5m from now, buffer=10m) → false",
			types.AuthSession{Token: "tok", Expiry: time.Now().Add(5 * time.Minute)}, false),
		Entry("expiry well beyond buffer (2h from now, buffer=10m) → true",
			types.AuthSession{Token: "tok", Expiry: time.Now().Add(2 * time.Hour)}, true),
	)
})

func TestChildAuthUserSpecRoundTrip(t *testing.T) {
	exp := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	in := types.ChildAuthUserSpec{AuthSession: types.AuthSession{Token: "jwt-abc", Expiry: exp, InstanceUUID: "be-uuid"}}

	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out types.ChildAuthUserSpec
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.AuthSession.Token != "jwt-abc" || out.AuthSession.InstanceUUID != "be-uuid" || !out.AuthSession.Expiry.Equal(exp) {
		t.Fatalf("round-trip mismatch: got %+v", out.AuthSession)
	}
}
