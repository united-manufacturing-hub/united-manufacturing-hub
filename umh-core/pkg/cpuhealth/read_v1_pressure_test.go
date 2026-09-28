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

// Pressure on cgroup v1: there is none, and the machine-wide /proc/pressure/cpu
// is not read in its place. That absence puts such a box on the
// limited-visibility arm, where usage-fraction answers for host-cpu-full.
package cpuhealth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
)

var _ = Describe("pressure on cgroup v1", func() {
	It("reports no pressure, and never claims the kernel published any", func() {
		smp, err := v1Sampler(v1Files{
			"/sys/fs/cgroup/cpu,cpuacct/cpu.stat": "nr_periods 10\nnr_throttled 0\n",
			// Served, and still not read: the v1 source has no pressure file to
			// read, and this is the machine-wide one.
			"/proc/pressure/cpu": "some avg10=1.00 avg60=42.00 avg300=1.00 total=1\n",
		}).Read(context.Background())

		Expect(err).NotTo(HaveOccurred())
		_, ok := smp.Pressure.Get()
		Expect(ok).To(BeFalse(), "a machine-wide pressure figure must not reach a cgroup-scoped field")
		Expect(smp.PsiAvailable).To(BeFalse())

		env := cpuhealth.DeriveEnvironment(smp)
		Expect(env.Has(cpuhealth.HasPressureStats)).To(BeFalse())
		Expect(env.Has(cpuhealth.HasLimitedVisibility)).To(BeTrue(),
			"no quota and no pressure is the limited-visibility arm")
	})
})
