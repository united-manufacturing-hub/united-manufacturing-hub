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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
	benthosmonitorfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// failureT is the instant of the first output error in every timeline below.
var failureT = time.Unix(1_800_000_000, 0)

// outputErrors returns a scrape whose output error total is n.
func outputErrors(n int64) benthosmonitorservice.Metrics {
	return benthosmonitorservice.Metrics{
		Inputs:  map[string]benthosmonitorservice.InputInstance{"root.input": {Received: 10}},
		Outputs: map[string]benthosmonitorservice.OutputInstance{"root.output": {Sent: 10, Error: n}},
	}
}

// errorTimeline drives observe once per second on one deps value. errorsAt
// gives the output error total for each scrape time.
type errorTimeline struct {
	t        *testing.T
	d        *benthosMonitorDeps
	next     time.Time
	errorsAt func(at time.Time) int64
	verdict  benthosmonitorservice.FailureVerdict
}

func newErrorTimeline(t *testing.T, start time.Time, errorsAt func(time.Time) int64) *errorTimeline {
	return &errorTimeline{t: t, d: &benthosMonitorDeps{}, next: start, errorsAt: errorsAt}
}

// runTo observes every second up to and including until.
func (tl *errorTimeline) runTo(until time.Time) {
	for !tl.next.After(until) {
		_, tl.verdict = observe(tl.d, testPort, outputErrors(tl.errorsAt(tl.next)), tl.next)
		tl.next = tl.next.Add(time.Second)
	}
}

// errorFree reads the verdict the way the Benthos FSM receives it: through
// mapObserved into BenthosMetrics.Failures.
func (tl *errorTimeline) errorFree() (bool, string) {
	tl.t.Helper()
	v := tl.verdict
	observed := mapObserved(testConfig(), simple.Status[BenthosMonitorStatus]{Result: BenthosMonitorStatus{Failures: &v}})
	failures := observed.(benthosmonitorfsm.BenthosMonitorObservedState).ServiceInfo.BenthosStatus.LastScan.BenthosMetrics.Failures
	if failures == nil {
		tl.t.Fatalf("mapObserved dropped the verdict: BenthosMetrics.Failures is nil")
	}

	return failures.ErrorFree()
}

