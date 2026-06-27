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
		Expect(status2.CPU.IsThrottled).To(BeFalse(),
			"nr_throttled is pinned at 0; the degrade must not come from the throttle path")
		Expect(status2.CPUHealth).To(Equal(models.Degraded),
			"a PSI-pressure scenario (some avg60=0.25 > PressureHigh 0.20) must degrade CPUHealth via the sampler's full Sample wired to Decide")
		Expect(status2.OverallHealth).To(Equal(models.Degraded),
			"OverallHealth must co-set with CPUHealth on a CPU degrade")
		Expect(status2.CPU.Health.Category).To(Equal(models.Degraded),
			"the CPU Health category must reflect the degraded verdict")

		// PressureAvg60 (observability-only, populated unconditionally) must reach
		// the wire carrying the same fraction Decide thresholded: some avg60=25.00
		// → 0.25 fraction. This proves the value crosses the container_monitor
		// adapter onto models.CPU, not just the internal Signals struct.
		Expect(status2.CPU.PressureAvg60).To(BeNumerically("~", 0.25, 1e-9),
			"PressureAvg60 reaches the wire as a fraction (some avg60=25.00 / 100)")
		pressureJSON, err := json.Marshal(status2.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(pressureJSON).To(ContainSubstring(`"pressureAvg60":0.25`),
			"pressureAvg60 must be present on the JSON wire when non-zero")
	})
})

var _ = Describe("steal p95 wired onto the wire via the sampler", func() {
	It("carries StealP95 from the sampler's /proc/stat steal delta onto models.CPU, populated unconditionally even when the steal latch has not fired", func() {
		// StealP95 is observability-only and populated UNCONDITIONALLY (like
		// ThrottleRatio), independent of the steal latch. This test drives a
		// virtualized box (/proc/cpuinfo "hypervisor" flag) with a /proc/stat
		// steal delta through the real sampler and pins that the computed steal
		// p95 reaches models.CPU.StealP95 on the wire — proving the value crosses
		// the container_monitor adapter, not just the internal Signals struct.
		//
		// cpu.max is capped (Quota=2.0) so the sample is outside the dead-zone
		// (no saturation backstop), nr_throttled is pinned at 0 (no throttle), and
		// cpu.pressure is absent (no pressure) — steal is the only signal moving.
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "steal-p95-wire-test")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n"
		const cpuInfo = "processor\t: 0\nflags\t\t: fpu vme hypervisor\n"

		// /proc/stat first "cpu " line: 10 counters; index 7 is steal. Tick 1
		// baselines (steal=0, total=1000). Tick 2 advances total by 200 and steal
		// by 100 → StealFraction = 100/200 = 0.5. With Virtualized=true the steal
		// ring then holds [0, 0.5], so the nearest-rank p95 is ~0.5.
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
		// single 0 sample, below the 2-sample floor, so StealP95 is 0.
		nrPeriods, usageUsec = 1000, 1_000_000
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPU.StealP95).To(BeZero(),
			"StealP95 is 0 on the baseline tick (single sample, below the 2-sample floor)")

		// Tick 2 — steal delta 100/200 = 0.5; the ring now holds [0, 0.5] and the
		// p95 reaches the wire. The steal latch need not have fired — StealP95 is
		// populated unconditionally.
		procStat = "cpu 0 0 0 1100 0 0 0 100 0 0\n"
		nrPeriods, usageUsec = 2000, 2_000_000
		status2, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status2.CPU.StealP95).To(BeNumerically("~", 0.5, 1e-9),
			"StealP95 reaches the wire as a fraction (steal delta 100 / total delta 200)")
		stealJSON, err := json.Marshal(status2.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(stealJSON).To(ContainSubstring(`"stealP95":0.5`),
			"stealP95 must be present on the JSON wire when non-zero")
	})
})

