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

package container_monitor_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("sampler full Sample wired into Decide (rung 12b)", func() {
	It("degrades on a PSI-pressure scenario via the sampler's full Sample, not the hand-built throttle-only Sample", func() {
		// Rung 12b wires the sampler's full Sample() output into the
		// cpuhealth.Decide call site in getCPUMetrics. Today getRawCPUMetrics
		// calls c.sampler.Sample(ctx) and uses ONLY sample.UsageCores,
		// discarding PressureAvg60/PsiAvailable/StealFraction/Virtualized/
		// HostBusyCores/LogicalCpus/Quota. getCPUMetrics then hand-builds a
		// Sample{Timestamp, NrPeriods, NrThrottled} from cgroupInfo (which lacks
		// every non-throttle signal) and passes THAT to Decide. The non-throttle
		// causes (pressure, steal, host-contention, saturation) are therefore
		// inert in production: Decide never sees a non-zero PressureAvg60, so the
		// pressure Schmitt flip-latch cannot fire.
		//
		// This test injects a PSI-pressure scenario through the mock fs —
		// cpu.pressure "some avg60=25.00" (0.25 fraction > PressureHigh 0.20) —
		// alongside a capped cpu.max and a throttle-free cpu.stat (nr_throttled
		// pinned at 0 so the throttle latch cannot fire). With the sampler's full
		// Sample wired to Decide, the pressure latch fires and GetStatus returns
		// CPUHealth=Degraded via the non-throttle path. The sampler's full Sample
		// carries PressureAvg60=0.25 to Decide, so the pressure latch fires and
		// GetStatus returns Degraded.
		//
		// The sampler reads the same cgroup files as getCgroupCPUInfo, so both
		// reads see the injected bodies; the double-read of cpu.stat is the
		// accepted rung-12b tradeoff (a later cleanup consolidates them).
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "rung-12b-pressure-wire-test")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		// cpu.max: "200000 100000" => quota 2.0 cores (capped, so the sample is
		// not in the dead-zone and the saturation backstop cannot fire — pressure
		// is the only cause that can degrade here).
		const cpuMax = "200000 100000\n"

		// cpu.pressure: "some avg60=25.00" => 0.25 fraction > PressureHigh 0.20.
		// The sampler divides the raw kernel value by 100 before assigning
		// PressureAvg60, so 25.00 becomes 0.25.
		const cpuPressure = "some avg10=10.00 avg60=25.00 avg300=15.00\ncpu avg10=5.00 avg60=10.00 avg300=8.00\n"

		// nr_throttled is pinned at 0 and nr_periods advances monotonically, so
		// the throttle ratio is 0 across both ticks and the throttle latch cannot
		// fire — the only possible degrade cause is pressure. usage_usec advances
		// so the sampler reports a non-baseline UsageCores on tick 2 (irrelevant
		// to pressure but exercises the real sampler path, not a no-op).
		var nrPeriods int64 = 1000
		var usageUsec int64 = 1_000_000

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.pressure":
				return []byte(cpuPressure), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods %d\nnr_throttled 0\nthrottled_usec 0\n",
					usageUsec, nrPeriods,
				)), nil
			default:
				// /proc/cpuinfo and /proc/stat are absent: the sampler treats
				// those reads as best-effort failures (Virtualized=false,
				// StealFraction=0, HostBusyCores=0), so steal and host-contention
				// cannot fire — pressure remains the sole degrade cause.
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1 — baselines the sampler's usage_usec counter; pressure is read
		// fresh and thresholded directly (no ring/small-N floor), so it fires
		// on this very tick. Pin that the degrade is already visible on tick 1
		// so a future change that delayed the pressure latch to tick 2 would
		// not silently pass.
		nrPeriods, usageUsec = 1000, 1_000_000
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPUHealth).To(Equal(models.Degraded),
			"pressure is thresholded directly on tick 1 (some avg60=0.25 > PressureHigh 0.20); the latch must fire immediately")

		// Tick 2 — pressure sustained; the latch is held above PressureHigh. With
		// the sampler's full Sample wired to Decide, verdict.State is degraded and
		// GetStatus propagates CPUHealth=Degraded. The sampler's full Sample
		// carries PressureAvg60=0.25 to Decide, so the pressure latch fires and
		// GetStatus returns Degraded.
		nrPeriods, usageUsec = 2000, 2_000_000
		status2, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// The degrade must come from the pressure cause, not throttle: nr_throttled
		// is pinned at 0, so IsThrottled must be false. This pins that the
		// non-throttle path is the one firing.
		Expect(status2.CPU.VerdictBasis.Throttle.Fired).To(BeFalse(),
			"nr_throttled is pinned at 0; the degrade must not come from the throttle path")
		Expect(status2.CPUHealth).To(Equal(models.Degraded),
			"a PSI-pressure scenario (some avg60=0.25 > PressureHigh 0.20) must degrade CPUHealth via the sampler's full Sample wired to Decide")
		Expect(status2.OverallHealth).To(Equal(models.Degraded),
			"OverallHealth must co-set with CPUHealth on a CPU degrade")
		Expect(status2.CPU.Health.Category).To(Equal(models.Degraded),
			"the CPU Health category must reflect the degraded verdict")

		// The verdict basis carries the pressure value Decide thresholded: some
		// avg60=25.00 → 0.25 fraction. basis.pressure.value is populated
		// unconditionally (whenever Decide ran), so the value crosses the
		// container_monitor adapter onto models.CPU inside the verdict basis, not
		// as a separate flat mirror.
		Expect(status2.CPU.VerdictBasis).ToNot(BeNil(),
			"verdictBasis is emitted whenever Decide ran (cgroup read succeeded here)")
		Expect(status2.CPU.VerdictBasis.Pressure.Value).To(BeNumerically("~", 0.25, 1e-9),
			"verdictBasis.pressure.value carries the same fraction Decide thresholded (some avg60=25.00 / 100)")
		Expect(status2.CPU.VerdictBasis.Pressure.Applies).To(BeTrue(),
			"verdictBasis.pressure.applies is true when PSI is fetchable (cpu.pressure present)")
		pressureJSON, err := json.Marshal(status2.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(pressureJSON).To(ContainSubstring(`"pressure":{"value":0.25`),
			"verdictBasis.pressure.value is present on the JSON wire carrying 0.25")
	})
})

