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

// TestIsActiveEquivalenceSteadyState pins that the worker's tick-free IsActive
// (input MessagesPerSecond > 0) means the same thing as FSMv1's (input
// MessagesPerTick > 0) on the SAME count sequence — for steady-state arrivals
// only. The reset tick is deliberately excluded: there the worker publishes
// 0/false while FSMv1 publishes the cumulative counter as a rate (D5a). Activity
// is asserted inside the window (30s cadence), never at the pathological
// window-length boundary where FSMv1 flakes too.
//
// Equivalence holds because MessagesPerTick = MessagesPerSecond x tickSeconds
// with tickSeconds > 0, so the two >0 predicates are identical. What this guards
// is the DERIVATION: a count-comparison or per-poll-delta rule, or a hysteresis
// hold in the worker, would diverge from FSMv1's metrics_state.go:95.
// The cadence is 100ms because the two windows are only comparable at that rate.
// FSMv1's window is COUNT-based — metrics_state.go:122 trims to the last
// ThroughputWindowSize=600 ENTRIES — and its comment at :60 states the 600 assumes
// 100ms per tick, i.e. one minute. The worker's window is TIME-based over 60s
// (throughput_window.go:138). At 100ms both span ~60s. At the 30s cadence an
// earlier version of this test used, 600 entries span five hours, so FSMv1 could
// not reach idle at all: both derivations were true at every asserted step and the
// equality assertion held against a hardcoded `true`. The crossing below is what
// makes the assertion bite.
const (
	isActiveTickInterval = 100 * time.Millisecond
	// Flat samples needed before BOTH windows contain no counter movement: 600
	// entries evicts FSMv1's window, and 600 x 100ms = 60s clears the worker's.
	// The margin covers the boundary sample at exactly 60s.
	isActiveFlatSamples = benthos_monitor.ThroughputWindowSize + 10
)

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

	// Phase 0 — prime with one sample, asserting nothing. The first tick is
	// FSMv1's reset/cold-start branch (metrics_state.go:110-117 publishes the
	// cumulative count as a rate, so IsActive is true) while the worker, holding a
	// single sample, reads 0/false. That divergence is documented as D5a and is
	// deliberately excluded.
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
	// but different eviction rules, so they legitimately disagree for about one
	// sample at the boundary. Measured here: at the tick where FSMv1's 600-entry
	// window has just evicted the last rising sample, the worker's 60s time window
	// still contains it, so worker reads active while FSMv1 reads idle. That is the
	// "pathological window-length boundary" the steady-state comment above avoids,
	// and it is a property of the two window shapes, not a defect in either. What
	// must hold is the settled state at the end.
	var lastWorker, lastFSM bool

	for i := 0; i < isActiveFlatSamples; i++ {
		lastWorker, lastFSM = step(in) // in unchanged: no new messages
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
