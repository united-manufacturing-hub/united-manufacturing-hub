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
	"math"
	"testing"
	"time"
)

// TestThroughputWindowWipesOnCounterReset pins the window-reset behaviour. When
// the process restarts, both Prometheus counters zero, so a drop in BOTH counters
// against the immediately preceding sample is the restart signal: the window must
// FULL-WIPE and re-seed with only the new sample. The window persists across a
// child restart, so a both-counter drop is the signal the by-time series changed
// and the old span is no longer comparable. A drop in only ONE counter (a config
// reload or transient scrape gap) is not a restart and must not wipe — pinned by
// TestThroughputWindowDoesNotWipeOnOneSidedDrop.
//
// The rate is computed from the post-restart baseline alone. Without the wipe, a
// newest-vs-oldest-in-window comparison would let a restarted counter that climbs
// BACK ABOVE the pre-restart count within the span misreport the
// recovered-above-baseline rate for up to 60s; wiping at the drop tick reads only
// the post-restart baseline, so the recovered rate is not diluted by the
// pre-restart count.
//
// After a full-wipe the window holds a single sample, so the rate reads 0 and,
// by extension, IsActive is false on the reset tick — never FSMv1's cumulative-
// count-as-rate spike. The status-level IsActive=false on the reset tick is
// pinned by TestPollResetTickReportsIsActiveFalse.
func TestThroughputWindowWipesOnCounterReset(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// (a) Counter-drop full-wipe: baseline 100 @ t0, restart drops it to 5 @
	// t0+10s. The wipe must leave the window holding ONLY the new sample, not
	// append it alongside the old.
	w := &throughputWindow{}
	w.Add(t0, 100, 100)
	w.Add(t0.Add(10*time.Second), 5, 5)
	if got := len(w.samples); got != 1 {
		t.Errorf("after counter drop the window holds %d samples, want 1 (a both-counter drop wipes and re-seeds)", got)
	}

	// (d) First tick after the wipe: a single sample cannot compute a rate, so
	// MessagesPerSecond must be 0 (never the cumulative count as a rate). This is
	// documentation-intent only. The discriminating assertion is len(samples)==1
	// above: a revert that appends instead of wiping leaves two samples and fails
	// there.
	if r := w.inputRate(); r != 0 {
		t.Errorf("first tick after counter-drop wipe: Input MessagesPerSecond = %v, want 0", r)
	}
	if r := w.outputRate(); r != 0 {
		t.Errorf("first tick after counter-drop wipe: Output MessagesPerSecond = %v, want 0", r)
	}

	// Post-restart clean second sample 1s later: the rate is computed from the
	// post-restart baseline only (5 -> 12 over ~1s ~= 7/s), not diluted by the
	// pre-restart 100.
	w.Add(t0.Add(11*time.Second), 12, 12)
	if r := w.inputRate(); r < 6 || r > 8 {
		t.Errorf("post-restart input rate = %v, want ~7/s (5->12 over 1s, not diluted by pre-restart 100)", r)
	}

	// (b) Recovered above the pre-restart baseline: {100 @ t0, 0 @ t0+15s,
	// 150 @ t0+30s}. The 0 tick is a both-counter drop so it wipes the 100; the
	// true post-restart rate is 0 -> 150 over 15s = 10/s. A newest-vs-oldest
	// comparison without the wipe would read (150-100)/30 ~= 1.67/s, diluting the
	// recovered rate by the pre-restart count.
	w2 := &throughputWindow{}
	w2.Add(t0, 100, 100)
	w2.Add(t0.Add(15*time.Second), 0, 0) // restart: both counters dropped -> wipe
	w2.Add(t0.Add(30*time.Second), 150, 150)
	if r := w2.inputRate(); r < 9 || r > 11 {
		t.Errorf("recovered-above-baseline input rate = %v, want ~10/s (0->150 over 15s, not diluted by pre-restart 100)", r)
	}
}

// TestThroughputWindowDoesNotWipeOnOneSidedDrop pins the && wipe condition: a
// drop in only ONE counter (an aggregate one-sided drop on a config reload or a
// transient scrape gap) is not a restart and must not wipe the window. The
// both-direction baseline must survive; the failing direction is handled by the
// rate methods' newest-vs-oldest fallback, not by destroying both baselines.
func TestThroughputWindowDoesNotWipeOnOneSidedDrop(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Input drops 100 -> 5 while output holds 100 -> 110. Only input fell, so
	// this is a one-sided aggregate drop, not a restart: the window keeps both
	// samples and does NOT wipe.
	w := &throughputWindow{}
	w.Add(t0, 100, 100)
	w.Add(t0.Add(10*time.Second), 5, 110)
	if got := len(w.samples); got != 2 {
		t.Errorf("after a one-sided input drop the window holds %d samples, want 2 (a single-sided drop must not wipe)", got)
	}
	// Input's newest is below its oldest -> rate clamps to 0 (never negative).
	if r := w.inputRate(); r != 0 {
		t.Errorf("one-sided input drop: Input MessagesPerSecond = %v, want 0 (negative delta clamped, window kept)", r)
	}
	// Output's baseline must survive the input drop: 100 -> 110 over 10s ~= 1/s.
	if r := w.outputRate(); r < 0.9 || r > 1.1 {
		t.Errorf("one-sided input drop: Output MessagesPerSecond = %v, want ~1.0 (output baseline must NOT be wiped)", r)
	}
}

// TestThroughputWindowEqualTimestampsNeverDivideByZero pins that two samples with
// the same observed time cannot yield a NaN or infinite rate. When a wipe re-seed
// (a single sample) is immediately re-appended at the same instant, the window
// holds two equal-timestamp samples, so newest == oldest, the in-window span is 0
// seconds, and the rate division would be 0/0 (NaN for an equal counter) or
// nonzero/0 (Inf for a raised counter) without a guard. Both must read 0 and
// IsActive false — never a non-finite float that a later json.Marshal rejects.
func TestThroughputWindowEqualTimestampsNeverDivideByZero(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Equal counter at the same instant: 0/0 would be NaN.
	w := &throughputWindow{}
	w.Add(t0, 100, 100)
	w.Add(t0, 100, 100) // same timestamp: not newer, so no wipe; two in-window samples
	if r := w.inputRate(); r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp equal-input rate = %v, want 0 (never NaN)", r)
	}
	if r := w.outputRate(); r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp equal-output rate = %v, want 0 (never NaN)", r)
	}

	// Raised counter at the same instant: nonzero/0 would be +Inf.
	w2 := &throughputWindow{}
	w2.Add(t0, 100, 100)
	w2.Add(t0, 150, 150)
	if r := w2.inputRate(); r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp raised-input rate = %v, want 0 (never Inf)", r)
	}
	if r := w2.outputRate(); r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp raised-output rate = %v, want 0 (never Inf)", r)
	}
}
