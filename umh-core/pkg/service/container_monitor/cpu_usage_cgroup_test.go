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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("CPU usage from cgroup (rung 1b)", func() {
	It("tracks the cgroup cpu.stat usage_usec delta instead of the host gopsutil read", func() {
		mockFS := filesystem.NewMockFileSystem()
		ctx := context.Background()

		testDataPath, err := os.MkdirTemp("", "rung-1b-cpu-test")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(testDataPath) }()

		// cpu.max: "200000 100000" => quota 200000 / period 100000 = 2.0 cores.
		const cpuMax = "200000 100000\n"

		// cpuStatUsec is the usage_usec value the mock returns for every cpu.stat
		// read. It is held stable across all reads within a single GetStatus call
		// (getCgroupCPUInfo may read cpu.stat more than once), then advanced
		// between calls so the cgroup Sampler — wired in by this rung — measures
		// a known delta over wall-clock.
		cpuStatUsec := int64(0)

		mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
			switch path {
			case "/sys/fs/cgroup/cpu.max":
				return []byte(cpuMax), nil
			case "/sys/fs/cgroup/cpu.stat":
				return []byte(fmt.Sprintf(
					"usage_usec %d\nnr_periods 100\nnr_throttled 0\nthrottled_usec 0\n",
					cpuStatUsec,
				)), nil
			default:
				return nil, errors.New("file not found: " + path)
			}
		})

		svc := container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

		// First GetStatus: the cgroup Sampler (when wired in) stores its baseline
		// usage_usec counter; on the first read it reports UsageCores == 0.
		_, err = svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Advance the cgroup counter by 1_000_000 usec (exactly 1.0 core-second)
		// and let ~1 second of wall-clock elapse so the Sampler's
		// delta/elapsed computation yields ~1.0 core of cgroup usage.
		cpuStatUsec = 1_000_000
		time.Sleep(1 * time.Second)

		// Second GetStatus: reported TotalUsageMCpu must track the cgroup delta
		// (~1000 mCPU for 1.0 core), NOT the host CPU percentage from gopsutil's
		// cpu.PercentWithContext (the host numerator this rung removes).
		status, err := svc.GetStatus(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.CPU).NotTo(BeNil())

		// CgroupCores is populated from cpu.max (2.0 cores). This already holds
		// pre-1b (getCgroupCPUInfo parses cpu.max); the assertion pins it as a
		// preserved behavior.
		Expect(status.CPU.CgroupCores).To(Equal(2.0))

		// 1.0 core of cgroup usage => ~1000 mCPU. The [250, 1250] band absorbs
		// wall-clock timing jitter in the Sampler's elapsed-seconds denominator:
		// the lower bound tolerates elapsed up to ~4s on a loaded CI runner
		// (mCPU = 1000/elapsed), while the upper bound is never approached
		// (UsageCores <= 1.0 by construction, so TotalUsageMCpu <= ~1000).
		// With the gopsutil host numerator still in place (the pre-1b state),
		// the reported value tracks host load instead — on an idle test host
		// it is near 0, well outside this band.
		Expect(status.CPU.TotalUsageMCpu).To(SatisfyAll(
			BeNumerically(">=", 250.0),
			BeNumerically("<=", 1250.0),
		), "TotalUsageMCpu should track the cgroup usage_usec delta (~1000 for 1 core), not the host")
	})
})