var _ = Describe("steal p95 wired onto the wire via the sampler", func() {
	It("carries a sub-threshold StealP95 onto models.CPU even though the steal latch never fires, proving the value is populated unconditionally", func() {
		// StealP95 is observability-only and populated UNCONDITIONALLY (like
		// ThrottleRatio), independent of the steal latch. To prove that, this
		// test drives a steal delta that stays BELOW StealHigh (0.10): the latch
		// does NOT fire, yet the computed steal p95 must still reach
		// models.CPU.StealP95 on the wire — proving the value crosses the
		// container_monitor adapter without a fired latch to carry it.
		//
		// cpu.max is capped (Quota=2.0) so the sample is outside the dead-zone
		// (no saturation backstop), nr_throttled is pinned at 0 (no throttle), and
		// cpu.pressure is absent (no pressure) — steal is the only signal moving,
		// and it stays sub-threshold, so the verdict stays healthy.
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "steal-p95-wire-test")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n"
		const cpuInfo = "processor\t: 0\nflags\t\t: fpu vme hypervisor\n"

		// /proc/stat first "cpu " line: 10 counters; total is their sum, index 7
		// is steal. Tick 1 baselines (steal=0, total=1000). Tick 2 advances total
		// by 200 and steal by 1 → StealFraction = 1/200 = 0.005, well below
		// StealHigh 0.10. With Virtualized=true the steal ring then holds
		// [0, 0.005], so the nearest-rank p95 is ~0.005 — the latch does NOT fire.
		var procStat = "cpu 0 0 0 1000 0 0 0 0 0 0\n"

		var nrPeriods int64 = 1000
		var usageUsec int64 = 1_000_000

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods %d\nnr_throttled 0\nthrottled_usec 0\n",
					usageUsec, nrPeriods,
				)), nil
			case "/proc/cpuinfo":
				return []byte(cpuInfo), nil
			case "/proc/stat":
				return []byte(procStat), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1 — baselines /proc/stat (StealFraction=0); the steal ring holds a
		// single 0 sample, below the 2-sample floor, so signals.StealP95 is 0.
		// The basis is emitted whenever Decide ran; basis.steal.value mirrors
		// signals.StealP95, which is populated unconditionally (the box is
		// virtualized, so applies=true and value=0 on the baseline tick). This is
		// the "populated unconditionally" intent carried by the basis.
		nrPeriods, usageUsec = 1000, 1_000_000
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPU.VerdictBasis).ToNot(BeNil(),
			"verdictBasis is emitted whenever Decide ran (cgroup read succeeded here)")
		Expect(status1.CPU.VerdictBasis.Steal.Value).To(BeNumerically("~", 0.0, 1e-9),
			"verdictBasis.steal.value is 0 on the baseline tick (single sample, below the 2-sample floor)")
		Expect(status1.CPU.VerdictBasis.Steal.Applies).To(BeTrue(),
			"verdictBasis.steal.applies is true when the box is virtualized (hypervisor flag present)")
		stealBaselineJSON, err := json.Marshal(status1.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(stealBaselineJSON).To(ContainSubstring(`"steal":{"value":0`),
			"verdictBasis.steal.value is emitted as 0 on the wire when virtualized (applies=true), even on the baseline tick")

		// Tick 2 — steal delta 1, total delta 200 → StealFraction = 0.005; the ring
		// now holds [0, 0.005] and the p95 (~0.005) reaches the wire inside the
		// verdict basis. This p95 is below StealHigh 0.10, so the steal latch does
		// NOT fire, yet basis.steal.value is still populated — proving the value
		// is carried unconditionally, not gated on a fired latch.
		// usage_usec is pinned (no delta → UsageCores 0) so the limit-mode
		// headroom stays positive (2 − 0 − 0.2 = 1.8) under R10.1's two-rule
		// model — isolating the steal assertion from the headroom verdict (the
		// test's intent is "sub-threshold steal → Active," not a usage check).
		procStat = "cpu 0 0 0 1199 0 0 0 1 0 0\n"
		nrPeriods, usageUsec = 2000, 1_000_000
		status2, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status2.CPU.VerdictBasis.Steal.Value).To(BeNumerically("~", 0.005, 1e-9),
			"verdictBasis.steal.value carries the steal p95 as a fraction (steal delta 1 / total delta 200)")
		Expect(status2.CPUHealth).To(Equal(models.Active),
			"a sub-threshold steal p95 (0.005 < StealHigh 0.10) must not fire the steal latch, so CPUHealth stays Active")
		Expect(status2.CPU.VerdictBasis.Throttle.Fired).To(BeFalse(),
			"nr_throttled is pinned at 0; no throttle degrade either")
		stealJSON, err := json.Marshal(status2.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(stealJSON).To(ContainSubstring(`"steal":{"value":0.005`),
			"verdictBasis.steal.value must be present on the JSON wire when non-zero, even sub-threshold")
	})
})

