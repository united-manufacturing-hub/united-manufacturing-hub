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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// marshalOutboundQueue marshals a TransportStatus and returns the
// outbound_queue object as a decoded map.
func marshalOutboundQueue(s snapshot.TransportStatus) map[string]any {
	bytes, err := json.Marshal(s)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	var asMap map[string]any
	err = json.Unmarshal(bytes, &asMap)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, asMap).To(HaveKey("outbound_queue"))

	verdict, ok := asMap["outbound_queue"].(map[string]any)
	ExpectWithOffset(1, ok).To(BeTrue())
	return verdict
}

var _ = Describe("TransportStatus outbound queue JSON", func() {
	It("checks that the outbound queue verdict keeps its stored field names through a JSON round trip", func() {
		var s snapshot.TransportStatus
		s.OutboundQueue.Measured = true
		s.OutboundQueue.FillPercent = 40
		s.OutboundQueue.PeakPercent = 70

		verdict := marshalOutboundQueue(s)
		Expect(verdict).To(HaveLen(6))
		keys := make([]string, 0, len(verdict))
		for key := range verdict {
			keys = append(keys, key)
		}
		Expect(keys).To(ConsistOf("measured", "degraded", "fill_degraded", "peak_degraded", "fill_percent", "peak_percent"))
		Expect(verdict["measured"]).To(Equal(true))
		Expect(verdict["degraded"]).To(Equal(false))
		Expect(verdict["fill_degraded"]).To(Equal(false))
		Expect(verdict["peak_degraded"]).To(Equal(false))
		Expect(verdict["fill_percent"]).To(Equal(40.0))
		Expect(verdict["peak_percent"]).To(Equal(70.0))

		bytes, err := json.Marshal(s)
		Expect(err).NotTo(HaveOccurred())
		var roundTripped snapshot.TransportStatus
		err = json.Unmarshal(bytes, &roundTripped)
		Expect(err).NotTo(HaveOccurred())
		Expect(roundTripped.OutboundQueue).To(Equal(s.OutboundQueue))
	})

	It("checks that each degraded flag serialises under its own key, degraded and peak degrading together", func() {
		var s snapshot.TransportStatus
		s.OutboundQueue.Measured = true
		s.OutboundQueue.Degraded = true
		s.OutboundQueue.FillDegraded = false
		s.OutboundQueue.PeakDegraded = true
		s.OutboundQueue.FillPercent = 40
		s.OutboundQueue.PeakPercent = 70

		verdict := marshalOutboundQueue(s)
		Expect(verdict).To(HaveLen(6))
		Expect(verdict["measured"]).To(Equal(true))
		Expect(verdict["degraded"]).To(Equal(true))
		Expect(verdict["fill_degraded"]).To(Equal(false))
		Expect(verdict["peak_degraded"]).To(Equal(true))
		Expect(verdict["fill_percent"]).To(Equal(40.0))
		Expect(verdict["peak_percent"]).To(Equal(70.0))
	})

	It("checks that each degraded flag serialises under its own key, fill degrading alone", func() {
		var s snapshot.TransportStatus
		s.OutboundQueue.Measured = true
		s.OutboundQueue.Degraded = true
		s.OutboundQueue.FillDegraded = true
		s.OutboundQueue.PeakDegraded = false
		s.OutboundQueue.FillPercent = 40
		s.OutboundQueue.PeakPercent = 70

		verdict := marshalOutboundQueue(s)
		Expect(verdict).To(HaveLen(6))
		Expect(verdict["measured"]).To(Equal(true))
		Expect(verdict["degraded"]).To(Equal(true))
		Expect(verdict["fill_degraded"]).To(Equal(true))
		Expect(verdict["peak_degraded"]).To(Equal(false))
		Expect(verdict["fill_percent"]).To(Equal(40.0))
		Expect(verdict["peak_percent"]).To(Equal(70.0))
	})
})
