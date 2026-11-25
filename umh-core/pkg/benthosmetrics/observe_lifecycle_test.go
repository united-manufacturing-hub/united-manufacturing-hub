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

// When the failure state starts over: the first scrape, a port change, and a
// gap longer than ErrorWindow. observe_restart_test.go covers restarts.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The failure state's lifecycle", func() {
	// inputDown is a scrape whose one input failed its connect.
	inputDown := connScrape(map[string]conn{"root.input": {failed: 1}}, nil)

	// The first scrape is the baseline: only rises above it count.
	It("counts only rises above a first scrape that already reads errors", func() {
		s := newScrapes().at(failureT, testPort, 7)
		s.wantErrorFree("first scrape, counter 7")

		s.at(failureT.Add(time.Second), testPort, 9).wantFailure("counter 7 -> 9", 2, "just now")
		Expect(s.failures.OutputErrors.Since).To(Equal(failureT.Add(time.Second)))
		Expect(s.failures.OutputErrors.LastErrorAt).To(Equal(failureT.Add(time.Second)))
		Expect(s.failures.ObservedAt).To(Equal(failureT.Add(time.Second)))
	})

	// After a gap longer than ErrorWindow, the totals before the gap are the
	// baseline: the rise across the gap counts, the errors before it do not,
	// and later rises add to it.
	It("keeps the totals before a long gap as the next baseline", func() {
		gapEnd := failureT.Add(15 * time.Minute)

		s := firedAt(5).at(gapEnd, testPort, 8)
		s.wantFailure("first scrape after a 15m outage, counter 5 -> 8", 3, "just now")
		Expect(s.failures.OutputErrors.Since).To(Equal(gapEnd))

		s.at(gapEnd.Add(time.Second), testPort, 9).wantFailure("next scrape, counter 8 -> 9", 4, "just now")
		Expect(s.failures.OutputErrors.Since).To(Equal(gapEnd), "the failure stays open from the gap's end")
	})

	// A port change starts a not-connected failure over at the first scrape on
	// the new port.
	It("restarts a not-connected failure on a port change", func() {
		tr := NewTracker(func() time.Time { return failureT })
		_ = tr.observe(testPort, inputDown, failureT)

		f := *tr.observe(testPort+1, inputDown, failureT.Add(time.Second)).Failures
		Expect(failuresIn(f)).To(Equal(1))
		Expect(f.InputsNotConnected.Since).To(Equal(failureT.Add(time.Second)))
	})

	// A gap longer than ErrorWindow starts a not-connected failure over at the
	// first scrape after the gap; a shorter gap keeps its start.
	It("restarts a not-connected failure after a gap longer than ErrorWindow", func() {
		tr := NewTracker(func() time.Time { return failureT })
		_ = tr.observe(testPort, inputDown, failureT)

		short := failureT.Add(ErrorWindow - time.Second)
		f := *tr.observe(testPort, inputDown, short).Failures
		Expect(failuresIn(f)).To(Equal(1))
		Expect(f.InputsNotConnected.Since).To(Equal(failureT), "a gap shorter than ErrorWindow keeps the start")

		long := short.Add(ErrorWindow + time.Second)
		f = *tr.observe(testPort, inputDown, long).Failures
		Expect(failuresIn(f)).To(Equal(1))
		Expect(f.InputsNotConnected.Since).To(Equal(long), "a gap longer than ErrorWindow starts over")
	})

	// A gap of exactly ErrorWindow is not a long gap: the errors before it
	// still count.
	It("does not start over on a gap of exactly ErrorWindow", func() {
		firedAt(5).at(failureT.Add(ErrorWindow), testPort, 5).wantFailure("first scrape exactly 10m later, counter still 5", 5, "10m ago")
	})
})
