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

// An error holds its ten minutes even when successful scrapes are sparse.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// When scrapes are sparse — a Benthos unreachable for minutes, then back with
// an error — the reading before the error is far older than the error. An
// error must still count for ten minutes after the scrape that saw it, not
// ten minutes after that older reading: with a gap of 9m59s, a window that
// measured against the older reading would clear after two seconds.
//
// Every case below has exactly two scrapes before the error's window is
// measured: a clean one, then one `gap` later that carries the error.
var _ = Describe("An error after a gap in successful scrapes", func() {
	errorAfterGap := func(gap time.Duration) (*Tracker, time.Time) {
		start := failureT
		t := NewTracker(func() time.Time { return start })

		// The clean scrape that becomes the engine's baseline.
		t.observe(testPort, outputErrors(0), start)

		// The scrape that finds the error, gap later. This is the error's
		// own LastErrorAt.
		roseAt := start.Add(gap)
		f := t.observe(testPort, outputErrors(1), roseAt).Failures
		ok, _ := f.ErrorFree()
		ExpectWithOffset(1, ok).To(BeFalse(), "the scrape that finds the error degrades the flow")
		ExpectWithOffset(1, failuresIn(*f)).To(Equal(1))
		ExpectWithOffset(1, f.OutputErrors.LastErrorAt).To(Equal(roseAt))

		return t, roseAt
	}

	DescribeTable("holds the error ten minutes after the error, not after the reading before it",
		func(gap time.Duration) {
			t, roseAt := errorAfterGap(gap)

			// One second before the window is up the error must still
			// count, although the baseline is `gap` older than the error.
			justInside := roseAt.Add(ErrorWindow - time.Second)
			f := t.observe(testPort, outputErrors(1), justInside).Failures
			ok, reason := f.ErrorFree()
			Expect(ok).To(BeFalse(),
				"the error is %s old, inside the %s window", ErrorWindow-time.Second, ErrorWindow)
			Expect(reason).To(ContainSubstring("output error"))
			Expect(failuresIn(*f)).To(Equal(1))
			Expect(f.OutputErrors.LastErrorAt).To(Equal(roseAt),
				"the failure keeps the error's own time")

			// One second past the window it clears, with no restart.
			f = t.observe(testPort, outputErrors(1), roseAt.Add(ErrorWindow+time.Second)).Failures
			ok, _ = f.ErrorFree()
			Expect(ok).To(BeTrue(), "the error clears once it is older than the window")
		},
		Entry("a one-minute gap", time.Minute),
		Entry("a five-minute gap", 5*time.Minute),
		Entry("a nine-minute gap", 9*time.Minute),
		Entry("a gap one second under the window", ErrorWindow-time.Second),
		Entry("a gap of exactly the window", ErrorWindow),
	)

	It("still counts a second error as a new error after a sparse first one", func() {
		t, roseAt := errorAfterGap(9 * time.Minute)

		// A second error nine minutes in restarts the clock, so the failure
		// must outlive the first error's own window.
		secondAt := roseAt.Add(9 * time.Minute)
		f := t.observe(testPort, outputErrors(2), secondAt).Failures
		Expect(failuresIn(*f)).To(Equal(1))
		Expect(f.OutputErrors.LastErrorAt).To(Equal(secondAt), "the second error moves the clock")

		f = t.observe(testPort, outputErrors(2), roseAt.Add(ErrorWindow+time.Second)).Failures
		ok, _ := f.ErrorFree()
		Expect(ok).To(BeFalse(), "the first error's window has passed but the second one's has not")

		f = t.observe(testPort, outputErrors(2), secondAt.Add(ErrorWindow+time.Second)).Failures
		ok, _ = f.ErrorFree()
		Expect(ok).To(BeTrue(), "ten minutes after the second error, the flow is clear")
	})

	// Each rise counts until ErrorWindow after the scrape that saw it, even
	// when the reading before it is older than the window.
	It("counts every error within the window when the readings are sparse", func() {
		start := failureT
		t := NewTracker(func() time.Time { return start })

		t.observe(testPort, outputErrors(0), start)
		t.observe(testPort, outputErrors(1), start.Add(9*time.Minute))

		// At T+18m the T reading is older than the window, but both errors
		// were seen within the last ten minutes.
		f := t.observe(testPort, outputErrors(2), start.Add(18*time.Minute)).Failures
		Expect(failuresIn(*f)).To(Equal(1))
		Expect(f.OutputErrors.Count).To(Equal(int64(2)))
		_, reason := f.ErrorFree()
		Expect(reason).To(HavePrefix("benthos reported 2 output errors in the last 10m"))

		// At T+20m the error seen at T+9m is 11m old and no longer counts.
		f = t.observe(testPort, outputErrors(2), start.Add(20*time.Minute)).Failures
		Expect(failuresIn(*f)).To(Equal(1))
		Expect(f.OutputErrors.Count).To(Equal(int64(1)), "only the error seen at T+18m is left")
	})

	It("recounts a failure as its errors age out", func() {
		start := failureT
		t := NewTracker(func() time.Time { return start })

		t.observe(testPort, outputErrors(0), start)
		t.observe(testPort, outputErrors(2), start.Add(9*time.Minute))
		t.observe(testPort, outputErrors(3), start.Add(12*time.Minute))

		f := t.observe(testPort, outputErrors(3), start.Add(18*time.Minute)).Failures
		Expect(failuresIn(*f)).To(Equal(1))
		Expect(f.OutputErrors.Count).To(Equal(int64(3)), "all three errors are within the window")

		// At T+20m the rise seen at T+9m has aged out; the rise at T+12m
		// still counts.
		f = t.observe(testPort, outputErrors(3), start.Add(20*time.Minute)).Failures
		Expect(failuresIn(*f)).To(Equal(1))
		Expect(f.OutputErrors.Count).To(Equal(int64(1)), "the two errors seen at T+9m are 11m old")
	})

	It("drops a sparse error when Benthos restarts", func() {
		t, roseAt := errorAfterGap(9 * time.Minute)

		// The counter drops: a new Benthos process. The errors it inherited
		// are not its own, so the failure must go even though the error is
		// well inside its window.
		f := t.observe(testPort, outputErrors(0), roseAt.Add(time.Minute)).Failures
		ok, _ := f.ErrorFree()
		Expect(ok).To(BeTrue(), "a restart clears the errors from before it")
	})
})