var _ = Describe("sampler failure dead-zone (rung 12b invariant)", func() {
	It("keeps the verdict healthy and lets the throttle latch fire when the sampler fails but cgroup throttle counters are present", func() {
		// Pinned invariant (see getCPUMetrics inline comment):
		// when the sampler's cpu.stat read fails, getRawCPUMetrics returns a
		// zero-valued Sample (Quota nil, PsiAvailable false, Virtualized false,
		// UsageCores 0) — the dead-zone. Decide still evaluates the throttle ring
		// from the NrPeriods/NrThrottled overlaid by getCgroupCPUInfo, and the
		// dead-zone saturation backstop runs harmlessly (UsageCores=0 → fraction=0
		// → never fires). The throttle latch therefore remains the only cause that
		// can fire, driven entirely by the cgroup throttle counters — the sampler
		// failure does NOT silently swallow a real throttle condition, nor does it
		// false-fire a saturation/steal/pressure/host-contention cause.
		//
		// The mock fs returns a cpu.stat body WITHOUT a usage_usec line. The
		// sampler's parseUsageUsec fails on that ("usage_usec not found") so
		// Sample() returns an error → getRawCPUMetrics returns a zero-valued Sample.
		// getCgroupCPUInfo's parseCPUStats ignores usage_usec entirely (it parses
		// only nr_periods/nr_throttled/throttled_usec), so it succeeds on the SAME
		// cpu.stat body — the cgroup throttle counters flow through to Decide and
		// the throttle latch is evaluated against a real delta. This mirrors the
		// production failure mode (cgroup v1 / transient read where the sampler
		// cannot parse cpu.stat but getCgroupCPUInfo can).
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "rung-12b-sampler-failure-deadzone")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		// cpu.max: "200000 100000" => quota 2.0 cores (capped). This is read by
		// getCgroupCPUInfo but the sampler's cpu.max read is irrelevant here — the
		// sampler already failed at cpu.stat and returns a zero-valued Sample
		// (Quota nil), so the sample is in the dead-zone regardless.
		const cpuMax = "200000 100000\n"

		// cpu.stat body has nr_periods/nr_throttled/throttled_usec but NO
		// usage_usec — the sampler's parseUsageUsec returns "usage_usec not
		// found" (an error), so Sample() returns (Sample{}, err). getCgroupCPUInfo
		// uses parseCPUStats which only looks at the three throttle keys, so it
		// succeeds.
		var nrPeriods, nrThrottled int64

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"nr_periods %d\nnr_throttled %d\nthrottled_usec 0\n",
					nrPeriods, nrThrottled,
				)), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1 — baseline the throttle ring. nr_throttled=0, nr_periods=1000.
		// The ring holds a single point, so throttleRatio returns 0 and the latch
		// is false. The verdict must be healthy: the dead-zone saturation backstop
		// cannot fire (only one usage point → small-N floor clears it), and no
		// other cause has a signal (Quota nil → no fraction; PsiAvailable false;
		// Virtualized false → steal skipped; HostBusyCores 0 → host-contention
		// cannot fire even if the demand gate were open, which it is not).
		nrPeriods, nrThrottled = 1000, 0
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPUHealth).To(Equal(models.Active),
			"sampler failure → dead-zone → UsageCores=0 → fraction=0 → saturation backstop must not fire; verdict healthy")
		Expect(status1.CPU.IsThrottled).To(BeFalse(),
			"baseline: single ring point, throttleRatio=0, latch false")

		// Tick 2 — nr_throttled jumps so the 60s windowed ratio is 0.10
		// (100 throttled / 1000 periods), above ThrottleHigh 0.05 → the throttle
		// latch fires EVEN THOUGH the sampler failed. This proves the throttle
		// path works when the sampler is down: the counters come from
		// getCgroupCPUInfo, not the sampler. Both counters are monotonic so the
		// clear-on-regression guard does not wipe the ring.
		nrPeriods, nrThrottled = 2000, 100
		status2, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status2.CPU.IsThrottled).To(BeTrue(),
			"throttle latch must fire from the overlaid cgroup counters even when the sampler fails (ratio 0.10 > 0.05)")
		Expect(status2.CPUHealth).To(Equal(models.Degraded),
			"a throttle fire must degrade CPUHealth via verdict.State even on a sampler failure")
		Expect(status2.OverallHealth).To(Equal(models.Degraded),
			"OverallHealth must co-set with CPUHealth on a CPU degrade")
		Expect(status2.CPU.ThrottleRatio).To(BeNumerically("~", 0.10, 1e-9),
			"ThrottleRatio is the Decide-computed windowed ratio from the overlaid counters")
	})
})
