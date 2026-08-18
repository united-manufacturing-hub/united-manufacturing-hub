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

package fsmv2benthosmonitor

import (
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

const (
	// isActiveTickInterval is 100ms because that is the only cadence at which the
	// two windows span the same interval. FSMv1's window is COUNT-based:
	// updateComponentThroughput keeps only the last ThroughputWindowSize=600
	// ENTRIES (pkg/service/benthos_monitor/metrics_state.go), and
	// ThroughputWindowSize itself (same file) records that the 600 assumes
	// 100ms per tick, i.e. one minute. The worker's window is TIME-based, over
	// windowSpan = 60s (throughput_window.go).
	isActiveTickInterval = 100 * time.Millisecond
	// Flat samples needed before BOTH windows contain no counter movement:
	// ThroughputWindowSize=600 entries evicts FSMv1's window, and 600 x 100ms = 60s
	// clears the worker's. The +10 margin covers the boundary sample at exactly 60s.
	isActiveFlatSamples = benthos_monitor.ThroughputWindowSize + 10
)

// TestIsActiveEquivalenceAcrossIdleCrossing asserts that the worker's rate-based
// IsActive means the same thing as FSMv1's per-tick one on the SAME count
// sequence, driven over one crossing from active to idle and back. The worker's
// predicate is the one Poll applies — IsActive = Input.MessagesPerSecond > 0, at
// manager.go. FSMv1's is the one UpdateFromMetrics applies — IsActive =
// Input.MessagesPerTick > 0, in pkg/service/benthos_monitor/metrics_state.go.
// This test reads the worker side straight off inputRate
// (throughput_window.go), the value Poll assigns to Input.MessagesPerSecond,
// so it needs no HTTP scrape.
//
// Equivalence holds for steady-state arrivals only: two kinds of divergence are
// excluded from the equality assertion — the reset tick and the window-length
// boundary — each stated at the phase where it occurs, Phase 0 and Phase 2 below.
//
// The two agree because MessagesPerTick = MessagesPerSecond x tickSeconds with
// tickSeconds > 0, so the two >0 predicates are identical. What this guards is the
// DERIVATION: comparing raw counts, differencing consecutive polls, or holding
// active for N ticks after the last message would each diverge from
// UpdateFromMetrics.
func TestIsActiveEquivalenceAcrossIdleCrossing(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	var w throughputWindow
	fsmState := benthos_monitor.NewBenthosMetricsState()

	tick := 0
	in := 0

	// step advances both derivations by one poll and returns their verdicts.
	step := func(inputCount int) (workerActive, fsmActive bool) {
		at := t0.Add(time.Duration(tick) * isActiveTickInterval)
		w.Add(at, testPort, inputCount, 0)
		fsmState.UpdateFromMetrics(benthos_monitor.Metrics{Inputs: map[string]benthos_monitor.InputInstance{
			"root.input": {Received: int64(inputCount)},
		}}, uint64(tick))
		tick++

		return w.inputRate() > 0, fsmState.IsActive
	}

	// Phase 0 — prime with one sample, asserting nothing. The first tick takes
	// FSMv1's reset/cold-start branch in updateComponentThroughput
	// (pkg/service/benthos_monitor/metrics_state.go), which publishes the
	// cumulative count as MessagesPerTick, so IsActive is true. The worker holds one
	// sample, and inputRate returns 0 below two samples (throughput_window.go),
	// so it reads false. TestPollComputesThroughput (throughput_red_test.go)
	// asserts the worker's side of it, so it is behaviour, not an accident here.
	in++
	step(in)

	// Phase 1 — sustained arrivals. Both must read active, and must agree.
	for i := 0; i < 30; i++ {
		in++

		workerActive, fsmActive := step(in)
		if workerActive != fsmActive {
			t.Fatalf("active phase tick %d (in=%d): worker=%v FSMv1=%v", tick-1, in, workerActive, fsmActive)
		}

		if !workerActive {
			t.Fatalf("active phase tick %d (in=%d): expected both to read active", tick-1, in)
		}
	}

	// Phase 2 — the counter stops moving. Both windows must drain to idle. Equality
	// is NOT asserted during the drain: the two windows have the same nominal span
	// but different eviction rules, so they disagree for about one sample at the
	// boundary. At that boundary, the tick where FSMv1's 600-entry window has just
	// evicted the last rising sample, the worker's 60s time window still contains
	// it, so worker reads active while FSMv1 reads idle. FSMv1 evicts by ENTRY
	// COUNT, so which sample falls out at the boundary follows the number of polls
	// rather than elapsed time, and its idle edge therefore moves with the poll
	// cadence while the worker's stays at 60s. This test does not adjudicate which
	// of the two readings is right at that tick; it asserts only the settled state
	// at the end.
	var lastWorker, lastFSM bool

	for i := 0; i < isActiveFlatSamples; i++ {
		lastWorker, lastFSM = step(in)
	}

	if lastWorker || lastFSM {
		t.Fatalf("after %d flat samples both derivations must read idle, got worker=%v FSMv1=%v",
			isActiveFlatSamples, lastWorker, lastFSM)
	}

	// Phase 3 — one new message. Both must cross back to active on the same tick.
	// Together with the idle assertion above, this is what a stuck-true or
	// stuck-false derivation cannot satisfy.
	in++

	workerActive, fsmActive := step(in)
	if workerActive != fsmActive {
		t.Fatalf("crossing tick %d (in=%d): worker=%v FSMv1=%v", tick-1, in, workerActive, fsmActive)
	}

	if !workerActive {
		t.Fatalf("crossing tick %d (in=%d): expected both to read active after a new message", tick-1, in)
	}
}
