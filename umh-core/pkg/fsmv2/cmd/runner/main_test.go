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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

func TestShutdownExitCode(t *testing.T) {
	tests := []struct {
		name   string
		result *examples.RunResult
		want   int
	}{
		{
			name:   "nil result exits zero",
			result: nil,
			want:   0,
		},
		{
			name:   "clean drain exits zero",
			result: &examples.RunResult{ShutdownClean: true},
			want:   0,
		},
		{
			name: "clean drain with Err exits non-zero",
			result: &examples.RunResult{
				ShutdownClean: true,
				Err:           errors.New("the scenario does not expect this warning: probe"),
			},
			want: 1,
		},
		{
			name:   "unclean drain exits non-zero",
			result: &examples.RunResult{ShutdownClean: false},
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shutdownExitCode(tt.result); got != tt.want {
				t.Errorf("shutdownExitCode(%+v) = %d, want %d", tt.result, got, tt.want)
			}
		})
	}
}

func TestFatalMessage(t *testing.T) {
	failedRun := fmt.Errorf("scenario %q %w: %w", "probe", examples.ErrScenarioFailed, errors.New("the mood file is corrupt"))
	if got := fatalMessage(failedRun); got != "Scenario failed" {
		t.Errorf("fatalMessage(%v) = %q, want %q", failedRun, got, "Scenario failed")
	}

	_, notStarted := examples.Run(context.Background(), examples.RunConfig{
		ScenarioV2: examples.ScenarioV2{Name: "probe"},
	})
	if notStarted == nil {
		t.Fatal("examples.Run must reject a v2 scenario whose Run is nil")
	}

	if got := fatalMessage(notStarted); got != "Failed to start scenario" {
		t.Errorf("fatalMessage(%v) = %q, want %q", notStarted, got, "Failed to start scenario")
	}
}

