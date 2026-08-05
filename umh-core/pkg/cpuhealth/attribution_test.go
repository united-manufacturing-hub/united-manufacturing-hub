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

// S3 R4 (D1, D2, D3): attribution consults its evidence. A verdict field is not
// asserted without the evidence for it. The host/container split reads both 60s
// means back from the engine's tracks and is STRICTLY greater (hbm > 2 x oum);
// an internal cause (throttling, pressure, the container's own limit budget)
// attributes unknown, never host; and when the split cannot run on untrusted
// means it is unknown. The saturation family folds to one cause before ranking.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("S3 R4 — attribution consults its evidence", func() {
	It("should fire host-full as its own check that stacks on limit saturation, because a limit is a ceiling and not a reservation", func() {
		// saturation/host-full-AND-limit: quota 2.0, 4 cores, usage 0.2 -> 1.95
		// and host busy 0.1 -> 3.8 at tick 40. Both arms over their marks, one
		// cause: the host arm's 4 - 3.8 - 1.0 = -0.8, while the limit arm sits
		// at 2 - 1.95 - 0.2 = -0.15. The fold keeps the host arm, so the value
		// is -0.8 and the cause list holds exactly one saturation entry.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		for i := 0; i <= 100; i++ {
			usage, hb := 0.2, 0.1
			if i >= 40 {
				usage, hb = 1.95, 3.8
			}
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				UsageCores:  diagnosis.Known(usage),
				HostBusy:    diagnosis.Known(hb),
			}
			verdict, sig := Decide(engine, smp, env)
			if i == 100 {
				Expect(sig.HostFullFired).To(BeTrue(), "the machine-full arm must fire")
				Expect(sig.LimitSaturationFired).To(BeTrue(), "the own-budget arm must fire on top of it")
				Expect(verdict.State).To(Equal(StateDegraded))
				Expect(verdict.Causes).To(HaveLen(1), "the two saturation arms fold to exactly one cause")
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindSaturation))
				Expect(verdict.Causes[0].Value).To(BeNumerically("~", -0.8, 1e-9), "the folded value is the host arm's 4 - 3.8 - 1.0")
				Expect(verdict.Causes[0].Unit).To(Equal(Unit("cores")))
			}
		}
	})

	It("should not attribute a full machine to the host when our own sustained usage exceeds the host's non-container share", func() {
		// The same scenario at ticks 100+: our usage mean 1.95, the host's
		// non-container share 3.80 - 1.95 = 1.85, which is NOT greater than our
		// 1.95 — the split says it is not the host's fault. Today it says host;
		// D1 makes it unknown.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		for i := 0; i <= 100; i++ {
			usage, hb := 0.2, 0.1
			if i >= 40 {
				usage, hb = 1.95, 3.8
			}
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				UsageCores:  diagnosis.Known(usage),
				HostBusy:    diagnosis.Known(hb),
			}
			verdict, _ := Decide(engine, smp, env)
			if i == 100 {
				hbm, _ := engine.Track(trackHostBusy).Get()
				oum, _ := engine.Track(trackUsageCores).Get()
				Expect(hbm).To(BeNumerically("~", 3.8, 1e-9))
				Expect(oum).To(BeNumerically("~", 1.95, 1e-9))
				Expect(2*oum).To(BeNumerically(">", hbm), "2 x 1.95 = 3.90 > 3.80")
				Expect(verdict.Attribution).To(Equal(AttributionUnknown), "a machine full on our own load is not the host's fault")
			}
		}

		// The equality boundary: the comparison is STRICTLY greater, matching
		// the parked branch, so hostBusyMean == 2 x ourUsageMean is unknown, not
		// host. The difference is measure-zero and the recording cannot see it,
		// so a rung that asserts only the two recorded rows leaves it to whoever
		// types the operator. Drive hbm 3.2 against oum 1.6 (host-headroom -0.2
		// fires, 3.2 == 2 x 1.6) and require unknown.
		engine2, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env2 := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base2 := time.Now()
		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base2.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				HostBusy:    diagnosis.Known(3.2),
				UsageCores:  diagnosis.Known(1.6),
				Pressure:    diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
				Steal:       diagnosis.Known(0),
			}
			verdict, sig := Decide(engine2, smp, env2)
			if i == 5 {
				Expect(sig.HostFullFired).To(BeTrue(), "host-headroom 4 - 3.2 - 1.0 = -0.2 fires")
				hbm, _ := engine2.Track(trackHostBusy).Get()
				oum, _ := engine2.Track(trackUsageCores).Get()
				Expect(hbm).To(BeNumerically("~", 3.2, 1e-9))
				Expect(oum).To(BeNumerically("~", 1.6, 1e-9))
				Expect(hbm).To(BeNumerically("~", 2*oum, 1e-9), "3.2 == 2 x 1.6 exactly")
				Expect(verdict.Attribution).To(Equal(AttributionUnknown), "exact equality is unknown, not host — the comparison is strict")
			}
		}
	})

	It("should attribute an internal cause as unknown, never as host, whatever the split says", func() {
		// The throttling scenarios: host busy 1.00, our usage 0.20 -> the split
		// says host (1.00 > 2 x 0.20 = 0.40). The dominant cause is throttling,
		// which is internal — the kernel capping US against OUR OWN quota — so
		// attribution is unknown, never host.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				NrPeriods:   diagnosis.Known(1000 * float64(i)),
				NrThrottled: diagnosis.Known(100 * float64(i)),
				HostBusy:    diagnosis.Known(1.0),
				UsageCores:  diagnosis.Known(0.2),
			}
			verdict, _ := Decide(engine, smp, env)
			if i == 5 {
				hbm, _ := engine.Track(trackHostBusy).Get()
				oum, _ := engine.Track(trackUsageCores).Get()
				Expect(hbm).To(BeNumerically("~", 1.0, 1e-9))
				Expect(oum).To(BeNumerically("~", 0.2, 1e-9))
				Expect(hbm).To(BeNumerically(">", 2*oum), "the split itself says host")
				Expect(verdict.Causes).To(HaveLen(1))
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindThrottling))
				Expect(verdict.Attribution).To(Equal(AttributionUnknown), "an internal cause is unknown whatever the split says")
			}
		}
	})

	It("should report unknown attribution when the host-container split cannot be computed", func() {
		// Host stats absent: the host-busy track has nothing to fold, so the
		// split cannot run, and the saturation signal answers through the
		// usage-fraction fallback (3.0 / 4 = 0.75 fires). The dominant cause is
		// saturation, but the machine-full question has no host evidence, so
		// attribution is unknown.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		base := time.Now()

		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				HostBusy:    diagnosis.Unknown(),
				UsageCores:  diagnosis.Known(3.0),
				Pressure:    diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
				Steal:       diagnosis.Known(0),
			}
			verdict, sig := Decide(engine, smp, env)
			if i == 5 {
				_, hbState := engine.Track(trackHostBusy).Get()
				Expect(hbState).NotTo(Equal(diagnosis.StateValue), "the host-busy mean cannot run with no host stats")
				Expect(sig.NoHostStatsSaturationFired).To(BeTrue())
				Expect(verdict.Causes).To(HaveLen(1))
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindSaturation))
				Expect(verdict.Causes[0].Unit).To(Equal(Unit("fraction")))
				Expect(verdict.Attribution).To(Equal(AttributionUnknown), "a split that cannot run attributes unknown")
			}
		}
	})
})
