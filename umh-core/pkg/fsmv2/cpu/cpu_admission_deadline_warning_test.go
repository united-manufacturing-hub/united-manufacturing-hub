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

// sentryWarnRecord captures one actual SentryWarn invocation: its structured
// fields plus the two routing arguments. (The worker logs a SentryWarn, not an
// error — there is no error value to capture, and the structured fields carry
// the data.) The feature and hierarchy path are what deps/logger.go documents as
// required for Sentry routing — which alert bucket the warning lands in and
// which worker instance raised it — so they are captured, not discarded.
type sentryWarnRecord struct {
	fields        map[string]any
	feature       deps.Feature
	hierarchyPath string
}

// sentrySpyLogger wraps NopFSMLogger and records every actual SentryWarn
// invocation — its message and structured fields — so a test can assert on
// real call counts rather than a field.
type sentrySpyLogger struct {
	deps.FSMLogger
	sentryWarnMsgs []string
	sentryWarns    []sentryWarnRecord
}

func (s *sentrySpyLogger) SentryWarn(feature deps.Feature, hierarchyPath string, msg string, fields ...deps.Field) {
	s.sentryWarnMsgs = append(s.sentryWarnMsgs, msg)

	rec := sentryWarnRecord{
		fields:        make(map[string]any, len(fields)),
		feature:       feature,
		hierarchyPath: hierarchyPath,
	}
	for _, f := range fields {
		rec.fields[f.Key] = f.Value
	}

	s.sentryWarns = append(s.sentryWarns, rec)
}

func (s *sentrySpyLogger) With(_ ...deps.Field) deps.FSMLogger { return s }

// testHierarchyPath is the hierarchy path the test worker is given. It is
// deliberately non-empty and instance-shaped: the worker must route its warning
// with its OWN path, and an empty-string regression would be invisible against
// an identity that carried no path.
const testHierarchyPath = "scenario-test(application)/cpu-test(" + WorkerType + ")"

// newDepsWithLogger is newDeps with an explicit logger, so a test can observe
// SentryWarn calls (newDeps always binds a Nop logger).
func newDepsWithLogger(log deps.FSMLogger, s cpuhealth.Sampler, cores, quota float64) *CPUDeps {
	d := newDeps(s, cores, quota)
	d.BaseDependencies = deps.NewBaseDependencies(log, nil, deps.Identity{
		ID:            "cpu-test",
		WorkerType:    WorkerType,
		HierarchyPath: testHierarchyPath,
	})

	return d
}

