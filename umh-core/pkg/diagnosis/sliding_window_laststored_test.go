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

// lastStored reads the newest instant off the points slice instead of keeping a
// field for it. That is correct only while points are appended at the tail,
// dropped as a PREFIX (prune), or cleared WHOLESALE (demote, counter restart) —
// an invariant currently held by a comment on prune and by nothing else. The
// demote-boundary specs elsewhere pass whether or not that shape changes, so
// these assert the outcome the derivation promises: lastStored is the instant of
// the most recent stored reading. Change prune to drop from anywhere but the
// front and these fail; that is what they are for.
var _ = Describe("SlidingWindow.lastStored", func() {

	It("should be the instant just stored, on every tick, while pruning is actively dropping entries", func() {
		// span 5s with points 2s apart, so prune drops on most ticks. demoteSpan
		// is an hour so the demote arm cannot interfere.
		const span = 5 * time.Second
		w, err := NewSlidingWindow(span, time.Hour, Mean, false)
		Expect(err).ToNot(HaveOccurred())

		t0 := time.Unix(9_000_000, 0)
		maxHeld := 0

		for i := range 10 {
			at := t0.Add(time.Duration(i) * 2 * time.Second)
			w.age(at)
			w.appendPoint(Known(float64(i)), Unknown(), at)

			Expect(w.lastStored()).To(Equal(at),
				"tick %d: lastStored must be the instant just stored, not an older surviving point", i)

			if len(w.points) > maxHeld {
				maxHeld = len(w.points)
			}
		}

		// Without this the spec would pass on a window that never pruned at all,
		// which is the case it exists to exercise. 10 points 2s apart against a
		// 5s span cannot all be held.
		Expect(maxHeld).To(BeNumerically("<", 10),
			"prune must actually have dropped entries, or this spec proves nothing about the surviving order")
	})

	It("should be the instant stored after a wholesale clear, not the zero time and not the cleared point", func() {
		// A counter going backwards clears the whole slice and then stores.
		w, err := NewSlidingWindow(time.Hour, time.Hour, Mean, true)
		Expect(err).ToNot(HaveOccurred())

		t0 := time.Unix(9_100_000, 0)
		w.age(t0)
		w.appendPoint(Known(10), Unknown(), t0)

		back := t0.Add(time.Second)
		w.age(back)
		w.appendPoint(Known(5), Unknown(), back) // 5 < 10 on a counter: restart

		Expect(w.points).To(HaveLen(1),
			"the restart cleared wholesale and stored the new reading, or this is not the path under test")
		Expect(w.lastStored()).To(Equal(back),
			"a wholesale clear leaves the instant of the reading stored after it")
	})

	It("should be the zero time only when the window holds nothing", func() {
		w, err := NewSlidingWindow(time.Hour, time.Hour, Mean, false)
		Expect(err).ToNot(HaveOccurred())

		Expect(w.lastStored().IsZero()).To(BeTrue(),
			"a window that has never stored has no last instant")

		t0 := time.Unix(9_200_000, 0)
		w.age(t0)
		w.appendPoint(Known(1), Unknown(), t0)
		Expect(w.lastStored().IsZero()).To(BeFalse(),
			"one stored reading is enough to have one")

		// A failed read stores nothing, so the instant must not move to it.
		w.age(t0.Add(time.Second))
		w.appendPoint(Unknown(), Unknown(), t0.Add(time.Second))
		Expect(w.lastStored()).To(Equal(t0),
			"an unsuccessful read stores nothing, so the last STORED instant is unchanged")
	})
})
