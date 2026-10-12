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

	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

const testPort = 4195

// counts returns a scrape whose input and output totals are input and output.
func counts(input, output int) benthosmonitorservice.Metrics {
	return benthosmonitorservice.Metrics{
		Inputs:  map[string]benthosmonitorservice.InputInstance{"root.input": {Received: int64(input)}},
		Outputs: map[string]benthosmonitorservice.OutputInstance{"root.output": {Sent: int64(output)}},
	}
}

// wipeOnRestart (throughput_window.go) states why an input-counter drop is the
// restart signal and why the input counter alone decides it. fsmv1 keys on the
// same drop: 'count < throughput.LastCount' in updateComponentThroughput
// (pkg/service/benthos_monitor/metrics_state.go).
func TestThroughputWindowWipesOnCounterReset(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// A restart drops both counters: input 100 -> 5, output 100 -> 5.
	d := &benthosMonitorDeps{}
	observe(d, testPort, counts(100, 100), t0)

	// First tick after the restart: the post-restart sample has nothing to
	// subtract from, so the rate reads 0, never the cumulative count as a rate.
	got := observe(d, testPort, counts(5, 5), t0.Add(10*time.Second))
	if r := got.Input.MessagesPerSecond; r != 0 {
		t.Errorf("first tick after counter drop: MessagesPerSecond = %v, want 0", r)
	}
	if c := got.Input.LastCount; c != 5 {
		t.Errorf("first tick after counter drop: LastCount = %d, want 5 (the post-restart count)", c)
	}

	// Post-restart second sample 1s later: the rate is ~7/s from the
	// post-restart baseline only (5 -> 12). Had the pre-restart 100 stayed in
	// the window, it would be the oldest sample and the rate would read 0.
	got = observe(d, testPort, counts(12, 12), t0.Add(11*time.Second))
	if r := got.Input.MessagesPerSecond; r < 6 || r > 8 {
		t.Errorf("post-restart input rate = %v, want ~7/s (5->12 over 1s, not diluted by pre-restart 100)", r)
	}
}

// TestThroughputWindowWipesOnOneCounterZeroRestart asserts the input-only
// detector: a restart can leave the OUTPUT counter at ~0, a backed-up broker for
// instance, and such a restart must still wipe. wipeOnRestart
// (throughput_window.go) states why it does not also require an output drop.
func TestThroughputWindowWipesOnOneCounterZeroRestart(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Input 100 -> 5 (reset) while output was already 0 and stays 0. A strict
	// both-drop condition would not wipe here, the 100 would stay the oldest
	// input sample, and the rate on the next tick would read 0.
	d := &benthosMonitorDeps{}
	observe(d, testPort, counts(100, 0), t0)
	observe(d, testPort, counts(5, 0), t0.Add(10*time.Second))
	got := observe(d, testPort, counts(12, 0), t0.Add(11*time.Second))
	if r := got.Input.MessagesPerSecond; r < 6 || r > 8 {
		t.Errorf("after a one-counter-zero restart: input rate = %v, want ~7/s (input drop wipes even when output was already 0)", r)
	}
}

// A config update can change this monitor's MetricsPort (BenthosMonitorConfig,
// pkg/config/config.go) while it keeps running, so the next poll scrapes a
// different endpoint with no restart in between. A new endpoint is a new counter
// series, so a sample whose port differs from the previous poll's wipes the
// window and re-seeds with only the new-port sample. The new port's counter is
// above the old one, so no counter drop can cause the wipe instead.
func TestThroughputWindowWipesOnPortChange(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	d := &benthosMonitorDeps{}
	observe(d, 4195, counts(100, 100), t0)
	got := observe(d, 4196, counts(150, 150), t0.Add(10*time.Second))
	if r := got.Input.MessagesPerSecond; r != 0 {
		t.Errorf("first tick after a port change: MessagesPerSecond = %v, want 0", r)
	}

	// The next new-port sample reads its rate from the new port's first sample.
	// Had the old port's 100 stayed in the window, the rate would read
	// (157-100)/11 ≈ 5.2/s.
	got = observe(d, 4196, counts(157, 157), t0.Add(11*time.Second))
	if r := got.Input.MessagesPerSecond; r < 6.5 || r > 7.5 {
		t.Errorf("second tick on the new port: input rate = %v, want ~7/s (150->157 over 1s, the old port's 100 dropped)", r)
	}
}

// TestThroughputWindowRecoveredAboveBaseline asserts that the rate is read from
// the post-restart baseline: {100, 0, 150} over 30s, with the 0 tick dropping the
// 100, reads 10/s. Keeping the pre-restart sample and dividing across the full
// span would instead read (150-100)/30 ≈ 1.67/s.
func TestThroughputWindowRecoveredAboveBaseline(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	d := &benthosMonitorDeps{}
	observe(d, testPort, counts(100, 100), t0)
	observe(d, testPort, counts(0, 0), t0.Add(15*time.Second)) // restart: input drop wipes 100
	got := observe(d, testPort, counts(150, 150), t0.Add(30*time.Second))
	if r := got.Input.MessagesPerSecond; r < 9 || r > 11 {
		t.Errorf("recovered-above-baseline input rate = %v, want ~10/s (0->150 over 15s, not diluted by pre-restart 100)", r)
	}
}

// Two scrapes with the same observed time span zero seconds, so the rate reads
// 0 instead of dividing by zero. The second case has a counter that rose
// between the two equally-stamped scrapes; it still reads 0, not (150-100)/0.
func TestThroughputWindowEqualTimestampsNeverDivideByZero(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	d := &benthosMonitorDeps{}
	observe(d, testPort, counts(100, 100), t0)
	got := observe(d, testPort, counts(100, 100), t0)
	if r := got.Input.MessagesPerSecond; r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp equal-input rate = %v, want 0 (never NaN)", r)
	}
	if r := got.Output.MessagesPerSecond; r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp equal-output rate = %v, want 0 (never NaN)", r)
	}

	d2 := &benthosMonitorDeps{}
	observe(d2, testPort, counts(100, 100), t0)
	got = observe(d2, testPort, counts(150, 150), t0)
	if r := got.Input.MessagesPerSecond; r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp raised-input rate = %v, want 0 (a tie in observed time yields no rate, whatever the counter did)", r)
	}
}
