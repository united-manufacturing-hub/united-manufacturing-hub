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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// sentryErrorRecord captures one actual SentryError invocation: the fixed error
// (the Sentry grouping key), the message, and the structured fields. A test
// asserts on the grouping invariant (fixed error string) and on the queryable
// fields without importing zap.
type sentryErrorRecord struct {
	err    error
	fields map[string]any
}

// sentrySpyLogger wraps NopFSMLogger and records every actual SentryError
// invocation — its fixed error, message, and structured fields — so a test can
// assert on real call counts rather than a field. Test-local: only this test
// needs to observe SentryError.
type sentrySpyLogger struct {
	deps.FSMLogger
	sentryErrorMsgs []string
	sentryErrors    []sentryErrorRecord
}

func (s *sentrySpyLogger) SentryError(_ deps.Feature, _ string, err error, msg string, fields ...deps.Field) {
	s.sentryErrorMsgs = append(s.sentryErrorMsgs, msg)
	rec := sentryErrorRecord{err: err, fields: make(map[string]any, len(fields))}
	for _, f := range fields {
		rec.fields[f.Key] = f.Value
	}
	s.sentryErrors = append(s.sentryErrors, rec)
}

func (s *sentrySpyLogger) With(_ ...deps.Field) deps.FSMLogger { return s }

// newDepsWithLogger is newDeps with an explicit logger, so a test can observe
// SentryError calls (newDeps always binds a Nop logger).
func newDepsWithLogger(log deps.FSMLogger, s cpuhealth.Sampler, cores, quota float64) *CPUDeps {
	d := newDeps(s, cores, quota)
	d.BaseDependencies = deps.NewBaseDependencies(log, nil, deps.Identity{ID: "cpu-test", WorkerType: WorkerType})
	return d
}

