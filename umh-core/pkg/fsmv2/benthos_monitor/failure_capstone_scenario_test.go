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

package fsmv2benthosmonitor_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	fsmv2benthosmonitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

// failingBenthosStub serves the endpoints Poll scrapes, with error counters
// the test sets and a switch that makes /metrics answer 500.
type failingBenthosStub struct {
	server            *httptest.Server
	processorErrors   atomic.Int64
	inputConnFailures atomic.Int64
	inputConnUps      atomic.Int64
	metricsDown       atomic.Bool
}

func newFailingBenthosStub() *failingBenthosStub {
	s := &failingBenthosStub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			_, _ = w.Write([]byte(`{"error":"","statuses":[]}`))
		case "/version":
			_, _ = w.Write([]byte(`{"version":"4.95.0","built":"capstone"}`))
		case "/metrics":
			if s.metricsDown.Load() {
				http.Error(w, "down", http.StatusInternalServerError)

				return
			}
			fmt.Fprintf(w, "input_received{label=\"\",path=\"root.input\"} 10\n")
			fmt.Fprintf(w, "input_connection_up{label=\"\",path=\"root.input\"} %d\n", s.inputConnUps.Load())
			fmt.Fprintf(w, "input_connection_failed{label=\"\",path=\"root.input\"} %d\n", s.inputConnFailures.Load())
			fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output\"} 10\n")
			fmt.Fprintf(w, "processor_error{label=\"\",path=\"root.pipeline.processors.0\"} %d\n", s.processorErrors.Load())
		default:
			http.NotFound(w, r)
		}
	}))

	return s
}

// TestScenarioFailureVerdictFromScrapeToFSM scrapes a stub benthos through the
// framework (factory.NewWorkerByType, then CollectObservedState) and asserts
// status.Result.Failures.ErrorFree() after the framework's JSON round trip.
// Each observation is one second later on a test clock. How the verdict
// reaches the Benthos FSM through mapObserved and IsMetricsErrorFree is tested
// in failure_window_test.go and pkg/service/benthos.
func TestScenarioFailureVerdictFromScrapeToFSM(t *testing.T) {
	stub := newFailingBenthosStub()
	defer stub.server.Close()

	// The clock starts at the real present and runs ahead of it. The worker's
	// freshness check compares PollStartedAt with real time, so it reads every
	// scrape as fresh.
	now := time.Now().Truncate(time.Second)
	restore := fsmv2benthosmonitor.SetScrapeClock(func() time.Time { return now })
	defer restore()

	spec := userSpecFor(t, "capstone-benthos", uint16(stub.server.Listener.Addr().(*net.TCPAddr).Port))
	worker, err := factory.NewWorkerByType(
		fsmv2benthosmonitor.WorkerType,
		deps.Identity{ID: "capstone-benthos", Name: "capstone-benthos", WorkerType: fsmv2benthosmonitor.WorkerType},
		deps.NewNopFSMLogger(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("factory.NewWorkerByType: %v", err)
	}

	// step and stepFor drive the verdict timeline; stepFor returns the verdict
	// of its last scrape.
	step := func() (bool, string) {
		t.Helper()
		at := now
		status := observe(t, worker, spec)
		now = now.Add(time.Second)
		if !status.Result.PollStartedAt.Equal(at) {
			t.Fatalf("scrape stamped %v, want the test clock's %v: SetScrapeClock did not reach the worker", status.Result.PollStartedAt, at)
		}
		if status.Degraded {
			t.Fatalf("at %v: a successful scrape reads Degraded (%q)", at, status.Reason)
		}
		if status.Result.Failures == nil {
			t.Fatalf("at %v: the observation carries no failure verdict", at)
		}

		return status.Result.Failures.ErrorFree()
	}
	stepFor := func(d time.Duration) (bool, string) {
		t.Helper()
		var ok bool
		var reason string
		for i := time.Duration(0); i < d; i += time.Second {
			ok, reason = step()
		}

		return ok, reason
	}
	window := constants.BenthosLogWindow

	if ok, reason := step(); !ok {
		t.Fatalf("first scrape, no errors: ErrorFree() = (false, %q), want true", reason)
	}

	stub.processorErrors.Store(1)
	if ok, reason := step(); ok || reason != "benthos reported 1 processor error in the last 10m0s" {
		t.Fatalf("one processor error: ErrorFree() = (%v, %q), want not error-free with 1 processor error", ok, reason)
	}
	if ok, reason := stepFor(window + time.Second); !ok {
		t.Fatalf("one window after the processor error, no new error and no restart: ErrorFree() = (false, %q), want true", reason)
	}

	stub.processorErrors.Store(2)
	if ok, reason := step(); ok || reason != "benthos reported 1 processor error in the last 10m0s" {
		t.Fatalf("a new processor error right after recovery: ErrorFree() = (%v, %q), want not error-free on that scrape", ok, reason)
	}
	if ok, reason := stepFor(window + time.Second); !ok {
		t.Fatalf("one window after the second processor error: ErrorFree() = (false, %q), want true", reason)
	}

	// An input that failed to connect keeps the verdict not error-free until
	// benthos reconnects, however long that takes.
	// IsBenthosRunningForSomeTimeWithoutErrors (pkg/fsm/benthos) reads this
	// verdict while a flow is starting, so it holds the flow in starting.
	stub.inputConnFailures.Store(1)
	if ok, reason := step(); ok || reason != "benthos reports 1 input not connected" {
		t.Fatalf("one failed input connection: ErrorFree() = (%v, %q), want not error-free", ok, reason)
	}
	if ok, reason := stepFor(window + time.Minute); ok {
		t.Fatalf("11m without a reconnect: ErrorFree() = (true, %q), want still not error-free", reason)
	}
	stub.inputConnUps.Store(1)
	if ok, reason := stepFor(2 * time.Second); !ok {
		t.Fatalf("two scrapes after the reconnect: ErrorFree() = (false, %q), want true", reason)
	}

	// /metrics fails for the scrapes in the loop below while benthos counts
	// more processor errors. The worker's own observation is degraded, and the
	// first successful scrape after the outage counts the new errors.
	stub.metricsDown.Store(true)
	stub.processorErrors.Store(5)
	for i := 0; i < 3; i++ {
		status := observe(t, worker, spec)
		now = now.Add(time.Second)
		if !status.Degraded {
			t.Fatalf("failed /metrics scrape %d: observation Degraded = false, want true", i+1)
		}
	}
	stub.metricsDown.Store(false)
	if ok, reason := step(); ok || reason != "benthos reported 3 processor errors in the last 10m0s" {
		t.Fatalf("first scrape after the outage: ErrorFree() = (%v, %q), want the 3 errors from the outage", ok, reason)
	}
}
