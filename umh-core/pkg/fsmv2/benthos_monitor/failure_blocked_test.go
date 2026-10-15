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
	"testing"
	"time"

	benthosmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
)

// outputStep is one scrape: its time after failureT, and the counters it shows.
type outputStep struct {
	at              time.Duration
	sent, errors    int64
	processorErrors int64
}

func (s outputStep) metrics() benthosmonitorservice.Metrics {
	return benthosmonitorservice.Metrics{
		Inputs:  map[string]benthosmonitorservice.InputInstance{"root.input": {Received: 10, ConnectionUp: 1}},
		Outputs: map[string]benthosmonitorservice.OutputInstance{"root.output": {Sent: s.sent, Error: s.errors, ConnectionUp: 1}},
		Process: benthosmonitorservice.ProcessMetrics{Processors: map[string]benthosmonitorservice.ProcessorMetrics{
			"root.pipeline.processors.0": {Error: s.processorErrors},
		}},
	}
}

// observeVerdict observes each step in order on fresh deps and returns the
// verdict at the last one.
func observeVerdict(steps []outputStep) benthosmonitorservice.FailureVerdict {
	d := &benthosMonitorDeps{}
	var v benthosmonitorservice.FailureVerdict
	for _, s := range steps {
		_, v = observe(d, testPort, s.metrics(), failureT.Add(s.at))
	}

	return v
}

// observeSteps is observeVerdict's reason.
func observeSteps(steps []outputStep) (bool, string) {
	v := observeVerdict(steps)

	return v.ErrorFree()
}

// outputRun is observeSteps with one scrape per second for n seconds. At
// second i the output has sent(i) messages and errors(i) errors.
func outputRun(n int, sent, errors func(i int) int64) (bool, string) {
	steps := make([]outputStep, 0, n)
	for i := 0; i < n; i++ {
		steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: sent(i), errors: errors(i)})
	}

	return observeSteps(steps)
}

func constant(c int64) func(int) int64 { return func(int) int64 { return c } }
func perSecond(i int) int64            { return int64(i) }

