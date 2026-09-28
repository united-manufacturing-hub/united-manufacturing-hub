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

// Which hierarchy the sampler resolved. The same container image runs on a v2
// host and a v1 one, so the layout is read off the filesystem: where cpu.stat
// sits is the discriminant. It is asked once, unless nothing matched.
package cpuhealth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
)

var _ = Describe("the resolved cgroup layout", func() {
	It("reads the v2 files when cpu.stat sits at the base", func() {
		smp, err := v1Sampler(v1Files{
			cgroupBase + "/cpu.stat": "usage_usec 5000000\nnr_periods 10\nnr_throttled 1\n",
			cgroupBase + "/cpu.max":  "200000 100000\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		usage, ok := smp.UsageUsec.Get()
		Expect(ok).To(BeTrue(), "a v2 mount's usage comes from cpu.stat itself")
		Expect(usage).To(Equal(5_000_000.0))
		quota, ok := smp.Quota.Get()
		Expect(ok).To(BeTrue(), "a v2 mount's limit comes from cpu.max")
		Expect(quota).To(Equal(2.0))
	})

	It("reads the split controller directories a runtime mounts separately", func() {
		// systemd mounts the two controllers together as cpu,cpuacct; a
		// container runtime may bind-mount cpu and cpuacct at their own paths,
		// so each is probed for on its own.
		smp, err := v1Sampler(v1Files{
			cgroupBase + "/cpu/cpu.stat":          "nr_periods 10\nnr_throttled 2\n",
			cgroupBase + "/cpu/cpu.cfs_quota_us":  "400000\n",
			cgroupBase + "/cpu/cpu.cfs_period_us": "100000\n",
			cgroupBase + "/cpuacct/cpuacct.usage": "7000000000\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		quota, ok := smp.Quota.Get()
		Expect(ok).To(BeTrue())
		Expect(quota).To(Equal(4.0))
		usage, ok := smp.UsageUsec.Get()
		Expect(ok).To(BeTrue(), "cpuacct is probed for separately from cpu")
		Expect(usage).To(Equal(7_000_000.0))
		throttled, ok := smp.NrThrottled.Get()
		Expect(ok).To(BeTrue())
		Expect(throttled).To(Equal(2.0))
	})

	It("keeps an identified layout, so later ticks probe nothing", func() {
		probes := 0
		fs := serveFiles(v1Files{cgroupBase + "/cpu,cpuacct/cpu.stat": "nr_periods 10\nnr_throttled 0\n"})
		served := fs.FileExistsFunc
		fs.FileExistsFunc = func(ctx context.Context, path string) (bool, error) {
			probes++

			return served(ctx, path)
		}
		sampler := cpuhealth.NewLinuxSampler(fs, cgroupBase)

		_, err := sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		afterFirst := probes

		_, err = sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(afterFirst).To(BeNumerically(">", 0))
		Expect(probes).To(Equal(afterFirst))
	})

	It("probes again next tick while no layout answers", func() {
		probes := 0
		fs := serveFiles(v1Files{})
		fs.FileExistsFunc = func(_ context.Context, _ string) (bool, error) {
			probes++

			return false, nil
		}
		sampler := cpuhealth.NewLinuxSampler(fs, cgroupBase)

		_, err := sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		afterFirst := probes

		_, err = sampler.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(probes).To(BeNumerically(">", afterFirst))
	})

	It("probes once per tick while the layout is unresolved", func() {
		probesPerTick := func(files v1Files) int {
			probes := 0
			fs := serveFiles(files)
			fs.FileExistsFunc = func(_ context.Context, _ string) (bool, error) {
				probes++

				return false, nil
			}

			_, err := cpuhealth.NewLinuxSampler(fs, cgroupBase).Read(context.Background())
			Expect(err).NotTo(HaveOccurred())

			return probes
		}

		withCpusetRead := probesPerTick(v1Files{"/proc/stat": "cpu  1 0 1 1 0 0 0 0 0 0\ncpu0 1 0 1 1 0 0 0 0 0 0\n"})
		withoutCpusetRead := probesPerTick(v1Files{})

		Expect(withCpusetRead).To(Equal(withoutCpusetRead))
	})
})
