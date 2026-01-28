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

package dependency_test

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/dependency"
)

func TestDependency(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dependency Suite")
}

var _ = Describe("StateTracker", func() {
	var (
		mockClock *clock.Mock
		tracker   dependency.StateTracker
	)

	BeforeEach(func() {
		mockClock = clock.NewMock()
		tracker = dependency.NewDefaultStateTracker(mockClock)
	})

	Describe("NewDefaultStateTracker", func() {
		It("should initialize with current time as enteredAt", func() {
			Expect(tracker.GetStateEnteredAt()).To(Equal(mockClock.Now()))
		})

		It("should initialize with empty state", func() {
			Expect(tracker.GetCurrentState()).To(BeEmpty())
		})

		It("should use real clock when nil is passed", func() {
			realTracker := dependency.NewDefaultStateTracker(nil)
			Expect(realTracker).NotTo(BeNil())
			// Just verify it works - can't easily test real time
			Expect(realTracker.GetCurrentState()).To(BeEmpty())
		})
	})

	Describe("RecordStateChange", func() {
		It("should update state and enteredAt when state changes", func() {
			initialTime := mockClock.Now()

			// Advance clock and change state
			mockClock.Add(5 * time.Second)
			tracker.RecordStateChange("running")

			Expect(tracker.GetCurrentState()).To(Equal("running"))
			Expect(tracker.GetStateEnteredAt()).To(Equal(initialTime.Add(5 * time.Second)))
		})

		It("should not update enteredAt when state is the same", func() {
			// Set initial state
			tracker.RecordStateChange("running")
			enteredAt := tracker.GetStateEnteredAt()

			// Advance clock but record same state
			mockClock.Add(10 * time.Second)
			tracker.RecordStateChange("running")

			// enteredAt should not change
			Expect(tracker.GetStateEnteredAt()).To(Equal(enteredAt))
		})

		It("should update enteredAt when transitioning to different state", func() {
			tracker.RecordStateChange("stopped")
			mockClock.Add(5 * time.Second)

			tracker.RecordStateChange("running")
			runningEnteredAt := tracker.GetStateEnteredAt()

			mockClock.Add(10 * time.Second)
			tracker.RecordStateChange("stopped")

			Expect(tracker.GetStateEnteredAt()).To(Equal(runningEnteredAt.Add(10 * time.Second)))
		})
	})

	Describe("Elapsed", func() {
		It("should return duration since state was entered", func() {
			tracker.RecordStateChange("running")

			mockClock.Add(5 * time.Second)
			Expect(tracker.Elapsed()).To(Equal(5 * time.Second))

			mockClock.Add(3 * time.Second)
			Expect(tracker.Elapsed()).To(Equal(8 * time.Second))
		})

		It("should reset elapsed when state changes", func() {
			tracker.RecordStateChange("running")
			mockClock.Add(10 * time.Second)
			Expect(tracker.Elapsed()).To(Equal(10 * time.Second))

			// Change state - elapsed should reset
			tracker.RecordStateChange("stopped")
			Expect(tracker.Elapsed()).To(Equal(0 * time.Second))

			// Advance clock after state change
			mockClock.Add(3 * time.Second)
			Expect(tracker.Elapsed()).To(Equal(3 * time.Second))
		})
	})

	Describe("Concurrent access", func() {
		It("should be safe for concurrent reads and writes", func() {
			done := make(chan struct{})

			// Writer goroutine
			go func() {
				defer close(done)
				for i := range 100 {
					if i%2 == 0 {
						tracker.RecordStateChange("running")
					} else {
						tracker.RecordStateChange("stopped")
					}
				}
			}()

			// Reader goroutines
			for range 3 {
				go func() {
					for range 100 {
						_ = tracker.GetCurrentState()
						_ = tracker.GetStateEnteredAt()
						_ = tracker.Elapsed()
					}
				}()
			}

			Eventually(done).Should(BeClosed())
		})
	})
})
