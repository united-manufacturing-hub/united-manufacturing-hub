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

// These specs pin the SCALE a fired episode is released on. A latch fires against
// one mark pair; the clear arm must only release it on a reduction measured
// against that same pair.
//
// The two shapes below are both real, and a fix that satisfies one while breaking
// the other is the trap here:
//
//   - Different pairs. A signal answers one question two ways, in different units
//     and opposite polarities (spare cores falling is bad; usage fraction rising
//     is bad). The foreign arm's number must not release an episode it never
//     measured.
//   - One shared pair. A p95 and the mean fallback behind it share their pair and
//     differ only in minimum sample count, so selection moves to the p95 the tick
//     it reaches that minimum, on every start. Recovery must still release. Gating
//     on the instrument NAME passes the first shape and strands this one fired
//     forever, because the arm that fired it is never selected again.

package diagnosis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Latch release is judged on the pair the episode fired under", func() {
	const fireSpan = 60 * time.Second

	full := Coverage{span: fireSpan, covered: fireSpan}

	// Two answers to one question, in different units and opposite directions:
	// spare cores falling is bad (LowerIsWorse), usage fraction rising is bad
	// (HigherIsWorse).
	headroom := Marks{
		Unit:     "cores",
		Fire:     Mark{At: 0},
		Clear:    Mark{At: 0.5},
		Polarity: LowerIsWorse,
		Worst:    -1,
	}
	usage := Marks{
		Unit:     "fraction",
		Fire:     Mark{At: 0.70},
		Clear:    Mark{At: 0.60},
		Polarity: HigherIsWorse,
		Worst:    1.0,
	}

	It("should not release a fired episode on a different instrument's clear", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{Signal: "s"})

		// -0.2 spare cores is past the cores fire mark of 0, so the latch fires on
		// the headroom arm.
		l.Update("host-headroom", Reduced{v: -0.2, state: StateValue}, full, headroom, t0)
		f, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value below the cores fire mark fires the latch")
		Expect(f.Instrument).To(Equal("host-headroom"))

		// The headroom arm is gone; a different instrument now measures a fraction.
		// 0.55 sits inside the fraction arm's own HOLD band (between its clear 0.60
		// and fire 0.70), so on its own marks it holds — but under the cores arm the
		// same 0.55 is on the releasing side of a 0.5 clear. The episode was fired
		// by the cores arm, so a fraction cannot be what declares its recovery.
		l.Update("usage-fraction", Reduced{v: 0.55, state: StateValue}, full, usage, t0.Add(time.Second))
		f, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a different instrument's value inside its own hold band does not release the fired episode")
		Expect(f.Instrument).To(Equal("host-headroom"), "the held episode keeps the instrument that fired it")
		Expect(f.Marks.Unit).To(Equal("cores"), "the held episode keeps the marks it fired against")
	})

	// The shared-pair half of the contract, and the reason the gate is the pair
	// rather than the instrument name. Two arms reduce ONE series against ONE mark
	// pair: a p95 that needs twenty samples, and the mean fallback that carries the
	// signal until then. The p95 is declared first, so it wins from the tick it
	// becomes trustworthy and the mean is never selected again — which is exactly
	// the steady state of a real virtualized box. An episode the mean fired must
	// still be releasable by the p95, or it is held for the process's lifetime.
	It("should release a shared-pair episode through the arm that took over from the one that fired it", func() {
		type stealSnap struct{ steal float64 }

		// One pair, both arms. Sharing it is the point: the question and its unit
		// are the same, only the reduction's minimum differs.
		shared := Marks{
			Unit:     "fraction",
			Fire:     Mark{At: 0.10},
			Clear:    Mark{At: 0.05},
			Polarity: HigherIsWorse,
			Worst:    1.0,
		}
		read := func(s stealSnap) Reading { return Known(s.steal) }

		sig := Signal[stealSnap]{
			Name:       "steal",
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[stealSnap]{
				{Name: "steal-p95", Extract: read, Reduction: P95, Span: 60 * time.Second, Marks: shared},
				{Name: "steal-mean", Extract: read, Reduction: Mean, Span: 60 * time.Second, Marks: shared},
			},
		}
		e, err := NewEngine(Table[stealSnap]{Signals: []Signal[stealSnap]{sig}, Interval: time.Second})
		Expect(err).ToNot(HaveOccurred())

		env := NewEnvironment()
		base := time.Unix(7_000_000, 0)

		// Four ticks at 0.90. The mean crosses the 0.10 fire mark at the second
		// (0.45); the p95 is still short of its twenty samples, so the mean is the
		// arm that fires.
		var fired []Fired
		for i := range 4 {
			fired, _ = e.Observe(stealSnap{steal: 0.90}, env, base.Add(time.Duration(i)*time.Second))
		}
		Expect(fired).To(HaveLen(1), "the mean arm fires while the p95 is below its minimum sample count")
		Expect(fired[0].Instrument).To(Equal("steal-mean"),
			"the episode must be fired by the arm that is about to lose selection, or the handover is never exercised")

		// Then quiet. The p95 reaches twenty samples and takes over for good; the
		// spikes leave the sixty-second window, so its value falls to 0, well past
		// the shared 0.05 clear, with the window covered.
		var last []Fired
		var winner string
		for i := 4; i < 90; i++ {
			at := base.Add(time.Duration(i) * time.Second)
			last, _ = e.Observe(stealSnap{steal: 0}, env, at)
			inst, _, _, _ := e.Select(sig, env)
			winner = inst.Name
		}
		Expect(winner).To(Equal("steal-p95"),
			"the p95 must hold selection at the end, or this spec never leaves the arm that fired and proves nothing")
		Expect(last).To(BeEmpty(),
			"a quiet signal must release even though the arm that fired it is no longer selected")
	})

	It("should still release on the instrument that fired the episode when its own clear is crossed", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{Signal: "s"})

		l.Update("host-headroom", Reduced{v: -0.2, state: StateValue}, full, headroom, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value below the cores fire mark fires the latch")

		// 0.6 spare cores is past the cores clear of 0.5 under LowerIsWorse, and it
		// still comes from the SAME arm that fired the episode, so it must release.
		// This is the guard against reading the ownership rule above as "never
		// release": the owning arm's own clear still works.
		l.Update("host-headroom", Reduced{v: 0.6, state: StateValue}, full, headroom, t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "the owning arm crossing its own clear releases the episode")
	})

	It("still holds a fired episode reached through the engine when the winning instrument changes", func() {
		type cpuSnap struct {
			headroom float64
			usage    float64
		}

		headroomReadable := true
		headroomArm := Instrument[cpuSnap]{
			Name: "host-headroom",
			Extract: func(s cpuSnap) Reading {
				if !headroomReadable {
					return Unknown()
				}

				return Known(s.headroom)
			},
			Reduction: Last,
			Span:      3 * time.Second,
			Marks:     headroom,
		}
		usageArm := Instrument[cpuSnap]{
			Name:      "usage-fraction",
			Extract:   func(s cpuSnap) Reading { return Known(s.usage) },
			Reduction: Last,
			Span:      3 * time.Second,
			Marks:     usage,
		}
		sig := Signal[cpuSnap]{
			Name:        "saturation",
			DemoteSpan:  3 * time.Second,
			Instruments: []Instrument[cpuSnap]{headroomArm, usageArm},
		}
		tbl := Table[cpuSnap]{Signals: []Signal[cpuSnap]{sig}, Interval: time.Second}
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())

		env := NewEnvironment()
		base := time.Unix(5_000_000, 0)

		// Drive the headroom arm below its fire mark until its 3s window is full, so
		// a later release arm has full coverage to act on. The usage arm reads
		// nothing on these ticks; resolve picks the ready headroom arm each time.
		for i := range 4 {
			e.Observe(cpuSnap{headroom: -0.2}, env, base.Add(time.Duration(i)*time.Second))
		}
		fired, _ := e.Observe(cpuSnap{headroom: -0.2}, env, base.Add(4*time.Second))
		Expect(fired).To(HaveLen(1), "the headroom arm fires and stays the winner")
		Expect(fired[0].Instrument).To(Equal("host-headroom"))

		// The headroom arm's read fails this tick, so it reduces to untrusted and
		// resolve hands over to the usage arm, whose 0.55 sits inside its own hold
		// band. Full coverage is live; under the buggy release the fraction's 0.55
		// is read against the cores clear and releases the cores-fired episode.
		headroomReadable = false
		fired, readiness := e.Observe(cpuSnap{usage: 0.55}, env, base.Add(5*time.Second))
		Expect(readiness).To(HaveLen(1))
		Expect(readiness[0].Availability).To(Equal(Ready), "the usage arm now answers the question")
		Expect(fired).To(HaveLen(1), "a change of winning instrument does not release the episode it fired")
		Expect(fired[0].Instrument).To(Equal("host-headroom"), "the fired verdict still names the arm that fired it")
		Expect(fired[0].Marks.Unit).To(Equal("cores"), "the fired verdict still carries the marks it fired under")
	})
})
