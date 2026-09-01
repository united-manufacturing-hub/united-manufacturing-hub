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

package topicbrowser_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
)

// newestProducedAtMs returns the highest ProducedAtMs held in the communicator's
// event map, which advances only when a bundle newer than everything already
// ingested is processed.
func newestProducedAtMs(comm *topicbrowser.TopicBrowserCommunicator) uint64 {
	var newest uint64

	for _, entry := range comm.GetEventMap() {
		if produced := entry.GetProducedAtMs(); produced > newest {
			newest = produced
		}
	}

	return newest
}

var _ = Describe("Simulator buffer ordering", func() {
	It("builds the simulated observed state oldest-to-newest, matching the RingBufferSnapshot.Items contract", func() {
		simulator := topicbrowser.NewSimulator()
		simulator.InitializeSimulator()

		// Tick twice so there are at least two distinct bundles, then read the
		// state the simulator hands to ProcessSimulatedData. The newest bundle
		// must be last (the same contract the production ring and
		// processIncrementalBuffers rely on); a newest-first arrangement would
		// freeze demo mode reporting seq 1.
		simulator.Tick()
		simulator.Tick()
		state := simulator.GetSimObservedState()

		items := state.ServiceInfo.Status.BufferSnapshot.Items
		Expect(items).NotTo(BeEmpty(), "simulated state must produce buffer items")

		Expect(items[len(items)-1].SequenceNum).To(Equal(state.ServiceInfo.Status.BufferSnapshot.LastSequenceNum),
			"the newest item is last, matching LastSequenceNum")
	})

	It("ingests the newest simulated bundle end to end, not one already seen", func() {
		comm := topicbrowser.NewTopicBrowserCommunicatorWithSimulator(zap.NewNop().Sugar())

		// The first call bootstraps: lastProcessedSequence is 0, so every buffer
		// is ingested and the incremental path is not exercised yet.
		_, err := comm.ProcessSimulatedData()
		Expect(err).NotTo(HaveOccurred())

		before := newestProducedAtMs(comm)
		Expect(before).ToNot(BeZero(), "bootstrap must ingest at least one event")

		// Every simulated bundle carries the SAME two hardcoded topics, so the
		// topic map cannot tell two bundles apart and asserting on it would pass
		// vacuously. The event payload is the only signal for which buffer was
		// ingested.
		//
		// updateInternalCache replaces an event only when ProducedAtMs is
		// STRICTLY greater, and ProducedAtMs has millisecond resolution, so two
		// bundles generated inside one millisecond are indistinguishable even
		// when selection is correct. The gap is required by that production
		// semantic, not by the test.
		time.Sleep(2 * time.Millisecond)

		// The second call takes the incremental path with exactly one new buffer.
		_, err = comm.ProcessSimulatedData()
		Expect(err).NotTo(HaveOccurred())

		// Assert the COMPOSITION of producer order and consumer selection: the
		// newest event must advance. Selecting an already-ingested buffer leaves
		// this unchanged, because its ProducedAtMs is not strictly greater.
		Expect(newestProducedAtMs(comm)).To(BeNumerically(">", before),
			"the incremental pass must ingest the newest simulated bundle, not one already seen")
	})
})
