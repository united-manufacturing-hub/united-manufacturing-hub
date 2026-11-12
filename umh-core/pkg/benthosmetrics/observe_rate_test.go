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

// The rates one scrape produces, over time.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The throughput one scrape produces, over time", func() {
	It("computes the throughput by time", func() {
		// Every scrape time is injected, so no assertion here depends on the
		// wall clock.
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		got := tr.observe(testPort, counts(10, 5), t0)
		Expect(got.InputMessagesPerSecond).To(Equal(unmeasured),
			"one sample cannot be a throughput")

		got = tr.observe(testPort, counts(12, 6), t0.Add(time.Second))
		Expect(perSecond(got.InputMessagesPerSecond)).To(BeNumerically("~", 2, 0.5),
			"10 -> 12 over 1s")
		Expect(perSecond(got.OutputMessagesPerSecond)).To(BeNumerically("~", 1, 0.5),
			"5 -> 6 over 1s")
		Expect(got.IsActive).To(BeTrue(), "the input moves, so the Benthos is active")
	})

	It("ignores a scrape older than the previous one", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		tr.observe(testPort, counts(10, 5), t0)
		tr.observe(testPort, counts(12, 6), t0.Add(time.Second))
		got := tr.observe(testPort, counts(5, 5), t0.Add(-61*time.Second))
		Expect(perSecond(got.InputMessagesPerSecond)).To(BeNumerically("~", 2, 0.5),
			"an older scrape keeps the previous throughput")
		// observe_reset_test pins the equal-timestamp case.
	})

	It("does not stretch the window on a skipped poll", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// Samples at t0, +30s, +60s with input 0, 30, 90 read 90/60 = 1.5/s,
		// the real-time delta over the 60s window, not 60/30 from the two
		// newest samples.
		tr.observe(testPort, counts(0, 0), t0)
		tr.observe(testPort, counts(30, 30), t0.Add(30*time.Second))
		got := tr.observe(testPort, counts(90, 90), t0.Add(60*time.Second))
		Expect(perSecond(got.InputMessagesPerSecond)).To(BeNumerically("~", 1.5, 0.1),
			"the window is 60s of time, not two readings")
	})

	It("drops a sample older than the window", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// A sample older than 60s leaves the window: 0@t0, 30@+30s, 90@+61s
		// reads (90-30)/31, from the samples the window still holds.
		tr.observe(testPort, counts(0, 0), t0)
		tr.observe(testPort, counts(30, 30), t0.Add(30*time.Second))
		got := tr.observe(testPort, counts(90, 90), t0.Add(61*time.Second))
		Expect(perSecond(got.InputMessagesPerSecond)).To(BeNumerically("~", 1.94, 0.1),
			"the t0 sample is aged out, so the slope starts at +30s")
	})

	It("reads no throughput on the first scrape of a fresh Tracker", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		got := tr.observe(testPort, counts(100, 100), t0)

		Expect(got.InputMessagesPerSecond).To(Equal(unmeasured),
			"the first scrape only seeds the window; it is not an idle flow")
		Expect(got.OutputMessagesPerSecond).To(Equal(unmeasured))
		Expect(got.IsActive).To(BeFalse(),
			"IsActive needs a measured input throughput")
	})

	It("does not count output-only traffic as active", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })
		tr.observe(testPort, counts(10, 0), t0)
		got := tr.observe(testPort, counts(10, 5), t0.Add(time.Second))
		Expect(perSecond(got.OutputMessagesPerSecond)).To(BeNumerically("~", 5, 0.5))
		Expect(got.IsActive).To(BeFalse(),
			"output traffic does not count")
	})
})
