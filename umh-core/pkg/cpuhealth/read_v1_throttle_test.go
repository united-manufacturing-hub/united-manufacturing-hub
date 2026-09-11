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

// The cgroup v1 throttle counters. v1 uses the same nr_periods and nr_throttled
// key names v2 does, under its cpu controller directory, so only the path
// differs.
package cpuhealth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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
