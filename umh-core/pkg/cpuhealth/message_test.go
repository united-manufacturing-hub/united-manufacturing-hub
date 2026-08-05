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

// S4 R1 — the healthy headline. composeHealthy renders the two-layer healthy
// budget dashboard. The headline is a two-by-two dispatch over
// (LimitApplies && rounded total > 0) and (displayed headroom < 0.05); the
// subject follows the mode and not the column; the displayed headroom is
// derived from the already-rounded total/used/reserve. The R2 withholding
// ("CPU: starting up.") and the R3 readiness-gated budget lines are later
// rungs, so their fields (UsageRingActive/HostBusyRingActive/proc-readability)
// are set true here to keep the assertions stable across the ladder.
package cpuhealth

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// healthySig builds a Signals bag with every healthy-message input in a
// usable state, so individual fields can be overridden per assertion.
func healthySig() Signals {
	return Signals{
		LimitApplies:           true,
		CapacityCores:          2.0,
		AvgUsageCores:          0.0,
		ReserveCores:           0.2,
		UsageRingActive:        true,
		HostBusyRingActive:     true,
		HostBusyCoresAvailable: true,
		ThrottleSignalReady:    true,
		PressureSignalReady:    true,
		StealSignalReady:       true,
		HostHeadroomAvailable:  true,
		HostCpus:               8,
		LogicalCpus:            2,
	}
}