func TestRunnerCLIRouting(t *testing.T) {
	t.Run("duration default: without --duration settles 1s", func(t *testing.T) {
		got, defaulted := resolveDuration(false, 0)
		if got != defaultSettleWindow {
			t.Errorf("a run given no --duration must settle %s after Run returns, got %v", defaultSettleWindow, got)
		}

		if !defaulted {
			t.Error("a run given no --duration must report the default as applied")
		}
	})

	t.Run("duration default: explicit --duration 0 stays endless", func(t *testing.T) {
		got, defaulted := resolveDuration(true, 0)
		if got != 0 {
			t.Errorf("an explicit --duration 0 must stay endless, got %v", got)
		}

		if defaulted {
			t.Error("an explicit --duration 0 must not report the default as applied")
		}
	})

	t.Run("duration default: explicit --duration is kept", func(t *testing.T) {
		got, defaulted := resolveDuration(true, 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("an explicit --duration must be kept as given, got %v", got)
		}

		if defaulted {
			t.Error("an explicit --duration must not report the default as applied")
		}
	})

	t.Run("duration flag detection: an explicit --duration is seen, its absence is not", func(t *testing.T) {
		withFlag := flag.NewFlagSet("runner", flag.ContinueOnError)
		withFlag.Duration("duration", 0, "")

		if err := withFlag.Parse([]string{"--duration=5s"}); err != nil {
			t.Fatalf("parse with --duration failed: %v", err)
		}

		if !durationWasSet(withFlag) {
			t.Error("an explicit --duration must be detected as set")
		}

		withoutFlag := flag.NewFlagSet("runner", flag.ContinueOnError)
		withoutFlag.Duration("duration", 0, "")

		if err := withoutFlag.Parse([]string{}); err != nil {
			t.Fatalf("parse without --duration failed: %v", err)
		}

		if durationWasSet(withoutFlag) {
			t.Error("a run without --duration must not be detected as set")
		}
	})

	t.Run("interrupt-induced ctx.Err is a clean exit", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// The runner wraps Run's error; an interrupt that cancelled ctx
		// surfaces as a ctx.Err()-wrapping error and must read as clean, not
		// as a fatal startup failure.
		runErr := fmt.Errorf("scenario %q failed: %w", "noop", ctx.Err())
		if !isCleanInterruptExit(runErr, ctx.Err()) {
			t.Error("an interrupt-induced ctx.Err() must be reported as a clean exit, not a fatal exit-1")
		}
	})

	t.Run("genuine startup failure is not a clean exit", func(t *testing.T) {
		ctx := context.Background()
		runErr := errors.New("v2 scenario \"probe\" is not properly configured: Run is nil")

		if isCleanInterruptExit(runErr, ctx.Err()) {
			t.Error("a genuine startup failure with no ctx cancellation must remain fatal exit-1")
		}
	})

	t.Run("second signal force-exits", func(t *testing.T) {
		sigCh := make(chan os.Signal, 2)
		done := make(chan struct{})

		firstSignalSeen := make(chan struct{})
		onFirstSignal := func() { close(firstSignalSeen) }

		forced := make(chan struct{})
		forceExit := func() { close(forced) }

		go handleSignals(sigCh, done, onFirstSignal, forceExit)

		sigCh <- syscall.SIGINT

		select {
		case <-firstSignalSeen:
		case <-time.After(2 * time.Second):
			t.Fatal("the first signal must trigger teardown")
		}

		sigCh <- syscall.SIGINT

		select {
		case <-forced:
		case <-time.After(2 * time.Second):
			t.Fatal("a second signal must force-exit instead of waiting for Done")
		}
	})

	t.Run("run logger keeps every line: 20 identical info lines in a second all arrive", func(t *testing.T) {
		obsCore, logs := observer.New(zapcore.InfoLevel)

		runLogger := newRunLogger(zap.New(obsCore))

		// 20 is more identical lines in one second than deps.samplerWrap lets through.
		for range 20 {
			runLogger.Info("state_transition")
		}

		if got := len(logs.TakeAll()); got != 20 {
			t.Errorf("the run logger must keep every log line a scenario run emits, got %d of 20", got)
		}
	})

	t.Run("first signal then done returns cleanly without force-exit", func(t *testing.T) {
		sigCh := make(chan os.Signal, 1)
		done := make(chan struct{})

		firstSignalSeen := make(chan struct{})
		onFirstSignal := func() { close(firstSignalSeen) }

		forced := make(chan struct{})
		forceExit := func() { close(forced) }

		returned := make(chan struct{})

		go func() {
			handleSignals(sigCh, done, onFirstSignal, forceExit)
			close(returned)
		}()

		sigCh <- syscall.SIGINT

		select {
		case <-firstSignalSeen:
		case <-time.After(2 * time.Second):
			t.Fatal("the first signal must trigger teardown")
		}

		// Graceful teardown completes: the runner closes done. handleSignals
		// must return without force-exiting.
		close(done)

		select {
		case <-returned:
		case <-time.After(2 * time.Second):
			t.Fatal("handleSignals must return once done closes, not block")
		}

		select {
		case <-forced:
			t.Error("a clean teardown must not force-exit")
		default:
		}
	})
}

