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

// The CPUs a cgroup v1 container may run on. v1 keeps the set under its own
// cpuset controller directory and names the kernel-narrowed one
// cpuset.effective_cpus, where v2 writes cpuset.cpus.effective at the base.
package cpuhealth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
)

var _ = Describe("the CPUs a cgroup v1 container may run on", func() {
	const (
		statPath      = "/sys/fs/cgroup/cpu,cpuacct/cpu.stat"
		effectivePath = "/sys/fs/cgroup/cpuset/cpuset.effective_cpus"
		writtenPath   = "/sys/fs/cgroup/cpuset/cpuset.cpus"
		// Four CPUs, of the eight the fixture /proc/stat reports.
		procStat = "cpu  100 0 100 1000 0 0 0 0 0 0\n" +
			"cpu0 1 0 1 1 0 0 0 0 0 0\ncpu1 1 0 1 1 0 0 0 0 0 0\n" +
			"cpu2 1 0 1 1 0 0 0 0 0 0\ncpu3 1 0 1 1 0 0 0 0 0 0\n" +
			"cpu4 1 0 1 1 0 0 0 0 0 0\ncpu5 1 0 1 1 0 0 0 0 0 0\n" +
			"cpu6 1 0 1 1 0 0 0 0 0 0\ncpu7 1 0 1 1 0 0 0 0 0 0\n"
	)

	It("counts the effective cpuset, and reads a strict subset of the machine as pinned", func() {
		smp, err := v1Sampler(v1Files{
			statPath:      "nr_periods 10\nnr_throttled 0\n",
			effectivePath: "0-3\n",
			"/proc/stat":  procStat,
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		logical, ok := smp.LogicalCpus.Get()
		Expect(ok).To(BeTrue())
		Expect(logical).To(Equal(4.0))
		Expect(smp.CpuScope).To(Equal(cpuhealth.ScopeAffinity),
			"four of the machine's eight CPUs is a pinned container")
	})

	It("falls back to the written cpuset when the effective one is absent", func() {
		smp, err := v1Sampler(v1Files{
			statPath:     "nr_periods 10\nnr_throttled 0\n",
			writtenPath:  "0-7\n",
			"/proc/stat": procStat,
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		logical, ok := smp.LogicalCpus.Get()
		Expect(ok).To(BeTrue(), "cpuset.cpus answers where cpuset.effective_cpus does not")
		Expect(logical).To(Equal(8.0))
		Expect(smp.CpuScope).To(Equal(cpuhealth.ScopeHost),
			"a set covering every machine CPU is the whole machine")
	})

	It("leaves the scope unknown when no cpuset is readable", func() {
		smp, err := v1Sampler(v1Files{
			statPath:     "nr_periods 10\nnr_throttled 0\n",
			"/proc/stat": procStat,
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		_, ok := smp.LogicalCpus.Get()
		Expect(ok).To(BeFalse())
		Expect(smp.CpuScope).To(Equal(cpuhealth.ScopeUnknown),
			"an unreadable cpuset is never a silent whole-machine scope")
	})
})
