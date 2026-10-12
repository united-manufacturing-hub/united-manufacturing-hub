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

// This file is the engine-level companion to the RefireAtOnce latch spec in
// latch_test.go. That spec drives Latch.Update directly, so it cannot see
// whether a Signal declaration reaches the latch at all; this one builds two
// engines whose tables differ only in RefireAtOnce, drives both through one
// timeline, and reads the fired set each reports. The cooldown is the wait
// after a release: a latch may fire again only one window span later. The one
// place a Signal reaches its latch is buildSignalState (engine.go), which copies
// RefireAtOnce into the Latch literal. If that copy is lost, both engines keep
// the cooldown, and this spec fails on the engine that declared RefireAtOnce.

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

	// One signal per engine, reading the snapshot's value field. Last answers
	// off a single reading, so the signal is judged on every tick with no
	// warm-up, and the ten-second window span at the table's one-second
	// interval makes a full window eleven ticks: t0 through t10 span it
	// exactly.
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

		tick := func(e *Engine[snap], value float64, at time.Time) []string {
			fired, _ := e.Observe(snap{value: value}, NewEnvironment(), at)

			return names(fired)
		}

		for _, e := range engines {
			Expect(tick(e, 1.0, base)).To(Equal([]string{"S"}))
		}

		// Nine readings below the 0.4 clear mark age the window without
		// releasing: the clear arm is gated on Coverage.Full(), and the window
		// spans its full ten seconds first on the tenth of them.
		for i := 1; i <= 9; i++ {
			for _, e := range engines {
				tick(e, 0.0, base.Add(time.Duration(i)*time.Second))
			}
		}

		for _, e := range engines {
			Expect(tick(e, 0.0, base.Add(10*time.Second))).To(BeEmpty(),
				"a full window of readings below the clear mark releases the fired signal in both engines")
		}

		// One reading past the fire mark, one interval after the release. The
		// cooldown engine's latch released at t10 and cannot fire again before
		// t20, one window span after that release; RefireAtOnce drops the cooldown.
		refired, _ := refireEngine.Observe(snap{value: 1.0}, NewEnvironment(), base.Add(11*time.Second))
		Expect(names(refired)).To(Equal([]string{"S"}),
			"with RefireAtOnce the released signal fires on the next reading past its fire mark")
		Expect(refired[0].Since).To(Equal(base.Add(11*time.Second)),
			"a re-fired episode is stamped with its own start, not the first episode's")
		Expect(tick(cooldownEngine, 1.0, base.Add(11*time.Second))).To(BeEmpty(),
			"without RefireAtOnce the released signal keeps its cooldown, so one second after the release it has not fired")

		// The cooldown engine keeps reading past the fire mark. It stays quiet
		// until t20 and fires there, one window span after its t10 release.
		for i := 12; i <= 19; i++ {
			Expect(tick(cooldownEngine, 1.0, base.Add(time.Duration(i)*time.Second))).To(BeEmpty(),
				"the cooldown holds until one window span after the release (t=%ds)", i)
		}
		Expect(tick(cooldownEngine, 1.0, base.Add(20*time.Second))).To(Equal([]string{"S"}),
			"without RefireAtOnce the signal fires again exactly one window span after its release")
	})
})
