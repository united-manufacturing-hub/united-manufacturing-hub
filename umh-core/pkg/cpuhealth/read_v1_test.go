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
	iofs "io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const cgroupBase = "/sys/fs/cgroup"

type v1Files map[string]string

func serveFiles(files v1Files) *filesystem.MockFileSystem {
	fs := filesystem.NewMockFileSystem()
	fs.FileExistsFunc = func(_ context.Context, path string) (bool, error) {
		_, served := files[path]

		return served, nil
	}
	fs.ReadFileFunc = func(_ context.Context, path string) ([]byte, error) {
		content, served := files[path]
		if !served {
			return nil, &iofs.PathError{Op: "open", Path: path, Err: iofs.ErrNotExist}
		}

		return []byte(content), nil
	}

	return fs
}

func v1Sampler(files v1Files) cpuhealth.Sampler {
	return cpuhealth.NewLinuxSampler(serveFiles(files), cgroupBase)
}

var _ = Describe("the cgroup v1 CPU limit", func() {
	const (
		statPath   = "/sys/fs/cgroup/cpu,cpuacct/cpu.stat"
		quotaPath  = "/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_quota_us"
		periodPath = "/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_period_us"
	)

	It("reads a positive quota as a capacity in cores", func() {
		smp, err := v1Sampler(v1Files{
			statPath:   "nr_periods 10\nnr_throttled 1\n",
			quotaPath:  "200000\n",
			periodPath: "100000\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		quota, ok := smp.Quota.Get()
		Expect(ok).To(BeTrue(), "two readable v1 files name a limit")
		Expect(quota).To(Equal(2.0))
	})

	It("reads a quota of -1 as a present no-limit, the way v2 reads max", func() {
		smp, err := v1Sampler(v1Files{
			statPath:   "nr_periods 10\nnr_throttled 0\n",
			quotaPath:  "-1\n",
			periodPath: "100000\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		quota, ok := smp.Quota.Get()
		Expect(ok).To(BeTrue(), "an uncapped v1 cgroup is a definite no-limit, not a no-signal")
		Expect(quota).To(BeZero())
	})

	It("leaves the quota absent when either file is unreadable", func() {
		smp, err := v1Sampler(v1Files{
			statPath:  "nr_periods 10\nnr_throttled 0\n",
			quotaPath: "200000\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		_, ok := smp.Quota.Get()
		Expect(ok).To(BeFalse(), "a quota with no period is no capacity, never a bare 200000")
	})

	It("publishes quota and period as the raw limit text, the way v2 cpu.max holds both", func() {
		smp, err := v1Sampler(v1Files{
			statPath:   "nr_periods 10\nnr_throttled 0\n",
			quotaPath:  "200000\n",
			periodPath: "abc\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(smp.Troubleshooting.CPUMaxRaw).To(Equal("200000 abc"))
	})

	It("reads an empty quota file as empty, the way v2 reads an empty cpu.max", func() {
		smp, err := v1Sampler(v1Files{
			statPath:   "nr_periods 10\nnr_throttled 0\n",
			quotaPath:  "\n",
			periodPath: "100000\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(smp.Troubleshooting.Reads).To(ContainElement(cpuhealth.ReadResult{
			Operation: cpuhealth.OperationCPUMax,
			Outcome:   cpuhealth.ReadEmpty,
		}))
	})

	It("names the v1 file when the quota will not parse", func() {
		smp, err := v1Sampler(v1Files{
			statPath:   "nr_periods 10\nnr_throttled 0\n",
			quotaPath:  "abc\n",
			periodPath: "100000\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(smp.Troubleshooting.ReadErrors[cpuhealth.OperationCPUMax]).To(MatchError(ContainSubstring(quotaPath)))
	})
})

var _ = Describe("the cgroup v1 throttle counters", func() {
	const statPath = "/sys/fs/cgroup/cpu,cpuacct/cpu.stat"

	It("reads both counters from the v1 cpu controller's cpu.stat", func() {
		smp, err := v1Sampler(v1Files{
			statPath: "nr_periods 1000\nnr_throttled 50\nthrottled_time 1234567\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		periods, ok := smp.NrPeriods.Get()
		Expect(ok).To(BeTrue())
		Expect(periods).To(Equal(1000.0))
		throttled, ok := smp.NrThrottled.Get()
		Expect(ok).To(BeTrue())
		Expect(throttled).To(Equal(50.0))
	})

	It("fails the sample when a counter cannot be parsed", func() {
		_, err := v1Sampler(v1Files{
			statPath: "nr_periods not-a-number\nnr_throttled 3\n",
		}).Read(context.Background())

		Expect(err).To(HaveOccurred(), "a v1 cpu.stat that does not parse is corrupt, the same as a v2 one")
		Expect(err.Error()).To(ContainSubstring(statPath), "the error names the file that failed, not the v2 path")
	})
})

var _ = Describe("usage as a rate on cgroup v1", func() {
	const (
		statPath  = cgroupBase + "/cpu,cpuacct/cpu.stat"
		usagePath = cgroupBase + "/cpu,cpuacct/cpuacct.usage"
	)

	It("converts the nanosecond total to microseconds and derives the rate from two reads", func() {
		ctx := context.Background()
		files := v1Files{
			statPath:  "nr_periods 1000\nnr_throttled 5\n",
			usagePath: "5000000000\n",
		}
		sampler := v1Sampler(files)

		first, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		usage, ok := first.UsageUsec.Get()
		Expect(ok).To(BeTrue(), "cpuacct.usage is the v1 usage total")
		Expect(usage).To(Equal(5_000_000.0), "nanoseconds reach UsageUsec as microseconds")
		_, ok = first.UsageCores.Get()
		Expect(ok).To(BeFalse(), "the first read fixes a baseline and derives no rate")

		files[usagePath] = "6000000000\n"

		second, err := sampler.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		rate, ok := second.UsageCores.Get()
		Expect(ok).To(BeTrue(), "the second read has an edge to subtract from")
		elapsed := second.Timestamp.Sub(first.Timestamp).Seconds()
		Expect(rate).To(BeNumerically("~", 1.0/elapsed, 1e-6),
			"one second of CPU time over the elapsed interval is the rate in cores")
	})

	It("reads usage on a v1 host whose kernel writes no cpu.stat", func() {
		smp, err := v1Sampler(v1Files{usagePath: "5000000000\n"}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		usage, ok := smp.UsageUsec.Get()
		Expect(ok).To(BeTrue())
		Expect(usage).To(Equal(5_000_000.0))
	})

	It("records a missing cpuacct.usage against its own read, not against cpu.stat", func() {
		smp, err := v1Sampler(v1Files{statPath: "nr_periods 10\nnr_throttled 0\n"}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		_, ok := smp.UsageUsec.Get()
		Expect(ok).To(BeFalse(), "no cpuacct.usage is no usage, never a trusted 0")
		Expect(smp.Troubleshooting.Reads).To(ContainElements(
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUStat, Outcome: cpuhealth.ReadOK},
			cpuhealth.ReadResult{Operation: cpuhealth.OperationCPUAcctUsage, Outcome: cpuhealth.ReadMissing},
		))
	})

	It("fails the sample when cpuacct.usage will not parse, as v2 does for usage_usec", func() {
		_, err := v1Sampler(v1Files{
			statPath:  "nr_periods 10\nnr_throttled 0\n",
			usagePath: "abc\n",
		}).Read(context.Background())

		Expect(err).To(MatchError(ContainSubstring(usagePath)))
	})
})

var _ = Describe("the CPUs a cgroup v1 container may run on", func() {
	const (
		statPath      = "/sys/fs/cgroup/cpu,cpuacct/cpu.stat"
		effectivePath = "/sys/fs/cgroup/cpuset/cpuset.effective_cpus"
		writtenPath   = "/sys/fs/cgroup/cpuset/cpuset.cpus"
		procStat      = "cpu  100 0 100 1000 0 0 0 0 0 0\n" +
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

var _ = Describe("pressure on cgroup v1", func() {
	It("reports no pressure, and never claims the kernel published any", func() {
		smp, err := v1Sampler(v1Files{
			"/sys/fs/cgroup/cpu,cpuacct/cpu.stat": "nr_periods 10\nnr_throttled 0\n",
			"/proc/pressure/cpu":                  "some avg10=1.00 avg60=42.00 avg300=1.00 total=1\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		_, ok := smp.Pressure.Get()
		Expect(ok).To(BeFalse(), "a machine-wide pressure figure must not reach a cgroup-scoped field")
		Expect(smp.PsiAvailable).To(BeFalse())
		Expect(smp.Troubleshooting.Reads).To(ContainElement(cpuhealth.ReadResult{
			Operation: cpuhealth.OperationCPUPressure,
			Outcome:   cpuhealth.ReadMissing,
		}))

		env := cpuhealth.DeriveEnvironment(smp)
		Expect(env.Has(cpuhealth.HasPressureStats)).To(BeFalse())
		Expect(env.Has(cpuhealth.HasLimitedVisibility)).To(BeTrue(),
			"no quota and no pressure is the limited-visibility arm")
	})
})
