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

var _ = Describe("three-field CPU-health wire contract on models.CPU", func() {
	It("maps the Decide verdict onto State/Attribution/Causes on a throttle degrade, preserves the existing fields, and emits the always-on/omitempty wire shape", func() {
		// Verifies the JSON wire contract on models.CPU: "state" is always
		// emitted (no omitempty), even when healthy; "attribution" and "causes"
		// are omitempty and present only when State == "degraded". Drives a
		// throttle degrade through the real mock-fs path and pins both the new
		// contract fields and that the existing fields stay populated.
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "cpu-wire-contract")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		// cpu.max: "200000 100000" => quota 2.0 cores (capped, so the sample is
		// not in the dead-zone; the only degrade cause available here is throttle).
		const cpuMax = "200000 100000\n"

		// Throttle counters advance monotonically; nr_throttled jumps so the 60s
		// windowed ratio is 0.10 (100 throttled / 1000 periods) > ThrottleHigh
		// 0.05, so the throttle Schmitt latch fires and Decide returns
		// {degraded, unknown, [{throttling, 0.10}]}.
		var nrPeriods, nrThrottled int64
		var usageUsec int64 = 1_000_000

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

		// Tick 1 — baseline the throttle ring (single point, ratio 0, latch
		// false). Verdict is healthy: State must be "healthy" and always emitted
		// (no omitempty) even though Attribution/Causes are absent.
		nrPeriods, nrThrottled, usageUsec = 1000, 0, 1_000_000
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPU.State).To(Equal("healthy"),
			"State is always emitted (no omitempty), even when healthy")

		// Wire contract: "state" is present, "attribution"/"causes" absent on
		// the healthy path (omitempty).
		healthyJSON, err := json.Marshal(status1.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(healthyJSON).To(ContainSubstring(`"state":"healthy"`),
			"the JSON wire must always carry state (no omitempty)")
		Expect(healthyJSON).NotTo(ContainSubstring(`"attribution"`),
			"attribution is omitempty and must be absent when healthy")
		Expect(healthyJSON).NotTo(ContainSubstring(`"causes"`),
			"causes is omitempty and must be absent when healthy")

		Expect(status1.CPU.Health).NotTo(BeNil(),
			"the existing Health field must remain populated")
		Expect(status1.CPU.VerdictBasis).NotTo(BeNil(),
			"verdictBasis is emitted on the healthy path (Decide ran: cgroup readable)")
		Expect(status1.CPU.VerdictBasis.Throttle.Value).To(BeNumerically("~", 0.0, 1e-9),
			"basis.throttle.value is 0 on the healthy path (no throttled periods)")
		Expect(status1.CPU.VerdictBasis.Throttle.Fired).To(BeFalse(),
			"basis.throttle.fired is false on the healthy path (latch not fired)")

		// Tick 2 — throttle fires (ratio 0.10 > 0.05). Decide returns
		// {degraded, unknown, [{throttling, 0.10}]}; getCPUMetrics must map the
		// verdict onto the new wire fields AND keep the existing fields.
		// usage_usec is pinned (no delta → UsageCores 0) so the limit-mode
		// headroom stays positive (2 − 0 − 0.2 = 1.8) and saturation does NOT
		// co-fire — isolating the throttle degrade from the two-rule headroom.
		nrPeriods, nrThrottled, usageUsec = 2000, 100, 1_000_000
		status2, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// New contract fields:
		Expect(status2.CPU.State).To(Equal("degraded"),
			"State maps verdict.State (always emitted, no omitempty)")
		Expect(status2.CPU.Attribution).To(BeEquivalentTo("unknown"),
			"Attribution maps verdict.Attribution, set only when degraded")
		Expect(status2.CPU.Causes).To(ContainElement(SatisfyAll(
			HaveField("Kind", BeEquivalentTo("throttling")),
			HaveField("Value", BeNumerically("~", 0.10, 1e-9)),
		)), "Causes maps verdict.Causes, each {kind, value}; throttle cause carries the windowed ratio")

		// Wire contract: "state" present as "degraded"; "attribution"/"causes"
		// now present (degraded path).
		degradedJSON, err := json.Marshal(status2.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(degradedJSON).To(ContainSubstring(`"state":"degraded"`),
			"the JSON wire must carry state=degraded")
		Expect(degradedJSON).To(ContainSubstring(`"attribution":"unknown"`),
			"attribution must be present on the wire when degraded")
		Expect(degradedJSON).To(ContainSubstring(`"causes"`),
			"causes must be present on the wire when degraded")

		// Existing fields preserved:
		Expect(status2.CPU.Health).NotTo(BeNil(),
			"existing Health field preserved")
		Expect(status2.CPU.Health.Category).To(Equal(models.Degraded),
			"existing Health category reflects the degrade")
		Expect(status2.CPU.VerdictBasis.Throttle.Fired).To(BeTrue(),
			"basis.throttle.fired reflects the throttle latch on the degraded path")
		Expect(status2.CPU.VerdictBasis.Throttle.Value).To(BeNumerically("~", 0.10, 1e-9),
			"basis.throttle.value carries the windowed ratio on the degraded path")
		Expect(status2.CPU.CgroupCores).To(BeNumerically("~", 2.0, 1e-9),
			"existing CgroupCores field preserved")

		// Percentile mCPU fields (AvgMCpu/P95MCpu/P99MCpu) mirror the usage
		// ring. Since R10.1 the ring fills every tick in ALL modes, so the
		// percentiles are fetchable (non-nil) outside the dead-zone too — the
		// container's own usage is available in limit mode. With usage_usec
		// pinned (UsageCores 0), the two-tick ring holds [0, 0] → AvgMCpu=0
		// (non-nil pointer to 0, emitted on the wire as 0). The contract: non-nil
		// (emitted, even 0) whenever the ring holds >= 2 entries; nil/omitted
		// only on the first tick before the ring has 2 entries.
		Expect(status2.CPU.AvgMCpu).ToNot(BeNil(),
			"AvgMCpu is non-nil outside the dead-zone (ring fills every tick under R10.1 → fetchable)")
		Expect(*status2.CPU.AvgMCpu).To(BeNumerically("~", 0.0, 1e-9),
			"AvgMCpu is 0 (usage_usec pinned → UsageCores 0 → 60s mean 0)")
		Expect(status2.CPU.P95MCpu).ToNot(BeNil(),
			"P95MCpu is non-nil outside the dead-zone (ring fills every tick)")
		Expect(*status2.CPU.P95MCpu).To(BeNumerically("~", 0.0, 1e-9))
		Expect(status2.CPU.P99MCpu).ToNot(BeNil(),
			"P99MCpu is non-nil outside the dead-zone (ring fills every tick)")
		Expect(*status2.CPU.P99MCpu).To(BeNumerically("~", 0.0, 1e-9))
		Expect(degradedJSON).To(ContainSubstring(`"avgMCpu"`),
			"AvgMCpu is emitted on the wire (non-nil, fetchable)")
		Expect(degradedJSON).To(ContainSubstring(`"p95MCpu"`),
			"P95MCpu is emitted on the wire (non-nil, fetchable)")
		Expect(degradedJSON).To(ContainSubstring(`"p99MCpu"`),
			"P99MCpu is emitted on the wire (non-nil, fetchable)")
	})
})

