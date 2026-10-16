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

package fsmv2benthosmonitor

import (
	"testing"
	"time"
)

// TestObserveFailureRightAfterReleaseFiresAtOnce: a new output error on the
// scrape after the signal released for an earlier one makes the metrics not
// error-free on that scrape, not one window later.
func TestObserveFailureRightAfterReleaseFiresAtOnce(t *testing.T) {
	var secondAt time.Time

	tl := newErrorTimeline(t, failureT.Add(-time.Second), func(at time.Time) int64 {
		switch {
		case at.Before(failureT):
			return 0
		case secondAt.IsZero() || at.Before(secondAt):
			return 1
		default:
			return 2
		}
	})

	// T is failureT, the first output error. Find R, the first scrape at which
	// the metrics are error-free again. It must come between T+10m and T+11m.
	tl.runTo(failureT.Add(10*time.Minute - time.Second))
	var released time.Time
	for at := failureT.Add(10 * time.Minute); !at.After(failureT.Add(11 * time.Minute)); at = at.Add(time.Second) {
		tl.runTo(at)
		if ok, _ := tl.errorFree(); ok {
			released = at

			break
		}
	}
	if released.IsZero() {
		t.Fatalf("the signal did not release between T+10m and T+11m")
	}

	secondAt = released.Add(time.Second)
	tl.runTo(secondAt)
	ok, reason := tl.errorFree()
	if ok || reason != "benthos reported 1 output error in the last 10m0s" {
		t.Fatalf("one second after the release at R=T+%v, a new error: ErrorFree() = (%v, %q), want (false, %q)",
			released.Sub(failureT), ok, reason, "benthos reported 1 output error in the last 10m0s")
	}
	if since := tl.verdict.Fired[0].Since; !since.Equal(secondAt) {
		t.Errorf("re-fired entry Since = T+%v, want R+1s = T+%v", since.Sub(failureT), secondAt.Sub(failureT))
	}
}
