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

package deps_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var _ deps.Scheduler = deps.DefaultScheduler{}

var _ = Describe("DefaultScheduler", func() {
	var s deps.DefaultScheduler

	BeforeEach(func() {
		s = deps.DefaultScheduler{}
	})

	DescribeTable("IsPreferredMaintenanceWindow",
		func(t time.Time, expected bool) {
			Expect(s.IsPreferredMaintenanceWindow(t)).To(Equal(expected))
		},
		Entry("Saturday 03:00 — weekend AND low-traffic",
			time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC), true),
		Entry("Monday 03:00 — low-traffic but NOT weekend",
			time.Date(2025, 2, 3, 3, 0, 0, 0, time.UTC), false),
		Entry("Saturday 12:00 — weekend but NOT low-traffic",
			time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC), false),
		Entry("Monday 12:00 — neither",
			time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC), false),
	)

	DescribeTable("IsAcceptableMaintenanceWindow",
		func(t time.Time, expected bool) {
			Expect(s.IsAcceptableMaintenanceWindow(t)).To(Equal(expected))
		},
		Entry("Saturday 03:00 — weekend AND low-traffic",
			time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC), true),
		Entry("Monday 03:00 — low-traffic only",
			time.Date(2025, 2, 3, 3, 0, 0, 0, time.UTC), true),
		Entry("Saturday 12:00 — weekend only",
			time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC), true),
		Entry("Monday 12:00 — neither",
			time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC), false),
	)

	Describe("Timezone independence", func() {
		It("gives different results for the same UTC instant in different timezones", func() {
			utc := time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC)
			cet := utc.In(time.FixedZone("CET", 3600))

			Expect(s.IsPreferredMaintenanceWindow(utc)).To(BeTrue(), "Sat 03:00 UTC is preferred")
			Expect(s.IsPreferredMaintenanceWindow(cet)).To(BeTrue(), "Sat 04:00 CET is still preferred")

			utc2 := time.Date(2025, 2, 1, 4, 30, 0, 0, time.UTC)
			cet2 := utc2.In(time.FixedZone("CET", 3600))

			Expect(s.IsPreferredMaintenanceWindow(utc2)).To(BeTrue(), "Sat 04:30 UTC is preferred")
			Expect(s.IsPreferredMaintenanceWindow(cet2)).To(BeFalse(), "Sat 05:30 CET is NOT preferred")
		})
	})
})
