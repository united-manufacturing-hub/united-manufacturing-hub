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
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// TestThroughputWindowIsTimeBased asserts that the throughput window is held by
// time over the 60s windowSpan (throughput_window.go) rather than by FSMv1's
// count-based ThroughputWindowSize of 600 entries
// (pkg/service/benthos_monitor/metrics_state.go). windowSpan's doc is where
// the reason for that choice is stated.
//
// Semantics under test: throughputWindow.inputRate and inputCount
// (throughput_window.go), plus BenthosMonitorStatus.IsActive, which
// Poll derives from Input.MessagesPerSecond > 0 (manager.go), so a zero rate
// means inactive.
//
// Every sample time is injected so the test is deterministic: no real 1s
// real-time sleeps. throughputWindow.Add (throughput_window.go) accepts the
// explicit sample time precisely so an aged sample can be injected.
func TestThroughputWindowIsTimeBased(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)
	deps := &benthosMonitorDeps{}

	deps.window.Add(t0, 4195, 10, 5)
	if inRate := deps.window.inputRate(); inRate != 0 {
		t.Errorf("single-sample window: Input MessagesPerSecond = %v, want 0 (one sample cannot compute a rate)", inRate)
	}
	if outRate := deps.window.outputRate(); outRate != 0 {
		t.Errorf("single-sample window: Output MessagesPerSecond = %v, want 0", outRate)
	}

	deps.window.Add(t0.Add(1*time.Second), 4195, 12, 6)
	if inRate := deps.window.inputRate(); inRate < 1.5 || inRate > 2.5 {
		t.Errorf("1s-apart +2 input: Input MessagesPerSecond = %v, want ~2.0 (within tolerance)", inRate)
	}
	if outRate := deps.window.outputRate(); outRate < 0.5 || outRate > 1.5 {
		t.Errorf("1s-apart +1 output: Output MessagesPerSecond = %v, want ~1.0", outRate)
	}
	if in := deps.window.inputCount(); in != 12 {
		t.Errorf("inputCount = %d, want 12 (LastCount is the newest sample's count)", in)
	}
	if out := deps.window.outputCount(); out != 6 {
		t.Errorf("outputCount = %d, want 6", out)
	}

	// A sample 61s older than the newest is outside the 60s window and must be
	// dropped, leaving the rate unchanged. A count-based window of 3 entries would
	// keep that sample and change the rate, so this is the assertion that
	// separates the two designs.
	deps.window.Add(t0.Add(-61*time.Second), 4195, 5, 5)
	if inRate2 := deps.window.inputRate(); inRate2 < 1.5 || inRate2 > 2.5 {
		t.Errorf("after adding a 61s-old sample: Input MessagesPerSecond = %v, want ~2.0 (an aged sample must be dropped)", inRate2)
	}
	if got := len(deps.window.samples); got != 2 {
		t.Errorf("after adding a 61s-old sample, window holds %d samples, want 2 (aged samples must be pruned)", got)
	}

	// A mid-window dropped-tick gap must not drift the by-time rate. Samples
	// at t0, +30s, +60s with input 0, 30, 90: the in-window span is the full
	// t0..t0+60s (oldest in-window to newest), so the rate is the real-time delta
	// 90/60 = 1.5/s — not a two-newest count-window rate of 60/30 = 2/s.
	gap := &benthosMonitorDeps{}
	gap.window.Add(t0, 4195, 0, 0)
	gap.window.Add(t0.Add(30*time.Second), 4195, 30, 30)
	gap.window.Add(t0.Add(60*time.Second), 4195, 90, 90)
	if r := gap.window.inputRate(); r < 1.4 || r > 1.6 {
		t.Errorf("mid-window gap: Input MessagesPerSecond = %v, want ~1.5 (real-time delta 90/60, not a count-window 2.0)", r)
	}

	// A restart zeroes benthos' counters. Add keys the wipe on the input counter
	// alone, so this drop wipes the window and re-seeds it with the new sample.
	// The rate then reads 0 — never negative.
	restart := &benthosMonitorDeps{}
	restart.window.Add(t0, 4195, 100, 100)
	restart.window.Add(t0.Add(10*time.Second), 4195, 5, 5)
	if r := restart.window.inputRate(); r != 0 {
		t.Errorf("after both-counter drop: Input MessagesPerSecond = %v, want 0 (wipe leaves a single sample)", r)
	}
}

// TestThroughputWindowZeroValueIsSafe asserts the contract stated on the
// throughputWindow type (throughput_window.go): the zero value is a valid
// empty window, so every accessor returns 0 rather than panicking and no
// constructor is needed. The closing Add covers the next case up — one sample is
// still not two, so the rates stay 0 while inputCount tracks that sample; the
// fewer-than-two rule is stated on inputRate (throughput_window.go).
func TestThroughputWindowZeroValueIsSafe(t *testing.T) {
	var w throughputWindow

	if in := w.inputRate(); in != 0 {
		t.Errorf("inputRate on empty window = %v, want 0", in)
	}
	if out := w.outputRate(); out != 0 {
		t.Errorf("outputRate on empty window = %v, want 0", out)
	}
	if in := w.inputCount(); in != 0 {
		t.Errorf("inputCount on empty window = %d, want 0", in)
	}
	if out := w.outputCount(); out != 0 {
		t.Errorf("outputCount on empty window = %d, want 0", out)
	}
	w.Add(time.Now(), 4195, 7, 3)
	if in := w.inputRate(); in != 0 {
		t.Errorf("inputRate on single-sample window = %v, want 0", in)
	}
	if in := w.inputCount(); in != 7 {
		t.Errorf("inputCount after one Add = %d, want 7", in)
	}
}

