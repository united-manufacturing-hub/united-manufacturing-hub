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

// The reason FailureReport.ErrorFree writes for the first entry, and the kind
// an entry is.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The reason FailureReport.ErrorFree writes", func() {
	reason := func(count int64) string {
		f := FailureReport{Entries: []FailureEntry{{
			Signal: "output_error",
			Label:  "output error",
			Count:  count,
		}}}
		ok, r := f.ErrorFree()
		Expect(ok).To(BeFalse())

		return r
	}

	It("names one failure in the singular", func() {
		Expect(reason(1)).To(Equal("benthos reported 1 output error in the last 10m, just now"))
	})

	It("names more than one failure in the plural", func() {
		Expect(reason(2)).To(Equal("benthos reported 2 output errors in the last 10m, last one just now"))
	})
})

var _ = Describe("The kind of a FailureEntry", func() {
	// An error entry built without a window is still an error, not a
	// connection: a test literal, or a record decoded from before the
	// Connection field existed, must not silently count as a connection.
	It("reads an error entry built without a window as an error", func() {
		e := FailureEntry{Signal: "output_error", Label: "output error", Count: 1}

		Expect(e.Connection).To(BeFalse(), "an error entry is not a connection entry")

		ok, reason := (&FailureReport{Entries: []FailureEntry{e}}).ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reported 1 output error in the last 10m, just now"),
			"not \"benthos reports 1 output error not connected\"")
	})

	It("reads a connection entry as a connection", func() {
		e := FailureEntry{Signal: "input_connection", Label: "input", Count: 1, Connection: true}

		Expect(e.Connection).To(BeTrue())

		ok, reason := (&FailureReport{Entries: []FailureEntry{e}}).ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reports 1 input not connected"))
	})
})

var _ = Describe("The time in the reason", func() {
	// observedT is the scrape the reason is written from. Every expectation
	// below is exact against it, so a time taken from the wrong entry, or
	// from the window instead of the error, reads as a different string.
	observedT := time.Unix(1_800_000_000, 0)

	errorEntry := func(count int64, lastErrorAgo time.Duration) FailureEntry {
		return FailureEntry{
			Signal: "output_error",
			Label:  "output error",
			Count:  count,
			// Since is the first error of the run and LastErrorAt the
			// newest. They differ here: the reason clocks the newest.
			Since:       observedT.Add(-42 * time.Minute),
			LastErrorAt: observedT.Add(-lastErrorAgo),
		}
	}

	reasonFor := func(entries ...FailureEntry) string {
		f := FailureReport{Entries: entries, ObservedAt: observedT}
		ok, r := f.ErrorFree()
		ExpectWithOffset(1, ok).To(BeFalse())

		return r
	}

	It("times the newest error of the entry it names", func() {
		// The named entry last rose 4 minutes ago. The second entry rose 7
		// minutes ago, and the window alone would read 10m.
		Expect(reasonFor(
			errorEntry(3, 4*time.Minute),
			errorEntry(1, 7*time.Minute),
		)).To(Equal("benthos reported 3 output errors in the last 10m, last one 4m ago"))
	})

	// Both entries carry the same count, so only the time tells them apart:
	// a reason built from the second one differs in nothing else.
	It("takes the time from the entry it names, not from another one", func() {
		Expect(reasonFor(
			errorEntry(3, 7*time.Minute),
			errorEntry(3, 4*time.Minute),
		)).To(Equal("benthos reported 3 output errors in the last 10m, last one 7m ago"))
	})

	// One error needs no "last one": there is no other error for the time to
	// belong to.
	It("drops the last one clause for a single error", func() {
		Expect(reasonFor(errorEntry(1, 4*time.Minute))).
			To(Equal("benthos reported 1 output error in the last 10m, 4m ago"))
	})

	It("rounds the time to whole seconds", func() {
		Expect(reasonFor(errorEntry(2, 4*time.Minute+30*time.Second+600*time.Millisecond))).
			To(Equal("benthos reported 2 output errors in the last 10m, last one 4m31s ago"),
				"rounded, not truncated to 4m30s")
	})

	// An error from this very scrape has no elapsed time to name, and
	// "0s ago" reads as a stopped clock. The reset to "just now" is itself
	// the signal that a new error landed.
	It("reads an error from this scrape as just now", func() {
		Expect(reasonFor(errorEntry(3, 0))).
			To(Equal("benthos reported 3 output errors in the last 10m, last one just now"))
		Expect(reasonFor(errorEntry(1, 999*time.Millisecond))).
			To(Equal("benthos reported 1 output error in the last 10m, just now"))
	})

	// One clock stamps both, so this cannot happen; it lands in the same
	// branch so that no reader ever meets a negative duration.
	It("never prints a negative time", func() {
		Expect(reasonFor(errorEntry(1, -time.Minute))).
			To(Equal("benthos reported 1 output error in the last 10m, just now"))
	})

	// A connection entry lasts until Benthos reconnects, so it carries no
	// time at all: neither the window it is not judged over, nor an error
	// time it does not have.
	It("leaves a connection entry without a time", func() {
		Expect(reasonFor(FailureEntry{
			Signal:     "input_connection",
			Label:      "input",
			Count:      1,
			Since:      observedT.Add(-time.Minute),
			Connection: true,
		})).To(Equal("benthos reports 1 input not connected"))
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
