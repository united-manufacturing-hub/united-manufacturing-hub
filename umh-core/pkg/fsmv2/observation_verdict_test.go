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

package fsmv2_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// The health verdict (Degraded + Reason) lets a worker report "the target is
// down / I could not determine it, and here is why" on the same Observation as
// its business status, so both the fsmv2 state reason and the fsmv1 adapter can
// read one authoritative verdict (ENG-5305).
var _ = Describe("Observation health verdict", func() {
	type VerdictStatus struct {
		Port int `json:"port"`
	}

	It("round-trips Degraded and Reason through flattened JSON", func() {
		obs := fsmv2.Observation[VerdictStatus]{
			Status:   VerdictStatus{Port: 502},
			Degraded: true,
			Reason:   "poll error: dial timeout",
		}

		data, err := json.Marshal(obs)
		Expect(err).NotTo(HaveOccurred())

		var got fsmv2.Observation[VerdictStatus]
		Expect(json.Unmarshal(data, &got)).To(Succeed())
		Expect(got.Degraded).To(BeTrue())
		Expect(got.Reason).To(Equal("poll error: dial timeout"))
		Expect(got.Status.Port).To(Equal(502), "TStatus must survive alongside the verdict")
	})

	It("reserves the verdict JSON names against TStatus collisions", func() {
		type CollidingStatus struct {
			Reason string `json:"health_reason"`
		}

		Expect(fsmv2.DetectFieldCollisions[CollidingStatus]()).To(HaveOccurred(),
			"a TStatus using the reserved health_reason tag must be rejected")
	})
})
