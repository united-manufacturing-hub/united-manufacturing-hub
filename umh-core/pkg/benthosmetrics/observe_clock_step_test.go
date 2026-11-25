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

// The time since the last error across a wall-clock step and the store's JSON
// round-trip.

package benthosmetrics

import (
	"encoding/json"
	"time"
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// wallStepped returns t advanced by elapsed on the monotonic clock and by
// elapsed+step on the wall clock: a time.Now() read after an NTP step. The
// time package offers no way to build one, so it edits the monotonic field.
func wallStepped(t time.Time, elapsed, step time.Duration) time.Time {
	s := t.Add(elapsed + step)
	(*struct {
		wall uint64
		ext  int64
		loc  *time.Location
	})(unsafe.Pointer(&s)).ext -= int64(step)

	return s
}

var _ = Describe("A stored failure after a wall-clock step", func() {
	// An error fires, then NTP steps the wall clock forward 15 minutes while
	// only 2 minutes pass. The failure is still open, and once stored its last
	// error must still read as 2 minutes old.
	It("measures real elapsed time, not wall-clock time", func() {
		t0 := time.Now()
		Expect(wallStepped(t0, 2*time.Minute, 15*time.Minute).Sub(t0)).To(Equal(2*time.Minute),
			"wallStepped must keep the monotonic reading at 2m")

		tr := NewTracker(time.Now)
		tr.observe(testPort, outputErrors(0), t0.Add(-time.Second))
		tr.observe(testPort, outputErrors(2), t0)
		f := *tr.observe(testPort, outputErrors(2), wallStepped(t0, 2*time.Minute, 15*time.Minute)).Failures

		ok, _ := f.ErrorFree()
		Expect(ok).To(BeFalse(), "2 real minutes after the error, the failure is open")

		raw, err := json.Marshal(f)
		Expect(err).NotTo(HaveOccurred())

		var stored FailureReport
		Expect(json.Unmarshal(raw, &stored)).To(Succeed())

		Expect(stored.ObservedAt.Sub(stored.OutputErrors.LastErrorAt)).To(Equal(2*time.Minute),
			"stored failures after a 15m forward wall-clock step")
	})
})

var _ = Describe("A stored failure after a sparse error and a wall-clock step", func() {
	// The reading before the error is 9m59s old, so 5s after the error that
	// reading has left the window, and only the error's own scrape keeps the
	// failure open. NTP steps the wall clock forward 15 minutes in those 5s.
	It("re-dates the failure's LastErrorAt", func() {
		t0 := time.Now()

		tr := NewTracker(time.Now)
		tr.observe(testPort, outputErrors(0), t0.Add(-9*time.Minute-59*time.Second))
		tr.observe(testPort, outputErrors(2), t0)
		f := *tr.observe(testPort, outputErrors(2), wallStepped(t0, 5*time.Second, 15*time.Minute)).Failures

		ok, _ := f.ErrorFree()
		Expect(ok).To(BeFalse(), "5 real seconds after the error, the failure is open")

		raw, err := json.Marshal(f)
		Expect(err).NotTo(HaveOccurred())

		var stored FailureReport
		Expect(json.Unmarshal(raw, &stored)).To(Succeed())

		Expect(stored.ObservedAt.Sub(stored.OutputErrors.LastErrorAt)).To(Equal(5*time.Second),
			"stored failures after a 15m forward wall-clock step")
	})
})