var _ = Describe("sampler failure dead-zone (rung 12b invariant)", func() {
	It("holds a prior throttle fire across a sampler failure instead of running Decide on a zero sample", func() {
		// Pinned invariant: when the sampler's cpu.stat read fails, Decide is
		// skipped (gated on sampler success, not cgroupErr) and the prior
		// verdict + signals are held. The windowState latches also hold
		// because Decide is their only mutator. This prevents the zero sample
		// (LogicalCpus=0, UsageCores=0) from hitting the saturation default
		// branch and clearing all sub-latches, which would flap a prior
		// degraded verdict to healthy for the failure tick.
		//
		// The prior throttle fire comes from successful sampler ticks where
		// cpu.stat carried usage_usec; the sampler-failure tick returns a
		// cpu.stat body WITHOUT usage_usec so parseUsageUsec fails and
		// Sample() returns an error. getCgroupCPUInfo's parseCPUStats ignores
		// usage_usec, but that no longer matters: Decide is not called on the
		// failure tick.
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "rung-12b-sampler-failure-deadzone")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n" // quota 2.0 cores (capped)

		var (
			nrPeriods, nrThrottled, usageUsec int64
			samplerFails                       bool
		)

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				if samplerFails {
					return []byte(fmt.Sprintf(
						"nr_periods %d\nnr_throttled %d\nthrottled_usec 0\n",
						nrPeriods, nrThrottled,
					)), nil
				}

				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods %d\nnr_throttled %d\nthrottled_usec 0\n",
					usageUsec, nrPeriods, nrThrottled,
				)), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1: sampler succeeds, throttle baseline. nr_throttled=0,
		// nr_periods=1000. The ring holds a single point, so throttleRatio
		// returns 0 and the latch is false. Verdict healthy.
		samplerFails = false
		nrPeriods, nrThrottled, usageUsec = 1000, 0, 0
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPUHealth).To(Equal(models.Active),
			"baseline tick: throttle latch false, verdict healthy")
		Expect(status1.CPU.VerdictBasis.Throttle.Fired).To(BeFalse(),
			"baseline: single ring point, throttleRatio=0, latch false")

		// Tick 2: sampler succeeds, nr_throttled jumps so the 60s windowed
		// ratio is 0.10 (100 throttled / 1000 periods), above ThrottleHigh
		// 0.05. The throttle latch fires. Verdict degrades.
		nrPeriods, nrThrottled = 2000, 100
		usageUsec = 1_000_000
		time.Sleep(1 * time.Second)
		status2, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status2.CPU.VerdictBasis.Throttle.Fired).To(BeTrue(),
			"throttle latch must fire from the cgroup counters when the sampler succeeds (ratio 0.10 > 0.05)")
		Expect(status2.CPUHealth).To(Equal(models.Degraded),
			"a throttle fire must degrade CPUHealth via verdict.State")
		Expect(status2.OverallHealth).To(Equal(models.Degraded),
			"OverallHealth must co-set with CPUHealth on a CPU degrade")
		Expect(status2.CPU.VerdictBasis.Throttle.Value).To(BeNumerically("~", 0.10, 1e-9),
			"basis.throttle.value is the Decide-computed windowed ratio from the overlaid counters")

		// Tick 3: sampler fails (cpu.stat missing usage_usec). Decide is
		// skipped. The throttle latch HOLDS (stays fired) and the verdict
		// stays degraded. This proves the hold-on-missing discipline: a
		// transient sampler failure does not flap the verdict to healthy.
		samplerFails = true
		status3, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status3.CPU.VerdictBasis.Throttle.Fired).To(BeTrue(),
			"throttle latch must HOLD across a sampler failure (Decide skipped, latch not mutated)")
		Expect(status3.CPUHealth).To(Equal(models.Degraded),
			"prior degraded verdict must hold across a sampler failure (no flap to healthy)")
		Expect(status3.OverallHealth).To(Equal(models.Degraded),
			"OverallHealth must stay co-set with CPUHealth across a sampler failure")
	})
})

