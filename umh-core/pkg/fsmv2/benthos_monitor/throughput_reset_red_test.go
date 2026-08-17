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

// A drop in benthos' input_received counter against the immediately preceding
// sample means the process restarted, so the window must full-wipe and re-seed
// with only the new sample; wipeOnRestart (throughput_window.go) states why
// that drop is the restart signal and why the input counter alone decides it.
// fsmv1 keys on the same drop: 'count < throughput.LastCount' in
// updateComponentThroughput (pkg/service/benthos_monitor/metrics_state.go).
func TestThroughputWindowWipesOnCounterReset(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// A restart drops both counters: input 100 -> 5, output 100 -> 5.
	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 100)
	w.Add(t0.Add(10*time.Second), testPort, 5, 5)
	if got := len(w.samples); got != 1 {
		t.Errorf("after input drop the window holds %d samples, want 1 (an input counter drop wipes and re-seeds)", got)
	}

	// First tick after the wipe: one sample has nothing to subtract from, so
	// inputRate() reads 0, never the cumulative count as a rate. Its value is
	// reported as ComponentThroughput.MessagesPerSecond (manager.go).
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
// detector: a restart can leave the OUTPUT counter at ~0, a backed-up broker for
// instance, and such a restart must still wipe. wipeOnRestart
// (throughput_window.go) states why it does not also require an output drop.
func TestThroughputWindowWipesOnOneCounterZeroRestart(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Input 100 -> 5 (reset) while output was already 0 and stays 0. The input
	// drop must wipe; a strict both-drop condition (input<prev && output<prev)
	// would see 0<0 false and never wipe, leaving the 100 baseline inside the
	// window's 60s span (windowSpan, throughput_window.go).
	w := &throughputWindow{}
	w.Add(t0, testPort, 100, 0)
	w.Add(t0.Add(10*time.Second), testPort, 5, 0)
	if got := len(w.samples); got != 1 {
		t.Errorf("after a one-counter-zero restart the window holds %d samples, want 1 (input drop wipes even when output was already 0)", got)
	}
}

// A config update can change this monitor's MetricsPort (BenthosMonitorConfig,
// pkg/config/config.go) while it keeps running, so the next poll scrapes a
// different endpoint with no restart in between. A new endpoint is a new counter
// series, so a sample whose port differs from the previous poll's wipes the
// window and re-seeds with only the new-port sample.
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
// 100, reads 10/s. The wipe happens at the drop tick (Add,
// throughput_window.go); keeping the pre-restart sample and dividing across
// the full span would instead read (150-100)/30 ≈ 1.67/s.
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

// Two samples with the same observed time make the elapsed span zero, so
// inputRate's `elapsed <= 0` guard (throughput_window.go) reads 0 instead
// of dividing by it: an unchanged counter cannot yield NaN (window w below) and a
// counter that rose between the two samples cannot yield +Inf (w2).
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