// TestPollComputesThroughput asserts the end-to-end wiring: Poll feeds the
// scrape of /metrics into the by-time window and returns Input/LastCount and
// IsActive on the status. The first cold poll is a single sample, so its rate is
// 0 and IsActive false; a second poll with a raised counter advances LastCount
// and flips IsActive on, both of which happen only if Poll fed the window. The
// rate VALUE is wall-clock sensitive, so asserting a number would be flaky by
// construction; its SIGN is not, which is what IsActive reads. The deterministic
// assertions are therefore the single-sample 0, the LastCount progression, and
// the IsActive flip.
func TestPollComputesThroughput(t *testing.T) {
	var inputCounter int64 = 10
	var outputCounter int64 = 5

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			_, _ = w.Write([]byte(`{"error":""}`))
		case "/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/metrics":
			_, _ = fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", inputCounter)
			_, _ = fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output\"} %d\n", outputCounter)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: "benthos-1", DesiredFSMState: "active"},
		MetricsPort:       uint16(srv.Listener.Addr().(*net.TCPAddr).Port),
	}
	deps := &benthosMonitorDeps{client: &http.Client{Timeout: 400 * time.Millisecond}}

	status, err := Poll(context.Background(), deps, cfg)
	if err != nil {
		t.Fatalf("first Poll errored: %v", err)
	}
	if status.Input.MessagesPerSecond != 0 {
		t.Errorf("cold poll Input.MessagesPerSecond = %v, want 0 (single sample)", status.Input.MessagesPerSecond)
	}
	if status.IsActive {
		t.Errorf("cold poll IsActive = true, want false (zero rate)")
	}
	if status.Input.LastCount != 10 {
		t.Errorf("cold poll Input.LastCount = %d, want 10", status.Input.LastCount)
	}

	inputCounter = 12
	outputCounter = 6
	status, err = Poll(context.Background(), deps, cfg)
	if err != nil {
		t.Fatalf("second Poll errored: %v", err)
	}
	if status.Input.LastCount != 12 {
		t.Errorf("second poll Input.LastCount = %d, want 12 (window fed)", status.Input.LastCount)
	}
	if status.Output.LastCount != 6 {
		t.Errorf("second poll Output.LastCount = %d, want 6", status.Output.LastCount)
	}
	if !status.IsActive {
		t.Errorf("second poll IsActive = false, want true (input rose by 2 over a positive elapsed span); this is the only assertion of the true case directly on Poll's return value, the framework-seam equivalent being TestScenarioWorkerObservedThroughFramework")
	}
}

// TestPollResetTickReportsIsActiveFalse asserts that the status layer reflects a
// counter drop through Poll. Both counters drop when the process restarts, and
// the input-counter drop alone makes Add wipe the window to a single sample
// (wipeOnRestart, throughput_window.go). On the reset tick — the poll on which
// that drop is first observed — the status must read IsActive=false, and
// LastCount must carry the post-restart counter.
//
// This test covers the Poll->status wiring and how LastCount propagates; the
// window-level wipe itself (len(samples)==1) is the discriminating assertion,
// covered by TestThroughputWindowWipesOnCounterReset
// (throughput_reset_red_test.go). The IsActive assertion below is vacuous with
// respect to the wipe: drop the wipe and both samples remain, but inputRate's
// newest-below-oldest guard (throughput_window.go) still returns 0, so
// IsActive reads false either way.
func TestPollResetTickReportsIsActiveFalse(t *testing.T) {
	inputCounter := 100
	outputCounter := 100

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			_, _ = w.Write([]byte(`{"error":""}`))
		case "/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/metrics":
			_, _ = fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", inputCounter)
			_, _ = fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output\"} %d\n", outputCounter)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.BenthosMonitorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{Name: "benthos-1", DesiredFSMState: "active"},
		MetricsPort:       uint16(srv.Listener.Addr().(*net.TCPAddr).Port),
	}
	deps := &benthosMonitorDeps{client: &http.Client{Timeout: 400 * time.Millisecond}}

	status, err := Poll(context.Background(), deps, cfg)
	if err != nil {
		t.Fatalf("baseline Poll errored: %v", err)
	}
	if status.IsActive {
		t.Errorf("baseline poll IsActive = true, want false (single sample)")
	}

	inputCounter = 5
	outputCounter = 5
	status, err = Poll(context.Background(), deps, cfg)
	if err != nil {
		t.Fatalf("reset-tick Poll errored: %v", err)
	}
	if status.IsActive {
		t.Errorf("reset tick after both-counter drop: IsActive = true, want false (window wiped to a single sample)")
	}
	if status.Input.LastCount != 5 {
		t.Errorf("reset tick Input.LastCount = %d, want 5 (post-restart counter)", status.Input.LastCount)
	}
}
