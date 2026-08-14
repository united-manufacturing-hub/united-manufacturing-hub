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

// Do not flicker. The two-mark latch is what stops a signal that sits
// between its fire and clear marks from alternating endpoint states tick to
// tick: a fired latch between the marks HOLDS, and a signal that enters the band
// from below does not fire at all because the fire mark is strict. The
// requirement has a measured production harm behind it — 188,915 events — which
// this pair asserts the two-mark latch actually delivers.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// stateSequence drives Decide for n ticks with the given throttle counters and
// returns the health state at every tick, so a spec can count transitions.
func stateSequence(n int, throttled func(i int) float64) []State {
	engine, err := NewEngine(4, 2.0)
	Expect(err).NotTo(HaveOccurred())
	env := diagnosis.NewEnvironment(HasLimit)
	base := time.Now()

	states := make([]State, n)
	for i := 0; i < n; i++ {
		smp := Sample{
			Timestamp:   base.Add(time.Duration(i) * time.Second),
			CpuScope:    ScopeHost,
			NrPeriods:   diagnosis.Known(100 * float64(i)),
			NrThrottled: diagnosis.Known(throttled(i)),
			Pressure:    diagnosis.Known(0),
			Steal:       diagnosis.Known(0),
			UsageCores:  diagnosis.Known(0),
			HostBusy:    diagnosis.Known(0.5),
		}
		verdict, _ := Decide(engine, smp, env)
		states[i] = verdict.State
	}
	return states
}

// transitions counts how many times the state changed between consecutive ticks.
func transitions(states []State) int {
	count := 0
	for i := 1; i < len(states); i++ {
		if states[i] != states[i-1] {
			count++
		}
	}
	return count
}

var _ = Describe("do not flicker", func() {
	It("should not alternate between healthy and degraded while the underlying reading sits between the marks", func() {
		// throttle/hold-between-marks: the ratio fires at tick 1 (0.20), decays,
		// and settles at 0.04 — inside the band between the 0.03 clear and 0.05
		// fire marks — for the rest of the run. The two-mark latch HOLDS between
		// the marks, so the state changes exactly once (healthy at tick 0 to
		// degraded at tick 1) and stays degraded. Assert the number of changes,
		// not the final state: a single-threshold latch that cleared at 0.04 and
		// re-fired on the next tick ends degraded too and passes any
		// last-tick-only assertion.
		throttled := func(i int) float64 {
			if i <= 1 {
				return 20 * float64(i)
			}
			return 4*float64(i-1) + 20
		}
		states := stateSequence(150, throttled)

		Expect(states[0]).To(Equal(StateHealthy))
		Expect(states[1]).To(Equal(StateDegraded), "a 0.20 ratio must degrade the verdict at the first windowed sample")
		Expect(transitions(states)).To(Equal(1), "a reading between the marks must not flicker")
		Expect(states[149]).To(Equal(StateDegraded), "the degraded state holds through the in-band settlement")
	})

	It("should not fire at all on a reading that enters the band from below, because the fire mark is strict", func() {
		// The other side of the same band, derived from the throttle marks (no
		// recorded scenario covers it): a signal that rises from 0 and holds at
		// 0.04 — below the 0.05 fire mark — must stay healthy for the whole run,
		// with ZERO state changes, because the fire mark is strict and 0.04 never
		// crosses it. A latch built with one mark at the clear value passes the
		// first spec (0.04 is above 0.03, so it holds) and fails only here.
		throttled := func(i int) float64 { return 4 * float64(i) }
		states := stateSequence(150, throttled)

		Expect(states[0]).To(Equal(StateHealthy))
		Expect(transitions(states)).To(Equal(0), "a below-fire reading sitting in the band must stay healthy, never firing")
		Expect(states[149]).To(Equal(StateHealthy))
	})
})
