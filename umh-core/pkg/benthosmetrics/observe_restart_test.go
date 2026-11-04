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

// Benthos restarts and scrape gaps: which errors survive which.

package benthosmetrics

import (
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// scrapes is one Tracker and the failures of its latest observe call.
type scrapes struct {
	tracker  *Tracker
	failures Failures
}

func newScrapes() *scrapes {
	return &scrapes{tracker: NewTracker(func() time.Time { return failureT })}
}

// at observes one scrape of the Benthos at port with output error total n.
func (s *scrapes) at(at time.Time, port int, n int64) *scrapes {
	_, s.failures = s.tracker.observe(port, outputErrors(n), at)

	return s
}

// wantErrorFree fails the spec unless the latest failures are error-free.
func (s *scrapes) wantErrorFree(when string) {
	ok, reason := s.failures.ErrorFree()
	Expect(ok).To(BeTrue(), "%s: %q", when, reason)
}

// wantReason fails the spec unless the latest failures are not error-free with
// exactly this reason.
func (s *scrapes) wantReason(when, want string) {
	ok, reason := s.failures.ErrorFree()
	Expect(ok).To(BeFalse(), "%s: %q, want %q", when, reason, want)
	Expect(reason).To(Equal(want))
}

// firedAt returns scrapes whose output_error went 0 -> n at failureT, so the
// signal fired at failureT.
func firedAt(n int64) *scrapes {
	s := newScrapes().at(failureT.Add(-time.Second), testPort, 0).at(failureT, testPort, n)
	s.wantFailure("setup, at T", n)

	return s
}

// wantFailure fails the spec unless the latest failures name n output errors.
func (s *scrapes) wantFailure(when string, n int64) {
	label := "output errors"
	if n == 1 {
		label = "output error"
	}
	s.wantReason(when, "benthos reported "+strconv.FormatInt(n, 10)+" "+label+" in the last 10 minutes")
}

var _ = Describe("Benthos restarts and scrape gaps", func() {
	// The config is fixed, Benthos restarts, and its counter starts again at
	// 0. The seed adds no false rise.
	It("clears at once on a restart to zero", func() {
		firedAt(3).at(failureT.Add(time.Second), testPort, 0).wantErrorFree("first scrape after a restart to 0")
	})

	// Benthos restarts and counts 3 errors before the first scrape after the
	// restart. The counter dropped from 7, yet the 3 count as new errors.
	It("counts errors before the first scrape after a restart", func() {
		s := newScrapes().at(failureT.Add(-time.Second), testPort, 7).at(failureT, testPort, 7)
		s.wantErrorFree("before the restart, counter flat at 7")
		s.at(failureT.Add(time.Second), testPort, 3).wantFailure("first scrape after a restart, counter 3", 3)
	})

	// Benthos restarts and counts 3 errors before the first scrape after the
	// restart, and that first scrape comes 15 minutes after the last one.
	It("counts errors before the first scrape after a restart and a long gap", func() {
		s := newScrapes().at(failureT.Add(-time.Second), testPort, 7).at(failureT, testPort, 7)
		s.at(failureT.Add(15*time.Minute), testPort, 3).wantFailure("first scrape 15m later, after a restart, counter 3", 3)
	})

	// Scrapes fail for 15 minutes while Benthos keeps counting errors.
	It("counts a rise during a long outage", func() {
		firedAt(5).at(failureT.Add(15*time.Minute), testPort, 8).wantFailure("first scrape after a 15m outage, counter 5 -> 8", 3)
	})

	// Scrapes fail for 15 minutes while Benthos counts no new error.
	It("releases with no rise during a long outage", func() {
		firedAt(5).at(failureT.Add(15*time.Minute), testPort, 5).wantErrorFree("first scrape after a 15m outage, counter still 5")
	})

	// Scrapes fail for just under the 10-minute window while Benthos counts
	// no new error. The window still holds the rise, so the signal stays
	// fired. A rebuild on a shorter gap would release it.
	It("keeps the signal across a gap shorter than the window", func() {
		firedAt(5).at(failureT.Add(10*time.Minute-2*time.Second), testPort, 5).wantFailure("first scrape after a 9m58s outage, counter still 5", 5)
	})

	// The metrics port changes without a worker restart, and the Benthos on
	// the new port reports 0.
	It("reads a port change as a restart", func() {
		firedAt(3).at(failureT.Add(time.Second), testPort+1, 0).wantErrorFree("first scrape on a new port, counter 0")
	})

	// The metrics port changes without a worker restart, and the Benthos on
	// the new port has already counted 2 output errors by the first scrape:
	// they count, as the errors before a restart's first scrape do.
	It("counts errors the new port's Benthos already reported", func() {
		s := firedAt(3).at(failureT.Add(time.Second), testPort+1, 2)
		s.wantFailure("first scrape on a new port, counter 2", 2)
		s.at(failureT.Add(2*time.Second), testPort+1, 2).wantFailure("next scrape on the new port, counter still 2", 2)
	})

	// A scrape stamped at the same instant as the previous one shows a dropped
	// counter; the next scrape, one second later, shows the same drop.
	It("waits for the next scrape on a drop on a same-instant scrape", func() {
		s := firedAt(3)
		s.at(failureT, testPort, 0).wantFailure("same-instant scrape with a dropped counter", 3)
		s.at(failureT.Add(time.Second), testPort, 0).wantErrorFree("next scrape, counter still 0")
	})
})
