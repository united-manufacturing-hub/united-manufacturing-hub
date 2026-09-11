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

package fsmv2timescale

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics schedule", func() {
	var start time.Time

	BeforeEach(func() {
		start = time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	})

	It("claims the first run", func() {
		schedule := newMetricsSchedule(time.Minute)

		Expect(schedule.claimNextRun(start)).To(BeTrue())
	})

	It("refuses a second run before the interval has elapsed", func() {
		schedule := newMetricsSchedule(time.Minute)
		Expect(schedule.claimNextRun(start)).To(BeTrue())

		Expect(schedule.claimNextRun(start.Add(59 * time.Second))).To(BeFalse())
	})

	It("claims again once the interval has elapsed", func() {
		schedule := newMetricsSchedule(time.Minute)
		Expect(schedule.claimNextRun(start)).To(BeTrue())

		Expect(schedule.claimNextRun(start.Add(time.Minute))).To(BeTrue())
	})

	It("measures the interval from the claimed run, not from the refused attempt", func() {
		schedule := newMetricsSchedule(time.Minute)
		Expect(schedule.claimNextRun(start)).To(BeTrue())
		Expect(schedule.claimNextRun(start.Add(30 * time.Second))).To(BeFalse())

		Expect(schedule.claimNextRun(start.Add(time.Minute))).To(BeTrue())
	})
})

var _ = Describe("Metrics retention between collections", func() {
	var start time.Time
	var collected TimescaleMetrics

	BeforeEach(func() {
		start = time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
		collected = TimescaleMetrics{Hypertables: 4, DatabaseBytes: 1234}
	})

	It("reports nothing before the first collection", func() {
		schedule := newMetricsSchedule(time.Minute)

		Expect(schedule.last()).To(Equal(TimescaleMetrics{}))
	})

	It("reports what was remembered", func() {
		schedule := newMetricsSchedule(time.Minute)
		schedule.remember(collected)

		Expect(schedule.last()).To(Equal(collected))
	})

	It("keeps the last collection on a tick that does not claim a run", func() {
		schedule := newMetricsSchedule(time.Minute)
		Expect(schedule.claimNextRun(start)).To(BeTrue())
		schedule.remember(collected)

		Expect(schedule.claimNextRun(start.Add(time.Second))).To(BeFalse())
		Expect(schedule.last()).To(Equal(collected), "an unclaimed tick must not blank the metrics")
	})
})
