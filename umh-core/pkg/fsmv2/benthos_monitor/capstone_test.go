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
func TestIsActiveEquivalenceSteadyState(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Poll events: (elapsed seconds, cumulative input messages). A 30s cadence
	// keeps every sample inside the 60s window; the zero-delta step exercises an
	// idle transition.
	events := []struct {
		sec int
		in  int
	}{
		{0, 1},
		{30, 2},
		{60, 3},
		{90, 3}, // idle: no new input, still within window
		{120, 4},
	}

	var w throughputWindow
	fsmState := benthos_monitor.NewBenthosMetricsState()

	// Prime both derivations with the first sample WITHOUT asserting: the first
	// tick is FSMv1's reset/cold-start branch (metrics_state.go:110-117 publishes
	// the cumulative count as a rate => IsActive true) while the worker, holding a
	// single sample, reads 0/false. That single-sample tick is the documented
	// divergence (D5a) and is deliberately excluded. Compare steady-state only,
	// from the second sample onward.
	prime := events[0]
	w.Add(t0.Add(time.Duration(prime.sec)*time.Second), testPort, prime.in, 0)
	fsmState.UpdateFromMetrics(benthos_monitor.Metrics{Inputs: map[string]benthos_monitor.InputInstance{
		"root.input": {Received: int64(prime.in)},
	}}, uint64(prime.sec)/30)

	// Compare steady-state events from the second sample onward, asserting the
	// two activations stay identical at every step. Iterate by index so the
	// derivation arrays are exactly one element smaller than events and the
	// worker/FSMv1 values at each index belong to the same event.
	for i := 1; i < len(events); i++ {
		e := events[i]
		w.Add(t0.Add(time.Duration(e.sec)*time.Second), testPort, e.in, 0)
		workerActive := w.inputRate() > 0

		m := benthos_monitor.Metrics{Inputs: map[string]benthos_monitor.InputInstance{
			"root.input": {Received: int64(e.in)},
		}}
		fsmState.UpdateFromMetrics(m, uint64(e.sec)/30)

		if workerActive != fsmState.IsActive {
			t.Errorf("event %d (in=%d): worker IsActive=%v, FSMv1 IsActive=%v",
				i, e.in, workerActive, fsmState.IsActive)
		}
		if i == 1 {
			// Sanity: the first steady-state step must read active in both, else
			// the test asserts nothing but constant-false.
			if !fsmState.IsActive {
				t.Errorf("expected FSMv1 to read active at the first steady-state event")
			}
		}
	}
}
