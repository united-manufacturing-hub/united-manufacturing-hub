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

// TestThroughputWindowIsTimeBased pins the by-time throughput window. The
// window is held BY TIME over a 60s span, not by FSMv1's count-based
// ThroughputWindowSize = 600 entries, so it matches the intent of a one-minute
// average without the drift FSMv1 gets from dropped ticks.
//
// MessagesPerSecond is a real rate in seconds over the in-window span: the
// oldest in-window sample to the newest, (latest.count - oldest.count) /
// elapsedSeconds. LastCount is the newest sample's count. With a single sample
// (no second in-window sample) the rate is 0 — and IsActive derives from
// Input.MessagesPerSecond > 0, so a zero rate means inactive. A counter reset
// (process restart zeroes the Prometheus counter) makes the newest count lower
// than the oldest; the rate then reads 0 until the pre-restart samples age out
// of the span.
//
// Every sample time is injected so the test is deterministic: no real 1s
// real-time sleeps and no fake clock. The window's Add accepts the explicit
// sample time precisely so an aged sample can be injected.
//
// This test would FAIL if the math is wrong:
//   - (b) single sample -> rate 0 (one sample cannot be delta-ed).
//   - (a) two samples exactly 1s apart with input rising +2 (10 -> 12) ->
//     rate ~2.0 within tolerance, LastCount 12.
//   - (c) a sample 61s old is DROPPED, so the rate is unchanged — proving the
//     window is time-based, not count-based. A count-based window holding the
//     three most recent samples would fold the aged sample in and move the
//     rate; a 60s time window must not.
//   - (e) a decreasing counter (process restart) must read 0, not a negative
//     rate: the reset delta is clamped, and the pre-restart samples age out
//     over the 60s span.
func TestThroughputWindowIsTimeBased(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)
	deps := &benthosMonitorDeps{}

	// (b) single sample: MessagesPerSecond must be 0 (and, by extension,
	// IsActive false), not the cumulative counter-as-rate FSMv1 publishes on its
	// first/reset tick.
	deps.window.Add(t0, 10)
	inRate := deps.window.messagesPerSecond()
	if inRate != 0 {
		t.Errorf("single-sample window: MessagesPerSecond = %v, want 0 (one sample cannot compute a rate)", inRate)
	}

	// (a) second sample exactly 1s later, input +2: real rate in seconds ~2.0.
	deps.window.Add(t0.Add(1*time.Second), 12)
	inRate = deps.window.messagesPerSecond()
	if inRate < 1.5 || inRate > 2.5 {
		t.Errorf("1s-apart +2 input: MessagesPerSecond = %v, want ~2.0 (within tolerance)", inRate)
	}
	if in := deps.window.lastCount(); in != 12 {
		t.Errorf("lastCount input = %d, want 12 (LastCount is the newest sample's count)", in)
	}

	// (c) an injected sample 61s older than the newest is outside the 60s window
	// and must be dropped: the rate must be unchanged. This is the by-time
	// discriminator — a count-based window of 3 entries would include the aged
	// sample and change the rate. The aged sample is pruned on Add, so the window
	// stays bounded to the span rather than accumulating it.
	deps.window.Add(t0.Add(-61*time.Second), 5)
	inRate2 := deps.window.messagesPerSecond()
	if inRate2 != inRate {
		t.Errorf("after adding a 61s-old sample: MessagesPerSecond = %v, want %v (an aged sample must be dropped; the window is time-based, not count-based)", inRate2, inRate)
	}
	if got := len(deps.window.samples); got != 2 {
		t.Errorf("after adding a 61s-old sample, window holds %d samples, want 2 (aged samples must be pruned to bound the window to the 60s span)", got)
	}

	// (d) a mid-window dropped-tick gap must not drift the by-time rate. Samples
	// at t0, +30s, +60s with input 0, 30, 90: the in-window span is the full
	// t0..t0+60s (oldest in-window to newest), so the rate is the real-time delta
	// 90/60 = 1.5/s — not a two-newest count-window rate of 60/30 = 2/s. A sparse
	// in-window window keeps the real elapsed span.
	gap := &benthosMonitorDeps{}
	gap.window.Add(t0, 0)
	gap.window.Add(t0.Add(30*time.Second), 30)
	gap.window.Add(t0.Add(60*time.Second), 90)
	if r := gap.window.messagesPerSecond(); r < 1.4 || r > 1.6 {
		t.Errorf("mid-window gap: MessagesPerSecond = %v, want ~1.5 (real-time delta 90/60 over the in-window span, not a count-window 2.0)", r)
	}

	// (e) a decreasing counter spanning a restart: benthos' input_received is a
	// Prometheus counter that zeroes when the process restarts, so a reset
	// landing inside the window yields a negative delta. The rate must be
	// clamped to 0, never negative. The misreport lasts up to the window span
	// (~60s) until the two pre-restart samples age out.
	restart := &benthosMonitorDeps{}
	restart.window.Add(t0, 100)
	restart.window.Add(t0.Add(10*time.Second), 5)
	if r := restart.window.messagesPerSecond(); r != 0 {
		t.Errorf("after counter reset: MessagesPerSecond = %v, want 0 (a negative delta is clamped; a restarted benthos reports no measurable traffic until the pre-restart samples age out)", r)
	}
}