func TestExpectedFields(t *testing.T) {
	t.Run("a scenario declaring all three kinds gets one field per kind", func(t *testing.T) {
		full := examples.ScenarioV2{
			ExpectedErrors:      []string{"action_failed"},
			ExpectedErrorCauses: []error{errors.New("boom")},
			ExpectedWarnings:    []string{"slow"},
		}

		fields := expectedFields(full)

		if len(fields) != 3 {
			t.Fatalf("expectedFields must return three fields for a scenario declaring errors, causes and warnings, got %d", len(fields))
		}

		want := []struct {
			key    string
			values []string
		}{
			{"expected_errors", []string{"action_failed"}},
			{"expected_error_causes", []string{"boom"}},
			{"expected_warnings", []string{"slow"}},
		}

		for i, w := range want {
			if fields[i].Key != w.key {
				t.Errorf("field %d must be %q, got %q", i, w.key, fields[i].Key)
			}

			got, ok := fields[i].Interface.([]string)
			if !ok {
				t.Errorf("field %q must hold a []string, got %T", w.key, fields[i].Interface)
				continue
			}

			if !slices.Equal(got, w.values) {
				t.Errorf("field %q must hold %v, got %v", w.key, w.values, got)
			}
		}
	})

	t.Run("a scenario declaring only warnings gets only the warnings field", func(t *testing.T) {
		fields := expectedFields(examples.ScenarioV2{ExpectedWarnings: []string{"slow"}})

		if len(fields) != 1 || fields[0].Key != "expected_warnings" {
			t.Fatalf("expectedFields must return only expected_warnings, got %v", fields)
		}
	})

	t.Run("a nil expected cause matches nothing and yields no entry", func(t *testing.T) {
		fields := expectedFields(examples.ScenarioV2{
			ExpectedErrorCauses: []error{errors.New("boom"), nil},
		})

		if len(fields) != 1 || fields[0].Key != "expected_error_causes" {
			t.Fatalf("expectedFields must return only expected_error_causes, got %v", fields)
		}

		got, ok := fields[0].Interface.([]string)
		if !ok || !slices.Equal(got, []string{"boom"}) {
			t.Errorf("expected_error_causes must hold only the non-nil causes, got %T %v", fields[0].Interface, got)
		}
	})

	t.Run("a scenario whose expected causes are all nil gets no causes field", func(t *testing.T) {
		fields := expectedFields(examples.ScenarioV2{ExpectedErrorCauses: []error{nil}})

		if len(fields) != 0 {
			t.Errorf("expectedFields must return no field when every expected cause is nil, got %v", fields)
		}
	})

	t.Run("a scenario declaring nothing gets no field", func(t *testing.T) {
		if got := expectedFields(examples.ScenarioV2{}); len(got) != 0 {
			t.Errorf("expectedFields must return no field for a scenario declaring nothing, got %d", len(got))
		}
	})
}

func TestStartingScenarioFields(t *testing.T) {
	obsCore, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(obsCore)

	full := examples.ScenarioV2{
		ExpectedErrors:      []string{"action_failed"},
		ExpectedErrorCauses: []error{errors.New("boom"), nil},
		ExpectedWarnings:    []string{"slow"},
	}

	logger.Info("Starting scenario",
		startingScenarioFields("probe", "a probe", "endless (until Ctrl+C)", time.Second, full)...)

	entries := logs.TakeAll()
	if len(entries) != 1 {
		t.Fatalf("the starting line must emit one entry, got %d", len(entries))
	}

	context := entries[0].ContextMap()

	if got := context["name"]; got != "probe" {
		t.Errorf("the starting line must carry the scenario name, got %v", got)
	}

	wantFields := []struct {
		key    string
		values []string
	}{
		{"expected_errors", []string{"action_failed"}},
		{"expected_error_causes", []string{"boom"}},
		{"expected_warnings", []string{"slow"}},
	}

	for _, w := range wantFields {
		got, ok := context[w.key].([]string)
		if !ok || !slices.Equal(got, w.values) {
			t.Errorf("the starting line must carry %s %v (a nil expected cause matches nothing and is left out), got %T %v",
				w.key, w.values, context[w.key], context[w.key])
		}
	}

	logger.Info("Starting scenario",
		startingScenarioFields("probe", "a probe", "endless (until Ctrl+C)", time.Second,
			examples.ScenarioV2{ExpectedWarnings: []string{"slow"}})...)

	entries = logs.TakeAll()
	if len(entries) != 1 {
		t.Fatalf("a scenario declaring only warnings must still emit one starting entry, got %d", len(entries))
	}

	context = entries[0].ContextMap()

	got, ok := context["expected_warnings"].([]string)
	if !ok || !slices.Equal(got, []string{"slow"}) {
		t.Errorf("the starting line must carry expected_warnings %v, got %T %v", []string{"slow"}, context["expected_warnings"], context["expected_warnings"])
	}

	for _, key := range []string{"expected_errors", "expected_error_causes"} {
		if _, has := context[key]; has {
			t.Errorf("a scenario declaring only warnings must not carry %s", key)
		}
	}
}