var _ = Describe("Decide gate: sampler success independent of cpu.max (C1)", func() {
	It("runs Decide and degrades on high usage when cpu.max is unreadable but the sampler succeeded", func() {
		// C1: a cgroup-v1-style host where cpu.max is unreadable but the
		// sampler read cpu.stat successfully. Today Decide is gated on
		// cgroupErr == nil && cgroupInfo != nil, which is false when cpu.max
		// is unreadable, so Decide is never called and the verdict stays
		// healthy permanently. The fix gates Decide on sampler success, so
		// the no-host-stats saturation latch evaluates and the verdict
		// degrades on high usage.
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "c1-cpu-max-unreadable")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		var usageUsec int64

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return nil, errors.New("cpu.max not readable")
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods 1000\nnr_throttled 0\nthrottled_usec 0\n",
					usageUsec,
				)), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1: baseline the sampler's usage_usec counter.
		usageUsec = 0
		_, err = svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Tick 2: advance usage_usec so usageCores is far above the
		// HighUsageFraction*LogicalCpus threshold on any host. The
		// no-host-stats saturation latch (no /proc/stat, no limit) fires on
		// usageCores60sMean/LogicalCpus >= 0.70.
		usageUsec = 10_000_000_000
		time.Sleep(1 * time.Second)
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status.CPU.State).To(Equal("degraded"),
			"cgroup v1 host with unreadable cpu.max must still degrade on high usage (sampler succeeded, Decide must run)")
	})
})

var _ = Describe("Decide gate: hold latches on sampler failure (C2)", func() {
	It("holds a prior degraded verdict across a cpu.stat read failure (no flap to healthy)", func() {
		// C2: a prior degraded verdict + a cpu.stat read failure on the next
		// tick. Today the sampler returns Sample{} early (before setting
		// LogicalCpus), getCgroupCPUInfo swallows the same cpu.stat failure
		// returning (info, nil) with NrPeriods=0, cgroupErr==nil, so Decide
		// runs on the zero sample: the saturation switch hits the default
		// branch (LogicalCpus=0) clearing all sub-latches, the throttle ring
		// regresses (NrPeriods=0 < newest) and wipes, and the verdict flaps
		// to healthy. The fix gates Decide on sampler success and holds the
		// prior verdict when the sampler fails.
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "c2-cpu-stat-failure-hold")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		var usageUsec int64
		tick := 0

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte("max 100000\n"), nil
			case "/sys/fs/cgroup/cpu.stat":
				if tick >= 2 {
					return nil, errors.New("cpu.stat transient read error")
				}

				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods 1000\nnr_throttled 0\nthrottled_usec 0\n",
					usageUsec,
				)), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 0: baseline the sampler's usage_usec counter.
		tick, usageUsec = 0, 0
		_, err = svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Tick 1: advance usage_usec so noHostStatsSaturation fires
		// (uncapped, no /proc/stat, high usage fraction). Verdict degrades.
		tick, usageUsec = 1, 10_000_000_000
		time.Sleep(1 * time.Second)
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPU.State).To(Equal("degraded"),
			"prior tick: noHostStatsSaturation must fire on high usage before the failure tick")

		// Tick 2: cpu.stat read fails. Sampler fails. Before the fix Decide
		// runs on the zero sample and flaps to healthy; after the fix Decide
		// is skipped and the prior degraded verdict holds.
		tick = 2
		status2, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status2.CPU.State).To(Equal("degraded"),
			"prior degraded verdict must hold across a cpu.stat read failure (no flap to healthy)")
	})
})
