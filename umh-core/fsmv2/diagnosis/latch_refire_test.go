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

// These specs pin when a released latch may fire again: one whose
// immediateRefire is set fires on the reading past its fire mark after a
// release, and an ordinary latch keeps its cooldown of one window span.

package diagnosis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("A latch that may fire again after a release", func() {
	const latchSpan = 60 * time.Second

	march := func() Marks {
		return Marks{
			Fire:     Mark{At: 0.10, Inclusive: false},
			Clear:    Mark{At: 0.06, Inclusive: false},
			Polarity: HigherIsWorse,
		}
	}
	full := func() Coverage { return Coverage{span: latchSpan, collectedFor: latchSpan} }

	// The same timeline runs with immediateRefire false and true: fire at t0,
	// release at t0+1s, then a reading past the fire mark at t0+2s. Only the
	// latch with immediateRefire set fires on that last reading.
	It("fires on the reading past its fire mark after a release only when its immediateRefire is set", func() {
		t0 := time.Unix(1_000_000, 0)
		for _, refire := range []bool{false, true} {
			l := NewLatch(Identity{})
			l.immediateRefire = refire

			l.Update("march", Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
			_, firstFired := l.Fired()
			Expect(firstFired).To(BeTrue(), "the latch fired at t0 (refire=%t)", refire)
			l.Update("march", Reduced{v: 0.01, state: StateValue}, full(), march(), t0.Add(time.Second))
			_, fired := l.Fired()
			Expect(fired).To(BeFalse(), "the latch released at t0+1s")

			l.Update("march", Reduced{v: 0.08, state: StateValue}, full(), march(), t0.Add(2*time.Second))
			_, fired = l.Fired()
			Expect(fired).To(BeFalse(), "a reading between Clear and Fire does not refire (refire=%t)", refire)

			l.Update("march", Reduced{v: 0.20, state: StateValue}, full(), march(), t0.Add(2*time.Second))
			_, fired = l.Fired()
			Expect(fired).To(Equal(refire), "one second after a release, only a latch with immediateRefire set fires (refire=%t)", refire)
			if refire {
				f, _ := l.Fired()
				Expect(f.Since).To(Equal(t0.Add(2*time.Second)), "Since of the re-fired latch is the re-fire time t0+2s, not the first fire at t0")
				l.Update("march", Reduced{v: 0.20, state: StateValue}, full(), march(), t0.Add(3*time.Second))
				f, _ = l.Fired()
				Expect(f.Since).To(Equal(t0.Add(2*time.Second)), "a second reading past the fire mark while fired keeps Since")
			}
		}
	})
})
