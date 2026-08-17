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

const testPort = 4195

// benthos' input_received is a monotonic Prometheus counter that resets to 0 on a
// process restart, so a drop against the immediately preceding sample means the
// old sample run ended and the window must full-wipe and re-seed with only the
// new sample. The detector reads the INPUT counter only; wipeOnRestart states why
// (throughput_window.go:79-87). fsmv1 keys on the same drop:
// 'count < throughput.LastCount' (metrics_state.go:110-117).
func TestThroughputWindowWipesOnCounterReset(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// A restart drops both counters: input 100 -> 5, output 100 -> 5.
	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 100)
	w.Add(t0.Add(10*time.Second), testPort, 5, 5)
	if got := len(w.samples); got != 1 {
		t.Errorf("after input drop the window holds %d samples, want 1 (an input counter drop wipes and re-seeds)", got)
	}

	// First tick after the wipe: a single sample cannot be delta-ed, so inputRate()
	// reads 0, never the cumulative count as a rate. inputRate() is the value
	// reported as MessagesPerSecond (manager.go:73).
	if r := w.inputRate(); r != 0 {
		t.Errorf("first tick after counter-drop wipe: MessagesPerSecond = %v, want 0", r)
	}

	// Post-restart clean second sample 1s later: the rate is ~7/s from the
	// post-restart baseline only (5 -> 12).
	w.Add(t0.Add(11*time.Second), testPort, 12, 12)
	if r := w.inputRate(); r < 6 || r > 8 {
		t.Errorf("post-restart input rate = %v, want ~7/s (5->12 over 1s, not diluted by pre-restart 100)", r)
	}
}

// TestThroughputWindowWipesOnOneCounterZeroRestart asserts the input-only
// detector. A restart can leave the OUTPUT counter at ~0; a backed-up broker does
// this. Such a restart must still wipe, and wipeOnRestart states why
// (throughput_window.go:79-87).
func TestThroughputWindowWipesOnOneCounterZeroRestart(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Input 100 -> 5 (reset) while output was already 0 and stays 0. The input
	// drop must wipe; a strict both-drop condition (input<prev && output<prev)
	// would see 0<0 false and never wipe, leaving the 100 baseline inside the
	// window's 60s span (windowSpan, throughput_window.go:22).
	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 0)
	w.Add(t0.Add(10*time.Second), testPort, 5, 0)
	if got := len(w.samples); got != 1 {
		t.Errorf("after a one-counter-zero restart the window holds %d samples, want 1 (input drop wipes even when output was already 0)", got)
	}
}

// An in-place config update re-points the child at a different MetricsPort
// without a worker restart, and a new endpoint is a new counter series. A sample
// with a different port than the previous poll wipes the window and re-seeds with
// only the new-port sample.
func TestThroughputWindowWipesOnPortChange(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	w := &throughputWindow{}
	w.Add(t0, 4195, 100, 100)
	w.Add(t0.Add(10*time.Second), 4196, 5, 5)
	if got := len(w.samples); got != 1 {
		t.Errorf("after a port change the window holds %d samples, want 1 (the window re-seeds on a new scrape port)", got)
	}
	if w.port != 4196 {
		t.Errorf("window port = %d, want 4196 (the new port becomes the window's key)", w.port)
	}
	if r := w.inputRate(); r != 0 {
		t.Errorf("first tick after port-change wipe: MessagesPerSecond = %v, want 0", r)
	}
}

// TestThroughputWindowRecoveredAboveBaseline asserts that the rate is read from
// the post-restart baseline: {100, 0, 150} over 30s, with the 0 tick dropping the
// 100, reads 10/s. The window wipes at the drop tick rather than clamping
// newest-vs-oldest across the span (inputRate, throughput_window.go:130-132); such
// a clamp would read (150-100)/30 ≈ 1.67/s.
func TestThroughputWindowRecoveredAboveBaseline(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 100)
	w.Add(t0.Add(15*time.Second), testPort, 0, 0) // restart: input drop wipes 100
	w.Add(t0.Add(30*time.Second), testPort, 150, 150)
	if r := w.inputRate(); r < 9 || r > 11 {
		t.Errorf("recovered-above-baseline input rate = %v, want ~10/s (0->150 over 15s, not diluted by pre-restart 100)", r)
	}
}

// The window guards an elapsed span of zero (inputRate,
// throughput_window.go:135-137), reading 0 instead of dividing by zero.
func TestThroughputWindowEqualTimestampsNeverDivideByZero(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 100)
	w.Add(t0, testPort, 100, 100) // same timestamp: not newer, so wipeOnRestart skips it; two samples in the window
	if r := w.inputRate(); r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp equal-input rate = %v, want 0 (never NaN)", r)
	}
	if r := w.outputRate(); r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp equal-output rate = %v, want 0 (never NaN)", r)
	}

	w2 := &throughputWindow{}
	w2.Add(t0, testPort, 100, 100)
	w2.Add(t0, testPort, 150, 150)
	if r := w2.inputRate(); r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp raised-input rate = %v, want 0 (never Inf)", r)
	}
}
