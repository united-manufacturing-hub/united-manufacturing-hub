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

package fsmv2benthos

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/benthosmetrics"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

const (
	// capstoneTickInterval is 100ms because that is the only cadence at which
	// the two windows span the same interval. FSMv1's window is COUNT-based:
	// updateComponentThroughput keeps only the last ThroughputWindowSize=600
	// ENTRIES (pkg/service/benthos_monitor/metrics_state.go), and those 600
	// assume 100ms per tick, i.e. one minute. The worker's window is TIME-based
	// over 60 s (pkg/benthosmetrics).
	capstoneTickInterval = 100 * time.Millisecond

	// Flat samples needed before BOTH windows contain no counter movement: 600
	// entries evict FSMv1's window, and 600 x 100ms = 60s clears the worker's.
	// The +10 margin covers the boundary sample at exactly 60s.
	capstoneFlatSamples = benthosmonitorservice.ThroughputWindowSize + 10
)

var _ = Describe("IsActive equivalence", func() {
	// The IsActive the worker reports matches what FSMv1's monitor would have
	// concluded for the same scrapes, across the transition into and out of an
	// idle period.
	//
	// The worker's predicate is the library's: IsActive =
	// Input.MessagesPerSecond > 0 (pkg/benthosmetrics). FSMv1's is
	// IsActive = Input.MessagesPerTick > 0
	// (pkg/service/benthos_monitor/metrics_state.go). The two agree because
	// MessagesPerTick = MessagesPerSecond x the tick length, which is
	// constants.DefaultTickerTime and never zero.
	//
	// Equivalence holds for steady-state arrivals only: the cold-start tick
	// (Phase 0) and the window-length boundary (Phase 2) are excluded, each
	// stated where it occurs.
	It("holds across the idle crossing", func() {
		s := newFakeBenthos()
		defer s.close()

		cfg := testConfig()
		cfg.MetricsPort = s.port()

		// A fake clock steps the scrape timestamps, so the run needs no sleeps
		// and its window arithmetic is exact.
		t0 := time.Now().Truncate(time.Second)
		next := t0
		tracker := benthosmetrics.NewTracker(func() time.Time { return next })
		deps := &benthosDeps{tracker: tracker}

		fsmState := benthosmonitorservice.NewBenthosMetricsState()

		tick := 0
		in := int64(0)

		// step advances both derivations by one scrape and returns their
		// verdicts.
		step := func(inputCount int64) (workerActive, fsmActive bool) {
			next = t0.Add(time.Duration(tick) * capstoneTickInterval)

			sample, err := Poll(context.Background(), deps, cfg)
			Expect(err).NotTo(HaveOccurred(), "tick %d", tick)
			workerActive = sample.Sample.IsActive

			fsmState.UpdateFromMetrics(benthosmonitorservice.Metrics{
				Inputs: map[string]benthosmonitorservice.InputInstance{
					"root.input": {Received: inputCount},
				},
			}, uint64(tick))
			fsmActive = fsmState.IsActive
			tick++

			return workerActive, fsmActive
		}

		// Phase 0 — prime with one scrape, asserting nothing. The first tick
		// takes FSMv1's reset branch, which publishes the cumulative count as
		// MessagesPerTick, so it reads active; the worker holds one sample, and
		// one sample has no rate, so it reads inactive.
		in++
		s.setCounters(in, in/2)
		step(in)

		// Phase 1 — sustained arrivals. Both must read active, and must agree.
		for i := 0; i < 30; i++ {
			in++
			s.setCounters(in, in/2)

			workerActive, fsmActive := step(in)
			Expect(workerActive).To(Equal(fsmActive),
				"active phase tick %d (in=%d): the two IsActive verdicts diverged", tick-1, in)
			Expect(workerActive).To(BeTrue(),
				"active phase tick %d (in=%d): expected both to read active", tick-1, in)
		}

		// Phase 2 — the counter stops. The two windows can disagree near the
		// 60s edge, because FSMv1 drops samples by count and the worker drops
		// them by age. So this phase checks only that both read idle at the
		// end.
		var lastWorker, lastFSM bool

		for i := 0; i < capstoneFlatSamples; i++ {
			lastWorker, lastFSM = step(in)
		}

		Expect(lastWorker).To(BeFalse(),
			"after %d flat samples the worker must read idle", capstoneFlatSamples)
		Expect(lastFSM).To(BeFalse(),
			"after %d flat samples FSMv1 must read idle", capstoneFlatSamples)

		// Phase 3 — one new message. Both must cross back to active on the same
		// tick. Together with the idle assertion above, this is what a
		// stuck-true or stuck-false derivation cannot satisfy.
		in++
		s.setCounters(in, in/2)

		workerActive, fsmActive := step(in)
		Expect(workerActive).To(Equal(fsmActive),
			"crossing tick %d (in=%d): the two IsActive verdicts diverged", tick-1, in)
		Expect(workerActive).To(BeTrue(),
			"crossing tick %d (in=%d): expected both to read active after a new message", tick-1, in)
	})
})
