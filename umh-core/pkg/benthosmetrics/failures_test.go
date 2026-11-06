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

// The reason Failures.ErrorFree writes for the first entry, and the kind
// an entry is.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The reason Failures.ErrorFree writes", func() {
	reason := func(count int64) string {
		f := Failures{Entries: []FailureEntry{{
			Signal: "output_error",
			Label:  "output error",
			Count:  count,
		}}}
		ok, r := f.ErrorFree()
		Expect(ok).To(BeFalse())

		return r
	}

	It("names one failure in the singular", func() {
		Expect(reason(1)).To(Equal("benthos reported 1 output error in the last 10 minutes"))
	})

	It("names more than one failure in the plural", func() {
		Expect(reason(2)).To(Equal("benthos reported 2 output errors in the last 10 minutes"))
	})
})

var _ = Describe("Rendering a duration in words", func() {
	// windowWords renders the user-facing duration of a reason. Below an
	// hour it stays as it was; from an hour on a reader must not divide
	// minutes in their head.
	DescribeTable("windowWords",
		func(d time.Duration, want string) {
			Expect(windowWords(d)).To(Equal(want))
		},
		Entry("one minute", time.Minute, "1 minute"),
		Entry("ten minutes", 10*time.Minute, "10 minutes"),
		Entry("one hour", time.Hour, "1 hour"),
		Entry("one hour and a minute", 61*time.Minute, "1 hour 1 minute"),
		Entry("one hour and a quarter", 75*time.Minute, "1 hour 15 minutes"),
		Entry("twelve hours", 12*time.Hour, "12 hours"),
	)
})

var _ = Describe("The kind of a FailureEntry", func() {
	// An error entry built without a window is still an error, not a
	// connection: a test literal, or a record decoded from before the
	// Connection field existed, must not silently count as a connection.
	It("reads an error entry built without a window as an error", func() {
		e := FailureEntry{Signal: "output_error", Label: "output error", Count: 1}

		Expect(e.IsConnection()).To(BeFalse(), "an error entry is not a connection entry")

		ok, reason := (&Failures{Entries: []FailureEntry{e}}).ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reported 1 output error in the last 10 minutes"),
			"not \"benthos reports 1 output error not connected\"")
	})

	It("reads a connection entry as a connection", func() {
		e := FailureEntry{Signal: "input_connection", Label: "input", Count: 1, Connection: true}

		Expect(e.IsConnection()).To(BeTrue())

		ok, reason := (&Failures{Entries: []FailureEntry{e}}).ErrorFree()
		Expect(ok).To(BeFalse())
		Expect(reason).To(Equal("benthos reports 1 input not connected"))
	})
})
