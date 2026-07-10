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

package simple_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

type wrapProbe struct {
	PortState string `json:"port_state"`
	ScanError string `json:"scan_error"`
}

var _ = Describe("Status", func() {
	Describe("HealthVerdict", func() {
		It("reports the carried verdict", func() {
			s := simple.Status[wrapProbe]{Degraded: true, Reason: "port 502 unreachable"}

			degraded, reason := s.HealthVerdict()
			Expect(degraded).To(BeTrue())
			Expect(reason).To(Equal("port 502 unreachable"))
		})
	})

	Describe("MarshalJSON", func() {
		It("flattens the developer status fields to the top level alongside the verdict", func() {
			s := simple.Status[wrapProbe]{
				Result:   wrapProbe{PortState: "open", ScanError: ""},
				Degraded: false,
				Reason:   "running (no health check)",
			}

			raw, err := json.Marshal(s)
			Expect(err).NotTo(HaveOccurred())

			var m map[string]any
			Expect(json.Unmarshal(raw, &m)).To(Succeed())
			Expect(m).To(HaveKeyWithValue("port_state", "open"))
			Expect(m).To(HaveKeyWithValue("scan_error", ""))
			Expect(m).To(HaveKeyWithValue("degraded", false))
			Expect(m).To(HaveKeyWithValue("reason", "running (no health check)"))
			Expect(m).NotTo(HaveKey("result"), "developer fields are hoisted, not nested under result")
			Expect(m).NotTo(HaveKey("Result"))
		})

		It("round-trips through Unmarshal", func() {
			s := simple.Status[wrapProbe]{
				Result:   wrapProbe{PortState: "closed", ScanError: "timeout"},
				Degraded: true,
				Reason:   "poll error: dial timeout",
			}

			raw, err := json.Marshal(s)
			Expect(err).NotTo(HaveOccurred())

			var back simple.Status[wrapProbe]
			Expect(json.Unmarshal(raw, &back)).To(Succeed())
			Expect(back).To(Equal(s))
		})

		It("rejects a developer field that collides with a verdict field", func() {
			type collides struct {
				Degraded string `json:"degraded"`
			}

			_, err := json.Marshal(simple.Status[collides]{Result: collides{Degraded: "oops"}})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("degraded"))
		})
	})
})
