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

// Usage as a rate, read from cgroup v1: cpuacct.usage in nanoseconds rather than
// cpu.stat's microseconds, converted once at the read. Elapsed time comes from
// the returned snapshots' own Timestamps, which makes the arithmetic exact
// against the real clock.
package cpuhealth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
)

var _ = Describe("usage as a rate on cgroup v1", func() {
	const (
		statPath  = cgroupBase + "/cpu,cpuacct/cpu.stat"
		usagePath = cgroupBase + "/cpu,cpuacct/cpuacct.usage"
	)

	It("converts the nanosecond total to microseconds and derives the rate from two reads", func() {
		ctx := context.Background()
		// Five seconds of CPU time, as the nanoseconds the kernel writes.
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

		// One more second of CPU time, the way a live counter moves between reads.
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
