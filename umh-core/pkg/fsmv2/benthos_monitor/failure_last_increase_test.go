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

	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// lastIncreaseAfter observes processor_error n at time at on d and returns the
// verdict's LastIncreaseAt.
func lastIncreaseAfter(d *benthosMonitorDeps, at time.Time, n int64) time.Time {
	_, v := observe(d, testPort, failureScrape(map[string]int64{"processor_error": n}), at)

	return v.LastIncreaseAt
}

// TestObserveLastIncreaseAtFollowsRawCounters: LastIncreaseAt is the latest
// scrape at which an error counter was higher than at the scrape before. It
// stays put while the counter is flat, both before and after the
// processor_error signal that fired at T clears.
func TestObserveLastIncreaseAtFollowsRawCounters(t *testing.T) {
	d := &benthosMonitorDeps{}

	if got := lastIncreaseAfter(d, failureT.Add(-time.Second), 0); !got.IsZero() {
		t.Fatalf("first scrape: LastIncreaseAt = %v, want zero (no earlier scrape to compare)", got)
	}

	for at := failureT; !at.After(failureT.Add(20 * time.Minute)); at = at.Add(time.Second) {
		if got := lastIncreaseAfter(d, at, 3); !got.Equal(failureT) {
			t.Fatalf("at T+%v with processor_error flat at 3 since T: LastIncreaseAt = T+%v, want T", at.Sub(failureT), got.Sub(failureT))
		}
	}
}

// TestObserveLastIncreaseAtAfterRestart: after a restart the previous values
// are 0, so a counter above 0 on the first scrape after it is an increase, and
// a counter at 0 is not.
func TestObserveLastIncreaseAtAfterRestart(t *testing.T) {
	d := &benthosMonitorDeps{}
	lastIncreaseAfter(d, failureT.Add(-time.Second), 0)
	lastIncreaseAfter(d, failureT, 3)

	restartAt := failureT.Add(time.Minute)
	if got := lastIncreaseAfter(d, restartAt, 2); !got.Equal(restartAt) {
		t.Fatalf("first scrape after a restart, counter 3 -> 2: LastIncreaseAt = T+%v, want T+1m", got.Sub(failureT))
	}

	d2 := &benthosMonitorDeps{}
	lastIncreaseAfter(d2, failureT.Add(-time.Second), 0)
	lastIncreaseAfter(d2, failureT, 3)
	if got := lastIncreaseAfter(d2, failureT.Add(time.Minute), 0); !got.Equal(failureT) {
		t.Fatalf("first scrape after a restart to 0: LastIncreaseAt = T+%v, want T (no increase)", got.Sub(failureT))
	}
}

// TestObserveLastIncreaseAtAfterLongGap: after a gap longer than the window,
// the previous values are the last scrape's, so only a rise during the gap
// counts.
func TestObserveLastIncreaseAtAfterLongGap(t *testing.T) {
	d := &benthosMonitorDeps{}
	lastIncreaseAfter(d, failureT.Add(-time.Second), 0)
	lastIncreaseAfter(d, failureT, 3)

	if got := lastIncreaseAfter(d, failureT.Add(15*time.Minute), 3); !got.Equal(failureT) {
		t.Fatalf("15m gap, counter flat at 3: LastIncreaseAt = T+%v, want T", got.Sub(failureT))
	}
	if got := lastIncreaseAfter(d, failureT.Add(30*time.Minute), 4); !got.Equal(failureT.Add(30 * time.Minute)) {
		t.Fatalf("15m gap, counter 3 -> 4: LastIncreaseAt = T+%v, want T+30m", got.Sub(failureT))
	}
}

// TestObserveLastIncreaseAtMovesOnEveryRise: every rise moves LastIncreaseAt,
// while a signal is still fired and on any counter: processor_error rises at T
// and again at T+2m, output_error at T+5m.
func TestObserveLastIncreaseAtMovesOnEveryRise(t *testing.T) {
	d := &benthosMonitorDeps{}
	scrape := func(at time.Time, processor, output int64) benthosmonitorservice.FailureVerdict {
		_, v := observe(d, testPort, failureScrape(map[string]int64{"processor_error": processor, "output_error": output}), at)

		return v
	}

	scrape(failureT.Add(-time.Second), 0, 0)
	scrape(failureT, 3, 0)
	if got := scrape(failureT.Add(2*time.Minute), 4, 0).LastIncreaseAt; !got.Equal(failureT.Add(2 * time.Minute)) {
		t.Fatalf("processor_error 3 -> 4 while fired: LastIncreaseAt = T+%v, want T+2m", got.Sub(failureT))
	}
	if got := scrape(failureT.Add(5*time.Minute), 4, 1).LastIncreaseAt; !got.Equal(failureT.Add(5 * time.Minute)) {
		t.Fatalf("output_error 0 -> 1: LastIncreaseAt = T+%v, want T+5m", got.Sub(failureT))
	}
}

// TestObserveLastIncreaseAtKeptOnSameInstantScrape: a scrape stamped at the
// same instant as the previous one returns the previous verdict,
// LastIncreaseAt included, and the next scrape keeps it.
func TestObserveLastIncreaseAtKeptOnSameInstantScrape(t *testing.T) {
	d := &benthosMonitorDeps{}
	lastIncreaseAfter(d, failureT.Add(-time.Second), 0)
	lastIncreaseAfter(d, failureT, 3)

	if got := lastIncreaseAfter(d, failureT, 9); !got.Equal(failureT) {
		t.Fatalf("same-instant scrape: LastIncreaseAt = T+%v, want T", got.Sub(failureT))
	}
	if got := lastIncreaseAfter(d, failureT.Add(time.Second), 3); !got.Equal(failureT) {
		t.Fatalf("next scrape, counter flat at 3: LastIncreaseAt = T+%v, want T", got.Sub(failureT))
	}
}
