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

// S3 R3 — limit-mode saturation. limit-headroom extracts quota - usage - 0.10 x
// quota and is reduced as a Mean over the 60s window, so a per-sample headroom
// folds to Appendix A's windowed one by linearity. The fire mark is headroom < 0
// (usage above 0.90 x quota); the clear mark is headroom > 0.05 x quota (usage
// below 0.85 x quota). The worked case saturation/limit/fire steps usage from
// 0.2 to 1.95 at tick 40: the windowed mean crosses zero at tick 95
// (value -0.0066) and settles at -0.15 only at tick 100 once the window is
// entirely post-step. Assert the value as well as the state, from the right
// tick.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("S3 R3 — limit-mode saturation", func() {
	It("should fire limit-mode saturation when the container's sustained usage enters the quota reserve band", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)

		base := time.Now()
		// quota 2.0: a 10% reserve opens the band at 0.90 x quota = 1.8. Usage
		// holds at 0.2 (headroom 1.6, well clear) until tick 39, then steps to
		// 1.95 (headroom -0.15, inside the band) at tick 40.
		usage := func(i int) float64 {
			if i < 40 {
				return 0.2
			}
			return 1.95
		}

		for i := 0; i <= 100; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				UsageCores:  diagnosis.Known(usage(i)),
			}
			fired, _ := engine.Observe(smp, env, smp.Timestamp)
			firedNames := firedSignalNames(fired)

			if i == 94 {
				// The windowed mean is still positive: 6 pre-step samples at 1.6
				// headroom outweigh the 55 post-step ones at -0.15. Not fired.
				Expect(firedNames).NotTo(ContainElement("limit-saturation"), "the mean must not cross zero at tick 94")
				_, st := engine.Reduction("limit-saturation", "limit-headroom").Get()
				Expect(st).To(Equal(diagnosis.StateValue))
			}
			if i == 95 {
				// First crossing: the window holds 5 pre-step samples (headroom
				// 1.6) and 56 post-step ones (-0.15) over ticks 35..95, mean
				// (8 - 8.4)/61 = -0.0066. The latch fires on THIS value, not the
				// settled -0.15.
				Expect(firedNames).To(ContainElement("limit-saturation"), "the windowed mean crosses zero at tick 95")
				v, st := engine.Reduction("limit-saturation", "limit-headroom").Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(BeNumerically("~", -0.0066, 1e-4), "the firing value is the windowed mean at the crossing tick")
			}
			if i == 100 {
				// Settled: the window is entirely post-step (ticks 40..100), so
				// the mean is 2 - 1.95 - 0.2 = -0.15 exactly.
				Expect(firedNames).To(ContainElement("limit-saturation"), "the latch stays fired once inside the reserve band")
				v, st := engine.Reduction("limit-saturation", "limit-headroom").Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(BeNumerically("~", -0.15, 1e-9), "the settled value is 2 - 1.95 - 0.2 = -0.15")
			}
		}
	})
})
