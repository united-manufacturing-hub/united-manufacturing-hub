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

// A new error right after an error entry cleared shows at once.

package benthosmetrics

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("A signal that just cleared", func() {
	// A new output error on the scrape after the signal released for an
	// earlier one makes the flow not error-free on that scrape, not one
	// window later.
	It("fires at once on the next error", func() {
		var secondAt time.Time

		tl := newErrorTimeline(failureT.Add(-time.Second), func(at time.Time) int64 {
			switch {
			case at.Before(failureT):
				return 0
			case secondAt.IsZero() || at.Before(secondAt):
				return 1
			default:
				return 2
			}
		})

		// T is failureT, the first output error. Find R, the first scrape at
		// which the flow is error-free again. It must come between T+10m and
		// T+11m.
		tl.runTo(failureT.Add(10*time.Minute - time.Second))
		var released time.Time
		for at := failureT.Add(10 * time.Minute); !at.After(failureT.Add(11 * time.Minute)); at = at.Add(time.Second) {
			tl.runTo(at)
			if ok, _ := tl.failures.ErrorFree(); ok {
				released = at

				break
			}
		}
		Expect(released).NotTo(BeZero(), "the signal did not release between T+10m and T+11m")

		secondAt = released.Add(time.Second)
		tl.runTo(secondAt)
		ok, reason := tl.failures.ErrorFree()
		Expect(ok).To(BeFalse(),
			"one second after the release at R=T+%v, a new error", released.Sub(failureT))
		Expect(reason).To(Equal("benthos reported 1 output error in the last 10 minutes"))

		Expect(tl.failures.Entries).To(HaveLen(1))
		Expect(tl.failures.Entries[0].Since).To(Equal(secondAt),
			"the re-fired entry starts at the new error")
	})
})
