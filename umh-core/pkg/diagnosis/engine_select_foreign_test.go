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

package diagnosis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Select is the only route to a missing window. It is exported and takes the
// CALLER'S Signal, but resolve looks windows up by (signal name, instrument
// name) in the map NewEngine built, so a signal the table never declared misses
// every lookup. Observe cannot reach that state: it walks the engine's own
// signals, whose windows all exist. The nil-window arms in resolve are
// therefore live code on this path and dead on every other one, which is
// exactly why they read as removable.
var _ = Describe("Engine.Select on a signal the engine was not built from", func() {

	type outSnap struct{ v float64 }

	marks := Marks{
		Unit:     "ratio",
		Fire:     Mark{At: 2.0, Inclusive: true},
		Worst:    4.0,
		Clear:    Mark{At: 1.0, Inclusive: true},
		Polarity: HigherIsWorse,
	}

	extract := func(s outSnap) Reading { return Known(s.v) }

	// filled builds an engine over one declared signal and drives it far enough
	// that the declared signal's own window holds a trustworthy value. Selecting
	// the DECLARED signal therefore reaches Ready, which is the control: it
	// proves the engine is working and the foreign result is about the signal,
	// not about an engine that answers NoInstrument to everything.
	filled := func() (*Engine[outSnap], Signal[outSnap], Environment) {
		declared := Signal[outSnap]{
			Name:       "declared",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[outSnap]{{
				Name:      "mean",
				Extract:   extract,
				Reduction: Mean,
				Span:      3 * time.Second,
				Marks:     marks,
			}},
		}

		e, err := NewEngine(Table[outSnap]{
			Signals:  []Signal[outSnap]{declared},
			Interval: time.Second,
		})
		Expect(err).ToNot(HaveOccurred())

		env := NewEnvironment()
		base := time.Now()
		e.Observe(outSnap{v: 1.0}, env, base)
		e.Observe(outSnap{v: 1.0}, env, base.Add(time.Second))

		return e, declared, env
	}

	It("should report NoInstrument rather than dereferencing the window it has no entry for", func() {
		e, declared, env := filled()

		// Control. The engine answers Ready for the signal it was built from, so
		// a NoInstrument below is a statement about the foreign signal and not
		// about an engine that has nothing to say at all.
		_, _, _, declaredAvail := e.Select(declared, env)
		Expect(declaredAvail).To(Equal(Ready),
			"control: the declared signal resolves Ready at two samples, so the engine is answering")

		// The foreign signal's instrument declares no Requires, so Capable
		// returns it and resolve enters its instrument loop. Without that,
		// resolve's empty-capable early return would produce the same
		// NoInstrument without ever reaching the nil-window arms, and this spec
		// would certify nothing. The instrument NAME is deliberately the same as
		// the declared one: the map key is the pair, so matching half of it
		// still misses.
		foreign := Signal[outSnap]{
			Name:       "never-declared",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[outSnap]{{
				Name:      "mean",
				Extract:   extract,
				Reduction: Mean,
				Span:      3 * time.Second,
				Marks:     marks,
			}},
		}
		Expect(foreign.Capable(env)).To(HaveLen(1),
			"resolve must enter its instrument loop; an empty capable set short-circuits before the missing-window arms and makes this spec vacuous")

		inst, reduced, cov, avail := e.Select(foreign, env)

		Expect(avail).To(Equal(NoInstrument),
			"a signal the engine holds no windows for is indistinguishable from one with no usable instrument")
		Expect(inst.Name).To(BeEmpty(),
			"no instrument was selected, so the zero Instrument comes back")
		_, st := reduced.Get()
		Expect(st).To(Equal(StateAbsent),
			"nothing was reduced, so the reduction is absent rather than a trusted zero")
		Expect(cov.Full()).To(BeFalse(),
			"no window was read, so there is no coverage to report")
	})

	It("should survive a foreign signal carrying more instruments than the engine holds windows for", func() {
		e, _, env := filled()

		// Several misses in one call, so the loop iterates rather than falling
		// out on its first entry. seen stays 0 across all three.
		foreign := Signal[outSnap]{
			Name:       "never-declared",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[outSnap]{
				{Name: "a", Extract: extract, Reduction: Mean, Span: 3 * time.Second, Marks: marks},
				{Name: "b", Extract: extract, Reduction: Mean, Span: 3 * time.Second, Marks: marks},
				{Name: "c", Extract: extract, Reduction: Mean, Span: 3 * time.Second, Marks: marks},
			},
		}
		Expect(foreign.Capable(env)).To(HaveLen(3),
			"all three arms must pass the capability gate, or the loop does not run three times")

		_, _, _, avail := e.Select(foreign, env)
		Expect(avail).To(Equal(NoInstrument),
			"every arm misses, so the engine reports no instrument rather than panicking on the first")
	})
})