var _ = Describe("admission is refused while a capable signal has not first-measured", func() {
	Describe("Sentry-once at the 10s admission deadline", func() {
		It("raises SentryError exactly once, naming the never-measured signal, when the deadline passes — never per tick, never on a no-PSI box", func() {
			// A PSI box whose only capable signal is pressure, which never
			// first-measures: cores=0/quota=0 drops saturation and makes every
			// other signal NoInstrument, so exactly pressure is capable and it
			// stays non-Ready forever — measured 0 < capable 1 on every tick.
			// Past the 10s window admission opens and the worker must raise one
			// SentryError naming "pressure", not one per tick.
			start := time.Unix(1_700_000_000, 0).UTC()

			spy := &sentrySpyLogger{FSMLogger: deps.NewNopFSMLogger()}
			tick := 0
			d := newDepsWithLogger(spy, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				return cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tick) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     diagnosis.Unknown(),
					PsiAvailable: true,
				}, nil
			}}, 0, 0)

			// Pin the window width to a literal, so a widened code constant
			// cannot silently push the deadline past these ticks.
			Expect(f16AdmissionWindow).To(Equal(10 * time.Second))

			// Poll 13 times: indices 0..9 are strictly inside the 10s window
			// (deltas 0..9s of sample time), indices 10..12 are past/at the
			// deadline. Record the cumulative SentryError count after each Poll
			// call; Poll must keep succeeding (never a Poll error) throughout.
			boundary := int(f16AdmissionWindow / time.Second)
			all := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, boundary, boundary + 1, boundary + 2}

			counts := make([]int, len(all))
			for i := range all {
				_, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred(),
					"a never-measured capable signal at the deadline must never surface as a Poll error")
				counts[i] = len(spy.sentryErrorMsgs)
			}

			// (a) No SentryError before/at the deadline: every Poll strictly
			// inside the window (deltas 0..9s, refusing) stays silent.
			for i := 0; i < boundary; i++ {
				Expect(counts[i]).To(Equal(0),
					"no SentryError while still refusing inside the admission window (delta %ds)", i)
			}

			// (b) Exactly ONE across all past-deadline Polls — the once-per-worker
			// contract. If the worker raised per tick, the three polls at/after the
			// deadline (boundary, boundary+1, boundary+2) would accumulate 3, not 1.
			Expect(counts[boundary]).To(Equal(1),
				"the SentryError fires by the first past-deadline Poll, and only once")
			Expect(counts[len(all)-1]).To(Equal(1),
				"the SentryError fires once per worker, never once per tick")

			// (c) It names the never-measured capable signal.
			Expect(strings.Join(spy.sentryErrorMsgs, " ")).To(ContainSubstring("pressure"),
				"the SentryError names the capable signal that never measured")

			// (e) FIX A: the error string stays FIXED (the Sentry grouping key, so all
			// instances group together) while the queryable data rides in structured
			// fields — never back into the error. A regression that moves the signal
			// names into the error fragments Sentry groups per instance and must fail
			// here.
			Expect(len(spy.sentryErrors)).To(Equal(1), "the fixed error is recorded once, like the call")
			Expect(spy.sentryErrors[0].err.Error()).To(Equal("never-measured capable signal at admission deadline"),
				"the error string is the fixed grouping key, never instance-varying")
			Expect(spy.sentryErrors[0].fields["never_measured_signals"]).To(Equal("pressure"),
				"the never-measured signal name is a queryable structured field")
			Expect(spy.sentryErrors[0].fields["signals_measured"]).To(Equal(0),
				"the measured shortfall is a queryable structured field")
			Expect(spy.sentryErrors[0].fields["signals_capable"]).To(Equal(1),
				"the capable count is a queryable structured field")
			Expect(spy.sentryErrors[0].fields["admission_window"]).To(Equal(10*time.Second),
				"the admission window is a queryable structured field")

			// (d) A box no instrument can answer fires none at all: on a no-PSI,
			// no-limit, non-virtualized box pressure is NoInstrument (not capable),
			// so nothing starts the clock and nothing is ever reported — even past
			// the window: quiet when expected.
			spyQuiet := &sentrySpyLogger{FSMLogger: deps.NewNopFSMLogger()}
			tickQ := 0
			dQ := newDepsWithLogger(spyQuiet, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tickQ++
				return cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tickQ) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					PsiAvailable: false,
					Virtualized:  false,
				}, nil
			}}, 0, 0)

			for i := 0; i <= boundary+2; i++ {
				_, err := Poll(context.Background(), dQ, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(spyQuiet.sentryErrorMsgs).To(BeEmpty(),
				"a no-PSI box with no capable signal raises no SentryError at or past the deadline")
		})

		It("raises no SentryError on a box whose only capable signal measured within the window", func() {
			// A PSI box whose only capable signal is pressure, which first-measures
			// on the second tick — measured==capable==1 thereafter. Past the 10s
			// window the box is healthy, so no SentryError may ever fire. This pins
			// the measured < capable discrimination on a healthy capable box: if a
			// regression changed < to <=, this worker would raise a spurious
			// SentryError past the deadline.
			start := time.Unix(1_700_000_000, 0).UTC()

			spy := &sentrySpyLogger{FSMLogger: deps.NewNopFSMLogger()}
			tick := 0
			d := newDepsWithLogger(spy, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				pressure := diagnosis.Unknown()
				if tick > 1 {
					pressure = diagnosis.Known(0)
				}
				return cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tick) * time.Second),
					Quota:        diagnosis.Known(0),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     pressure,
					PsiAvailable: true,
				}, nil
			}}, 0, 0)

			boundary := int(f16AdmissionWindow / time.Second)
			for i := 0; i <= boundary+2; i++ {
				_, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(spy.sentryErrorMsgs).To(BeEmpty(),
				"a box whose capable signal measured within the window raises no SentryError at or past the deadline")
		})

		It("on a many-capable box names EVERY never-measured signal and the shortfall, once, never the measured one", func() {
			// A quota'd PSI box (cores=4, quota=1, HasLimit + HasPressureStats in the
			// environment): pressure becomes capable only because PsiAvailable is
			// true; throttle and limit-saturation become capable because the quota
			// is positive; saturation is capable because cores are readable. Only
			// pressure ever produces a reading (Known on every tick); throttle
			// (counter, no NrThrottled), limit-saturation and saturation (no
			// usage/host-busy input) stay AllAbsent forever. So SignalsMeasured==1
			// and SignalsCapable==4 on every tick, and the never-measured set is
			// THREE names — the plural name-assembly path a single-capable box never
			// exercises. Past the 10s window exactly one SentryError must name all
			// three, name none of the measured signal, and carry the shortfall
			// (measured %d of %d capable) plus queryable structured fields.
			start := time.Unix(1_700_000_000, 0).UTC()

			spy := &sentrySpyLogger{FSMLogger: deps.NewNopFSMLogger()}
			tick := 0
			d := newDepsWithLogger(spy, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				return cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tick) * time.Second),
					Quota:        diagnosis.Known(1),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     diagnosis.Known(0.2),
					PsiAvailable: true,
				}, nil
			}}, 4, 1)

			boundary := int(f16AdmissionWindow / time.Second)
			var last CPUStatus
			for i := 0; i <= boundary+2; i++ {
				st, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				// Every tick keeps the two-capable-plus shortfall (1 of 4): pressure
				// measured, the other three capable signals never first-measured.
				Expect(st.SignalsMeasured).To(Equal(1),
					"exactly one capable signal (pressure) first-measures; the rest never do")
				Expect(st.SignalsCapable).To(Equal(4),
					"throttling, limit-saturation and saturation are capable alongside pressure")
				last = st
			}

			// (1) Exactly ONE SentryError across every past-deadline Poll.
			Expect(len(spy.sentryErrorMsgs)).To(Equal(1),
				"the many-capable box raises exactly one SentryError, never per tick")

			// (2) The message names EVERY never-measured signal and never the measured
			// one. If the join dropped all-but-the-first name, throttling would appear
			// and limit-saturation/saturation would be missing — this asserts the
			// full set, not just the first.
			msg := strings.Join(spy.sentryErrorMsgs, " ")
			for _, name := range []string{"throttling", "saturation", "limit-saturation"} {
				Expect(msg).To(ContainSubstring(name),
					"the SentryError names %q, a capable signal that never first-measured", name)
			}
			Expect(msg).NotTo(ContainSubstring("pressure"),
				"the SentryError never names the measured capable signal")

			// (3) The measured/capable shortfall is in the message and surfaced on the
			// status — a regression that drops the shortfall must fail here.
			Expect(msg).To(ContainSubstring("measured 1 of 4 capable"),
				"the message reports the measured/capable shortfall")
			Expect(last.SignalsMeasured).To(Equal(1), "the status reports measured==1")
			Expect(last.SignalsCapable).To(Equal(4), "the status reports capable==4")

			// (4) FIX A: the fixed grouping error is intact and the full plural name
			// set rides in the structured field, so all three never-measured names are
			// queryable — not just the first.
			Expect(len(spy.sentryErrors)).To(Equal(1))
			Expect(spy.sentryErrors[0].err.Error()).To(Equal("never-measured capable signal at admission deadline"),
				"the error string stays the fixed grouping key on the plural path too")
			Expect(spy.sentryErrors[0].fields["never_measured_signals"]).To(Equal("throttling, saturation, limit-saturation"),
				"all three never-measured names are queryable, not just the first")
			Expect(spy.sentryErrors[0].fields["signals_measured"]).To(Equal(1))
			Expect(spy.sentryErrors[0].fields["signals_capable"]).To(Equal(4))
		})
	})
})