// TestObserveOutputBlocked: writes fail every second and nothing is sent for
// more than two minutes, so the reason says the output is blocked.
func TestObserveOutputBlocked(t *testing.T) {
	ok, reason := outputRun(150, constant(10), perSecond)
	if want := "benthos output blocked: no message written for 2m0s"; ok || reason != want {
		t.Fatalf("errors every second, nothing sent for 149s: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveBlockedReasonNamesHowLong: the reason names how long nothing was
// written, in whole minutes, not the 2 minutes it takes to call it blocked.
func TestObserveBlockedReasonNamesHowLong(t *testing.T) {
	ok, reason := outputRun(200, constant(10), perSecond)
	if want := "benthos output blocked: no message written for 3m0s"; ok || reason != want {
		t.Fatalf("errors every second, nothing sent for 199s: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveOutputErrorsWhileSendingIsNotBlocked: the same errors while
// messages still go out are output errors, not a blocked output.
func TestObserveOutputErrorsWhileSendingIsNotBlocked(t *testing.T) {
	ok, reason := outputRun(150, perSecond, perSecond)
	if want := "benthos reported 149 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("errors and sends every second: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveFailingOutputUnderTwoMinutesIsNotBlocked: 90 seconds after umh-core
// starts, nothing shows that nothing was written for 2 minutes.
func TestObserveFailingOutputUnderTwoMinutesIsNotBlocked(t *testing.T) {
	ok, reason := outputRun(90, constant(10), perSecond)
	if want := "benthos reported 89 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("errors every second, nothing sent for 89s: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveQuietOutputAfterOneErrorIsNotBlocked: one error, then a flow with
// no traffic, is an output error. Nothing written does not mean blocked when
// writes stopped failing.
func TestObserveQuietOutputAfterOneErrorIsNotBlocked(t *testing.T) {
	errors := func(i int) int64 { return min(int64(i), 1) }
	ok, reason := outputRun(200, constant(10), errors)
	if want := "benthos reported 1 output error in the last 10m0s"; ok || reason != want {
		t.Fatalf("one error, then no traffic for 200s: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveQuietOutputAfterTwoErrorsIsNotBlocked: two errors are still below
// the three a blocked output needs.
func TestObserveQuietOutputAfterTwoErrorsIsNotBlocked(t *testing.T) {
	errors := func(i int) int64 { return min(int64(i), 2) }
	ok, reason := outputRun(200, constant(10), errors)
	if want := "benthos reported 2 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("two errors, then no traffic for 200s: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveOutputThatStoppedFailingIsNotBlocked: five writes failed, then
// none for over 2 minutes. Nothing was written, but the output is not failing
// now.
func TestObserveOutputThatStoppedFailingIsNotBlocked(t *testing.T) {
	errors := func(i int) int64 { return min(int64(i), 5) }
	ok, reason := outputRun(200, constant(10), errors)
	if want := "benthos reported 5 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("five errors, then none for 195s: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveBlockedOutputRecovers: once messages go out again, the reason
// goes back to counting output errors.
func TestObserveBlockedOutputRecovers(t *testing.T) {
	sent := func(i int) int64 {
		if i < 150 {
			return 10
		}

		return int64(i)
	}
	errors := func(i int) int64 { return min(int64(i), 150) }
	ok, reason := outputRun(155, sent, errors)
	if want := "benthos reported 150 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("5s after messages go out again: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveBlockedOutputComesFirst: a blocked output is named even while a
// processor error counter rises faster than the output error counter.
func TestObserveBlockedOutputComesFirst(t *testing.T) {
	steps := make([]outputStep, 0, 150)
	for i := int64(0); i < 150; i++ {
		steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: 10, errors: i, processorErrors: 3 * i})
	}

	v := observeVerdict(steps)
	ok, reason := v.ErrorFree()
	if want := "benthos output blocked: no message written for 2m0s"; ok || reason != want {
		t.Fatalf("output blocked while processor errors rise 3 a second: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}

	for _, e := range v.Fired {
		if e.Signal != outputError && e.Blocked != 0 {
			t.Fatalf("entry %q: Blocked = %v, want 0 on every entry but the output error entry", e.Signal, e.Blocked)
		}
	}
}

// TestObserveSendDuringScrapePauseIsNotBlocked: messages went out during a
// 3-minute pause in scrapes, so errors in the seconds after it are not a
// blocked output.
func TestObserveSendDuringScrapePauseIsNotBlocked(t *testing.T) {
	steps := make([]outputStep, 0, 66)
	for i := int64(0); i < 60; i++ {
		steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: i})
	}
	for k := int64(0); k < 6; k++ {
		steps = append(steps, outputStep{at: (240 + time.Duration(k)) * time.Second, sent: 300, errors: k + 1})
	}

	ok, reason := observeSteps(steps)
	if want := "benthos reported 6 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("sent rose during the pause, 6 errors in the 6s after it: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveRestartStartsTheBlockedTimeAgain: a quiet flow restarts and its
// output then fails every write. A minute after the restart it is not blocked,
// and a little over 2 minutes after it it is.
func TestObserveRestartStartsTheBlockedTimeAgain(t *testing.T) {
	steps := make([]outputStep, 0, 331)
	for i := int64(0); i < 200; i++ {
		steps = append(steps, outputStep{at: time.Duration(i) * time.Second, sent: 10})
	}
	for i := int64(200); i <= 330; i++ {
		steps = append(steps, outputStep{at: time.Duration(i) * time.Second, errors: i - 199})
	}

	ok, reason := observeSteps(steps[:261])
	if want := "benthos reported 61 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("60s after the restart: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}

	ok, reason = observeSteps(steps)
	if want := "benthos output blocked: no message written for 2m0s"; ok || reason != want {
		t.Fatalf("130s after the restart: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveErrorsBeforeFirstScrapeDoNotCountTowardBlocked: output_error is
// already 100 at umh-core's first scrape. Two more errors after it are below
// the three a blocked output needs.
func TestObserveErrorsBeforeFirstScrapeDoNotCountTowardBlocked(t *testing.T) {
	errors := func(i int) int64 { return 100 + min(max(int64(i)-125, 0), 2) }
	ok, reason := outputRun(130, constant(10), errors)
	if want := "benthos reported 2 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("counter at 100 before the first scrape, 2 errors after it: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}

// TestObserveErrorsWhileSendingDoNotCountTowardBlocked: 99 writes failed while
// messages still went out. After the last message, two more failed, below the
// three a blocked output needs.
func TestObserveErrorsWhileSendingDoNotCountTowardBlocked(t *testing.T) {
	sent := func(i int) int64 { return min(int64(i), 99) }
	errors := func(i int) int64 { return min(int64(i), 99) + min(max(int64(i)-225, 0), 2) }
	ok, reason := outputRun(230, sent, errors)
	if want := "benthos reported 101 output errors in the last 10m0s"; ok || reason != want {
		t.Fatalf("errors while sending, then 2 after the last send: ErrorFree() = (%v, %q), want (false, %q)", ok, reason, want)
	}
}
