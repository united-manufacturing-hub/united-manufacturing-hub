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
func TestThroughputWindowIsTimeBased(t *testing.T) {
	t0 := time.Now().Truncate(time.Second)
	deps := &benthosMonitorDeps{}

	// (b) single sample: MessagesPerSecond must be 0 (and, by extension,
	// IsActive false), not the cumulative counter-as-rate FSMv1 publishes on its
	// first/reset tick.
	deps.window.Add(t0, 10, 5)
	if inRate := deps.window.inputRate(); inRate != 0 {
		t.Errorf("single-sample window: Input MessagesPerSecond = %v, want 0 (one sample cannot compute a rate)", inRate)
	}
	if outRate := deps.window.outputRate(); outRate != 0 {
		t.Errorf("single-sample window: Output MessagesPerSecond = %v, want 0", outRate)
	}

	// (a) second sample exactly 1s later, input +2, output +1: real rate in
	// seconds ~2.0 input / ~1.0 output.
	deps.window.Add(t0.Add(1*time.Second), 12, 6)
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

	// (c) an injected sample 61s older than the newest is outside the 60s window
	// and must be dropped: the rate must be unchanged. This is the by-time
	// discriminator — a count-based window of 3 entries would include the aged
	// sample and change the rate.
	deps.window.Add(t0.Add(-61*time.Second), 5, 5)
	if inRate2 := deps.window.inputRate(); inRate2 < 1.5 || inRate2 > 2.5 {
		t.Errorf("after adding a 61s-old sample: Input MessagesPerSecond = %v, want ~2.0 (an aged sample must be dropped)", inRate2)
	}
	if got := len(deps.window.samples); got != 2 {
		t.Errorf("after adding a 61s-old sample, window holds %d samples, want 2 (aged samples must be pruned)", got)
	}

	// (d) a mid-window dropped-tick gap must not drift the by-time rate. Samples
	// at t0, +30s, +60s with input 0, 30, 90: the in-window span is the full
	// t0..t0+60s (oldest in-window to newest), so the rate is the real-time delta
	// 90/60 = 1.5/s — not a two-newest count-window rate of 60/30 = 2/s.
	gap := &benthosMonitorDeps{}
	gap.window.Add(t0, 0, 0)
	gap.window.Add(t0.Add(30*time.Second), 30, 30)
	gap.window.Add(t0.Add(60*time.Second), 90, 90)
	if r := gap.window.inputRate(); r < 1.4 || r > 1.6 {
		t.Errorf("mid-window gap: Input MessagesPerSecond = %v, want ~1.5 (real-time delta 90/60, not a count-window 2.0)", r)
	}

	// (e) a decreasing counter spanning a restart: benthos' input_received is a
	// Prometheus counter that zeroes when the process restarts, so a reset
	// landing inside the window yields a negative delta. The rate must be clamped
	// to 0, never negative.
	restart := &benthosMonitorDeps{}
	restart.window.Add(t0, 100, 100)
	restart.window.Add(t0.Add(10*time.Second), 5, 5)
	if r := restart.window.inputRate(); r != 0 {
		t.Errorf("after counter reset: Input MessagesPerSecond = %v, want 0 (a negative delta is clamped)", r)
	}
}

// TestThroughputWindowZeroValueIsSafe pins that the window's zero value is a
// valid empty window: every accessor returns the zero/0 result rather than
// panicking on an un-populated window. The deps doc guarantees this contract.
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
	// A single sample is still not two, so rates stay 0 but nothing panics.
	w.Add(time.Now(), 7, 3)
	if in := w.inputRate(); in != 0 {
		t.Errorf("inputRate on single-sample window = %v, want 0", in)
	}
	if in := w.inputCount(); in != 7 {
		t.Errorf("inputCount after one Add = %d, want 7", in)
	}
}

// TestPollComputesThroughput pins the end-to-end wiring: Poll feeds the scrape
// of /metrics into the by-time window and returns Input/LastCount and IsActive on
// the status. The first cold poll is a single sample, so its rate is 0 and
// IsActive false; a second poll with a raised counter advances LastCount and flips
// IsActive, proving Poll actually feeds the window. (The rate itself is
// wall-clock sensitive, so the deterministic assertions are the single-sample 0
// and the LastCount progression.)
func TestPollComputesThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-style poll test")
	}
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
			fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} %d\n", inputCounter)
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output\"} %d\n", outputCounter)
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

	// Cold first poll: a single sample cannot compute a rate.
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

	// Second poll with a raised input counter: LastCount advances and IsActive
	// flips on, proving Poll fed the window.
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
}
