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

package channelusage_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/channelusage"
)

var _ = Describe("Monitor", func() {
	var (
		now     time.Time
		monitor *channelusage.Monitor
	)

	observe := func(length, ticks int) {
		for range ticks {
			monitor.Observe(length, 100, now)
			now = now.Add(channelusage.SampleInterval)
		}
	}

	degraded := func() bool {
		verdict, _ := monitor.Verdict()

		return verdict.Degraded
	}

	windowSamples := int(channelusage.Window / channelusage.SampleInterval)
	atFireMark := int(channelusage.FirePercent)
	atClearMark := int(channelusage.ClearPercent)

	BeforeEach(func() {
		var err error

		now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		monitor, err = channelusage.NewMonitor()
		Expect(err).NotTo(HaveOccurred())
	})

	It("has no verdict until twenty readings", func() {
		observe(50, 19)

		_, ok := monitor.Verdict()
		Expect(ok).To(BeFalse())

		observe(50, 1)

		verdict, ok := monitor.Verdict()
		Expect(ok).To(BeTrue())
		Expect(verdict.P95FillPercent).To(BeNumerically("~", 50, 0.001))
	})

	It("degrades on repeated bursts a mean would hide", func() {
		for range windowSamples / 6 {
			observe(100, 1)
			observe(0, 5)
		}

		verdict, _ := monitor.Verdict()
		Expect(verdict.P95FillPercent).To(BeNumerically("~", 100, 0.001))
		Expect(verdict.Degraded).To(BeTrue())
	})

	It("holds a degraded p95 until it falls below the clear mark", func() {
		observe(atFireMark+1, windowSamples+1)
		Expect(degraded()).To(BeTrue())

		observe((atClearMark+atFireMark)/2, windowSamples+1)
		Expect(degraded()).To(BeTrue())

		observe(atClearMark-1, windowSamples+1)
		Expect(degraded()).To(BeFalse())
	})

	It("degrades on a single peak until it leaves the peak window", func() {
		observe(100, 1)
		observe(0, windowSamples-50)

		verdict, _ := monitor.Verdict()
		Expect(verdict.P95FillPercent).To(BeNumerically("~", 0, 0.001))
		Expect(verdict.PeakFillPercent).To(BeNumerically("~", 100, 0.001))
		Expect(verdict.Degraded).To(BeTrue())

		observe(0, 51)

		verdict, _ = monitor.Verdict()
		Expect(verdict.PeakFillPercent).To(BeNumerically("~", 0, 0.001))
		Expect(verdict.Degraded).To(BeFalse())
	})
})
