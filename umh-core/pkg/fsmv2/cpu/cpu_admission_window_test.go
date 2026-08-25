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

package fsmv2cpu

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("absence of evidence is not health", func() {
	Describe("the 10-second admission bound", func() {
		It("should stop refusing admission ten seconds after the worker starts, even when a capable signal has still never measured", func() {
			// A PSI box whose pressure never first-measures: pressure is capable
			// (PsiAvailable true, so not NoInstrument) but stays non-Ready, so
			// measured stays 0 < capable on every tick — the refusal condition.
			// The sample clock is synthetic: each Poll advances Sample.Timestamp
			// by exactly 1s, with no wall clock in the path. The first timestamp
			// the worker sees is the anchor (delta 0s); tick N is at delta (N-1)s.
			start := time.Unix(1_700_000_000, 0).UTC()
			tick := 0
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				return cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tick) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     diagnosis.Unknown(),
					PsiAvailable: true,
				}, nil
			}}, 4, 0)

			// Pin the window's width to a literal independent of the code
			// constant, so widening admissionWindow alone cannot silently pass.
			const wantWindow = 10 * time.Second
			Expect(admissionWindow).To(Equal(wantWindow),
				"the admission window is pinned to a literal width; a widened code constant must fail here")

			var refusing []bool
			var capable, measured []int
			var verdict, message []string
			for range int(wantWindow/time.Second) + 2 {
				st, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				refusing = append(refusing, st.RefusingAdmission)
				capable = append(capable, st.SignalsCapable)
				measured = append(measured, st.SignalsMeasured)
				verdict = append(verdict, st.Verdict)
				message = append(message, st.Message)
			}

			// (a) Inside the 10s window the worker refuses while a capable
			// signal has never measured: early ticks (deltas 0-4s) refuse.
			Expect(refusing[0]).To(BeTrue(), "refusing on the first tick (delta 0s)")
			Expect(refusing[4]).To(BeTrue(), "still refusing at delta 4s of sample time")

			// (b) At/after the admission window of sample time it stops refusing
			// even though a capable signal has STILL never measured — the refusal
			// is bounded, not fixed to the counts. Tick index i sits at delta i
			// seconds, so the first non-refusing index is exactly the window in
			// whole seconds.
			boundary := int(admissionWindow / time.Second)
			// Pin the width on the left edge with a literal index: delta 9s is the
			// last tick strictly inside a 10s window. A literal (not boundary-1,
			// which scales with the window and stays satisfied at any width) fails
			// if the window is shortened below 10s.
			Expect(refusing[9]).To(BeTrue(), "still refusing at delta 9s, the last tick inside the window")
			Expect(refusing[boundary]).To(BeFalse(), "stops refusing at >=10s of sample time")
			Expect(refusing[boundary+1]).To(BeFalse(), "stays non-refusing past the window")
			Expect(measured[boundary]).To(BeNumerically("<", capable[boundary]),
				"the capable-but-never-measured signal still holds at the deadline (admission opens, counts do not change)")

			// (c) The deadline is a separate boolean, not a count change: the
			// capable and measured counts are identical inside the window
			// (tick 2, delta 1s) vs past the window (tick boundary+1, delta 10s).
			Expect(capable[boundary]).To(Equal(capable[1]), "capable count unchanged across the 10s window boundary")
			Expect(measured[boundary]).To(Equal(measured[1]), "measured count unchanged across the 10s window boundary")

			// (d) The reported health is unchanged across the boundary too. The
			// deadline releases admission and nothing else: it must not turn the
			// never-measured signal into a bad verdict. The intended consumer is
			// bridge admission, where a degraded verdict stops new bridges from
			// starting. That consumer is not built: nothing outside this
			// package's own specs and the demo scenarios reads Verdict, and
			// nothing outside this package reads RefusingAdmission at all. Once
			// it exists, "the window expired and the signal still never
			// measured, surely that is degraded" would reinstate exactly the
			// permanent blocking the window exists to end. It would do so
			// silently, because every other spec that reads Verdict uses
			// time.Now() timestamps and never crosses the deadline.
			// The inside-window value (index 1) is the reference on both sides.
			Expect(verdict[1]).NotTo(Equal(string(cpuhealth.StateDegraded)),
				"the reference tick inside the window is not already degraded, so the comparison below can discriminate")
			Expect(message[1]).NotTo(BeEmpty(),
				"the reference tick inside the window reports a message, so the comparison below can discriminate")
			Expect(verdict[boundary]).To(Equal(verdict[1]), "verdict unchanged at the 10s window boundary")
			Expect(verdict[len(verdict)-1]).To(Equal(verdict[1]), "verdict unchanged past the window")
			Expect(message[boundary]).To(Equal(message[1]), "message unchanged at the 10s window boundary")
			Expect(message[len(message)-1]).To(Equal(message[1]), "message unchanged past the window")
		})

		It("ends the refusal the moment measured reaches capable, without waiting for the 10s deadline", func() {
			// Twin A: a bare no-quota box whose ONLY capable signal is pressure
			// (cores=0 and quota=0 keep both capacity signals out of the table;
			// throttling and steal are NoInstrument without a limit or
			// virtualization). Pressure lands
			// Ready at tick 3, so measured reaches capable then — admission must
			// open immediately, still inside the window. This guards the
			// `measured < capable` term: a regression that deleted it would
			// refuse this healthy box for the whole window. Twin B: the same box
			// whose pressure never measures — at the same window position it
			// keeps refusing, pinning the term's other side.
			start := time.Unix(1_700_000_000, 0).UTC()

			tickA := 0
			dA := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tickA++
				pressure := diagnosis.Unknown()
				if tickA >= 3 {
					pressure = diagnosis.Known(0.2)
				}
				return cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tickA) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     pressure,
					PsiAvailable: true,
				}, nil
			}}, 0, 0)

			tickB := 0
			dB := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tickB++
				return cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tickB) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     diagnosis.Unknown(),
					PsiAvailable: true,
				}, nil
			}}, 0, 0)

			var a, b []CPUStatus
			for range 3 {
				sa, err := Poll(context.Background(), dA, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				a = append(a, sa)
				sb, err := Poll(context.Background(), dB, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				b = append(b, sb)
			}

			// Tick 3 is at delta 2s — well inside the window. Twin A's pressure
			// first-measured, so measured == capable and the refusal ends now,
			// not at the deadline; twin B's never measured, so it still refuses.
			Expect(a[2].SignalsMeasured).To(Equal(a[2].SignalsCapable),
				"twin A's only capable signal first-measured at tick 3")
			Expect(a[2].RefusingAdmission).To(BeFalse(),
				"admission opens the moment measured reaches capable, still inside the 10s window")
			Expect(b[2].SignalsMeasured).To(BeNumerically("<", b[2].SignalsCapable),
				"twin B's capable signal still has not measured")
			Expect(b[2].RefusingAdmission).To(BeTrue(),
				"the never-measured twin keeps refusing at the same window position")
		})

		It("never refuses when no signal is capable, however fresh the worker", func() {
			// A box nothing can answer: no limit (quota 0), no PSI (PsiAvailable
			// false), no virtualization (Virtualized false), and no cores for a
			// host-cpu-full signal. Every signal resolves NoInstrument, so capable
			// is 0 and measured < capable is false on every tick — the refusal
			// cannot hold even inside the window. Guards the `measured < capable`
			// term against a regression that left only the elapsed clause.
			start := time.Unix(1_700_000_000, 0).UTC()
			tick := 0
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				return cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tick) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					PsiAvailable: false,
					Virtualized:  false,
				}, nil
			}}, 0, 0)

			var st CPUStatus
			var refusing []bool
			for range 3 {
				var err error
				st, err = Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				refusing = append(refusing, st.RefusingAdmission)
			}

			Expect(st.SignalsCapable).To(Equal(0), "no instrument on this box can answer a signal")
			Expect(st.SignalsMeasured).To(Equal(0), "nothing capable, nothing measured")
			Expect(refusing[0]).To(BeFalse(), "a box nothing can answer never refuses, on the first tick")
			Expect(refusing[2]).To(BeFalse(), "and still does not refuse later, inside the window")
		})

		It("keeps the deadline anchored on the first sample when a later timestamp steps backward", func() {
			// The deadline is anchored once, on the first sample timestamp the
			// worker ever sees; production timestamps come from monotonic
			// time.Now(), so a backward step cannot occur there. In the synthetic
			// clock a backward step yields a raw negative elapsed, which reads as
			// 'inside the window' exactly like 0 — so the deadline must still open
			// at delta 10s from the anchor, and must not re-anchor on the backward
			// tick (which would push the deadline forward forever).
			start := time.Unix(1_700_000_000, 0).UTC()
			tick := 0
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				ts := start.Add(time.Duration(tick) * time.Second)
				switch tick {
				case 2:
					ts = start.Add(-3 * time.Second) // backward step
				case 3:
					ts = start.Add(11 * time.Second) // 10s after the anchor
				}
				return cpuhealth.Sample{
					Timestamp:    ts,
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     diagnosis.Unknown(),
					PsiAvailable: true,
				}, nil
			}}, 4, 0)

			var refusing []bool
			for range 3 {
				st, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				refusing = append(refusing, st.RefusingAdmission)
			}

			Expect(refusing[0]).To(BeTrue(), "refusing at the anchor (delta 0s)")
			Expect(refusing[1]).To(BeTrue(), "the backward tick reads as inside the window (raw negative elapsed)")
			Expect(refusing[2]).To(BeFalse(), "the deadline opens admission 10s after the anchor despite the backward tick")
		})
	})
})
