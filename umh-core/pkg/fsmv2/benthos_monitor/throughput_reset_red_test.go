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

// TestThroughputWindowWipesOnCounterReset pins the counter-reset wipe (D5a).
// The reset detector is the INPUT counter only (fsmv1 metrics_state.go:110-117:
// 'count < throughput.LastCount' on input; re-sweep finding 1): benthos'
// input_received is a monotonic Prometheus counter that resets to 0 on a process
// restart, so a drop against the immediately preceding sample is the signal the
// series ended and the window must full-wipe and re-seed with only the new
// sample. The wipe must drop the pre-restart baseline so the post-restart rate is
// not diluted by the pre-restart count.
func TestThroughputWindowWipesOnCounterReset(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// A restart drops input 100 -> 5 while output holds 100 -> 5. The input drop
	// is the reset signal: the window wipes and holds ONLY the new sample.
	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 100)
	w.Add(t0.Add(10*time.Second), testPort, 5, 5)
	if got := len(w.samples); got != 1 {
		t.Errorf("after input drop the window holds %d samples, want 1 (an input counter drop wipes and re-seeds)", got)
	}

	// First tick after the wipe: a single sample cannot compute a rate, so
	// MessagesPerSecond must be 0 (never the cumulative count as a rate).
	if r := w.inputRate(); r != 0 {
		t.Errorf("first tick after counter-drop wipe: MessagesPerSecond = %v, want 0", r)
	}

	// Post-restart clean second sample 1s later: the rate is ~7/s from the
	// post-restart baseline only (5 -> 12), not diluted by the pre-restart 100.
	w.Add(t0.Add(11*time.Second), testPort, 12, 12)
	if r := w.inputRate(); r < 6 || r > 8 {
		t.Errorf("post-restart input rate = %v, want ~7/s (5->12 over 1s, not diluted by pre-restart 100)", r)
	}
}

// TestThroughputWindowWipesOnOneCounterZeroRestart pins the input-only detector:
// a restart where the OUTPUT counter was already ~0 (e.g. a backed-up broker)
// must still wipe, because requiring BOTH counters to drop would let a
// pre-restart high input baseline survive and misreport the recovered rate.
func TestThroughputWindowWipesOnOneCounterZeroRestart(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Input 100 -> 5 (reset) while output was already 0 and stays 0. The input
	// drop must wipe; a strict both-drop condition (input<prev && output<prev)
	// would see 0<0 false and never wipe, leaving the 100 baseline in-window.
	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 0)
	w.Add(t0.Add(10*time.Second), testPort, 5, 0)
	if got := len(w.samples); got != 1 {
		t.Errorf("after a one-counter-zero restart the window holds %d samples, want 1 (input drop wipes even when output was already 0)", got)
	}
}

// TestThroughputWindowWipesOnPortChange pins the port-change wipe (D5a): an
// in-place config update re-points the child at a different MetricsPort without
// a worker restart, and a new endpoint is a new counter series. A sample with a
// different port than the previous poll wipes the window and re-seeds with only
// the new-port sample.
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

// TestThroughputWindowRecoveredAboveBaseline pins that wiping at the drop tick,
// instead of clamping newest-vs-oldest in-window, reads the true post-restart
// rate. {100, 0, 150} over 30s with the 0 tick dropping the 100: the true rate is
// 0->150 over 15s = 10/s. A newest-vs-oldest clamp would read (150-100)/30 ≈ 1.67/s.
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

// TestThroughputWindowEqualTimestampsNeverDivideByZero pins that two samples with
// the same observed time cannot yield a NaN or infinite rate. The window guards
// an elapsed span of zero, reading 0 instead of dividing by zero.
func TestThroughputWindowEqualTimestampsNeverDivideByZero(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 100)
	w.Add(t0, testPort, 100, 100) // same timestamp: not newer, so no wipe; two in-window samples
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
