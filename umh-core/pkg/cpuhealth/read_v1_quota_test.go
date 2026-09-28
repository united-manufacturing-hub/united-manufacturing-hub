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