var _ = Describe("S4 R1 — the healthy headline", func() {
	It("should render the headline in limit mode with a percentage, and in no-limit mode with 'The machine' as subject", func() {
		// Limit mode: entry 9 "CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and can use %s more before it is marked degraded."
		limit := healthySig()
		Expect(composeHealthy(limit)).To(ContainSubstring(
			"CPU healthy. This instance is using 0.0 of 2 cores (0% of its limit) and can use 1.8 more before it is marked degraded."))

		// No-limit mode: entry 13 with subject entry 10 "The machine"; used is host-busy.
		nolimit := healthySig()
		nolimit.LimitApplies = false
		nolimit.CapacityCores = 8
		nolimit.AvgUsageCores = 0
		nolimit.HostBusyCores60sMean = 0.0
		nolimit.ReserveCores = 1.0
		Expect(composeHealthy(nolimit)).To(ContainSubstring(
			"CPU healthy. The machine is using 0.0 of 8 cores and can use 7.0 more before it is marked degraded."))
	})

	It("should derive the displayed headroom from the already-rounded total, used and reserve so the printed arithmetic is exact", func() {
		sig := healthySig()
		sig.AvgUsageCores = 0.3
		msg := composeHealthy(sig)
		// 2 total - 0.3 used - 0.2 reserved = 1.5, printed exactly.
		Expect(msg).To(ContainSubstring("Headroom 1.5 cores = 2 total - 0.3 used - 0.2 reserved (degraded below 0)."))
		Expect(msg).To(ContainSubstring("can use 1.5 more before it is marked degraded."))
	})

	It("should omit the percentage suffix when the rounded total is zero, as a sub-0.05-core quota produces", func() {
		sig := healthySig()
		sig.CapacityCores = 0.04 // rounds to 0.0 total
		sig.AvgUsageCores = 0.01
		sig.ReserveCores = 0.0
		msg := composeHealthy(sig)
		// Entry 12 with subject 11: no "(N% of its limit)" suffix, "This instance".
		Expect(msg).To(ContainSubstring("CPU healthy. This instance is using 0.0 of 0 cores and is close to being marked degraded."))
		Expect(msg).NotTo(ContainSubstring("% of its limit"))
	})

	It("should say the instance is close to being marked degraded once the displayed headroom falls below 0.05 cores, instead of offering more", func() {
		sig := healthySig()
		sig.AvgUsageCores = 1.8 // headroom 2.0-1.8-0.2 = 0.0 < 0.05
		Expect(composeHealthy(sig)).To(ContainSubstring("and is close to being marked degraded."))
		Expect(composeHealthy(sig)).NotTo(ContainSubstring("and can use"))

		above := healthySig()
		above.AvgUsageCores = 0.0 // headroom 1.8 >= 0.05
		Expect(composeHealthy(above)).To(ContainSubstring("and can use 1.8 more before it is marked degraded."))
	})

	It("should put the limited-visibility advisory between the headline and the technical details when the dead-zone annotation is set", func() {
		sig := healthySig()
		sig.LimitedVisibility = true
		msg := composeHealthy(sig)
		Expect(msg).To(ContainSubstring(
			"Limited visibility: this instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so UMH cannot fully tell when work is waiting for a free core. Set a CPU limit or enable Linux pressure stats (boot with psi=1) to turn on full monitoring."))
		// The advisory sits between the headline and the Technical Details separator.
		head := strings.Index(msg, "CPU healthy.")
		adv := strings.Index(msg, "Limited visibility:")
		sep := strings.Index(msg, "Technical Details:")
		Expect(head).To(BeNumerically(">=", 0))
		Expect(adv).To(BeNumerically(">", head))
		Expect(sep).To(BeNumerically(">", adv))
	})

	It("should render the monitoring-unavailable line alone when capacity is zero, with no headline, no advisory and no technical details", func() {
		sig := healthySig()
		sig.CapacityCores = 0
		Expect(composeHealthy(sig)).To(Equal(
			"CPU monitoring unavailable: cgroup read failed. Defaulting to healthy."))
	})

	It("should say host headroom is unavailable, naming both core counts, when the container's count describes only the CPUs it may run on", func() {
		sig := healthySig()
		sig.HostHeadroomAvailable = false
		sig.HostCpus = 8
		sig.LogicalCpus = 2
		msg := composeHealthy(sig)
		Expect(msg).To(ContainSubstring("host headroom unavailable: this container is pinned to 2 of 8 CPUs"))
		// It is an advisory-slot line, not the whole message.
		Expect(msg).To(ContainSubstring("CPU healthy."))
		Expect(msg).To(ContainSubstring("Technical Details:"))
		// On ScopeUnknown HostCpus is 0 (bare float64, unknown Get leaves it 0),
		// so the sentence is not rendered for an unknown machine count.
		unknown := healthySig()
		unknown.HostHeadroomAvailable = false
		unknown.HostCpus = 0
		unknown.LogicalCpus = 2
		Expect(composeHealthy(unknown)).NotTo(ContainSubstring("pinned to"))
	})

	It("conformance: Decide populates CapacityCores, ReserveCores, and HostBusyCoresAvailable for the message to read", func() {
		engine, err := NewEngine(8, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()
		smp := Sample{
			Timestamp:   base,
			CpuScope:    ScopeHost,
			Quota:       diagnosis.Known(2.0),
			UsageCores:  diagnosis.Known(0.3),
			HostBusy:    diagnosis.Known(1.0),
			LogicalCpus: diagnosis.Known(8),
			HostCpus:    diagnosis.Known(8),
			// PsiAvailable false => dead zone; supply pressure to keep it healthy.
			Pressure: diagnosis.Known(0.0),
		}
		_, sig := Decide(engine, smp, env)
		Expect(sig.CapacityCores).To(Equal(2.0), "limit mode: capacity is the quota")
		Expect(sig.ReserveCores).To(Equal(0.2), "limit mode: reserve is 0.10 x quota")
		Expect(sig.HostBusyCoresAvailable).To(BeTrue(), "the sample's HostBusy ok bit rides the readability flag")

		// No-limit mode uses a sample whose quota is not positive (the cpu.max
		// "max" case reads Known(0)) — a no-limit box never carries a positive
		// quota on its sample.
		engineNL, err := NewEngine(8, 0)
		Expect(err).NotTo(HaveOccurred())
		envNL := diagnosis.NewEnvironment()
		smpNL := smp
		smpNL.Quota = diagnosis.Known(0)
		_, sigNL := Decide(engineNL, smpNL, envNL)
		Expect(sigNL.CapacityCores).To(Equal(8.0), "no-limit mode: capacity is the logical CPU count")
		Expect(sigNL.ReserveCores).To(Equal(1.0), "no-limit mode: reserve is cpuReserveCores")
	})
})

var _ = Describe("S4 R2 — the healthy message reports only what it measured", func() {
	It("should not report host usage or headroom when the host reading is absent", func() {
		sig := healthySig()
		sig.LimitApplies = false
		sig.CapacityCores = 8
		sig.HostBusyCores60sMean = 0.0
		sig.ReserveCores = 1.0
		sig.HostBusyCoresAvailable = false // read failed, window still full
		Expect(composeHealthy(sig)).To(Equal("CPU: starting up."))
	})

	It("should not report the limit-mode usage figure when the container's own window holds too few samples to reduce, even though the reading succeeded", func() {
		sig := healthySig()
		sig.UsageRingActive = false
		Expect(composeHealthy(sig)).To(Equal("CPU: starting up."))
	})

	It("should not report the no-limit usage figure when the host window holds too few samples to reduce, even though the reading succeeded", func() {
		sig := healthySig()
		sig.LimitApplies = false
		sig.CapacityCores = 8
		sig.HostBusyCores60sMean = 0.0
		sig.ReserveCores = 1.0
		sig.HostBusyCoresAvailable = true
		sig.HostBusyRingActive = false
		Expect(composeHealthy(sig)).To(Equal("CPU: starting up."))
	})

	It("should render no headline at all on a tick whose usage figure is withheld, returning through the same single-line path the zero-capacity guard uses rather than a headline with a hole in it", func() {
		sig := healthySig()
		sig.UsageRingActive = false
		msg := composeHealthy(sig)
		Expect(msg).To(Equal("CPU: starting up."))
		Expect(msg).NotTo(ContainSubstring("CPU healthy."))
		Expect(msg).NotTo(ContainSubstring("Technical Details:"))

		// The floors are per track, not one flag: a limit-mode headline uses
		// the container's usage-cores, so a thin host-busy window must NOT
		// withhold it.
		limitOK := healthySig()
		limitOK.HostBusyRingActive = false
		Expect(composeHealthy(limitOK)).To(ContainSubstring("CPU healthy."))
	})
})
