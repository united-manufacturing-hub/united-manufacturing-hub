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

// These specs check that Signal.RefireAtOnce is copied into the Latch.
// The latch spec in latch_test.go sets the Latch field directly, so it
// still passes if buildSignalState (engine.go) stops copying the field.
// Here two engines differ only in RefireAtOnce and read the same values at
// the same times. If the copy is lost, the engine that set RefireAtOnce
// keeps the cooldown (see Signal.RefireAtOnce) and the spec fails.

package diagnosis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Engine.Observe on a released signal firing again", func() {

	type snap struct{ value float64 }

	base := time.Unix(1_000_000, 0)

	names := func(fired []Fired) []string {
		out := make([]string, len(fired))
		for i, f := range fired {
			out[i] = f.Signal
		}

		return out
	}

	// One signal per engine, reading the snapshot's value field. The Last
	// reduction needs only one reading, so the signal is judged from the first
	// tick. The window span is ten seconds and the table interval one second,
	// so the window first covers its full span at t10.
	signal := func(refire bool) Signal[snap] {
		return Signal[snap]{
			Name:         "S",
			RefireAtOnce: refire,
			DemoteSpan:   60 * time.Second,
			Instruments: []Instrument[snap]{{
				Measurement: Measurement[snap]{
					Name:      "I",
					Extract:   func(s snap) Reading { return Known(s.value) },
					Reduction: Last,
					Span:      10 * time.Second,
				},
				Marks: Marks{Unit: "ratio", Fire: Mark{At: 0.5}, Clear: Mark{At: 0.4}, Worst: 1.0, Polarity: HigherIsWorse},
			}},
		}
	}

	engineOver := func(s Signal[snap]) *Engine[snap] {
		e, err := NewEngine(Table[snap]{Signals: []Signal[snap]{s}, Interval: time.Second})
		Expect(err).ToNot(HaveOccurred())

		return e
	}

	It("fires a released signal again on the next reading past its fire mark only when the signal declares RefireAtOnce", func() {
		refireEngine := engineOver(signal(true))
		cooldownEngine := engineOver(signal(false))
		engines := []*Engine[snap]{refireEngine, cooldownEngine}

		tick := func(e *Engine[snap], value float64, at time.Time) []Fired {
			fired, _ := e.Observe(snap{value: value}, NewEnvironment(), at)

			return fired
		}

		for _, e := range engines {
			Expect(names(tick(e, 1.0, base))).To(Equal([]string{"S"}))
		}

		// Readings below the 0.4 clear mark at t1 through t9 do not release the
		// signal yet. Latch.Update releases only once the window covers its full
		// ten seconds, and that first happens at t10, below.
		for i := 1; i <= 9; i++ {
			for _, e := range engines {
				tick(e, 0.0, base.Add(time.Duration(i)*time.Second))
			}
		}

		for _, e := range engines {
			Expect(names(tick(e, 0.0, base.Add(10*time.Second)))).To(BeEmpty(),
				"a full window of readings below the clear mark releases the fired signal in both engines")
		}

		// At t11, one second after the release, both engines read a value past the
		// fire mark. The engine without RefireAtOnce cannot fire again before t20,
		// one window span after its t10 release.
		refired := tick(refireEngine, 1.0, base.Add(11*time.Second))
		Expect(names(refired)).To(Equal([]string{"S"}),
			"with RefireAtOnce the released signal fires on the next reading past its fire mark")
		Expect(refired[0].Since).To(Equal(base.Add(11*time.Second)),
			"Since of the re-fired signal is the re-fire time t11, not the first fire at t0")
		Expect(names(tick(cooldownEngine, 1.0, base.Add(11*time.Second)))).To(BeEmpty(),
			"without RefireAtOnce the released signal keeps its cooldown, so one second after the release it has not fired")

		// The cooldown engine keeps reading past the fire mark. It stays quiet
		// until t20 and fires there, one window span after its t10 release.
		for i := 12; i <= 19; i++ {
			Expect(names(tick(cooldownEngine, 1.0, base.Add(time.Duration(i)*time.Second)))).To(BeEmpty(),
				"the cooldown holds until one window span after the release (t=%ds)", i)
		}
		Expect(names(tick(cooldownEngine, 1.0, base.Add(20*time.Second)))).To(Equal([]string{"S"}),
			"without RefireAtOnce the signal fires again exactly one window span after its release")
	})
})
