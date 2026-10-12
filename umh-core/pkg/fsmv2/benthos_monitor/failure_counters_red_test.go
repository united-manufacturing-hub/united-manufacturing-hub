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

	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// failureScrape returns a scrape whose failure counters hold the values in v,
// keyed by counter name; a counter missing from v is 0. Each counter sits on
// one input, one output or one processor path.
func failureScrape(v map[string]int64) benthosmonitorservice.Metrics {
	return benthosmonitorservice.Metrics{
		Inputs: map[string]benthosmonitorservice.InputInstance{"root.input": {
			Received:         10,
			ConnectionFailed: v["input_connection_failed"],
			ConnectionLost:   v["input_connection_lost"],
		}},
		Outputs: map[string]benthosmonitorservice.OutputInstance{"root.output": {
			Sent:             10,
			Error:            v["output_error"],
			ConnectionFailed: v["output_connection_failed"],
			ConnectionLost:   v["output_connection_lost"],
		}},
		Process: benthosmonitorservice.ProcessMetrics{Processors: map[string]benthosmonitorservice.ProcessorMetrics{
			"root.pipeline.processors.0": {Error: v["processor_error"]},
		}},
	}
}

// failureLabels is the label each counter's reason shows, written out here so
// the tests pin the wording rather than read it from failureCounters.
// TestFailureLabelsCoverEveryCounter keeps the two lists in step.
var failureLabels = map[string]string{
	"output_error":             "output errors",
	"processor_error":          "processor errors",
	"input_connection_failed":  "failed input connections",
	"input_connection_lost":    "lost input connections",
	"output_connection_failed": "failed output connections",
	"output_connection_lost":   "lost output connections",
}

// verdictAfterRise scrapes before at T-1s, then after at T, and returns the
// verdict at T.
func verdictAfterRise(before, after map[string]int64) benthosmonitorservice.FailureVerdict {
	d := &benthosMonitorDeps{}
	observe(d, testPort, failureScrape(before), failureT.Add(-time.Second))
	_, v := observe(d, testPort, failureScrape(after), failureT)

	return v
}

// TestFailureLabelsCoverEveryCounter: failureLabels names exactly the counters
// in failureCounters, so the loops below cover every counter.
func TestFailureLabelsCoverEveryCounter(t *testing.T) {
	if len(failureLabels) != len(failureCounters) {
		t.Errorf("failureLabels has %d counters, failureCounters has %d", len(failureLabels), len(failureCounters))
	}
	for _, c := range failureCounters {
		if failureLabels[c.name] != c.label {
			t.Errorf("counter %q: failureLabels says %q, failureCounters says %q", c.name, failureLabels[c.name], c.label)
		}
	}
}

// TestObserveEveryFailureCounterDegrades: each counter in failureLabels, raised
// alone by 1, makes the metrics not error-free with its own label.
func TestObserveEveryFailureCounterDegrades(t *testing.T) {
	for name, label := range failureLabels {
		t.Run(name, func(t *testing.T) {
			v := verdictAfterRise(nil, map[string]int64{name: 1})
			want := fmt.Sprintf("benthos reported 1 %s in the last 10m0s", label)
			if ok, reason := v.ErrorFree(); ok || reason != want {
				t.Fatalf("%s 0 -> 1: ErrorFree() = (%v, %q), want (false, %q)", name, ok, reason, want)
			}
			if n := len(v.Fired); n != 1 {
				t.Errorf("%s raised alone: fired entries = %d, want 1", name, n)
			}
		})
	}
}

