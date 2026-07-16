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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("throttle verdict wired through cpuhealth.Decide (rung 4b)", func() {
	It("holds IsThrottled in the Schmitt band via the Decide flip-latch, not the raw ratio", func() {
		// The throttle verdict flows through cpuhealth.Decide's Schmitt flip-latch
		// (fires above ThrottleHigh 0.05, clears only below ThrottleRecover 0.03,
		// holds between). The old raw-ratio path evaluated isThrottled from a
		// ratio > 0.05 test with no hysteresis, so a ratio landing in the
		// [0.03, 0.05) band immediately read IsThrottled=false. Under Decide the
		// latch that fired at the high mark holds true through that band. The
		// test drives GetStatus end-to-end with a mock fs injecting a cpu.stat
		// counter series and asserts the latch holds.
		//
		// The counter series is monotonic on BOTH nr_periods and nr_throttled so
		// Decide's clear-on-either-counter-regression guard does NOT wipe the
		// ring: the hold-band behavior is exercised against a genuine two-point
		// delta, not a ring rebuilt to a single point.
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "rung-4b-throttle-test")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		// cpu.max: "200000 100000" => quota 2.0 cores (capped, so Decide computes
		// a usage fraction; throttle is independent of usage but a capped sample
		// exercises the real wiring path).
		const cpuMax = "200000 100000\n"

		// cpuStat holds the cpu.stat body returned for every read within a single
		// GetStatus call (the sampler reads cpu.stat once per tick),
		// then is advanced between calls. usage_usec is held at a constant 0 so
		// the sampler reports UsageCores == 0 (no saturation signal); only the
		// throttle counters move.
		var nrPeriods, nrThrottled int64

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec 0\nnr_periods %d\nnr_throttled %d\nthrottled_usec 0\n",
					nrPeriods, nrThrottled,
				)), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// Call 1: baseline. The ring holds a single point, so the two-point
		// delta is 0 and the latch is false.
		nrPeriods, nrThrottled = 1000, 10
		status1, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status1.CPU.VerdictBasis.Throttle.Fired).To(BeFalse(), "baseline: no delta yet")

		// Call 2: windowed ratio 0.10 (100 throttled / 1000 periods) fires the
		// latch above the 0.05 high mark. Both the old raw-ratio path and Decide
		// agree here, so this is a pre-condition, not the distinguishing
		// assertion.
		nrPeriods, nrThrottled = 2000, 110
		status2, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status2.CPU.VerdictBasis.Throttle.Fired).To(BeTrue(), "ratio 0.10 > 0.05 fires the latch")

		// Call 3: the windowed ratio drops to 0.035 (140 throttled / 4000
		// periods over the full call-1-to-call-3 span), which is inside the
		// Schmitt band [0.03, 0.05). Decide's flip-latch holds (ratio is neither
		// > 0.05 nor < 0.03), so IsThrottled stays true. Both counters are
		// monotonically increasing (nr_periods 1000→2000→5000, nr_throttled
		// 10→110→150), so the clear-on-either-counter-regression guard does NOT
		// fire and the ring keeps the call-1 anchor, so the hold is exercised
		// against a genuine two-point delta, not a rebuilt single-point ring.
		nrPeriods, nrThrottled = 5000, 150
		status3, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status3.CPU.VerdictBasis.Throttle.Fired).To(BeTrue(),
			"Schmitt latch must hold in the [0.03, 0.05) band after firing; the raw ratio is %.4f", 0.035)
		Expect(status3.CPU.VerdictBasis.Throttle.Value).To(BeNumerically("~", 0.035, 1e-9),
			"basis.throttle.value is the Decide-computed windowed ratio, read unconditionally of latch state")
	})

	// Keep the real 60s window honest: the calls above are milliseconds apart,
	// so no pruning occurs and the oldest point (call 1) anchors the delta. A
	// real wall-clock advance past 60s is not needed to distinguish the latch
	// from the raw ratio; the band-hold is the pinned behavior.
})
