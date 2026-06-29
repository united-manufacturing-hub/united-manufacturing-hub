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
		Expect(status1.CPU.ThrottleRatio).To(HaveValue(BeNumerically("~", 0.0, 1e-9)),
			"existing ThrottleRatio field preserved on the healthy path (fetchable-0: cgroup readable → non-nil pointer to 0)")
		Expect(status1.CPU.IsThrottled).To(BeFalse(),
			"existing IsThrottled field preserved on the healthy path")

		// Tick 2 — throttle fires (ratio 0.10 > 0.05). Decide returns
		// {degraded, unknown, [{throttling, 0.10}]}; getCPUMetrics must map the
		// verdict onto the new wire fields AND keep the existing fields.
		nrPeriods, nrThrottled, usageUsec = 2000, 100, 2_000_000
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
		Expect(status2.CPU.IsThrottled).To(BeTrue(),
			"existing IsThrottled field preserved and reflects the throttle latch")
		Expect(status2.CPU.ThrottleRatio).To(HaveValue(BeNumerically("~", 0.10, 1e-9)),
			"existing ThrottleRatio field preserved")
		Expect(status2.CPU.CgroupCores).To(BeNumerically("~", 2.0, 1e-9),
			"existing CgroupCores field preserved")

		// Percentile mCPU fields (AvgMCpu/P95MCpu/P99MCpu) mirror the dead-zone
		// usage ring. This throttle-degrade scenario uses cpu.max="200000
		// 100000" (Quota=2.0 cores), which is NOT the dead-zone — Decide clears
		// the usage ring on every tick outside the dead-zone, so the ring is
		// empty and signals.UsageRingActive is false. The three *float64 fields
		// are therefore nil (un-fetchable: no dead-zone → no usage ring → no
		// percentiles to report), and omitempty drops them from the wire. The
		// contract: non-nil (emitted, even 0) only when the ring holds >= 2
		// entries (the dead-zone); nil/omitted otherwise.
		Expect(status2.CPU.AvgMCpu).To(BeZero(),
			"AvgMCpu is nil outside the dead-zone (ring cleared → un-fetchable)")
		Expect(status2.CPU.P95MCpu).To(BeZero(),
			"P95MCpu is nil outside the dead-zone (ring cleared → un-fetchable)")
		Expect(status2.CPU.P99MCpu).To(BeZero(),
			"P99MCpu is nil outside the dead-zone (ring cleared → un-fetchable)")
		Expect(degradedJSON).NotTo(ContainSubstring(`"avgMCpu"`),
			"AvgMCpu is omitempty and absent when nil (un-fetchable)")
		Expect(degradedJSON).NotTo(ContainSubstring(`"p95MCpu"`),
			"P95MCpu is omitempty and absent when nil (un-fetchable)")
		Expect(degradedJSON).NotTo(ContainSubstring(`"p99MCpu"`),
			"P99MCpu is omitempty and absent when nil (un-fetchable)")
	})
})
