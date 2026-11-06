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

// NotCleared across a wall-clock step and the store's JSON round-trip.

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

var _ = Describe("NotCleared after a wall-clock step", func() {
	// An error fires, then NTP steps the wall clock forward 15 minutes while
	// only 2 minutes pass. The entry is still held, and once stored it must
	// not read as stuck past its window.
	It("measures real elapsed time, not wall-clock time", func() {
		t0 := time.Now()
		Expect(wallStepped(t0, 2*time.Minute, 15*time.Minute).Sub(t0)).To(Equal(2*time.Minute),
			"wallStepped must keep the monotonic reading at 2m")

		tr := NewTracker(nil)
		tr.observe(testPort, outputErrors(0), t0.Add(-time.Second))
		tr.observe(testPort, outputErrors(2), t0)
		_, f := tr.observe(testPort, outputErrors(2), wallStepped(t0, 2*time.Minute, 15*time.Minute))

		ok, _ := f.ErrorFree()
		Expect(ok).To(BeFalse(), "2 real minutes after the error, the entry is held")

		raw, err := json.Marshal(f)
		Expect(err).NotTo(HaveOccurred())
		var stored Failures
		Expect(json.Unmarshal(raw, &stored)).To(Succeed())

		Expect(stored.NotCleared(time.Minute)).To(BeFalse(), "stored failures after a 15m forward wall-clock step")
	})
})