var _ = Describe("a capable signal that has not first-measured is reported at the deadline", func() {
	Describe("Sentry-once at the 10s admission deadline", func() {
		It("raises a SentryWarn exactly once, naming the never-measured signal, when the deadline passes — never per tick, never on a no-PSI box", func() {
			// A PSI box whose only capable signal is pressure, which never
			// first-measures: cores=0/quota=0 keeps both capacity signals out of
			// the table and makes the rest NoInstrument, so exactly pressure is
			// capable and it stays non-Ready forever — measured 0 < capable 1 on
			// every tick. Past the 10s window the worker gives up waiting and
			// must raise one SentryWarn naming "pressure", not one per tick.
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
			Expect(admissionWindow).To(Equal(10 * time.Second))

			// Poll 13 times: indices 0..9 are strictly inside the 10s window
			// (deltas 0..9s of sample time), indices 10..12 are past/at the
			// deadline. Record the cumulative SentryWarn count after each Poll
			// call; Poll must keep succeeding (never a Poll error) throughout.
			boundary := int(admissionWindow / time.Second)
			all := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, boundary, boundary + 1, boundary + 2}

			counts := make([]int, len(all))
			for i := range all {
				_, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred(),
					"a never-measured capable signal at the deadline must never surface as a Poll error (a degrade, not an error)")
				counts[i] = len(spy.sentryWarnMsgs)
			}

			// No SentryWarn before the deadline: every Poll strictly inside
			// the window (deltas 0..9s) stays silent.
			for i := range boundary {
				Expect(counts[i]).To(Equal(0),
					"no SentryWarn while still inside the admission window (delta %ds)", i)
			}

			// Exactly ONE across all past-deadline Polls — the once-per-worker
			// contract. If the worker raised per tick, the three polls at/after the
			// deadline (boundary, boundary+1, boundary+2) would accumulate 3, not 1.
			Expect(counts[boundary]).To(Equal(1),
				"the SentryWarn fires by the first past-deadline Poll, and only once")
			Expect(counts[len(all)-1]).To(Equal(1),
				"the SentryWarn fires once per worker, never once per tick")

			// The event name is a FIXED literal — never interpolated. sentry's
			// BuildFingerprint groups on the log message verbatim, so a Sprintf
			// carrying the signal names would give every distinct combination its
			// own Sentry issue. A regression that interpolates must fail here.
			Expect(spy.sentryWarnMsgs[0]).To(Equal("cpu_admission_deadline_never_measured_signal"),
				"the event name is a fixed grouping key, never instance-varying")

			// The varying data rides in the structured fields — never in the
			// message prose. sentry/hook.go promotes only a fixed tag set; these
			// fields land in Contexts["umh_context"], which the Sentry issue page
			// shows per event but does not index for search (sentry/doc.go). What
			// keeps the occurrences together is the fixed event name asserted
			// above; the fields are what an operator reads once inside that one
			// issue, and what the structured log line carries locally. A
			// regression that stops emitting the structured fields must fail here.
			Expect(spy.sentryWarns).To(HaveLen(1), "the warning is recorded once, like the call")
			Expect(spy.sentryWarns[0].fields["never_measured_signals"]).To(Equal("pressure"),
				"the never-measured signal name rides in a structured field")
			Expect(spy.sentryWarns[0].fields["signals_measured"]).To(Equal(0),
				"the measured shortfall rides in a structured field")
			Expect(spy.sentryWarns[0].fields["signals_capable"]).To(Equal(1),
				"the capable count rides in a structured field")
			Expect(spy.sentryWarns[0].fields["admission_window"]).To(Equal(10*time.Second),
				"the admission window rides in a structured field")

			// The two ROUTING arguments — the ones deps/logger.go requires and
			// logger_impl.go emits as the `feature` and `hierarchy_path` fields.
			// The event name and the payload can be perfect while the warning
			// lands in another team's alert bucket, or names no instance at all,
			// and an operator would never find it.
			Expect(spy.sentryWarns[0].feature).To(Equal(deps.FeatureSupportCPU),
				"the warning routes to the CPU feature, not some other worker's bucket")
			Expect(spy.sentryWarns[0].hierarchyPath).NotTo(BeEmpty(),
				"the warning names the instance that raised it")
			Expect(spy.sentryWarns[0].hierarchyPath).To(Equal(testHierarchyPath),
				"the path is this worker's own, the one its identity carries")
			Expect(spy.sentryWarns[0].hierarchyPath).To(Equal(d.GetHierarchyPath()),
				"and it is read from the deps rather than hardcoded at the call site")

			// A box no instrument can answer fires none at all: on a no-PSI,
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
			Expect(spyQuiet.sentryWarnMsgs).To(BeEmpty(),
				"a no-PSI box with no capable signal raises no SentryWarn at or past the deadline")
		})

		It("raises no SentryWarn on a box whose only capable signal measured within the window", func() {
			// A PSI box whose only capable signal is pressure, which first-measures
			// on the second tick — measured==capable==1 thereafter. Past the 10s
			// window the box is healthy, so no SentryWarn may ever fire. This guards
			// the measured < capable discrimination on a healthy capable box: if a
			// regression changed < to <=, this worker would raise a spurious
			// SentryWarn past the deadline.
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

			boundary := int(admissionWindow / time.Second)
			for i := 0; i <= boundary+2; i++ {
				_, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(spy.sentryWarnMsgs).To(BeEmpty(),
				"a box whose capable signal measured within the window raises no SentryWarn at or past the deadline")
		})

		It("on a many-capable box names EVERY never-measured signal and the shortfall, once, never the measured one", func() {
			// A quota'd PSI box (cores=4, quota=1, HasLimit + HasPressureStats in the
			// environment): pressure becomes capable only because PsiAvailable is
			// true; throttle and container-limit-full become capable because the quota
			// is positive; host-cpu-full is capable because cores are readable. Only
			// pressure ever produces a reading (Known on every tick); throttle
			// (counter, no NrThrottled), container-limit-full and host-cpu-full (no
			// usage/host-busy input) stay AllAbsent forever. So measured==1 and
			// capable==4 on every tick, and the never-measured set is
			// THREE names — the plural name-assembly path a single-capable box never
			// exercises. Past the 10s window exactly one SentryWarn must name all
			// three, name none of the measured signal, and carry the shortfall
			// (measured %d of %d capable) in structured fields.
			start := time.Unix(1_700_000_000, 0).UTC()

			spy := &sentrySpyLogger{FSMLogger: deps.NewNopFSMLogger()}
			tick := 0
			var lastSample cpuhealth.Sample
			d := newDepsWithLogger(spy, stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				tick++
				lastSample = cpuhealth.Sample{
					Timestamp:    start.Add(time.Duration(tick) * time.Second),
					Quota:        diagnosis.Known(1),
					NrPeriods:    diagnosis.Known(1),
					Pressure:     diagnosis.Known(0.2),
					PsiAvailable: true,
				}

				return lastSample, nil
			}}, 4, 1)

			boundary := int(admissionWindow / time.Second)
			for i := 0; i <= boundary+2; i++ {
				_, err := Poll(context.Background(), d, CPUConfig{})
				Expect(err).NotTo(HaveOccurred())
				// Every tick keeps the same shortfall (1 of 4): pressure
				// measured, the other three capable signals never first-measured.
				capable, measured := countsFor(d, lastSample)
				Expect(measured).To(Equal(1),
					"exactly one capable signal (pressure) first-measures; the rest never do")
				Expect(capable).To(Equal(4),
					"throttling, container-limit-full and host-cpu-full are capable alongside pressure")
			}

			// Exactly ONE SentryWarn across every past-deadline Poll.
			Expect(spy.sentryWarnMsgs).To(HaveLen(1),
				"the many-capable box raises exactly one SentryWarn, never per tick")

			// The event name stays a FIXED literal even on the plural path — the
			// names must never reach the message, because sentry fingerprints on it
			// and a per-instance message fragments the issue into noise.
			Expect(spy.sentryWarnMsgs[0]).To(Equal("cpu_admission_deadline_never_measured_signal"),
				"the event name is a fixed grouping key on the plural path too")

			// The full plural name set rides in the structured field — EVERY
			// never-measured signal, never the measured one. If the join dropped
			// all-but-the-first name this fails; this is the plural-path guard.
			Expect(spy.sentryWarns).To(HaveLen(1))
			Expect(spy.sentryWarns[0].fields["never_measured_signals"]).To(Equal("throttling, host-cpu-full, container-limit-full"),
				"all three never-measured names ride in the field, not just the first")
			Expect(spy.sentryWarns[0].fields["never_measured_signals"]).NotTo(ContainSubstring("pressure"),
				"the measured capable signal is never named")
			Expect(spy.sentryWarns[0].fields["signals_measured"]).To(Equal(1),
				"the shortfall rides in the structured fields: one measured...")
			Expect(spy.sentryWarns[0].fields["signals_capable"]).To(Equal(4),
				"...of four capable")
		})
	})
})