// TestObserveTierOrdersReason: a processor error (tier 0; see failureCounter.tier
// in failures.go) is named before more failed input connections (tier 1). A sort by increase alone would name the
// connections.
func TestObserveTierOrdersReason(t *testing.T) {
	v := verdictAfterRise(nil, map[string]int64{"input_connection_failed": 5, "processor_error": 1})
	want := "benthos reported 1 processor errors in the last 10m0s"
	if ok, reason := v.ErrorFree(); ok || reason != want {
		t.Fatalf("input_connection_failed +5 and processor_error +1: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
	if n := len(v.Fired); n != 2 {
		t.Errorf("fired entries = %d, want 2", n)
	}
}

// TestObserveIncreaseOrdersReasonWithinTier: within tier 0, the larger
// current increase is named first.
func TestObserveIncreaseOrdersReasonWithinTier(t *testing.T) {
	v := verdictAfterRise(nil, map[string]int64{"output_error": 1, "processor_error": 3})
	want := "benthos reported 3 processor errors in the last 10m0s"
	if ok, reason := v.ErrorFree(); ok || reason != want {
		t.Fatalf("output_error +1 and processor_error +3: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveEmptyMetricsIsErrorFree: a benthos with no input, output or
// processor paths reports every counter as 0 and nothing fires.
func TestObserveEmptyMetricsIsErrorFree(t *testing.T) {
	d := &benthosMonitorDeps{}
	for i := 0; i < 3; i++ {
		_, v := observe(d, testPort, benthosmonitorservice.Metrics{}, failureT.Add(time.Duration(i)*time.Second))
		if ok, reason := v.ErrorFree(); !ok {
			t.Fatalf("scrape %d of empty Metrics: ErrorFree() = (false, %q), want true", i, reason)
		}
	}
}

// TestObserveDropInAnyFailureCounterIsARestart: for each counter, fire it,
// then drop only that counter. The drop is a restart, so the next verdict is
// error-free.
func TestObserveDropInAnyFailureCounterIsARestart(t *testing.T) {
	for name := range failureLabels {
		t.Run(name, func(t *testing.T) {
			d := &benthosMonitorDeps{}
			observe(d, testPort, failureScrape(map[string]int64{name: 3}), failureT.Add(-time.Second))
			_, v := observe(d, testPort, failureScrape(map[string]int64{name: 4}), failureT)
			if ok, _ := v.ErrorFree(); ok {
				t.Fatalf("setup: %s 3 -> 4 did not fire", name)
			}
			_, v = observe(d, testPort, failureScrape(map[string]int64{name: 0}), failureT.Add(time.Second))
			if ok, reason := v.ErrorFree(); !ok {
				t.Fatalf("%s 4 -> 0: ErrorFree() = (false, %q), want true (a drop is a restart)", name, reason)
			}
		})
	}
}

// TestPollJudgesEveryFailureCounterFromTheWire: each failure counter, raised
// in a real /metrics body between two polls, reaches the status verdict with
// its label. This covers the parser's name for each counter.
func TestPollJudgesEveryFailureCounterFromTheWire(t *testing.T) {
	lines := map[string]string{
		"output_error":             `output_error{label="",path="root.output"} %d`,
		"processor_error":          `processor_error{label="",path="root.pipeline.processors.0"} %d`,
		"input_connection_failed":  `input_connection_failed{label="",path="root.input"} %d`,
		"input_connection_lost":    `input_connection_lost{label="",path="root.input"} %d`,
		"output_connection_failed": `output_connection_failed{label="",path="root.output"} %d`,
		"output_connection_lost":   `output_connection_lost{label="",path="root.output"} %d`,
	}

	for name, label := range failureLabels {
		t.Run(name, func(t *testing.T) {
			var value int64

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/ping":
					_, _ = w.Write([]byte("pong"))
				case "/ready":
					_, _ = w.Write([]byte(`{"error":""}`))
				case "/version":
					_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
				case "/metrics":
					_, _ = fmt.Fprintf(w, lines[name]+"\n", value)
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

			if _, err := Poll(context.Background(), deps, cfg); err != nil {
				t.Fatalf("first Poll errored: %v", err)
			}
			value = 2
			status, err := Poll(context.Background(), deps, cfg)
			if err != nil {
				t.Fatalf("second Poll errored: %v", err)
			}
			want := fmt.Sprintf("benthos reported 2 %s in the last 10m0s", label)
			if ok, reason := status.Failures.ErrorFree(); ok || reason != want {
				t.Fatalf("%s 0 -> 2 on the wire: ErrorFree() = (%v, %q), want (false, %q)", name, ok, reason, want)
			}
		})
	}
}
