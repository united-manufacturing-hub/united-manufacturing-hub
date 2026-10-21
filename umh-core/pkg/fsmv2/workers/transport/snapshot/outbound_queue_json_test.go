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

var _ = Describe("TransportStatus outbound queue JSON", func() {
	It("checks that the outbound queue verdict keeps its stored field names through a JSON round trip", func() {
		var s snapshot.TransportStatus
		s.OutboundQueue.Measured = true
		s.OutboundQueue.FillPercent = 40
		s.OutboundQueue.PeakPercent = 70

		bytes, err := json.Marshal(s)
		Expect(err).NotTo(HaveOccurred())

		var asMap map[string]any
		err = json.Unmarshal(bytes, &asMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(asMap).To(HaveKey("outbound_queue"))

		verdict, ok := asMap["outbound_queue"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(verdict).To(HaveLen(6))
		keys := make([]string, 0, len(verdict))
		for key := range verdict {
			keys = append(keys, key)
		}
		Expect(keys).To(ConsistOf("measured", "degraded", "fill_degraded", "peak_degraded", "fill_percent", "peak_percent"))
		Expect(verdict["measured"]).To(Equal(true))
		Expect(verdict["fill_percent"]).To(Equal(40.0))
		Expect(verdict["peak_percent"]).To(Equal(70.0))

		var roundTripped snapshot.TransportStatus
		err = json.Unmarshal(bytes, &roundTripped)
		Expect(err).NotTo(HaveOccurred())
		Expect(roundTripped.OutboundQueue).To(Equal(s.OutboundQueue))
	})
})