// /proc/stat counter-reset guard (sampler.go: totalDelta <= 0 on host reboot
// or /proc/stat wrap). On a reset tick the busy delta would be negative, so the
// reset guard zeroes it; the 60s mean of host-busy cores (now carried on
// verdictBasis.headroom.hostBusyMean) is therefore 0, NOT negative. This pins
// the reset guard via the basis: a malformed/wrapped /proc/stat reading must
// not reach the verdict's saturation input as a poison negative value.
var _ = It("emits a non-negative verdictBasis.headroom.hostBusyMean on a /proc/stat counter-reset tick", func() {
	mockFS := filesystem.NewMockFileSystem()
	ctx := context.Background()

	testDataPath, err := os.MkdirTemp("", "cpu-wire-hostbusy-reset")
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = os.RemoveAll(testDataPath) }()

	const cpuMax = "200000 100000\n"
	usageUsec := int64(1_000_000)

	// procStat is mutated between ticks; tick 1 baselines, tick 2 has a
	// SMALLER total (counter reset / wrap) so the reset guard fires.
	procStatTick0 := "cpu  5001000 1000 1000 9000 0 0 0 50 0 0\n"
	procStatTick1 := "cpu  1000 1000 1000 8000 0 0 0 50 0 0\n"
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
		case "/proc/stat":
			return []byte(procStat), nil
		default:
			return nil, errors.New("file not found: " + path)
		}
	})

	svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

	// Tick 1 — baseline /proc/stat (busy delta 0, first read).
	usageUsec = 1_000_000
	procStat = procStatTick0
	_, err = svc.GetStatus(ctx)
	Expect(err).NotTo(HaveOccurred())

	// Tick 2 — total jiffies DECREASED (reset). The reset guard zeroes the
	// negative busy delta, so the per-tick host-busy is 0 and the 60s mean
	// (the verdict's saturation input, on verdictBasis.headroom.hostBusyMean)
	// is 0, not negative.
	usageUsec = 2_000_000
	procStat = procStatTick1
	status, err := svc.GetStatus(ctx)
	Expect(err).NotTo(HaveOccurred())

	Expect(status.CPU.VerdictBasis).ToNot(BeNil(),
		"verdictBasis is present (Decide ran: cpu.max capped and cgroup readable) on a counter-reset tick")
	Expect(status.CPU.VerdictBasis.Headroom.HostBusyMean).To(BeNumerically("~", 0.0, 1e-9),
		"hostBusyMean is 0 on a reset tick (guard zeroes the negative busy delta → 60s mean is 0)")
	Expect(status.CPU.VerdictBasis.Headroom.HostBusyMean).To(BeNumerically(">=", 0.0),
		"hostBusyMean must be non-negative on a reset tick — the reset guard prevents a poison negative value reaching the verdict's saturation input")
})

