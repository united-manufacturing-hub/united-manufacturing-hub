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

package cpuhealth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
)

var _ = Describe("the file each read opened", func() {
	It("names a v2 file under the sampler's base, and a machine-wide file absolutely", func() {
		const base = "/custom/tree"
		smp, err := cpuhealth.NewLinuxSampler(serveFiles(v1Files{
			base + "/cpu.stat": "usage_usec 1\nnr_periods 1\nnr_throttled 0\n",
		}), base).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(smp.Troubleshooting.CgroupLayout).To(Equal("v2"))
		Expect(smp.Troubleshooting.ReadPaths).To(HaveKeyWithValue(cpuhealth.OperationCPUStat, base+"/cpu.stat"))
		Expect(smp.Troubleshooting.ReadPaths).To(HaveKeyWithValue(cpuhealth.OperationCPUMax, base+"/cpu.max"))
		Expect(smp.Troubleshooting.ReadPaths).To(HaveKeyWithValue(cpuhealth.OperationProcStat, "/proc/stat"))
		Expect(smp.Troubleshooting.ReadPaths).To(HaveKeyWithValue(cpuhealth.OperationCPUAcctUsage, ""))
	})

	It("names the v1 files a v1 host has", func() {
		smp, err := v1Sampler(v1Files{
			cgroupBase + "/cpu,cpuacct/cpu.stat":      "nr_periods 1\nnr_throttled 0\n",
			cgroupBase + "/cpu,cpuacct/cpuacct.usage": "1000\n",
			cgroupBase + "/cpuset/cpuset.cpus":        "0-1\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(smp.Troubleshooting.CgroupLayout).To(Equal("v1"))
		Expect(smp.Troubleshooting.ReadPaths).To(And(
			HaveKeyWithValue(cpuhealth.OperationCPUMax, cgroupBase+"/cpu,cpuacct/cpu.cfs_quota_us"),
			HaveKeyWithValue(cpuhealth.OperationCPUStat, cgroupBase+"/cpu,cpuacct/cpu.stat"),
			HaveKeyWithValue(cpuhealth.OperationCPUAcctUsage, cgroupBase+"/cpu,cpuacct/cpuacct.usage"),
			HaveKeyWithValue(cpuhealth.OperationCpusetCPUs, cgroupBase+"/cpuset/cpuset.cpus"),
			HaveKeyWithValue(cpuhealth.OperationCPUPressure, ""),
		))
	})

	It("names the v1 file in the error of a v1 read that failed on its content", func() {
		smp, err := v1Sampler(v1Files{
			cgroupBase + "/cpu,cpuacct/cpu.stat": "nr_periods 1\nnr_throttled 0\n",
			cgroupBase + "/cpuset/cpuset.cpus":   "\n",
			"/proc/stat":                         "cpu  1 0 1 1 0 0 0 0 0 0\ncpu0 1 0 1 1 0 0 0 0 0 0\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(smp.Troubleshooting.ReadErrors[cpuhealth.OperationCpusetCPUs]).
			To(MatchError(ContainSubstring(cgroupBase + "/cpuset/cpuset.cpus")))
	})

	It("records the layout as unresolved where neither hierarchy answers", func() {
		smp, err := v1Sampler(v1Files{}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(smp.Troubleshooting.CgroupLayout).To(Equal("unresolved"))
	})
})
