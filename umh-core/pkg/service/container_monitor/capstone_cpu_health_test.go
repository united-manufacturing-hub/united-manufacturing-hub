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

// Rung 16 — the capstone. A PROOF (not RED->GREEN) integration test that drives
// container_monitor.GetStatus with a mock filesystem through the full CPU-health
// model: GetStatus -> sampler.Sample -> cpuhealth.Decide -> ComposeMessage ->
// models.CPU. It pins the spec's end-to-end scenarios so a wiring break between
// the rungs surfaces as a failing assertion here. Each scenario asserts the wire
// fields (State, Attribution, Causes, the existing Health/IsThrottled/
// ThrottleRatio) are consistent with the model.
//
// Each scenario is its own It block (with its own service + mock fs, since the
// WindowState is per-service and holds latch/ring state) so a regression in one
// reports independently of the others. The mock fs injects cgroup v2 + /proc
// files; the sampler reads them through the same filesystem.Service seam as
// production. usage_usec and /proc/stat counters advance between GetStatus calls
// so the sampler's delta-based signals (UsageCores, StealFraction,
// HostBusyCores) compute a real delta over wall-clock. A 1s Sleep between calls
// supplies the elapsed-seconds denominator the sampler divides by (the pattern
// from cpu_usage_cgroup_test.go); the assertions tolerate the resulting jitter
// via band/ContainElement matchers rather than exact values.
var _ = Describe("capstone: end-to-end CPU-health model through GetStatus (rung 16)", func() {
	// --- (1) BUSY-NOT-SICK: a capped container at 95% usage with NO throttle
	// and NO pressure -> State=healthy (busy is not sick), CPUHealth=Active,
	// no causes, bridges would start. ---
	It("(1) BUSY-NOT-SICK: capped container at ~95%% usage with no throttle/pressure is healthy", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()
		testDataPath, err := os.MkdirTemp("", "rung16-busy-not-sick")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n" // quota 2.0 cores (capped)
		// usage_usec advances ~1.9 core-sec/s => ~0.95 fraction of 2.0 quota
		// (high usage, but no throttle/pressure -> healthy).
		var usageUsec int64

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
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

		// Tick 1 — baseline the sampler's usage_usec counter.
		usageUsec = 0
		_, err = svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Advance usage_usec by ~1.9 core-seconds over ~1s wall-clock =>
		// ~0.95 of the 2.0-core quota (busy, not sick).
		usageUsec = 1_900_000
		time.Sleep(1 * time.Second)

		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Precondition: the container IS busy — total usage is in a high-usage
		// band (>= 0.5 core, i.e. >= 25%% of the 2.0-core quota). A band (not a
		// hard >= 1.0 core) tolerates the elapsed-seconds jitter the 1s Sleep
		// introduces on a loaded CI runner (TotalUsageMCpu = usage_usec_delta/
		// 1e6/elapsed*1000 shrinks as elapsed grows), mirroring the [250,1250]
		// band in cpu_usage_cgroup_test.go. Without this floor the healthy
		// verdict would be indistinguishable from an idle container, hollowing
		// the "busy is not sick" claim.
		Expect(status.CPU.TotalUsageMCpu).To(BeNumerically(">=", 500),
			"BUSY-NOT-SICK: precondition — usage must be in a high-usage band (>= 500 mCPU) so the healthy verdict actually proves high usage alone does not degrade")

		Expect(status.CPU.State).To(Equal("healthy"),
			"BUSY-NOT-SICK: a capped container at ~95%% usage with no throttle/pressure must be healthy (busy is not sick)")
		// CPUHealth (driven solely by the verdict) proves "busy is not sick".
		// OverallHealth is intentionally NOT asserted here: GetStatus also runs
		// the real gopsutil memory/disk paths (getMemoryMetrics/getDiskMetrics),
		// which are NOT mocked through filesystem.Service and set
		// OverallHealth=Degraded past MemoryHighThresholdPercent=80 /
		// DiskHighThresholdPercent=85. On a loaded CI runner or Docker Desktop
		// (host memory/disk > those cutoffs) an OverallHealth==Active assert
		// would flake for a reason unrelated to the CPU model under test.
		Expect(status.CPUHealth).To(Equal(models.Active),
			"BUSY-NOT-SICK: CPUHealth must be Active when the verdict is healthy")
		Expect(status.CPU.Causes).To(BeEmpty(),
			"BUSY-NOT-SICK: no causes when healthy")
		Expect(status.CPU.VerdictBasis.Throttle.Fired).To(BeFalse(),
			"BUSY-NOT-SICK: nr_throttled=0 -> throttle latch false")
	})

	// --- (2) THROTTLE-DEGRADE: the same capped container but with high
	// nr_throttled (ratio > 0.05) -> State=degraded, Attribution=unknown,
	// Causes contains {kind:'throttling'}, CPUHealth=Degraded, Health.Message
	// contains 'CPU limited'. ---
	It("(2) THROTTLE-DEGRADE: throttle ratio > 0.05 degrades with attribution unknown and cause throttling", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()
		testDataPath, err := os.MkdirTemp("", "rung16-throttle")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n" // quota 2.0 cores (capped)
		var nrPeriods, nrThrottled int64
		var usageUsec int64

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods %d\nnr_throttled %d\nthrottled_usec 0\n",
					usageUsec, nrPeriods, nrThrottled,
				)), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1 — baseline the throttle ring (single point, ratio 0).
		nrPeriods, nrThrottled, usageUsec = 1000, 0, 1_000_000
		_, err = svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Tick 2 — nr_throttled jumps so the 60s windowed ratio is 0.10
		// (100 throttled / 1000 periods) > ThrottleHigh 0.05 -> latch fires.
		nrPeriods, nrThrottled, usageUsec = 2000, 100, 2_000_000
		time.Sleep(1 * time.Second)
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status.CPU.State).To(Equal("degraded"),
			"THROTTLE-DEGRADE: throttle ratio > 0.05 must degrade")
		Expect(status.CPU.Attribution).To(BeEquivalentTo("unknown"),
			"THROTTLE-DEGRADE: throttle is internal -> attribution unknown")
		Expect(status.CPU.Causes).To(ContainElement(
			HaveField("Kind", BeEquivalentTo("throttling")),
		), "THROTTLE-DEGRADE: Causes must contain {kind:'throttling'}")
		Expect(status.CPUHealth).To(Equal(models.Degraded),
			"THROTTLE-DEGRADE: CPUHealth must be Degraded")
		Expect(status.OverallHealth).To(Equal(models.Degraded),
			"THROTTLE-DEGRADE: OverallHealth must co-set Degraded")
		Expect(status.CPU.VerdictBasis.Throttle.Fired).To(BeTrue(),
			"THROTTLE-DEGRADE: throttle latch must be true when the latch fires")
		Expect(status.CPU.VerdictBasis.Throttle.Value).To(BeNumerically("~", 0.10, 1e-9),
			"THROTTLE-DEGRADE: basis.throttle.value is the windowed ratio")
		Expect(status.CPU.Health).NotTo(BeNil())
		Expect(status.CPU.Health.Message).To(ContainSubstring("CPU limited"),
			"THROTTLE-DEGRADE: Health.Message must contain the 'CPU limited' headline")
	})

	// --- (3) PRESSURE-DEGRADE: a container with cpu.pressure avg60=25.0
	// (0.25 fraction > PressureHigh 0.20) -> State=degraded,
	// Attribution=unknown, Causes contains {kind:'pressure'},
	// Health.Message contains 'CPU contention'. ---
	It("(3) PRESSURE-DEGRADE: cpu.pressure avg60=0.25 > 0.20 degrades with cause pressure", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()
		testDataPath, err := os.MkdirTemp("", "rung16-pressure")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n" // capped (not dead-zone)
		// cpu.pressure some avg60=25.00 -> 0.25 fraction > PressureHigh 0.20.
		const cpuPressure = "some avg10=10.00 avg60=25.00 avg300=15.00\ncpu avg10=5.00 avg60=10.00 avg300=8.00\n"
		var nrPeriods, usageUsec int64

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
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Pressure is thresholded directly (kernel 60s-avg, no ring floor) so
		// it fires on tick 1.
		nrPeriods, usageUsec = 1000, 1_000_000
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status.CPU.State).To(Equal("degraded"),
			"PRESSURE-DEGRADE: some avg60=0.25 > PressureHigh 0.20 must degrade")
		Expect(status.CPU.Attribution).To(BeEquivalentTo("unknown"),
			"PRESSURE-DEGRADE: pressure is internal -> attribution unknown")
		Expect(status.CPU.Causes).To(ContainElement(
			HaveField("Kind", BeEquivalentTo("pressure")),
		), "PRESSURE-DEGRADE: Causes must contain {kind:'pressure'}")
		Expect(status.CPUHealth).To(Equal(models.Degraded),
			"PRESSURE-DEGRADE: CPUHealth must be Degraded")
		Expect(status.CPU.VerdictBasis.Throttle.Fired).To(BeFalse(),
			"PRESSURE-DEGRADE: nr_throttled=0 -> throttle latch false (degrade is from pressure, not throttle)")
		Expect(status.CPU.Health).NotTo(BeNil())
		Expect(status.CPU.Health.Message).To(ContainSubstring("CPU contention"),
			"PRESSURE-DEGRADE: Health.Message must contain the 'CPU contention' headline")
	})

	// --- (4) HEALTHY-DEAD-ZONE: a bare-metal container (no cpu.max quota,
	// no cpu.pressure, no /proc/cpuinfo hypervisor flag) at 40% usage ->
	// State=healthy (the guardrail: blind-but-quiet = healthy, never a distinct
	// unknown state). The dead-zone is the blind state where no starvation
	// signal exists; the verdict is binary healthy|degraded and blind-but-quiet
	// is healthy by design. ---
	It("(4) HEALTHY-DEAD-ZONE: blind-but-quiet (40%% usage, no starvation signal) is healthy", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()
		testDataPath, err := os.MkdirTemp("", "rung16-dead-zone-healthy")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		// cpu.max = "max 100000" => uncapped (sampler sets Quota=&0.0). No
		// cpu.pressure, no /proc/cpuinfo, no /proc/stat -> PsiAvailable=false,
		// Virtualized=false => the dead-zone. CgroupCores is not set by the
		// sampler, so the saturation fraction is 0 (the proxy is inert here ->
		// blind-but-quiet is healthy).
		const cpuMax = "max 100000\n"
		var usageUsec int64

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
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

		usageUsec = 0
		_, err = svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// ~0.4 core of usage over ~1s on an uncapped bare-metal box (40% of
		// 1 core) — well below the saturation backstop's 70% mark.
		usageUsec = 400_000
		time.Sleep(1 * time.Second)
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status.CPU.State).To(Equal("healthy"),
			"HEALTHY-DEAD-ZONE: blind-but-quiet (40%% usage, no starvation signal) must be healthy — never a distinct unknown state")
		Expect(status.CPU.Causes).To(BeEmpty(),
			"HEALTHY-DEAD-ZONE: no causes when healthy")
		Expect(status.CPUHealth).To(Equal(models.Active),
			"HEALTHY-DEAD-ZONE: CPUHealth must be Active (the guardrail)")
	})

	// NOTE: scenario (4) above asserts blind-but-quiet is healthy. The
	// dead-zone SATURATION-DEGRADE backstop is asserted in scenario (5) below,
	// which sets the CgroupCores dead-zone override and the 0-fraction
	// usage-ring skip.

	// --- (6) HOST-CONTENTION: a VM (hypervisor flag) with a busy host
	// (high /proc/stat) + pressure firing -> State=degraded,
	// Attribution=host, Causes contains {kind:'host-contention'}. This is
	// the host-contention scenario: the demand gate (pressure) is open AND the host
	// is busy (HostBusyCores/LogicalCpus > HostBusyHigh 0.70). ---
	It("(6) HOST-CONTENTION: VM with busy host + pressure firing degrades with attribution host", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()
		testDataPath, err := os.MkdirTemp("", "rung16-host-contention")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n" // capped (so the sample is not dead-zone)
		// cpu.pressure some avg60=25.00 -> 0.25 > PressureHigh 0.20 (the
		// demand gate that lets host-contention fire).
		const cpuPressure = "some avg10=10.00 avg60=25.00 avg300=15.00\ncpu avg10=5.00 avg60=10.00 avg300=8.00\n"
		// /proc/cpuinfo with "hypervisor" flag -> Virtualized=true (a VM).
		const procCpuinfo = "processor\t: 0\nflags\t: fpu vme de pse tsc msr hypervisor lm\n"

		// /proc/stat first "cpu " line. Fields: user nice system idle iowait
		// irq softirq steal guest guest_nice. Tick 0 baselines; tick 1 makes
		// the host very busy (high non-idle delta) so HostBusyCores is high.
		// LogicalCpus = runtime.NumCPU() (sampler.go, no override seam), so
		// hostBusyRatio = HostBusyCores/LogicalCpus = busy_delta/(100*elapsed*
		// NumCPU) must exceed HostBusyHigh 0.70. To keep this > 0.70 (with an
		// overwhelming margin) on ANY host — including 256+-core CI runners,
		// where the prior small delta failed at >=171 cores (120/171 = 0.70) —
		// we make the busy delta far larger than 100*elapsed*NumCPU can ever
		// consume: busy_delta = 5,000,000 => at elapsed=4s on 256 cores,
		// hostBusyRatio ~= 48, nowhere near the 0.70 floor. This removes the
		// NumCPU coupling without needing a sampler-side test seam.
		procStatTick0 := "cpu  1000 1000 1000 8000 0 0 0 50 0 0\n"
		// Tick 1: busy delta (user+nice+system+iowait+irq+softirq, EXCL
		// steal/guest/guest_nice) = 5,000,000 jiffies; idle grew 1,000,000;
		// steal unchanged (delta 0) -> StealFraction = 0 (steal negligible,
		// does not fire). total delta = 6,000,000 -> busy/total = 0.833.
		procStatTick1 := "cpu  5001000 1000 1000 1008000 0 0 0 50 0 0\n"

		var usageUsec int64
		procStat := procStatTick0

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.pressure":
				return []byte(cpuPressure), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods 1000\nnr_throttled 0\nthrottled_usec 0\n",
					usageUsec,
				)), nil
			case "/proc/cpuinfo":
				return []byte(procCpuinfo), nil
			case "/proc/stat":
				return []byte(procStat), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1 — baselines the sampler's usage_usec + /proc/stat (no deltas
		// yet). Pressure fires immediately (direct threshold) -> the demand
		// gate opens, but HostBusyCores is 0 (first /proc/stat read) so
		// host-contention cannot fire yet.
		usageUsec, procStat = 1_000_000, procStatTick0
		_, err = svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Tick 2 — /proc/stat advances so HostBusyCores is computed (busy
		// host). The demand gate (pressure) is open, host_busy_ratio >
		// HostBusyHigh -> host-contention fires. Steal is negligible (small
		// delta) so it does not dominate.
		usageUsec, procStat = 2_000_000, procStatTick1
		time.Sleep(1 * time.Second)
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// R5 fold: host-contention is folded into saturation + the
		// host/container attribution split. This scenario (busy host + pressure
		// firing, our load light) degrades on the pressure cause, with
		// Attribution=host via the split (host share > our share). No
		// host-contention cause is emitted.
		Expect(status.CPU.State).To(Equal("degraded"),
			"HOST-CONTENTION: busy host + pressure firing must degrade")
		Expect(status.CPU.Attribution).To(BeEquivalentTo("host"),
			"HOST-CONTENTION: host share > our share -> attribution host via the split")
		Expect(status.CPU.Causes).To(ContainElement(
			HaveField("Kind", BeEquivalentTo("pressure")),
		), "HOST-CONTENTION: Causes must contain {kind:'pressure'}")
		Expect(status.CPU.Causes).NotTo(ContainElement(
			HaveField("Kind", BeEquivalentTo("host-contention")),
		), "HOST-CONTENTION: host-contention is folded (R5) — must not appear")
		Expect(status.CPUHealth).To(Equal(models.Degraded),
			"HOST-CONTENTION: CPUHealth must be Degraded")
	})

	// --- (7) STEAL-DEGRADE: a VM (hypervisor flag) with a high steal fraction
	// in /proc/stat (StealFraction > StealHigh 0.10 over the 2-sample p95
	// floor) and NO pressure/throttle -> State=degraded, Attribution=host
	// (steal is external), Causes contains {kind:'steal'}. Steal is already-
	// built behavior (its own Schmitt latch + 60s ring at decide.go), so this
	// scenario forces ZERO production changes — it is exactly the kind of
	// already-built behavior a proof capstone should pin. Unlike saturation
	// (scenario 5, deferred because it requires NEW production behavior), steal
	// has no minimality reason to omit. ---
	It("(7) STEAL-DEGRADE: VM with steal fraction > 0.10 degrades with attribution host and cause steal", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()
		testDataPath, err := os.MkdirTemp("", "rung16-steal")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		const cpuMax = "200000 100000\n" // capped (so the sample is not dead-zone)
		// No cpu.pressure -> PsiAvailable=false -> pressure does not fire (and
		// the host-contention demand gate stays closed, so host-contention
		// cannot fire — only steal fires here, making Attribution=host
		// unambiguous rather than a severity tie-break).
		// /proc/cpuinfo with "hypervisor" flag -> Virtualized=true (a VM), the
		// gate for the steal ring/latch.
		const procCpuinfo = "processor\t: 0\nflags\t: fpu vme de pse tsc msr hypervisor lm\n"

		// /proc/stat first "cpu " line. Fields: user nice system idle iowait
		// irq softirq steal guest guest_nice. Tick 0 baselines (StealFraction
		// = 0 on the first read; the steal ring gets one sample). Tick 1 makes
		// the steal delta a large fraction of the total delta so StealFraction
		// > StealHigh 0.10. HostBusyCores is 0 (busy unchanged) — irrelevant
		// here since the demand gate is closed, but it also keeps
		// host-contention off.
		procStatTick0 := "cpu  1000 1000 1000 8000 0 0 0 50 0 0\n"
		// Tick 1: busy (user+nice+system+iowait+irq+softirq) unchanged =>
		// busy delta 0 (HostBusyCores 0). idle grew 1000; steal grew 2000 =>
		// total delta 3000; StealFraction = 2000/3000 = 0.667 > StealHigh 0.10.
		procStatTick1 := "cpu  1000 1000 1000 9000 0 0 0 2050 0 0\n"

		var usageUsec int64
		procStat := procStatTick0

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods 1000\nnr_throttled 0\nthrottled_usec 0\n",
					usageUsec,
				)), nil
			case "/proc/cpuinfo":
				return []byte(procCpuinfo), nil
			case "/proc/stat":
				return []byte(procStat), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1 — baselines the sampler's usage_usec + /proc/stat (no deltas
		// yet). Virtualized=true so the steal ring takes its first sample
		// (steal=0); the 2-sample floor means the latch is NOT evaluated yet.
		usageUsec, procStat = 1_000_000, procStatTick0
		_, err = svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Tick 2 — /proc/stat advances so StealFraction = 0.667. The steal
		// ring now holds 2 samples [0, 0.667]; nearest-rank p95 = 0.667 >
		// StealHigh 0.10 -> the steal latch fires. Steal is external, and it
		// is the only fired cause (no pressure/throttle/host-contention), so
		// Attribution=host unambiguously.
		usageUsec, procStat = 2_000_000, procStatTick1
		time.Sleep(1 * time.Second)
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status.CPU.State).To(Equal("degraded"),
			"STEAL-DEGRADE: steal fraction > 0.10 (p95 over the 2-sample floor) must degrade")
		Expect(status.CPU.Attribution).To(BeEquivalentTo("host"),
			"STEAL-DEGRADE: steal is external (hypervisor stole vCPU time) -> attribution host")
		Expect(status.CPU.Causes).To(ContainElement(
			HaveField("Kind", BeEquivalentTo("steal")),
		), "STEAL-DEGRADE: Causes must contain {kind:'steal'}")
		Expect(status.CPUHealth).To(Equal(models.Degraded),
			"STEAL-DEGRADE: CPUHealth must be Degraded")
		Expect(status.CPU.VerdictBasis.Throttle.Fired).To(BeFalse(),
			"STEAL-DEGRADE: nr_throttled=0 -> throttle latch false (degrade is from steal, not throttle)")
	})

	// --- (5) SATURATION-DEGRADE-DEAD-ZONE: the dead-zone saturation backstop
	// fires on a full dead-zone box — bare-metal (no hypervisor flag), uncapped
	// (cpu.max = "max"), no PSI (no cpu.pressure), no throttle — with the host
	// at capacity (HostBusyCores fills every core leaving less than one core of
	// reserve). Two invariants the assertions below cannot convey by
	// themselves are pinned by the setup:
	//
	// 1. NumCPU-decoupling. LogicalCpus = runtime.NumCPU() (sampler.go, no
	//    override seam), so the headroom trigger fires when hostBusyMean (the
	//    60s mean of per-tick HostBusyCores) > NumCPU - 1.0. To exceed NumCPU -
	//    1 on ANY host — including 256+-core CI runners — the busy delta is
	//    made far larger than 100*elapsed*NumCPU can ever consume (the same
	//    overwhelming-margin pattern as scenario 6), so the test does not
	//    depend on the host it runs on.
	//
	// 2. Demand-gate-closed => Attribution unambiguous. nr_throttled=0 and no
	//    PSI => the host-contention demand gate (pressure OR throttle) stays
	//    CLOSED, so host-contention cannot co-fire. Saturation is the SOLE
	//    fired cause, so Attribution=host comes from the host/container split
	//    (the host share of HostBusyCores exceeds the UMH share), not a
	//    severity tie-break against host-contention.
	It("(5) SATURATION-DEGRADE-DEAD-ZONE: full dead-zone box (uncapped, no PSI, host at capacity) degrades with attribution host, cause saturation, and a non-nil hostBusyCores on the wire", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()
		testDataPath, err := os.MkdirTemp("", "saturation-dead-zone")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		// cpu.max="max 100000" yields Quota=&0.0; with no cpu.pressure,
		// PsiAvailable=false, so the sample is full dead-zone (no CPU limit, no
		// PSI).
		const cpuMax = "max 100000\n"
		// No /proc/cpuinfo hypervisor flag and no DMI product_name (the mock's
		// default "file not found" for both) => Virtualized=false => steal is
		// not a readable signal and is not processed, so steal cannot co-fire
		// and muddle Attribution. Bare metal is valid here because
		// HostBusyCores is readable independent of virtualization.
		var nrPeriods, usageUsec int64

		// /proc/stat first "cpu " line. Fields: user nice system idle iowait
		// irq softirq steal guest guest_nice. Tick 0 baselines; tick 1 makes
		// the host very busy (huge non-idle delta) so HostBusyCores fills every
		// core. The busy delta is 5,000,000 jiffies (see invariant 1 above for
		// the NumCPU-decoupling margin); the test sleeps ~1s between ticks, so
		// HostBusyCores ~= 5,000,000 / 100 / 1 ~= 50,000 and hostBusyMean (the
		// mean of [0, ~50000]) ~= 25,000 — far above the NumCPU-1 fire floor on
		// any plausible host, so the saturation backstop fires.
		procStatTick0 := "cpu  1000 1000 1000 8000 0 0 0 50 0 0\n"
		// Tick 1: busy delta (user+nice+system+iowait+irq+softirq, EXCL
		// steal/guest/guest_nice) = 5,000,000 jiffies; idle grew 1,000; steal
		// unchanged (delta 0 -> StealFraction = 0, irrelevant on bare metal).
		procStatTick1 := "cpu  5001000 1000 1000 9000 0 0 0 50 0 0\n"

		procStat := procStatTick0

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods %d\nnr_throttled 0\nthrottled_usec 0\n",
					usageUsec, nrPeriods,
				)), nil
			case "/proc/stat":
				return []byte(procStat), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Tick 1 — baselines the sampler's usage_usec + /proc/stat (no deltas
		// yet). HostBusyCores is 0 (first /proc/stat read baselines), so
		// hostBusyMean is 0 (the 2-sample floor keeps the ring at 1 entry) and
		// HeadroomCores = NumCPU - 0 - 1 > 0 => saturation does NOT fire yet.
		// Pressure/throttle are absent => the verdict is healthy here.
		usageUsec, nrPeriods, procStat = 1_000_000, 1000, procStatTick0
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		// Pin the no-false-fire invariant the comment above relies on: on the
		// single-entry baseline tick the saturation backstop must NOT fire
		// (hostBusyMean=0 => HeadroomCores>0; usage ring below its 2-sample
		// floor). A regression that evaluated the rings on a single entry would
		// false-degrade every dead-zone container on its first status tick
		// (cold start) and still leave the tick-2 assertions green.
		Expect(status1.CPU.State).To(Equal("healthy"),
			"SATURATION-DEGRADE-DEAD-ZONE: tick-1 baseline (single ring entry) must be healthy — the saturation backstop must not false-fire before the 2-sample floor")
		Expect(status1.CPU.Causes).To(BeEmpty(),
			"SATURATION-DEGRADE-DEAD-ZONE: tick-1 baseline must carry no degrade causes")

		// Tick 2 puts the host at capacity, so saturation fires as the sole
		// cause; with the demand gate closed (no pressure/throttle),
		// attribution=host.
		usageUsec, nrPeriods, procStat = 2_000_000, 2000, procStatTick1
		time.Sleep(1 * time.Second)
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(status.CPU.State).To(Equal("degraded"),
			"SATURATION-DEGRADE-DEAD-ZONE: a full dead-zone box (host at capacity, less than one core of reserve) must degrade via the saturation backstop")
		Expect(status.CPU.Causes).To(ContainElement(
			HaveField("Kind", BeEquivalentTo("saturation")),
		), "SATURATION-DEGRADE-DEAD-ZONE: Causes must contain {kind:'saturation'} (the dead-zone backstop)")
		Expect(status.CPU.Causes).ToNot(ContainElement(
			HaveField("Kind", BeEquivalentTo("host-contention")),
		), "SATURATION-DEGRADE-DEAD-ZONE: Causes must NOT contain {kind:'host-contention'} — the demand gate (pressure OR throttle) is closed in the dead-zone")
		Expect(status.CPU.Attribution).To(BeEquivalentTo("host"),
			"SATURATION-DEGRADE-DEAD-ZONE: attribution is host via the host/container split — the host (non-UMH) share of HostBusyCores exceeds the UMH share")
		// Limited-visibility wire equivalent: the dead-zone (no CPU limit, no
		// PSI) surfaces on the wire as the absence of the fetchable cgroup/PSI
		// signals — CgroupCores=0 (uncapped, omitted via omitempty) and
		// verdictBasis.pressure.applies=false (PSI absent). The verdict basis is
		// present whenever Decide ran; on a PSI-absent box pressure.applies=false
		// and pressure.value=0, which is the wire signature of the no-PSI half of
		// the dead-zone. This is the same no-limit/no-pressure state the
		// limitedVisibilityNote names; in the degraded case ComposeMessage does
		// not append the note, so the dead-zone is read off these wire fields
		// instead.
		Expect(status.CPU.CgroupCores).To(BeZero(),
			"SATURATION-DEGRADE-DEAD-ZONE: CgroupCores is 0 (uncapped, cpu.max='max') — the wire signature of the no-CPU-limit half of the dead-zone")
		Expect(status.CPU.VerdictBasis).ToNot(BeNil(),
			"SATURATION-DEGRADE-DEAD-ZONE: verdictBasis is present (Decide ran on a capped-cpu dead-zone box)")
		Expect(status.CPU.VerdictBasis.Pressure.Applies).To(BeFalse(),
			"SATURATION-DEGRADE-DEAD-ZONE: verdictBasis.pressure.applies is false (PSI absent) — the wire signature of the no-PSI half of the dead-zone")
		// verdictBasis.hostBusy.mean carries the 60s mean of host-busy
		// cores (the host observation) — non-nil because Decide ran,
		// and > 0 because the degraded tick computed a large host-busy delta.
		Expect(status.CPU.VerdictBasis.HostBusy.Mean).To(BeNumerically(">", 0),
			"SATURATION-DEGRADE-DEAD-ZONE: verdictBasis.hostBusy.mean must carry the computed host-busy value (> 0) on the degraded tick")
		Expect(status.CPUHealth).To(Equal(models.Degraded),
			"SATURATION-DEGRADE-DEAD-ZONE: CPUHealth must be Degraded (consistent with scenarios 2/3/6/7)")
	})
})