// VerdictBasis wire contract. verdictBasis is the machine-readable why behind
// the CPU-health verdict: the decision variables (headroom + the three
// starvation causes) the verdict acted on. It is always emitted when a verdict
// was computed (healthy AND degraded), so the MC renders the headline, the
// host/container split, and the alert-rule budget dashboard from the verdict's
// own inputs rather than parsing the message text or mirroring per-tick samples.
// It is nil (JSON-omitted) only when no verdict exists — a cgroup read failure,
// where Decide is not called.
var _ = Describe("verdictBasis wire contract on models.CPU", func() {
	// cpu.max "200000 100000" => quota 2.0 cores (capped, NOT the dead-zone — a
	// limit is set, so LimitApplies=true; PSI absent and not virtualized, so
	// PsiApplies=StealApplies=false). The only degrade cause reachable here is
	// throttle.
	const cpuMax = "200000 100000\n"

	// Shared mock-fs factory: cpu.max fixed, cpu.stat driven by the counters,
	// /proc/stat intentionally absent (readProcStat fails → HostBusyCores=0,
	// so Headroom.Cores = capacity 2.0 − 0 − reserve 1.0 = 1.0, deterministic).
	newSvc := func(nrPeriods, nrThrottled int64, usageUsec int64) (*container_monitor.ContainerMonitorService, string) {
		mockFS := filesystem.NewMockFileSystem()
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
		testDataPath, err := os.MkdirTemp("", "cpu-wire-verdictbasis")
		Expect(err).NotTo(HaveOccurred())

		return container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath), testDataPath
	}

	It("emits the full verdictBasis on a HEALTHY verdict with all fired flags false", func() {
		ctx := context.Background()
		svc, dir := newSvc(1000, 0, 1_000_000)
		defer func() { _ = os.RemoveAll(dir) }()

		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.CPU.State).To(Equal("healthy"),
			"precondition: the verdict is healthy")

		Expect(status.CPU.VerdictBasis).NotTo(BeNil(),
			"verdictBasis is emitted on a healthy verdict (always when Decide ran)")

		b := status.CPU.VerdictBasis
		// Headroom (limit mode): capacity=quota 2.0, reserve=0.2
		// (LimitReserveFraction 0.10 × quota 2.0), used=0 (single tick → usage
		// ring < 2 → usageCores60sMean 0; /proc/stat absent → hostBusyMean 0),
		// so Cores=2.0 − 0 − 0.2=1.8. Fired=false (single tick, ring < 2 → the
		// limit-mode latch is not evaluated).
		Expect(b.Headroom.Capacity).To(BeNumerically("~", 2.0, 1e-9))
		Expect(b.Headroom.Reserve).To(BeNumerically("~", 0.2, 1e-9))
		Expect(b.Headroom.HostBusyMean).To(BeNumerically("~", 0.0, 1e-9))
		Expect(b.Headroom.Cores).To(BeNumerically("~", 1.8, 1e-9))
		Expect(b.Headroom.Fired).To(BeFalse(),
			"headroom fired is false on a healthy capped box (single tick, ring < 2 → latch not evaluated)")

		// Throttle: fetchable (limit set → Applies=true), value 0 (no throttled
		// periods), threshold 0.05, latch false.
		Expect(b.Throttle.Value).To(BeNumerically("~", 0.0, 1e-9))
		Expect(b.Throttle.Threshold).To(BeNumerically("~", 0.05, 1e-9))
		Expect(b.Throttle.Fired).To(BeFalse())
		Expect(b.Throttle.Applies).To(BeTrue(),
			"throttle applies: a cgroup limit is set")

		// Pressure: PSI absent → Applies=false, latch false, threshold 0.20.
		Expect(b.Pressure.Threshold).To(BeNumerically("~", 0.20, 1e-9))
		Expect(b.Pressure.Fired).To(BeFalse())
		Expect(b.Pressure.Applies).To(BeFalse(),
			"pressure does not apply: PSI is absent")

		// Steal: not virtualized → Applies=false, latch false, threshold 0.10.
		Expect(b.Steal.Threshold).To(BeNumerically("~", 0.10, 1e-9))
		Expect(b.Steal.Fired).To(BeFalse())
		Expect(b.Steal.Applies).To(BeFalse(),
			"steal does not apply: the box is not virtualized")

		// Wire: verdictBasis is JSON-present on the healthy path.
		wireJSON, err := json.Marshal(status.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(wireJSON).To(ContainSubstring(`"verdictBasis"`),
			"verdictBasis must be JSON-present on a healthy verdict")
		Expect(wireJSON).To(ContainSubstring(`"headroom"`))
		Expect(wireJSON).To(ContainSubstring(`"hostBusyMean"`))
	})

	It("emits the verdictBasis on a DEGRADED verdict with the fired cause's flag set", func() {
		ctx := context.Background()

		// Drive a real throttle fire across two ticks on a single shared
		// service (so the 60s throttle ring persists): tick 1 baselines the
		// ring (ratio 0, healthy); tick 2 advances nr_throttled so the windowed
		// ratio is 0.10 > 0.05 → the throttle Schmitt latch fires → degraded.
		status2, err := throttleFireStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status2.CPU.State).To(Equal("degraded"),
			"precondition: the verdict is degraded")

		b := status2.CPU.VerdictBasis
		Expect(b).NotTo(BeNil(), "verdictBasis is emitted on a degraded verdict too")
		Expect(b.Throttle.Fired).To(BeTrue(),
			"the throttle latch fired is reflected in the basis")
		Expect(b.Throttle.Value).To(BeNumerically("~", 0.10, 1e-9),
			"the basis carries the windowed throttle ratio (the verdict's input)")
		Expect(b.Throttle.Threshold).To(BeNumerically("~", 0.05, 1e-9))
		Expect(b.Throttle.Applies).To(BeTrue())
		// Headroom stays populated (numbers always computed); saturation did not
		// co-fire (usage_usec pinned in throttleFireStatus → UsageCores 0 →
		// limit-mode headroom 1.8 > 0, so the throttle degrade is the sole cause).
		Expect(b.Headroom.Capacity).To(BeNumerically("~", 2.0, 1e-9))
		Expect(b.Headroom.Reserve).To(BeNumerically("~", 0.2, 1e-9))
		Expect(b.Headroom.Fired).To(BeFalse())
		// Pressure/steal remain non-firing and non-applicable.
		Expect(b.Pressure.Fired).To(BeFalse())
		Expect(b.Pressure.Applies).To(BeFalse())
		Expect(b.Steal.Fired).To(BeFalse())
		Expect(b.Steal.Applies).To(BeFalse())

		wireJSON, err := json.Marshal(status2.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(wireJSON).To(ContainSubstring(`"verdictBasis"`),
			"verdictBasis must be JSON-present on a degraded verdict")
	})

	It("omits verdictBasis (nil) when no verdict exists (cgroup read failure)", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "cpu-wire-verdictbasis-noverdict")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		// cpu.max read failure → getCgroupCPUInfo returns (nil, err) →
		// cgroupErr != nil → Decide is not called → no verdict → no basis.
		// State defaults to "healthy" (the always-emitted contract), but
		// verdictBasis must be nil/omitted (the omitempty contract).
		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return nil, errors.New("permission denied: cpu.max")
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.CPU.State).To(Equal("healthy"),
			"State defaults to healthy when Decide was not called")

		Expect(status.CPU.VerdictBasis).To(BeNil(),
			"verdictBasis is nil when no verdict exists (cgroup read failure)")
		wireJSON, err := json.Marshal(status.CPU)
		Expect(err).NotTo(HaveOccurred())
		Expect(wireJSON).NotTo(ContainSubstring(`"verdictBasis"`),
			"verdictBasis is omitempty and must be absent when no verdict exists")
	})
})

