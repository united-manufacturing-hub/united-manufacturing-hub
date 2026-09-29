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

package diagnosis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The span and tick interval used by the coverage specs here and by the latch spec
// in latch_irregular_interval_test.go.
const (
	windowSpan = 60 * time.Second
	// A constant offset, not random jitter: under jitter a release still happens, at
	// a random delay, and the specs flake.
	nonDividingInterval = 1001 * time.Millisecond
)

var _ = Describe("SlidingWindow coverage", func() {
	It("should run its specs at a tick interval that does not divide the span", func() {
		// At 1000ms the specs keyed on this interval pass against the defect they
		// exist to catch. A one-character edit, so it is pinned rather than trusted.
		Expect(windowSpan % nonDividingInterval).NotTo(BeZero())
	})

	It("should report a full window on a tick interval that does not divide the span", func() {
		w, err := NewSlidingWindow(windowSpan, windowSpan, Last, false)
		Expect(err).NotTo(HaveOccurred())

		base := time.Unix(1_000_000, 0)
		for i := range 120 {
			w.Observe(Known(0.5), Unknown(), base.Add(time.Duration(i)*nonDividingInterval))
		}

		Expect(w.Coverage().Full()).To(BeTrue(),
			"the window collected for longer than its span, so it should report full whatever the interval")
	})

	It("should not report a full window before it has collected for its span", func() {
		w, err := NewSlidingWindow(windowSpan, windowSpan, Last, false)
		Expect(err).NotTo(HaveOccurred())

		// Two readings, so the answer turns on the measurement rather than on having
		// too little to measure.
		base := time.Unix(1_000_000, 0)
		w.Observe(Known(0.5), Unknown(), base)
		w.Observe(Known(0.5), Unknown(), base.Add(nonDividingInterval))

		Expect(w.Coverage().Full()).To(BeFalse(),
			"a window two ticks old reported a full span, so a latch could release before the window has collected for one")
	})
})
