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

// The reason FailureReport.ErrorFree writes, and which failure it names.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The reason FailureReport.ErrorFree writes", func() {
	reason := func(count int64) string {
		f := FailureReport{OutputErrors: &RecentErrors{Count: count}}
		ok, r := f.ErrorFree()
		Expect(ok).To(BeFalse())

		return r
	}

	It("reads an empty report as error-free", func() {
		ok, r := (&FailureReport{}).ErrorFree()
		Expect(ok).To(BeTrue())
		Expect(r).To(BeEmpty())
	})

	It("names one failure in the singular", func() {
		Expect(reason(1)).To(Equal("benthos reported 1 output error in the last 10m, just now"))
	})

	It("names more than one failure in the plural", func() {
		Expect(reason(2)).To(Equal("benthos reported 2 output errors in the last 10m, last one just now"))
	})

	It("words inputs and outputs not connected without a time", func() {
		ok, r := (&FailureReport{InputsNotConnected: &NotConnected{Count: 1}}).ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(r).To(Equal("benthos reports 1 input not connected"))

		ok, r = (&FailureReport{OutputsNotConnected: &NotConnected{Count: 2}}).ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(r).To(Equal("benthos reports 2 outputs not connected"))
	})
})

var _ = Describe("The failure ErrorFree names", func() {
	It("names errors before more inputs not connected", func() {
		f := FailureReport{
			ProcessorErrors:    &RecentErrors{Count: 1},
			InputsNotConnected: &NotConnected{Count: 5},
		}
		_, r := f.ErrorFree()
		Expect(r).To(Equal("benthos reported 1 processor error in the last 10m, just now"))
	})

	It("names the counter with more errors", func() {
		f := FailureReport{
			OutputErrors:    &RecentErrors{Count: 1},
			ProcessorErrors: &RecentErrors{Count: 3},
		}
		_, r := f.ErrorFree()
		Expect(r).To(HavePrefix("benthos reported 3 processor errors"))
	})

	It("names output errors on a tie", func() {
		f := FailureReport{
			OutputErrors:    &RecentErrors{Count: 2},
			ProcessorErrors: &RecentErrors{Count: 2},
		}
		_, r := f.ErrorFree()
		Expect(r).To(HavePrefix("benthos reported 2 output errors"))
	})

	It("names the side with more not connected, inputs on a tie", func() {
		f := FailureReport{
			InputsNotConnected:  &NotConnected{Count: 1},
			OutputsNotConnected: &NotConnected{Count: 2},
		}
		_, r := f.ErrorFree()
		Expect(r).To(Equal("benthos reports 2 outputs not connected"))

		f.OutputsNotConnected.Count = 1
		_, r = f.ErrorFree()
		Expect(r).To(Equal("benthos reports 1 input not connected"))
	})
})

var _ = Describe("The time in the reason", func() {
	// observedT is the scrape the reason is written from. Every expectation
	// below is exact against it, so a time taken from the wrong counter, or
	// from the window instead of the error, reads as a different string.
	observedT := time.Unix(1_800_000_000, 0)

	errorsAgo := func(count int64, lastErrorAgo time.Duration) *RecentErrors {
		return &RecentErrors{
			Count: count,
			// Since is the first error of the run and LastErrorAt the
			// newest. They differ here: the reason clocks the newest.
			Since:       observedT.Add(-42 * time.Minute),
			LastErrorAt: observedT.Add(-lastErrorAgo),
		}
	}

	reasonFor := func(f FailureReport) string {
		f.ObservedAt = observedT
		ok, r := f.ErrorFree()
		ExpectWithOffset(1, ok).To(BeFalse())

		return r
	}

	outputErrorsAgo := func(count int64, lastErrorAgo time.Duration) string {
		return reasonFor(FailureReport{OutputErrors: errorsAgo(count, lastErrorAgo)})
	}

	It("times the newest error of the counter it names", func() {
		// The named counter last rose 4 minutes ago. The other rose 7
		// minutes ago, and the window alone would read 10m.
		Expect(reasonFor(FailureReport{
			OutputErrors:    errorsAgo(3, 4*time.Minute),
			ProcessorErrors: errorsAgo(1, 7*time.Minute),
		})).To(Equal("benthos reported 3 output errors in the last 10m, last one 4m ago"))
	})

	// Both counters carry the same count, so only the time tells them apart
	// past the label.
	It("takes the time from the counter it names, not from another one", func() {
		Expect(reasonFor(FailureReport{
			OutputErrors:    errorsAgo(3, 7*time.Minute),
			ProcessorErrors: errorsAgo(3, 4*time.Minute),
		})).To(Equal("benthos reported 3 output errors in the last 10m, last one 7m ago"))
	})

	// One error needs no "last one": there is no other error for the time to
	// belong to.
	It("drops the last one clause for a single error", func() {
		Expect(outputErrorsAgo(1, 4*time.Minute)).
			To(Equal("benthos reported 1 output error in the last 10m, 4m ago"))
	})

	It("rounds the time to whole seconds", func() {
		Expect(outputErrorsAgo(2, 4*time.Minute+30*time.Second+600*time.Millisecond)).
			To(Equal("benthos reported 2 output errors in the last 10m, last one 4m31s ago"),
				"rounded, not truncated to 4m30s")
	})

	// An error from this very scrape has no elapsed time to name, and
	// "0s ago" reads as a stopped clock. The reset to "just now" is itself
	// the signal that a new error landed.
	It("reads an error from this scrape as just now", func() {
		Expect(outputErrorsAgo(3, 0)).
			To(Equal("benthos reported 3 output errors in the last 10m, last one just now"))
		Expect(outputErrorsAgo(1, 999*time.Millisecond)).
			To(Equal("benthos reported 1 output error in the last 10m, just now"))
	})

	// One clock stamps both, so this cannot happen; it lands in the same
	// branch so that no reader ever meets a negative duration.
	It("never prints a negative time", func() {
		Expect(outputErrorsAgo(1, -time.Minute)).
			To(Equal("benthos reported 1 output error in the last 10m, just now"))
	})
})

var _ = Describe("Rendering a duration for a user", func() {
	// shortDuration drops the trailing zero units Go leaves behind, so the
	// window in the reason reads "10m" and not "10m0s".
	DescribeTable("shortDuration",
		func(d time.Duration, want string) {
			Expect(shortDuration(d)).To(Equal(want))
		},
		Entry("the error window", ErrorWindow, "10m"),
		Entry("seconds", 59*time.Second, "59s"),
		Entry("minutes and seconds", 9*time.Minute+12*time.Second, "9m12s"),
		Entry("whole minutes", 4*time.Minute, "4m"),
		Entry("one hour", time.Hour, "1h"),
		Entry("an hour and a quarter", 75*time.Minute, "1h15m"),
		Entry("sub-second noise", 4*time.Minute+400*time.Millisecond, "4m"),
	)
})
