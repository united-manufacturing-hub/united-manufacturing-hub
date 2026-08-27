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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var _ = Describe("absence of evidence is not health", func() {
	Describe("the 10-second admission bound", func() {
		It("waits out the window before reporting a capable signal that has never measured, and reports it then", func() {
			// A PSI box whose pressure never first-measures: pressure is capable
			// (PsiAvailable true, so not NoInstrument) but stays non-Ready, so
			// measured stays 0 < capable on every tick — the shortfall. The
			// sample clock is synthetic: each Poll advances Sample.Timestamp by
			// exactly 1s, with no wall clock in the path. The first timestamp the
			// worker sees is the anchor (delta 0s); tick N is at delta (N-1)s.
			start := time.Unix(1_700_000_000, 0).UTC()
			tick := 0
			var lastSample cpuhealth.Sample
			spy := &sentrySpyLogger{FSMLogger: deps.NewNopFSMLogger()}
			d := newDepsWithLogger(spy, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				lastSample = cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tick) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     diagnosis.Unknown(),
					PsiAvailable: true,
				}

				return lastSample, nil
			}}, 4, 0)

			// Pin the window's width to a literal independent of the code
			// constant, so widening admissionWindow alone cannot silently pass.
			const wantWindow = 10 * time.Second
			Expect(admissionWindow).To(Equal(wantWindow),
				"the admission window is pinned to a literal width; a widened code constant must fail here")

			// The warning is the only thing the window drives, so the cumulative
			// count of warnings after each Poll is where the boundary shows up.
			var warned []int
			var capable, measured []int
			var verdict, message []string
			for range int(wantWindow/time.Second) + 2 {
				st, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				warned = append(warned, len(spy.sentryWarnMsgs))
				c, m := countsFor(d, lastSample)
				capable = append(capable, c)
				measured = append(measured, m)
				verdict = append(verdict, st.Verdict)
				message = append(message, st.Message)
			}

			// Inside the 10s window the worker says nothing while it waits
			// for the capable signal to measure: early ticks (deltas 0-4s) are
			// silent, and so is delta 9s. A literal 9 (not boundary-1, which
			// scales with the window and stays satisfied at any width) fails if
			// the window is shortened below 10s.
			Expect(warned[0]).To(Equal(0), "silent on the first tick (delta 0s)")
			Expect(warned[4]).To(Equal(0), "still silent at delta 4s of sample time")
			Expect(warned[9]).To(Equal(0), "still silent at delta 9s, the last tick inside the window")

			// At/after the admission window of sample time the worker gives
			// up waiting and reports, even though a capable signal has STILL
			// never measured — the wait is bounded, not fixed to the counts.
			// Tick index i sits at delta i seconds, so the reporting index is
			// exactly the window in whole seconds.
			boundary := int(admissionWindow / time.Second)
			Expect(warned[boundary]).To(Equal(1), "reports at >=10s of sample time")
			Expect(warned[boundary+1]).To(Equal(1), "and says it once, not again past the window")

			// The report comes on the window closing, not on the evidence
			// changing: the capable and measured counts are identical inside the
			// window (index 1, delta 1s) and at the boundary (delta 10s).
			Expect(measured[boundary]).To(BeNumerically("<", capable[boundary]),
				"the capable-but-never-measured signal still holds at the deadline (the report fires, the counts do not change)")
			Expect(capable[boundary]).To(Equal(capable[1]), "capable count unchanged across the 10s window boundary")
			Expect(measured[boundary]).To(Equal(measured[1]), "measured count unchanged across the 10s window boundary")

			// The reported health is unchanged across the boundary too. The
			// deadline raises the warning and nothing else: it must not turn the
			// never-measured signal into a bad verdict. The intended consumer is
			// bridge admission, where a degraded verdict stops new bridges from
			// starting. That consumer is not built: outside this package,
			// nothing reads Verdict. Once it exists, "the window expired and the
			// signal still never measured, surely that is degraded" would block
			// such a box for its whole life, which is what the bounded wait
			// exists to prevent. It would do so silently, because every other
			// spec that reads Verdict uses time.Now() timestamps and never
			// crosses the deadline.
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

		It("counts nothing capable on a box no instrument can answer, so that box is never short", func() {
			// A box nothing can answer: no limit (quota 0), no PSI (PsiAvailable
			// false), no virtualization (Virtualized false), and no cores for a
			// host-cpu-full signal. Every signal resolves NoInstrument, so capable
			// is 0 and measured < capable is false on every tick — there is no
			// shortfall to wait on, and none to report when the window closes.
			// Guards the `measured < capable` term against a regression that left
			// only the elapsed clause.
			start := time.Unix(1_700_000_000, 0).UTC()
			tick := 0
			var lastSample cpuhealth.Sample
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				lastSample = cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tick) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					PsiAvailable: false,
					Virtualized:  false,
				}

				return lastSample, nil
			}}, 0, 0)

			var capable, measured []int
			for range 3 {
				_, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				c, m := countsFor(d, lastSample)
				capable = append(capable, c)
				measured = append(measured, m)
			}

			Expect(capable[2]).To(Equal(0), "no instrument on this box can answer a signal")
			Expect(measured[2]).To(Equal(0), "nothing capable, nothing measured")
			Expect(measured[0] < capable[0]).To(BeFalse(), "a box nothing can answer is not short, on the first tick")
			Expect(measured[2] < capable[2]).To(BeFalse(), "and is still not short later, inside the window")
		})

		It("keeps the deadline anchored on the first sample when a later timestamp steps backward", func() {
			// The deadline is anchored once, on the first sample timestamp the
			// worker ever sees; production timestamps come from monotonic
			// time.Now(), so a backward step cannot occur there. In the synthetic
			// clock a backward step yields a raw negative elapsed, which reads as
			// 'inside the window' exactly like 0 — so the report must still come
			// at delta 10s from the anchor. Re-anchoring on every tick would hold
			// elapsed at zero and the report would never come at all.
			start := time.Unix(1_700_000_000, 0).UTC()
			tick := 0
			spy := &sentrySpyLogger{FSMLogger: deps.NewNopFSMLogger()}
			d := newDepsWithLogger(spy, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
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

			var warned []int
			for range 3 {
				_, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				warned = append(warned, len(spy.sentryWarnMsgs))
			}

			Expect(warned[0]).To(Equal(0), "silent at the anchor (delta 0s)")
			Expect(warned[1]).To(Equal(0), "the backward tick reads as inside the window (raw negative elapsed)")
			Expect(warned[2]).To(Equal(1), "the report comes 10s after the anchor despite the backward tick")
		})
	})
})
