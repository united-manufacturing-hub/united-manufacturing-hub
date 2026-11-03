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

// A dropped counter folds into its own offset, so only its series changes;
// a port change rebuilds everything.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("A dropped counter", func() {
	It("keeps the input rate when the output counter drops", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// The output counter restarts mid-flow (200 -> 210 -> 5) while the
		// input keeps flowing. Only the output's offset moves, so the input
		// rate and IsActive must not notice.
		tr.observe(testPort, counts(100, 200), t0)
		tr.observe(testPort, counts(110, 210), t0.Add(time.Second))
		got := tr.observe(testPort, counts(120, 5), t0.Add(2*time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 10, 0.5),
			"100 -> 110 -> 120 over 2s, undisturbed by the output drop")
		Expect(got.IsActive).To(BeTrue(), "the input keeps flowing")
	})

	It("keeps the output rate when the input counter drops", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// The input counter restarts (100 -> 5) while the output keeps
		// flowing 10/s. The input's offset absorbs the drop; the output
		// rate must not reset with it.
		tr.observe(testPort, counts(100, 200), t0)
		tr.observe(testPort, counts(110, 210), t0.Add(time.Second))
		got := tr.observe(testPort, counts(5, 220), t0.Add(2*time.Second))
		Expect(got.Output.MessagesPerSecond).To(BeNumerically("~", 10, 0.5),
			"200 -> 210 -> 220 over 2s, undisturbed by the input drop")
	})

	It("folds a counter restart into the series, not the rate", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// A restart drops both counters: input and output 100 -> 5. The
		// drop is folded into the offsets, so the series the engine sees
		// never goes down: the reset tick reads the 5 post-restart
		// messages over the 10 s span, never the cumulative count as a
		// rate and never a negative one.
		tr.observe(testPort, counts(100, 100), t0)
		got := tr.observe(testPort, counts(5, 5), t0.Add(10*time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 0.5, 0.1))

		// Post-restart 5 -> 12 over 1s: the window still holds the
		// pre-restart baseline, so the rate reads 12/11, not 12/1.
		got = tr.observe(testPort, counts(12, 12), t0.Add(11*time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 1.09, 0.1))
	})

	It("folds an input drop even when the output was already zero", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// A restart can leave the output counter at 0 before and after; the
		// input drop is folded into the input's offset all the same.
		tr.observe(testPort, counts(100, 0), t0)
		tr.observe(testPort, counts(5, 0), t0.Add(10*time.Second))
		got := tr.observe(testPort, counts(12, 0), t0.Add(11*time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 1.09, 0.1),
			"the drop reads as (12+100-100)/11, a small non-negative rate")
	})

	It("folds an output drop into the output's offset only", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// Output 200 -> 5 while the input stays put.
		tr.observe(testPort, counts(100, 200), t0)
		tr.observe(testPort, counts(100, 5), t0.Add(10*time.Second))
		got := tr.observe(testPort, counts(107, 12), t0.Add(11*time.Second))
		Expect(got.Output.MessagesPerSecond).To(BeNumerically("~", 1.09, 0.1),
			"(12+200-200)/11, never a negative rate")
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 0.64, 0.1),
			"the input window keeps its own samples: 100 -> 100 -> 107")
	})

	It("folds a restart-then-recover into the series", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// 100 -> restart at 0 -> 150 over 30s: the restart is folded in as
		// continuity, so the rate reads (150+100-100)/30 = 5/s.
		tr.observe(testPort, counts(100, 100), t0)
		tr.observe(testPort, counts(0, 0), t0.Add(15*time.Second))
		got := tr.observe(testPort, counts(150, 150), t0.Add(30*time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 5, 0.5))
	})

	It("rebuilds on a port change", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		// A config update can change the port of a running Benthos, so a new
		// port is a new counter series with no restart in between.
		tr.observe(4195, counts(100, 100), t0)
		got := tr.observe(4196, counts(150, 150), t0.Add(10*time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeZero(),
			"the first tick on the new port holds one sample")

		got = tr.observe(4196, counts(157, 157), t0.Add(11*time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 7, 0.5),
			"150->157 over 1s; the old port's 100 is gone from the window")
	})

	It("ignores a scrape that is not newer", func() {
		t0 := time.Now().Truncate(time.Second)
		tr := NewTracker(func() time.Time { return t0 })

		tr.observe(testPort, counts(100, 100), t0)
		tr.observe(testPort, counts(150, 150), t0.Add(time.Second))
		got := tr.observe(testPort, counts(160, 160), t0.Add(time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 50, 5),
			"the equal-timestamp scrape returns the previous rates, 50/s, not 60/s")

		// The next newer scrape reduces normally: the window still holds
		// 100@t0 and 150@+1s, so its slope is (160-100)/2. The equal-
		// timestamp check above is what pins that the ignored 160 never
		// entered the window; this slope reads the same either way.
		got = tr.observe(testPort, counts(160, 160), t0.Add(2*time.Second))
		Expect(got.Input.MessagesPerSecond).To(BeNumerically("~", 30, 5),
			"(160-100)/2 over the window the ignored scrape left untouched")
	})
})
