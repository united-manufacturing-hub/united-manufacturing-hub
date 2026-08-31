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

var _ = Describe("A latch driven from a real SlidingWindow", func() {
	// windowSpan and nonDividingInterval come from sliding_window_coverage_test.go,
	// which also pins the interval against dividing the span.
	const (
		ticks   = 120
		dropsAt = 90
	)

	marks := func() Marks {
		return Marks{
			Fire:     Mark{At: 0.10, Inclusive: false},
			Clear:    Mark{At: 0.06, Inclusive: false},
			Polarity: HigherIsWorse,
		}
	}

	It("should release once the value crosses the clear mark, on a tick interval that does not divide the span", func() {
		w, err := NewSlidingWindow(windowSpan, windowSpan, Last, false)
		Expect(err).NotTo(HaveOccurred())

		l := NewLatch(Identity{})
		base := time.Unix(1_000_000, 0)

		firedBeforeDrop := false

		for i := range ticks {
			at := base.Add(time.Duration(i) * nonDividingInterval)

			v := 0.20
			if i >= dropsAt {
				v = 0.02
			}

			w.Observe(Known(v), Unknown(), at)
			l.Update("probe", w.Reduce(), w.Coverage(), marks(), at)

			if i == dropsAt-1 {
				_, firedBeforeDrop = l.Fired()
			}
		}

		Expect(firedBeforeDrop).To(BeTrue(),
			"the value above the fire mark never fired the latch, so the release below is not being tested at all")

		_, fired := l.Fired()
		Expect(fired).To(BeFalse(),
			"the value sat below the clear mark for %d ticks and the window had been collecting for longer than %s, but the latch never released",
			ticks-dropsAt, windowSpan)
	})
})
