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

// Engine runs the per-tick loop: it owns every window and every latch for one
// table, and its two-gate Select either works or is dead code. Two instruments
// declaring the SAME capability both pass the capability gate; readiness then
// takes the first whose window can supply a value, so the fallback arm is
// judgeable at two samples while the percentile arm is still untrusted at
// twenty.
var _ = Describe("Engine", func() {

	It("should pass over an instrument whose window cannot supply a value and select the next one whose capabilities are satisfied", func() {
		// engSnap is a generic caller snapshot; both arms of signal A read the
		// same single value off it.
		type engSnap struct{ v float64 }
		extract := func(s engSnap) Reading { return Known(s.v) }

		sig := Signal[engSnap]{
			Name:       "A",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[engSnap]{
				{
					Name:     "A-p95",
					Requires: []Capability{"source-1"},
					Extract:  extract,
					Red:      P95,
					Span:     60 * time.Second,
					Marks: Marks{
						Unit:     "ratio",
						Fire:     Mark{At: 2.0, Inclusive: true},
						Worst:    4.0,
						Clear:    Mark{At: 1.0, Inclusive: true},
						Polarity: HigherIsWorse,
					},
				},
				{
					Name:     "A-mean",
					Requires: []Capability{"source-1"},
					Extract:  extract,
					Red:      Mean,
					Span:     3 * time.Second,
					Marks: Marks{
						Unit:     "ratio",
						Fire:     Mark{At: 2.0, Inclusive: true},
						Worst:    4.0,
						Clear:    Mark{At: 1.0, Inclusive: true},
						Polarity: HigherIsWorse,
					},
				},
			},
		}

		tbl := Table[engSnap]{
			Signals:  []Signal[engSnap]{sig},
			Interval: time.Second,
		}

		env := NewEnvironment("source-1")
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		// Drive two ticks so the mean arm (Min 2) reaches StateValue while the
		// p95 arm (Min 20) is still below its floor and reports untrusted.
		base := time.Now()
		e.Observe(engSnap{v: 1.0}, env, base)
		e.Observe(engSnap{v: 1.0}, env, base.Add(time.Second))

		inst, red, _, avail := e.Select(sig, env)
		Expect(inst.Name).To(Equal("A-mean"), "the engine skips the untrusted p95 arm and selects the ready mean arm")
		v, st := red.Get()
		Expect(st).To(Equal(StateValue), "the selected arm's reduction is trustworthy at two samples")
		Expect(v).To(Equal(1.0), "the selected arm reduces to the mean of the two ticks")
		Expect(avail).To(Equal(Ready), "a selected arm at StateValue reports Ready")
	})

	It("runs the tick: drives the per-signal latch and hands back the fired set and a readiness row per signal", func() {
		type snap struct{ v float64 }
		sig := Signal[snap]{
			Name:       "P",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[snap]{
				{
					Name:     "I",
					Requires: []Capability{"rise"},
					Extract:  func(s snap) Reading { return Known(s.v) },
					Red:      Last,
					Span:     60 * time.Second,
					Marks: Marks{
						Unit:     "u",
						Fire:     Mark{At: 2, Inclusive: true},
						Worst:    4,
						Clear:    Mark{At: 1, Inclusive: true},
						Polarity: HigherIsWorse,
					},
				},
			},
		}

		tbl := Table[snap]{Signals: []Signal[snap]{sig}, Interval: time.Second}
		env := NewEnvironment("rise")
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		fired, readiness := e.Observe(snap{v: 3.0}, env, time.Now())

		Expect(fired).To(HaveLen(1), "a value above the fire mark arms the latch in the tick")
		Expect(fired[0].Identity.Signal).To(Equal("P"))
		Expect(fired[0].Value).To(Equal(3.0))

		Expect(readiness).To(HaveLen(1), "one readiness row per signal, whether or not it fired")
		Expect(readiness[0].Signal).To(Equal("P"))
		Expect(readiness[0].Availability).To(Equal(Ready))
	})

	It("reads back a populated window's reduction rather than reporting a permanent absence", func() {
		type snap struct{ v float64 }
		sig := Signal[snap]{
			Name:       "P",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[snap]{
				{
					Name:     "I",
					Requires: []Capability{"rise"},
					Extract:  func(s snap) Reading { return Known(s.v) },
					Red:      Last,
					Span:     60 * time.Second,
					Marks: Marks{
						Unit:     "u",
						Fire:     Mark{At: 2, Inclusive: true},
						Worst:    4,
						Clear:    Mark{At: 1, Inclusive: true},
						Polarity: HigherIsWorse,
					},
				},
			},
		}

		tbl := Table[snap]{Signals: []Signal[snap]{sig}, Interval: time.Second}
		env := NewEnvironment("rise")
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		base := time.Now()
		e.Observe(snap{v: 2.0}, env, base)
		e.Observe(snap{v: 4.0}, env, base.Add(time.Second))

		red := e.Reduction("P", "I")
		v, st := red.Get()
		Expect(st).To(Equal(StateValue), "a populated window reads back a trustworthy value")
		Expect(v).To(Equal(4.0), "under a last-value reduction the newest entry is the answer")
	})

	It("should fold every declared track on every tick and reduce it back by name, without selecting, judging or firing anything", func() {
		type tsnap struct{ v float64 }
		// The signal is quiet (fire mark far above any value), so the fired set is
		// empty and any fired/readiness contribution from the TRACK is plainly
		// absent.
		sig := Signal[tsnap]{
			Name:       "S",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[tsnap]{{
				Name:     "I",
				Requires: []Capability{"c"},
				Extract:  func(s tsnap) Reading { return Known(s.v) },
				Red:      Last,
				Span:     60 * time.Second,
				Marks: Marks{
					Unit:     "u",
					Fire:     Mark{At: 100, Inclusive: true},
					Worst:    200,
					Clear:    Mark{At: 0, Inclusive: false},
					Polarity: HigherIsWorse,
				},
			}},
		}
		track := Track[tsnap]{Name: "T", Extract: func(s tsnap) Reading { return Known(s.v) }, Span: 60 * time.Second, Red: Mean}

		tbl := Table[tsnap]{Signals: []Signal[tsnap]{sig}, Tracks: []Track[tsnap]{track}, Interval: time.Second}
		env := NewEnvironment("c")
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		base := time.Unix(5_000_000, 0)
		e.Observe(tsnap{v: 1.0}, env, base)
		_, readiness := e.Observe(tsnap{v: 3.0}, env, base.Add(time.Second))

		v, st := e.Track("T").Get()
		Expect(st).To(Equal(StateValue), "a track that has met its floor is trustworthy")
		Expect(v).To(Equal(2.0), "the track folds the mean of its two ticks (1,3)")

		// A track is neither selected, judged nor fired: exactly one readiness row
		// (the signal), and the signal stays quiet (below its fire mark).
		Expect(readiness).To(HaveLen(1), "a track adds no readiness row of its own")
		Expect(readiness[0].Signal).To(Equal("S"))

		_, absent := e.Track("nope").Get()
		Expect(absent).To(Equal(StateAbsent), "an unnamed track reduces to absence")
	})

	It("should return one readiness row per signal beside the fired set, carrying the same availability the latch arm acted on, for signals that fired and signals that could not be read alike", func() {
		type s9 struct{ v float64 }
		ext := func(s s9) Reading { return Known(s.v) }

		fire := Signal[s9]{
			Name: "F", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[s9]{{
				Name: "I", Requires: []Capability{"c"}, Extract: ext, Red: Last, Span: 60 * time.Second,
				Marks: Marks{Unit: "u", Fire: Mark{At: 2, Inclusive: true}, Worst: 4, Clear: Mark{At: 0, Inclusive: false}, Polarity: HigherIsWorse},
			}},
		}
		quiet := Signal[s9]{
			Name: "Q", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[s9]{{
				Name: "I", Requires: []Capability{"c"}, Extract: ext, Red: Last, Span: 60 * time.Second,
				Marks: Marks{Unit: "u", Fire: Mark{At: 100, Inclusive: true}, Worst: 200, Clear: Mark{At: 0, Inclusive: false}, Polarity: HigherIsWorse},
			}},
		}
		incapable := Signal[s9]{
			Name: "N", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[s9]{{
				Name: "I", Requires: []Capability{"missing"}, Extract: ext, Red: Last, Span: 60 * time.Second,
				Marks: Marks{Unit: "u", Fire: Mark{At: 2, Inclusive: true}, Worst: 4, Clear: Mark{At: 1, Inclusive: true}, Polarity: HigherIsWorse},
			}},
		}

		tbl := Table[s9]{Signals: []Signal[s9]{fire, quiet, incapable}, Interval: time.Second}
		env := NewEnvironment("c")
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		fired, readiness := e.Observe(s9{v: 3.0}, env, time.Unix(6_000_000, 0))

		Expect(readiness).To(HaveLen(3), "one readiness row per signal, in table order")
		Expect(readiness[0].Signal).To(Equal("F"))
		Expect(readiness[0].Availability).To(Equal(Ready))
		Expect(readiness[1].Signal).To(Equal("Q"))
		Expect(readiness[1].Availability).To(Equal(Ready), "a capable quiet signal is Ready")
		Expect(readiness[2].Signal).To(Equal("N"))
		Expect(readiness[2].Availability).To(Equal(NoInstrument), "an incapable signal has a NoInstrument row, NOT an absent row")

		Expect(fired).To(HaveLen(1), "only the over-mark signal fires")
		Expect(fired[0].Identity.Signal).To(Equal("F"), "a Ready-but-quiet signal contributes nothing to the fired set")
	})

	It("should reset a fired latch the moment its window is AllAbsent when the signal declares release-on-absent, while a non-release signal holds until its own demote clock elapses", func() {
		type s6 struct{ v float64 }

		// Readability-switched extractors: when a signal's switch is on, its
		// window stores an over-fire value; when off, an absence on every tick.
		onT, onF := true, true
		mkElem := func(name string, on *bool, release bool) Signal[s6] {
			return Signal[s6]{
				Name: name, DemoteSpan: 60 * time.Second, ReleaseOnAbsent: release,
				Instruments: []Instrument[s6]{{
					Name: "I", Requires: []Capability{"c"},
					Extract: func(s s6) Reading {
						if !*on {
							return Unknown()
						}
						return Known(5.0)
					},
					Red: Last, Span: 60 * time.Second,
					Marks: Marks{Unit: "u", Fire: Mark{At: 2, Inclusive: true}, Worst: 4, Clear: Mark{At: 0, Inclusive: false}, Polarity: HigherIsWorse},
				}},
			}
		}

		tbl := Table[s6]{Signals: []Signal[s6]{mkElem("T", &onT, true), mkElem("F", &onF, false)}, Interval: time.Second}
		env := NewEnvironment("c")
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		base := time.Unix(7_000_000, 0)
		e.Observe(s6{}, env, base) // both readable: both fire

		// Give F one more trusted (Ready) tick just inside its demote boundary so
		// its latch clock restarts from a recent update. The Reset arm and the
		// ReleaseAfter arm are keyed by the SAME DemoteSpan, so at a tick where
		// BOTH windows have already emptied the two are indistinguishable: F would
		// release on the clock there, exactly as T does on the AllAbsent reset. To
		// make the arms observable, F's clock must still be running on the AllAbsent
		// tick, which means F's window must not yet be empty.
		onT = false
		e.Observe(s6{}, env, base.Add(55*time.Second)) // F still readable -> Ready; T freezes, holds

		// Silence both and advance past T's demote boundary. T's window (fired at
		// base, never re-updated) demotes -> AllAbsent -> Reset: released
		// immediately. F's window (fired at base+55s) is still populated and
		// frozen, so F is NoneReady and its ReleaseAfter, keyed off the base+55s
		// update, has NOT elapsed: F stays fired. T's Reset is what distinguishes
		// the two on this tick.
		onF = false
		fired, readiness := e.Observe(s6{}, env, base.Add(61*time.Second))

		byName := make(map[string]bool, len(fired))
		for _, f := range fired {
			byName[f.Identity.Signal] = true
		}
		Expect(byName["T"]).To(BeFalse(), "release-on-absent resets the fired latch the instant its window is AllAbsent")
		Expect(byName["F"]).To(BeTrue(), "a non-release signal stays fired while its demote clock has not elapsed")
		Expect(readiness).To(HaveLen(2), "one readiness row per signal")

		// Once F's own demote boundary passes (base+55s + 60s) F, too, releases on
		// the clock, so the two converge with the clock alone.
		onT, onF = false, false
		firedAfter, _ := e.Observe(s6{}, env, base.Add(116*time.Second))
		Expect(firedAfter).To(BeEmpty(), "F releases once its own demote boundary passes")
	})

	It("should release a fired latch on the demote-span clock when no instrument can answer the question at all", func() {
		type nsnap struct{ v float64 }
		sig := Signal[nsnap]{
			Name: "N", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[nsnap]{{
				Name: "I", Requires: []Capability{"c"},
				Extract: func(s nsnap) Reading { return Known(s.v) },
				Red:     Last, Span: 60 * time.Second,
				Marks: Marks{Unit: "u", Fire: Mark{At: 2, Inclusive: true}, Worst: 4, Clear: Mark{At: 0, Inclusive: false}, Polarity: HigherIsWorse},
			}},
		}
		tbl := Table[nsnap]{Signals: []Signal[nsnap]{sig}, Interval: time.Second}
		capable := NewEnvironment("c")
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		base := time.Unix(8_000_000, 0)
		e.Observe(nsnap{v: 3.0}, capable, base) // fires
		e.Observe(nsnap{v: 3.0}, capable, base.Add(time.Second))

		blind := NewEnvironment() // no capabilities -> NoInstrument
		fired, readiness := e.Observe(nsnap{v: 3.0}, blind, base.Add(2*time.Second))
		Expect(readiness[0].Availability).To(Equal(NoInstrument), "an absent capability is NoInstrument, not Ready")
		Expect(fired).To(HaveLen(1), "the latch is still fired before the demote clock elapses")

		firedAfter, _ := e.Observe(nsnap{v: 3.0}, blind, base.Add(62*time.Second))
		Expect(firedAfter).To(BeEmpty(), "a latch releases once the demote clock elapses with nothing able to answer")
	})

	It("should NOT reset a NoInstrument signal on release-on-absent — that flag applies to AllAbsent only, so the latch holds on the demote clock", func() {
		type rsnap struct{ v float64 }
		sig := Signal[rsnap]{
			Name: "R", DemoteSpan: 60 * time.Second, ReleaseOnAbsent: true,
			Instruments: []Instrument[rsnap]{{
				Name: "I", Requires: []Capability{"c"},
				Extract: func(s rsnap) Reading { return Known(s.v) },
				Red:     Last, Span: 60 * time.Second,
				Marks: Marks{Unit: "u", Fire: Mark{At: 2, Inclusive: true}, Worst: 4, Clear: Mark{At: 0, Inclusive: false}, Polarity: HigherIsWorse},
			}},
		}
		tbl := Table[rsnap]{Signals: []Signal[rsnap]{sig}, Interval: time.Second}
		capable := NewEnvironment("c")
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		base := time.Unix(9_000_000, 0)
		e.Observe(rsnap{v: 3.0}, capable, base) // fires
		e.Observe(rsnap{v: 3.0}, capable, base.Add(time.Second))

		blind := NewEnvironment() // no capabilities -> NoInstrument
		fired, readiness := e.Observe(rsnap{v: 3.0}, blind, base.Add(2*time.Second))
		Expect(readiness[0].Availability).To(Equal(NoInstrument))
		Expect(fired).To(HaveLen(1), "even with release-on-absent, a NoInstrument signal is NOT reset immediately — it holds until its demote clock elapses")

		firedAfter, _ := e.Observe(rsnap{v: 3.0}, blind, base.Add(62*time.Second))
		Expect(firedAfter).To(BeEmpty(), "a NoInstrument signal releases once its demote clock elapses, not on the absent flag")
	})
})
