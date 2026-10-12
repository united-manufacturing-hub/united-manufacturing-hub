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

// A counter drop means benthos restarted, so observe rebuilds its engine
// (restarted, observe.go) and the window holds only post-restart samples.
// fsmv1 keys on the same drop: 'count < throughput.LastCount' in
// updateComponentThroughput (pkg/service/benthos_monitor/metrics_state.go).
func TestObserveRebuildsOnCounterReset(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// A restart drops both counters: input 100 -> 5, output 100 -> 5.
	d := &benthosMonitorDeps{}
	observe(d, testPort, counts(100, 100), t0)

	// First tick after the restart: the rebuilt engine holds one sample, so
	// the rate reads 0, never the cumulative count as a rate.
	got := observe(d, testPort, counts(5, 5), t0.Add(10*time.Second))
	if r := got.Input.MessagesPerSecond; r != 0 {
		t.Errorf("first tick after counter drop: MessagesPerSecond = %v, want 0", r)
	}
	if c := got.Input.LastCount; c != 5 {
		t.Errorf("first tick after counter drop: LastCount = %d, want 5 (the post-restart count)", c)
	}

	// Post-restart second sample 1s later: the rate is ~7/s from the
	// post-restart baseline only (5 -> 12). Had the pre-restart 100 stayed in
	// the window, it would be the oldest sample and the rate would read
	// (12-100)/11 ≈ -8/s.
	got = observe(d, testPort, counts(12, 12), t0.Add(11*time.Second))
	if r := got.Input.MessagesPerSecond; r < 6 || r > 8 {
		t.Errorf("post-restart input rate = %v, want ~7/s (5->12 over 1s, not diluted by pre-restart 100)", r)
	}
}

// TestObserveRebuildsOnInputDropWithOutputAtZero asserts that a drop in the
// input counter alone is a restart: a restart can leave the OUTPUT counter at
// ~0, a backed-up broker for instance, and such a restart must still rebuild.
func TestObserveRebuildsOnInputDropWithOutputAtZero(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Input 100 -> 5 (reset) while output was already 0 and stays 0. Without
	// the rebuild the 100 would stay the oldest input sample, and the rate on
	// the next tick would read (12-100)/11 ≈ -8/s.
	d := &benthosMonitorDeps{}
	observe(d, testPort, counts(100, 0), t0)
	observe(d, testPort, counts(5, 0), t0.Add(10*time.Second))
	got := observe(d, testPort, counts(12, 0), t0.Add(11*time.Second))
	if r := got.Input.MessagesPerSecond; r < 6 || r > 8 {
		t.Errorf("after a one-counter-zero restart: input rate = %v, want ~7/s (an input drop rebuilds even when output was already 0)", r)
	}
}

// TestObserveRebuildsOnOutputDrop asserts that a drop in the output counter
// alone is a restart too, while the input counter is unchanged.
func TestObserveRebuildsOnOutputDrop(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	// Output 200 -> 5 while input stays 100. Without the rebuild the 200 would
	// stay the oldest output sample, and the output rate on the next tick would
	// read (12-200)/11 ≈ -17/s.
	d := &benthosMonitorDeps{}
	observe(d, testPort, counts(100, 200), t0)
	observe(d, testPort, counts(100, 5), t0.Add(10*time.Second))
	got := observe(d, testPort, counts(107, 12), t0.Add(11*time.Second))
	if r := got.Output.MessagesPerSecond; r < 6 || r > 8 {
		t.Errorf("after an output-only drop: output rate = %v, want ~7/s (5->12 over 1s)", r)
	}
}

// A config update can change this monitor's MetricsPort (BenthosMonitorConfig,
// pkg/config/config.go) while it keeps running, so the next poll scrapes a
// different endpoint with no restart in between. A new endpoint is a new
// counter series, so a scrape whose port differs from the previous one rebuilds
// the engine. The new port's counter is above the old one, so no counter drop
// can cause the rebuild instead.
func TestObserveRebuildsOnPortChange(t *testing.T) {
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

// TestObserveRecoveredAboveBaseline asserts that the rate is read from the
// post-restart baseline: {100, 0, 150} over 30s, with the 0 tick dropping the
// 100, reads 10/s. Keeping the pre-restart sample and dividing across the full
// span would instead read (150-100)/30 ≈ 1.67/s.
func TestObserveRecoveredAboveBaseline(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	d := &benthosMonitorDeps{}
	observe(d, testPort, counts(100, 100), t0)
	observe(d, testPort, counts(0, 0), t0.Add(15*time.Second)) // restart: the input drop rebuilds
	got := observe(d, testPort, counts(150, 150), t0.Add(30*time.Second))
	if r := got.Input.MessagesPerSecond; r < 9 || r > 11 {
		t.Errorf("recovered-above-baseline input rate = %v, want ~10/s (0->150 over 15s, not diluted by pre-restart 100)", r)
	}
}

// TestObserveIgnoresScrapeNotNewer asserts that a scrape stamped at the same
// instant as the previous one is not observed: observe returns the previous
// result, so the rate never divides by a zero time span and a counter that rose
// in between is not read. The engine's own guard against a zero time span is
// tested in pkg/diagnosis.
func TestObserveIgnoresScrapeNotNewer(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)

	d := &benthosMonitorDeps{}
	first := observe(d, testPort, counts(100, 100), t0)
	got := observe(d, testPort, counts(150, 150), t0)
	if got != first {
		t.Errorf("equal-timestamp scrape: got %+v, want the previous result %+v", got, first)
	}
	if r := got.Input.MessagesPerSecond; r != 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		t.Errorf("equal-timestamp raised-input rate = %v, want 0", r)
	}
	if c := got.Input.LastCount; c != 100 {
		t.Errorf("equal-timestamp scrape: LastCount = %d, want 100 (the 150 is not observed)", c)
	}
}
