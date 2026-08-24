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
					Name:      "A-p95",
					Requires:  []Capability{"source-1"},
					Extract:   extract,
					Reduction: P95,
					Span:      60 * time.Second,
					Marks: Marks{
						Unit:     "ratio",
						Fire:     Mark{At: 2.0, Inclusive: true},
						Worst:    4.0,
						Clear:    Mark{At: 1.0, Inclusive: true},
						Polarity: HigherIsWorse,
					},
				},
				{
					Name:      "A-mean",
					Requires:  []Capability{"source-1"},
					Extract:   extract,
					Reduction: Mean,
					Span:      3 * time.Second,
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

		inst, reduced, _, avail := e.Select(sig, env)
		Expect(inst.Name).To(Equal("A-mean"), "the engine skips the untrusted p95 arm and selects the ready mean arm")
		v, st := reduced.Get()
		Expect(st).To(Equal(StateValue), "the selected arm's reduction is trustworthy at two samples")
		Expect(v).To(Equal(1.0), "the selected arm reduces to the mean of the two ticks")
		Expect(avail).To(Equal(Ready), "a selected arm at StateValue reports Ready")
	})

	It("should keep a held latch's severity on the instrument it fired under when the capability behind that instrument disappears", func() {
		// Both instruments answer one question, in different units and opposite
		// directions: stall share rising is bad, spare cores falling is bad.
		type cpuSnap struct{ stall, headroom float64 }

		sig := Signal[cpuSnap]{
			Name:       "saturation",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[cpuSnap]{
				{
					Name:      "pressure",
					Requires:  []Capability{"psi"},
					Extract:   func(s cpuSnap) Reading { return Known(s.stall) },
					Reduction: Last,
					Span:      60 * time.Second,
					Marks: Marks{
						Unit:     "ratio",
						Fire:     Mark{At: 0.10},
						Clear:    Mark{At: 0.06},
						Worst:    1.0,
						Polarity: HigherIsWorse,
					},
				},
				{
					Name:      "headroom",
					Extract:   func(s cpuSnap) Reading { return Known(s.headroom) },
					Reduction: Last,
					Span:      60 * time.Second,
					Marks: Marks{
						Unit:     "cores",
						Fire:     Mark{At: 0},
						Clear:    Mark{At: 0.5},
						Worst:    -4.0,
						Polarity: LowerIsWorse,
					},
				},
			},
		}

		e, err := NewEngine(Table[cpuSnap]{Signals: []Signal[cpuSnap]{sig}, Interval: time.Second})
		Expect(err).ToNot(HaveOccurred())

		base := time.Now()
		fired, _ := e.Observe(cpuSnap{stall: 0.20, headroom: 0.3}, NewEnvironment("psi"), base)
		Expect(fired).To(HaveLen(1), "a stall share of 0.20 crosses the pressure instrument's fire mark of 0.10")
		Expect(fired[0].Severity()).To(BeNumerically("~", 1.0/9.0, 1e-12),
			"0.20 stands a tenth of the way from the 0.10 fire mark to the 1.0 worst value")

		// PSI goes away, so this tick the capability gate leaves only headroom, and
		// 0.3 spare cores is between its clear mark of 0.5 and its fire mark of 0:
		// the latch holds on evidence it can no longer re-read.
		fired, _ = e.Observe(cpuSnap{stall: 0.20, headroom: 0.3}, NewEnvironment(), base.Add(time.Second))
		Expect(fired).To(HaveLen(1), "the latch holds when the instrument that fired it drops out")
		Expect(fired[0].Severity()).To(BeNumerically("~", 1.0/9.0, 1e-12),
			"the held verdict keeps the severity it fired with; the cores pair never measured this value")
	})

	It("runs the tick: drives the per-signal latch and hands back the fired set and a readiness row per signal", func() {
		type snap struct{ v float64 }
		sig := Signal[snap]{
			Name:       "P",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[snap]{
				{
					Name:      "I",
					Requires:  []Capability{"rise"},
					Extract:   func(s snap) Reading { return Known(s.v) },
					Reduction: Last,
					Span:      60 * time.Second,
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
					Name:      "I",
					Requires:  []Capability{"rise"},
					Extract:   func(s snap) Reading { return Known(s.v) },
					Reduction: Last,
					Span:      60 * time.Second,
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

		reduced := e.Reduction("P", "I")
		v, st := reduced.Get()
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
				Name:      "I",
				Requires:  []Capability{"c"},
				Extract:   func(s tsnap) Reading { return Known(s.v) },
				Reduction: Last,
				Span:      60 * time.Second,
				Marks: Marks{
					Unit:     "u",
					Fire:     Mark{At: 100, Inclusive: true},
					Worst:    200,
					Clear:    Mark{At: 0, Inclusive: false},
					Polarity: HigherIsWorse,
				},
			}},
		}
		track := Track[tsnap]{Name: "T", Extract: func(s tsnap) Reading { return Known(s.v) }, Span: 60 * time.Second, Reduction: Mean}

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

	It("should reduce a track whose extractor never reads to an absence, not to a zero the caller would publish as a measurement", func() {
		type usnap struct{ v float64 }
		// Two tracks over the same ticks: one reads, one never does. The reading
		// one is the positive control, so an absence on the silent one is a fact
		// about its extractor and not about a track that was never folded.
		reads := Track[usnap]{Name: "reads", Extract: func(s usnap) Reading { return Known(s.v) }, Span: 60 * time.Second, Reduction: Mean}
		silent := Track[usnap]{Name: "silent", Extract: func(usnap) Reading { return Unknown() }, Span: 60 * time.Second, Reduction: Mean}

		sig := Signal[usnap]{
			Name: "S", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[usnap]{{
				Name: "I", Extract: func(s usnap) Reading { return Known(s.v) }, Reduction: Last, Span: 60 * time.Second,
				Marks: Marks{Unit: "u", Fire: Mark{At: 100, Inclusive: true}, Worst: 200, Clear: Mark{At: 0, Inclusive: false}, Polarity: HigherIsWorse},
			}},
		}
		e, err := NewEngine(Table[usnap]{Signals: []Signal[usnap]{sig}, Tracks: []Track[usnap]{reads, silent}, Interval: time.Second})
		Expect(err).ToNot(HaveOccurred())

		base := time.Unix(4_000_000, 0)
		env := NewEnvironment()
		e.Observe(usnap{v: 2.0}, env, base)
		e.Observe(usnap{v: 4.0}, env, base.Add(time.Second))

		v, st := e.Track("reads").Get()
		Expect(st).To(Equal(StateValue), "the reading track met its floor of two samples over these two ticks")
		Expect(v).To(Equal(3.0), "the reading track folds the mean of its two ticks (2,4)")

		sv, sst := e.Track("silent").Get()
		Expect(sst).To(Equal(StateAbsent), "a track whose extractor answered Unknown on every tick stored nothing, so it has no number to reduce")
		Expect(sv).To(Equal(0.0), "the absence carries the zero value, which Get hands back only alongside StateAbsent")
	})

	It("should return one readiness row per signal beside the fired set, carrying the same availability the latch arm acted on, for signals that fired and signals that could not be read alike", func() {
		type s9 struct{ v float64 }
		ext := func(s s9) Reading { return Known(s.v) }

		fire := Signal[s9]{
			Name: "F", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[s9]{{
				Name: "I", Requires: []Capability{"c"}, Extract: ext, Reduction: Last, Span: 60 * time.Second,
				Marks: Marks{Unit: "u", Fire: Mark{At: 2, Inclusive: true}, Worst: 4, Clear: Mark{At: 0, Inclusive: false}, Polarity: HigherIsWorse},
			}},
		}
		quiet := Signal[s9]{
			Name: "Q", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[s9]{{
				Name: "I", Requires: []Capability{"c"}, Extract: ext, Reduction: Last, Span: 60 * time.Second,
				Marks: Marks{Unit: "u", Fire: Mark{At: 100, Inclusive: true}, Worst: 200, Clear: Mark{At: 0, Inclusive: false}, Polarity: HigherIsWorse},
			}},
		}
		incapable := Signal[s9]{
			Name: "N", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[s9]{{
				Name: "I", Requires: []Capability{"missing"}, Extract: ext, Reduction: Last, Span: 60 * time.Second,
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
		// Carries nothing: this spec switches readability through the closures
		// below, not through the snapshot, so every Observe passes s6{}.
		type s6 struct{}

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
					Reduction: Last, Span: 60 * time.Second,
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
			byName[f.Signal] = true
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
				Extract:   func(s nsnap) Reading { return Known(s.v) },
				Reduction: Last, Span: 60 * time.Second,
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
				Extract:   func(s rsnap) Reading { return Known(s.v) },
				Reduction: Last, Span: 60 * time.Second,
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

	// Observe takes the Reset arm on every AllAbsent tick of a release-on-absent
	// signal, fired or not, and Reset stamps a release time. Stamping one on a
	// latch that never fired would put the re-fire bar in front of a FIRST firing.
	It("should not let an absent tick bar the first firing of a release-on-absent signal that has never fired", func() {
		type qsnap struct{ v float64 }
		marks := Marks{Unit: "u", Fire: Mark{At: 2, Inclusive: true}, Worst: 4, Clear: Mark{At: 1, Inclusive: false}, Polarity: HigherIsWorse}
		sig := Signal[qsnap]{
			Name: "S", DemoteSpan: 60 * time.Second, ReleaseOnAbsent: true,
			Instruments: []Instrument[qsnap]{
				{
					Name: "A", Requires: []Capability{"psi"}, Reduction: Last, Span: 60 * time.Second, Marks: marks,
					Extract: func(s qsnap) Reading { return Known(s.v) },
				},
				{
					Name: "B", Reduction: Last, Span: 60 * time.Second, Marks: marks,
					Extract: func(qsnap) Reading { return Unknown() },
				},
			},
		}
		e, err := NewEngine(Table[qsnap]{Signals: []Signal[qsnap]{sig}, Interval: time.Second})
		Expect(err).ToNot(HaveOccurred())

		withPSI, blind := NewEnvironment("psi"), NewEnvironment()
		t0 := time.Unix(2_000_000, 0)

		fired, readiness := e.Observe(qsnap{v: 1.0}, withPSI, t0)
		Expect(readiness[0].Availability).To(Equal(Ready))
		Expect(fired).To(BeEmpty(), "1.0 is below the fire mark of 2, so this trusted tick anchors the clock at t0 without firing")

		// psi drops, leaving only B, whose window never stored anything.
		fired, readiness = e.Observe(qsnap{v: 1.0}, blind, t0.Add(time.Second))
		Expect(readiness[0].Availability).To(Equal(AllAbsent), "the one surviving instrument reads nothing, so the tick is AllAbsent and takes the Reset arm")
		Expect(fired).To(BeEmpty())

		fired, _ = e.Observe(qsnap{v: 3.0}, withPSI, t0.Add(2*time.Second))
		Expect(fired).To(HaveLen(1), "3.0 crosses the fire mark 2s after t0, and a latch that never fired carries no release for the 60s re-fire bar to count from")
		Expect(fired[0].Value).To(Equal(3.0))
	})

	// The demote clock is anchored on Latch.lastUpdate, which only a trustworthy
	// reduction writes, so neither not-ready arm can move it however often the
	// signal flaps between them. Both arms are driven to the boundary tick here,
	// one per run, because a single run only ever exercises the arm that lands
	// there and would pass with the other arm's release deleted.
	It("should release a signal alternating between AllAbsent and NoneReady on the demote clock, whichever arm lands on the boundary tick", func() {
		type fsnap struct{}

		// A fires on the first tick and then reads nothing, so its window freezes
		// with one entry: untrusted, never empty, for the whole run. B reads
		// nothing ever, so its window is empty from the start. Dropping "psi"
		// leaves B alone and the tick reads AllAbsent; restoring it puts A's
		// frozen window back in the capable set and the tick reads NoneReady.
		firing := true
		marks := Marks{Unit: "u", Fire: Mark{At: 2, Inclusive: true}, Worst: 4, Clear: Mark{At: 1, Inclusive: true}, Polarity: HigherIsWorse}
		sig := Signal[fsnap]{
			Name: "S", DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[fsnap]{
				{
					Name: "A", Requires: []Capability{"psi"}, Reduction: Last, Span: 60 * time.Second, Marks: marks,
					Extract: func(fsnap) Reading {
						if !firing {
							return Unknown()
						}

						return Known(3.0)
					},
				},
				{
					Name: "B", Reduction: Last, Span: 60 * time.Second, Marks: marks,
					Extract: func(fsnap) Reading { return Unknown() },
				},
			},
		}
		tbl := Table[fsnap]{Signals: []Signal[fsnap]{sig}, Interval: time.Second}
		withPSI, blind := NewEnvironment("psi"), NewEnvironment()

		// 60s of demote span at the table's 1s interval puts the release bar on
		// tick 60 after the firing tick; tick 59 is the last one that still holds.
		const bar = 60

		for _, tc := range []struct {
			arm    Availability
			name   string
			offset int
		}{
			{name: "AllAbsent", arm: AllAbsent, offset: 0},
			{name: "NoneReady", arm: NoneReady, offset: 1},
		} {
			firing = true
			e, err := NewEngine(tbl)
			Expect(err).ToNot(HaveOccurred(), tc.name)

			t0 := time.Unix(1_000_000, 0)
			opening, _ := e.Observe(fsnap{}, withPSI, t0)
			Expect(opening).To(HaveLen(1), tc.name+": the readable first tick fires the latch, anchoring the clock at t0")
			firing = false

			seen := map[Availability]int{}
			var heldBeforeBar bool
			var atBar Availability
			var firedAtBar []Fired
			for i := 1; i <= bar; i++ {
				env := blind
				if (i+tc.offset)%2 == 1 {
					env = withPSI
				}
				f, r := e.Observe(fsnap{}, env, t0.Add(time.Duration(i)*time.Second))
				seen[r[0].Availability]++
				if i == bar-1 {
					heldBeforeBar = len(f) == 1
				}
				if i == bar {
					atBar, firedAtBar = r[0].Availability, f
				}
			}

			Expect(seen[AllAbsent]).To(Equal(bar/2), tc.name+": half of the 60 post-fire ticks read AllAbsent, so the alternation this spec needs really happened")
			Expect(seen[NoneReady]).To(Equal(bar/2), tc.name+": the other half read NoneReady")
			Expect(atBar).To(Equal(tc.arm), tc.name+": the boundary tick lands on the arm this run drives")
			Expect(heldBeforeBar).To(BeTrue(), tc.name+": the latch holds at tick 59, one tick short of the 60s demote span after the firing tick at t0")
			Expect(firedAtBar).To(BeEmpty(), tc.name+": the latch releases at tick 60, exactly 60s after t0, however often the two not-ready arms alternated in between")
		}
	})

	// NewEngine reads the caller's Table once, so the caller must be free to
	// keep editing that table afterwards. The positive control runs the same
	// table, same two ticks, unedited: the window reaches StateValue at two
	// samples (Mean needs two) and the signal reads Ready. Only then does the
	// failing half mutate the caller's own instrument name after construction.
	It("keeps the engine's windows reachable when the caller edits their own table after NewEngine returns", func() {
		type snap struct{ v float64 }

		mk := func() Table[snap] {
			sig := Signal[snap]{
				Name:       "s",
				DemoteSpan: 60 * time.Second,
				Instruments: []Instrument[snap]{{
					Name:      "i",
					Extract:   func(s snap) Reading { return Known(s.v) },
					Reduction: Mean,
					Span:      3 * time.Second,
					Marks: Marks{
						Unit:     "u",
						Fire:     Mark{At: 100, Inclusive: true},
						Worst:    200,
						Clear:    Mark{At: 0, Inclusive: false},
						Polarity: HigherIsWorse,
					},
				}},
			}

			return Table[snap]{Signals: []Signal[snap]{sig}, Interval: time.Second}
		}

		drive := func(e *Engine[snap], base time.Time) []Readiness {
			e.Observe(snap{v: 1.0}, NewEnvironment(), base)
			_, readiness := e.Observe(snap{v: 2.0}, NewEnvironment(), base.Add(time.Second))

			return readiness
		}

		control, err := NewEngine(mk())
		Expect(err).ToNot(HaveOccurred())
		Expect(drive(control, time.Unix(10_000_000, 0))[0].Availability).To(Equal(Ready))

		// The caller renames their own instrument after construction. The
		// windows were keyed by the names as they stood at construction; an
		// engine holding a shallow copy of Instruments looks every tick under
		// the new name, finds no window, and reports NoInstrument forever.
		tbl := mk()
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())
		tbl.Signals[0].Instruments[0].Name = "renamed"
		readiness := drive(e, time.Unix(10_000_001, 0))
		Expect(readiness[0].Availability).To(Equal(Ready),
			"a caller's later edit to their own table must not detach the engine from its windows")
	})

	// Fired must remember WHICH instrument crossed the fire mark, not which
	// instrument is the live winner on any later tick: the latch stamps Marks and
	// Value at the fire transition and never refreshes them, and Instrument must be
	// stamped the same way. Two arms declaring no capability and the same marks
	// differ only in reduction, so readiness alone picks the winner. The mean arm
	// (Min 2) fires the signal on tick two while the p95 arm (Min 20) is still
	// below its floor; once twenty samples accumulate, the declared-first p95 arm
	// turns ready and becomes the live winner, but the stamped verdict stays on
	// mean.
	It("stamps Fired with the instrument that fired and keeps it when the live winner later changes", func() {
		type armSnap struct{ v float64 }
		extract := func(s armSnap) Reading { return Known(s.v) }
		marks := Marks{
			Unit:     "ratio",
			Fire:     Mark{At: 2.0, Inclusive: true},
			Worst:    4.0,
			Clear:    Mark{At: 1.0, Inclusive: true},
			Polarity: HigherIsWorse,
		}
		sig := Signal[armSnap]{
			Name:       "S",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[armSnap]{
				{Name: "p95", Extract: extract, Reduction: P95, Span: 60 * time.Second, Marks: marks},
				{Name: "mean", Extract: extract, Reduction: Mean, Span: 60 * time.Second, Marks: marks},
			},
		}
		tbl := Table[armSnap]{Signals: []Signal[armSnap]{sig}, Interval: time.Second}
		env := NewEnvironment()
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		base := time.Unix(11_000_000, 0)
		e.Observe(armSnap{v: 3.0}, env, base)
		fired, _ := e.Observe(armSnap{v: 3.0}, env, base.Add(time.Second))

		Expect(fired).To(HaveLen(1), "a value past the fire mark arms the latch in the tick the mean arm turns trustworthy")
		Expect(fired[0].Instrument).To(Equal("mean"),
			"the mean arm (Min 2) is ready at two samples and fires the signal; the p95 arm is still untrusted at two")
		Expect(fired[0].Marks).To(Equal(marks), "the verdict stamps the marks it fired against")

		// Keep driving past the p95 arm's twenty-sample floor without letting the
		// latch clear (3.0 never crosses the clear mark of 1.0), so the p95 arm
		// turns ready and becomes the live winner. The stamped verdict must stay on
		// mean.
		var late []Fired
		for i := 2; i <= 25; i++ {
			late, _ = e.Observe(armSnap{v: 3.0}, env, base.Add(time.Duration(i)*time.Second))
		}
		Expect(late).To(HaveLen(1), "the latch never clears: every tick stays past the fire mark")
		Expect(late[0].Instrument).To(Equal("mean"),
			"the stamped instrument does not follow the live winner once the p95 arm turns ready")
		Expect(late[0].Marks).To(Equal(marks), "the stamped marks are still the pair that fired, not refreshed by the later winner")

		// Positive control: on this same tick the live winner HAS changed to the
		// p95 arm, so a stamped Instrument equal to mean is a fact about the stamp,
		// not about an unchanged selection.
		live, _, _, _ := e.Select(sig, env)
		Expect(live.Name).To(Equal("p95"),
			"twenty samples turn the declared-first p95 arm ready, so Select now returns it")
	})
})
