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

package failurerate_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry/failurerate"
)

var _ = Describe("Failure Rate Tracker", func() {
	var tracker *failurerate.Tracker

	// Standard config for most tests: window of 100, 90% threshold, 10 min samples.
	defaultCfg := failurerate.Config{
		WindowSize: 100,
		Threshold:  0.9,
		MinSamples: 10,
	}

	BeforeEach(func() {
		tracker = failurerate.New(defaultCfg)
	})

	Describe("RecordOutcome()", func() {
		It("should increase failure rate as failures are recorded", func() {
			for range 50 {
				tracker.RecordOutcome(false)
			}
			Expect(tracker.FailureRate()).To(BeNumerically("==", 1.0))
		})

		It("should decrease failure rate when successes are recorded", func() {
			// Fill with 20 failures
			for range 20 {
				tracker.RecordOutcome(false)
			}
			Expect(tracker.FailureRate()).To(BeNumerically("==", 1.0))

			// Add 20 successes — rate should drop to 0.5
			for range 20 {
				tracker.RecordOutcome(true)
			}
			Expect(tracker.FailureRate()).To(BeNumerically("~", 0.5, 0.01))
		})

		It("should evict oldest outcomes when window is full", func() {
			// Fill the entire window (100) with failures
			for range 100 {
				tracker.RecordOutcome(false)
			}
			Expect(tracker.FailureRate()).To(BeNumerically("==", 1.0))

			// Now record 50 successes — oldest 50 failures are evicted
			for range 50 {
				tracker.RecordOutcome(true)
			}
			// Window now: 50 failures + 50 successes = 50% failure rate
			Expect(tracker.FailureRate()).To(BeNumerically("~", 0.5, 0.01))
		})

		It("should fire escalation at threshold crossing", func() {
			// Record enough failures to cross 90% threshold with min 10 samples.
			// 10 failures out of 10 = 100% > 90% threshold.
			for i := range 10 {
				fired := tracker.RecordOutcome(false)
				if i < 9 {
					Expect(fired).To(BeFalse(), "should not fire before MinSamples at i=%d", i)
				} else {
					Expect(fired).To(BeTrue(), "should fire at MinSamples when rate exceeds threshold")
				}
			}
		})

		It("should be one-shot: second call does not fire again", func() {
			// Cross threshold
			for range 10 {
				tracker.RecordOutcome(false)
			}
			// Already fired on the 10th call above, subsequent should NOT fire
			fired := tracker.RecordOutcome(false)
			Expect(fired).To(BeFalse(), "should not fire twice without rearm")
		})

		It("should rearm after recovery and fire again on next crossing", func() {
			// Cross threshold (fires on 10th)
			for range 10 {
				tracker.RecordOutcome(false)
			}
			Expect(tracker.IsEscalated()).To(BeTrue())

			// Add enough successes to drop below threshold.
			// Current: 10 failures out of 10. Need rate < 0.9.
			// Adding 2 successes: 10 failures out of 12 = 83% < 90%
			tracker.RecordOutcome(true)
			tracker.RecordOutcome(true)
			Expect(tracker.IsEscalated()).To(BeFalse(), "should de-escalate after recovery")

			// Now cross threshold again — should fire again
			// Fill with failures until we cross 90% again
			var firedAgain bool
			for range 20 {
				if tracker.RecordOutcome(false) {
					firedAgain = true
					break
				}
			}
			Expect(firedAgain).To(BeTrue(), "should fire again after rearm")
		})

		It("should fire escalation at exact threshold boundary (rate == threshold)", func() {
			// 9 failures + 1 success = 90% rate == 0.9 threshold — should fire (>= comparator)
			cfg := failurerate.Config{
				WindowSize: 100,
				Threshold:  0.9,
				MinSamples: 10,
			}
			t := failurerate.New(cfg)

			for range 9 {
				t.RecordOutcome(false)
			}
			t.RecordOutcome(true)
			// 9/10 = 0.9 == threshold — should have fired on the 10th outcome
			Expect(t.IsEscalated()).To(BeTrue(), "should escalate when rate exactly equals threshold")
		})

		It("should not escalate before MinSamples even at 100% failure rate", func() {
			cfg := failurerate.Config{
				WindowSize: 100,
				Threshold:  0.9,
				MinSamples: 50,
			}
			t := failurerate.New(cfg)

			for range 49 {
				fired := t.RecordOutcome(false)
				Expect(fired).To(BeFalse(), "should not fire before MinSamples")
			}
			Expect(t.FailureRate()).To(BeNumerically("==", 0.0),
				"FailureRate should return 0.0 below MinSamples")

			// The 50th outcome should fire
			fired := t.RecordOutcome(false)
			Expect(fired).To(BeTrue(), "should fire at MinSamples")
		})
	})

	Describe("FailureRate()", func() {
		It("should return 0.0 when no outcomes are recorded", func() {
			Expect(tracker.FailureRate()).To(BeNumerically("==", 0.0))
		})

		It("should return 0.0 below MinSamples", func() {
			for range 9 {
				tracker.RecordOutcome(false)
			}
			Expect(tracker.FailureRate()).To(BeNumerically("==", 0.0))
		})

		It("should return correct rate at MinSamples", func() {
			// 7 failures + 3 successes = 70%
			for range 7 {
				tracker.RecordOutcome(false)
			}
			for range 3 {
				tracker.RecordOutcome(true)
			}
			Expect(tracker.FailureRate()).To(BeNumerically("~", 0.7, 0.01))
		})
	})

	Describe("IsEscalated()", func() {
		It("should return false initially", func() {
			Expect(tracker.IsEscalated()).To(BeFalse())
		})

		It("should return true after threshold crossing", func() {
			for range 10 {
				tracker.RecordOutcome(false)
			}
			Expect(tracker.IsEscalated()).To(BeTrue())
		})

		It("should return false after recovery", func() {
			for range 10 {
				tracker.RecordOutcome(false)
			}
			// Recover
			tracker.RecordOutcome(true)
			tracker.RecordOutcome(true)
			Expect(tracker.IsEscalated()).To(BeFalse())
		})
	})

	Describe("Reset()", func() {
		It("should clear all state", func() {
			for range 50 {
				tracker.RecordOutcome(false)
			}
			Expect(tracker.IsEscalated()).To(BeTrue())
			Expect(tracker.FailureRate()).To(BeNumerically(">", 0))

			tracker.Reset()

			Expect(tracker.IsEscalated()).To(BeFalse())
			Expect(tracker.FailureRate()).To(BeNumerically("==", 0.0))

			// Should be able to escalate again after reset
			for range 10 {
				tracker.RecordOutcome(false)
			}
			Expect(tracker.IsEscalated()).To(BeTrue())
		})
	})

	Describe("SetEscalatedForTest()", func() {
		It("should set escalated flag directly", func() {
			Expect(tracker.IsEscalated()).To(BeFalse())

			tracker.SetEscalatedForTest(true)
			Expect(tracker.IsEscalated()).To(BeTrue())

			tracker.SetEscalatedForTest(false)
			Expect(tracker.IsEscalated()).To(BeFalse())
		})
	})

	Describe("Thread Safety", func() {
		It("should handle concurrent RecordOutcome and Reset calls without panic", func() {
			const goroutines = 10
			const iterations = 100

			var wg sync.WaitGroup
			wg.Add(goroutines * 4)

			// Concurrent failures
			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						tracker.RecordOutcome(false)
					}
				}()
			}

			// Concurrent successes
			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						tracker.RecordOutcome(true)
					}
				}()
			}

			// Concurrent reads
			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						_ = tracker.FailureRate()
						_ = tracker.IsEscalated()
					}
				}()
			}

			// Concurrent resets
			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						tracker.Reset()
					}
				}()
			}

			wg.Wait()
			// If we get here without deadlock or panic, the test passes
		})
	})

	Describe("Config Validation", func() {
		It("should panic on WindowSize=0", func() {
			Expect(func() {
				failurerate.New(failurerate.Config{WindowSize: 0, Threshold: 0.9, MinSamples: 0})
			}).To(Panic())
		})

		It("should panic on negative WindowSize", func() {
			Expect(func() {
				failurerate.New(failurerate.Config{WindowSize: -1, Threshold: 0.9, MinSamples: 0})
			}).To(Panic())
		})

		It("should panic on Threshold=0", func() {
			Expect(func() {
				failurerate.New(failurerate.Config{WindowSize: 100, Threshold: 0.0, MinSamples: 10})
			}).To(Panic())
		})

		It("should panic on Threshold > 1.0", func() {
			Expect(func() {
				failurerate.New(failurerate.Config{WindowSize: 100, Threshold: 1.5, MinSamples: 10})
			}).To(Panic())
		})

		It("should panic on MinSamples > WindowSize", func() {
			Expect(func() {
				failurerate.New(failurerate.Config{WindowSize: 10, Threshold: 0.9, MinSamples: 20})
			}).To(Panic())
		})

		It("should not panic on valid edge configs", func() {
			// Threshold=1.0 is valid (only 100% failure escalates)
			Expect(func() {
				failurerate.New(failurerate.Config{WindowSize: 1, Threshold: 1.0, MinSamples: 0})
			}).NotTo(Panic())

			// MinSamples=0 is valid (no startup protection)
			Expect(func() {
				failurerate.New(failurerate.Config{WindowSize: 100, Threshold: 0.5, MinSamples: 0})
			}).NotTo(Panic())
		})
	})

	Describe("Circular Buffer Wraparound", func() {
		It("should correctly maintain failure count through multiple wraparounds", func() {
			cfg := failurerate.Config{
				WindowSize: 10,
				Threshold:  0.9,
				MinSamples: 5,
			}
			t := failurerate.New(cfg)

			// Fill with 10 failures (full window)
			for range 10 {
				t.RecordOutcome(false)
			}
			Expect(t.FailureRate()).To(BeNumerically("==", 1.0))

			// Wrap around: add 10 successes — evicts all 10 failures
			for range 10 {
				t.RecordOutcome(true)
			}
			Expect(t.FailureRate()).To(BeNumerically("==", 0.0))

			// Wrap again: add 5 failures — now 5 failures + 5 successes
			for range 5 {
				t.RecordOutcome(false)
			}
			Expect(t.FailureRate()).To(BeNumerically("~", 0.5, 0.01))
		})
	})
})