// TestObserveOutputErrorHoldsForLogWindow: one output error at T keeps the
// metrics not error-free until the scrape before it leaves the 10-minute window.
func TestObserveOutputErrorHoldsForLogWindow(t *testing.T) {
	tl := newErrorTimeline(t, failureT.Add(-time.Second), func(at time.Time) int64 {
		if at.Before(failureT) {
			return 0
		}

		return 1
	})

	tl.runTo(failureT.Add(time.Second))
	ok, reason := tl.errorFree()
	if ok || reason != "benthos reported 1 output error in the last 10m0s" {
		t.Fatalf("at T+1s: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, "benthos reported 1 output error in the last 10m0s")
	}
	if n := len(tl.verdict.Fired); n != 1 {
		t.Fatalf("at T+1s: %d fired entries, want 1", n)
	}
	e := tl.verdict.Fired[0]
	if e.Signal != "output_error" || e.Label != "output error" || e.Count != 1 || !e.Since.Equal(failureT) || e.Window != constants.BenthosLogWindow {
		t.Errorf("at T+1s: entry = %+v, want Signal output_error, Label output error, Count 1, Since T, Window %v", e, constants.BenthosLogWindow)
	}
	if !tl.verdict.ObservedAt.Equal(failureT.Add(time.Second)) {
		t.Errorf("at T+1s: ObservedAt = %v, want T+1s", tl.verdict.ObservedAt)
	}

	// With a span of 9m59s the T-1s scrape would already be out of the window
	// here, and the signal would release.
	tl.runTo(failureT.Add(10*time.Minute - time.Second))
	if ok, reason := tl.errorFree(); ok {
		t.Fatalf("at T+10m-1s: ErrorFree() = (true, %q), want false (the T-1s scrape is still in the window)", reason)
	}

	tl.runTo(failureT.Add(10*time.Minute + time.Second))
	if ok, reason := tl.errorFree(); !ok {
		t.Fatalf("at T+10m+1s: ErrorFree() = (false, %q), want true (no rise in the last 10 minutes)", reason)
	}
}

// TestObserveSecondOutputErrorRestartsWindow: a second error at T+5m holds the
// verdict until 10 minutes after that second error.
func TestObserveSecondOutputErrorRestartsWindow(t *testing.T) {
	tl := newErrorTimeline(t, failureT.Add(-time.Second), func(at time.Time) int64 {
		switch {
		case at.Before(failureT):
			return 0
		case at.Before(failureT.Add(5 * time.Minute)):
			return 1
		default:
			return 2
		}
	})

	tl.runTo(failureT.Add(14 * time.Minute))
	if ok, reason := tl.errorFree(); ok {
		t.Fatalf("at T+14m: ErrorFree() = (true, %q), want false (the T+5m error is 9 minutes old)", reason)
	}

	tl.runTo(failureT.Add(15*time.Minute + time.Second))
	if ok, reason := tl.errorFree(); !ok {
		t.Fatalf("at T+15m+1s: ErrorFree() = (false, %q), want true (the last error is more than 10 minutes old)", reason)
	}
}

// TestObserveFirstScrapeDoesNotBackfill: errors counted before the worker's
// first scrape are not judged, so a counter already at 5 is error-free.
func TestObserveFirstScrapeDoesNotBackfill(t *testing.T) {
	tl := newErrorTimeline(t, failureT, func(time.Time) int64 { return 5 })

	for i := 0; i < 5; i++ {
		tl.runTo(failureT.Add(time.Duration(i) * time.Second))
		if ok, reason := tl.errorFree(); !ok {
			t.Fatalf("scrape %d with a constant output error total of 5: ErrorFree() = (false, %q), want true (no backfill)", i, reason)
		}
	}
}

// TestObserveReasonNamesCurrentIncrease: the reason reports the increase as it
// stands now, not the increase at the moment the signal fired.
func TestObserveReasonNamesCurrentIncrease(t *testing.T) {
	tl := newErrorTimeline(t, failureT.Add(-time.Second), func(at time.Time) int64 {
		switch {
		case at.Before(failureT):
			return 0
		case at.Before(failureT.Add(3 * time.Second)):
			return 1
		default:
			return 5
		}
	})

	tl.runTo(failureT.Add(3 * time.Second))
	ok, reason := tl.errorFree()
	if ok || reason != "benthos reported 5 output errors in the last 10m0s" {
		t.Fatalf("at T+3s: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, "benthos reported 5 output errors in the last 10m0s")
	}
}

// TestPollCarriesFailureVerdict: Poll stores observe's verdict on the status,
// so a rise between two real scrapes reaches the status as not error-free.
func TestPollCarriesFailureVerdict(t *testing.T) {
	var outputError int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			_, _ = w.Write([]byte("pong"))
		case "/ready":
			_, _ = w.Write([]byte(`{"error":""}`))
		case "/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/metrics":
			_, _ = fmt.Fprintf(w, "output_sent{label=\"\",path=\"root.output\"} 10\n")
			_, _ = fmt.Fprintf(w, "output_error{label=\"\",path=\"root.output\"} %d\n", outputError)
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
	if status.Failures == nil {
		t.Fatalf("first Poll: status.Failures is nil, want the verdict")
	}
	if ok, reason := status.Failures.ErrorFree(); !ok {
		t.Errorf("first Poll: ErrorFree() = (false, %q), want true", reason)
	}

	outputError = 2
	status, err = Poll(context.Background(), deps, cfg)
	if err != nil {
		t.Fatalf("second Poll errored: %v", err)
	}
	if status.Failures == nil {
		t.Fatalf("second Poll: status.Failures is nil, want the verdict")
	}
	if ok, _ := status.Failures.ErrorFree(); ok {
		t.Errorf("second Poll after output_error rose 0 -> 2: ErrorFree() = true, want false")
	}
}

// failureFeed is the diagnosis.Run feed for table(): every counter readable
// and unchanged, or every counter unknown.
type failureFeed struct{}

func (failureFeed) Readable(time.Time) sample {
	s := sample{
		inputReceived: diagnosis.Known(0),
		outputSent:    diagnosis.Known(0),
		errors:        map[string]diagnosis.Reading{},
	}
	for _, c := range errorCounters {
		s.errors[c.name] = diagnosis.Known(0)
	}

	return s
}

// Unreadable leaves every error reading out of the map; a missing key reads
// as an unknown reading.
func (failureFeed) Unreadable(time.Time) sample {
	return sample{
		inputReceived: diagnosis.Unknown(),
		outputSent:    diagnosis.Unknown(),
		errors:        map[string]diagnosis.Reading{},
	}
}

// TestTableConformance drives every signal of table() through the generated
// diagnosis suite. An error signal's increase reduction needs two readings,
// so it is first ready on the second tick.
func TestTableConformance(t *testing.T) {
	errorWant := map[diagnosis.Case]diagnosis.Availability{
		diagnosis.CaseLive:          diagnosis.Ready,
		diagnosis.CaseBriefOutage:   diagnosis.NoneReady,
		diagnosis.CaseLongOutage:    diagnosis.AllAbsent,
		diagnosis.CaseUnsupported:   diagnosis.Ready,
		diagnosis.CasePostOutageDip: diagnosis.NoneReady,
		diagnosis.CaseBelowFloor:    diagnosis.NoneReady,
	}
	want := map[string]map[diagnosis.Case]diagnosis.Availability{}
	for _, c := range errorCounters {
		want[c.name] = errorWant
	}

	outcomes := diagnosis.Run(table(), diagnosis.NewEnvironment(), failureFeed{})
	seen := map[string]map[diagnosis.Case]bool{}
	for _, o := range outcomes {
		if seen[o.Signal] == nil {
			seen[o.Signal] = map[diagnosis.Case]bool{}
		}
		seen[o.Signal][o.Case] = true
		w, ok := want[o.Signal]
		if !ok {
			t.Errorf("signal %q: not an errorCounters entry", o.Signal)

			continue
		}
		if o.Availability != w[o.Case] {
			t.Errorf("signal %q, case %v: Availability %v, want %v", o.Signal, o.Case, o.Availability, w[o.Case])
		}
	}

	// A signal table() dropped has no outcome, so the loop above cannot see
	// it. Every error counter must reach every case.
	for name, w := range want {
		for cs := range w {
			if !seen[name][cs] {
				t.Errorf("signal %q, case %v: no outcome, want one per signal and case", name, cs)
			}
		}
	}
}
