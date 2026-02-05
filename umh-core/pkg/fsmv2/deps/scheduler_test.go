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

	Describe("IsWeekend", func() {
		It("returns true for Saturday", func() {
			sat := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
			Expect(sat.Weekday()).To(Equal(time.Saturday))
			Expect(s.IsWeekend(sat)).To(BeTrue())
		})

		It("returns true for Sunday", func() {
			sun := time.Date(2025, 2, 2, 12, 0, 0, 0, time.UTC)
			Expect(sun.Weekday()).To(Equal(time.Sunday))
			Expect(s.IsWeekend(sun)).To(BeTrue())
		})

		It("returns false for Monday", func() {
			mon := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC)
			Expect(s.IsWeekend(mon)).To(BeFalse())
		})

		It("returns false for Friday 23:59", func() {
			fri := time.Date(2025, 1, 31, 23, 59, 0, 0, time.UTC)
			Expect(fri.Weekday()).To(Equal(time.Friday))
			Expect(s.IsWeekend(fri)).To(BeFalse())
		})
	})

	Describe("IsLowTrafficHours", func() {
		It("returns true at 02:00 (boundary)", func() {
			Expect(s.IsLowTrafficHours(time.Date(2025, 2, 3, 2, 0, 0, 0, time.UTC))).To(BeTrue())
		})

		It("returns true at 03:00", func() {
			Expect(s.IsLowTrafficHours(time.Date(2025, 2, 3, 3, 0, 0, 0, time.UTC))).To(BeTrue())
		})

		It("returns true at 04:59", func() {
			Expect(s.IsLowTrafficHours(time.Date(2025, 2, 3, 4, 59, 0, 0, time.UTC))).To(BeTrue())
		})

		It("returns false at 05:00 (boundary)", func() {
			Expect(s.IsLowTrafficHours(time.Date(2025, 2, 3, 5, 0, 0, 0, time.UTC))).To(BeFalse())
		})

		It("returns false at 01:59", func() {
			Expect(s.IsLowTrafficHours(time.Date(2025, 2, 2, 1, 59, 0, 0, time.UTC))).To(BeFalse())
		})

		It("returns false at noon", func() {
			Expect(s.IsLowTrafficHours(time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC))).To(BeFalse())
		})
	})

	Describe("IsBusinessHours", func() {
		It("returns true at 07:00 (boundary)", func() {
			Expect(s.IsBusinessHours(time.Date(2025, 2, 3, 7, 0, 0, 0, time.UTC))).To(BeTrue())
		})

		It("returns true at 12:00", func() {
			Expect(s.IsBusinessHours(time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC))).To(BeTrue())
		})

		It("returns true at 19:59", func() {
			Expect(s.IsBusinessHours(time.Date(2025, 2, 3, 19, 59, 0, 0, time.UTC))).To(BeTrue())
		})

		It("returns false at 20:00 (boundary)", func() {
			Expect(s.IsBusinessHours(time.Date(2025, 2, 3, 20, 0, 0, 0, time.UTC))).To(BeFalse())
		})

		It("returns false at 06:59", func() {
			Expect(s.IsBusinessHours(time.Date(2025, 2, 3, 6, 59, 0, 0, time.UTC))).To(BeFalse())
		})

		It("returns false at 22:00", func() {
			Expect(s.IsBusinessHours(time.Date(2025, 2, 3, 22, 0, 0, 0, time.UTC))).To(BeFalse())
		})
	})

	Describe("IsPreferredMaintenanceWindow", func() {
		It("returns true for Saturday 03:00 (weekend AND low-traffic)", func() {
			sat3am := time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC)
			Expect(s.IsPreferredMaintenanceWindow(sat3am)).To(BeTrue())
		})

		It("returns false for Monday 03:00 (low-traffic but NOT weekend)", func() {
			mon3am := time.Date(2025, 2, 3, 3, 0, 0, 0, time.UTC)
			Expect(s.IsPreferredMaintenanceWindow(mon3am)).To(BeFalse())
		})

		It("returns false for Saturday 12:00 (weekend but NOT low-traffic)", func() {
			satNoon := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
			Expect(s.IsPreferredMaintenanceWindow(satNoon)).To(BeFalse())
		})

		It("returns false for Monday 12:00 (neither)", func() {
			monNoon := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC)
			Expect(s.IsPreferredMaintenanceWindow(monNoon)).To(BeFalse())
		})
	})

	Describe("IsAcceptableMaintenanceWindow", func() {
		It("returns true for Saturday 03:00 (both)", func() {
			sat3am := time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC)
			Expect(s.IsAcceptableMaintenanceWindow(sat3am)).To(BeTrue())
		})

		It("returns true for Monday 03:00 (low-traffic)", func() {
			mon3am := time.Date(2025, 2, 3, 3, 0, 0, 0, time.UTC)
			Expect(s.IsAcceptableMaintenanceWindow(mon3am)).To(BeTrue())
		})

		It("returns true for Saturday 12:00 (weekend)", func() {
			satNoon := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
			Expect(s.IsAcceptableMaintenanceWindow(satNoon)).To(BeTrue())
		})

		It("returns false for Monday 12:00 (neither)", func() {
			monNoon := time.Date(2025, 2, 3, 12, 0, 0, 0, time.UTC)
			Expect(s.IsAcceptableMaintenanceWindow(monNoon)).To(BeFalse())
		})
	})

	Describe("Timezone independence", func() {
		It("gives different results for the same UTC instant in different timezones", func() {
			utc := time.Date(2025, 2, 1, 3, 0, 0, 0, time.UTC)
			cet := utc.In(time.FixedZone("CET", 3600))

			Expect(s.IsLowTrafficHours(utc)).To(BeTrue(), "03:00 UTC is low-traffic")
			Expect(s.IsLowTrafficHours(cet)).To(BeTrue(), "04:00 CET is still low-traffic")

			utc2 := time.Date(2025, 2, 1, 4, 30, 0, 0, time.UTC)
			cet2 := utc2.In(time.FixedZone("CET", 3600))

			Expect(s.IsLowTrafficHours(utc2)).To(BeTrue(), "04:30 UTC is low-traffic")
			Expect(s.IsLowTrafficHours(cet2)).To(BeFalse(), "05:30 CET is NOT low-traffic")
		})
	})
})