// throttleFireStatus drives a real throttle fire across two ticks on a single
// shared service (so the 60s throttle ring persists) and returns the degraded
// status. cpu.max="200000 100000" (quota 2.0); tick 1 baselines the ring
// (ratio 0, healthy); tick 2 advances nr_throttled so the windowed ratio is
// 0.10 > 0.05 → the throttle Schmitt latch fires → Decide returns degraded.
func throttleFireStatus(ctx context.Context) (*container_monitor.ServiceInfo, error) {
	mockFS := filesystem.NewMockFileSystem()

	testDataPath, err := os.MkdirTemp("", "cpu-wire-verdictbasis-fire")
	if err != nil {
		return nil, err
	}

	defer func() { _ = os.RemoveAll(testDataPath) }()

	var nrPeriods, nrThrottled int64 = 1000, 0

	var usageUsec int64 = 1_000_000

	mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
		switch path {
		case "/sys/fs/cgroup/cpu.max":
			return []byte("200000 100000\n"), nil
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

	// Tick 1 — baseline.
	if _, err := svc.GetStatus(ctx); err != nil {
		return nil, err
	}

	// Tick 2 — fire: 100 throttled / 1000 new periods = 0.10 > 0.05.
	// usage_usec is pinned (no delta → UsageCores 0) so the limit-mode headroom
	// stays positive (2 − 0 − 0.2 = 1.8) and saturation does NOT co-fire — the
	// throttle degrade is the sole cause (the helper's callers assert
	// Headroom.Fired=false, which only holds when usage is pinned).
	nrPeriods, nrThrottled, usageUsec = 2000, 100, 1_000_000

	return svc.GetStatus(ctx)
}
