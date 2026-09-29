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

// TestRunnerCLIRouting locks the runner's routing seams so that duration
// routing and signal/exit routing are decidable without os.Exit or real OS
// signals.
func TestRunnerCLIRouting(t *testing.T) {
	t.Run("duration routing v2 takes RunConfig.Duration", func(t *testing.T) {
		runDuration, applyCtxTimeout := routeDuration(true, 5*time.Second)
		if applyCtxTimeout {
			t.Error("a v2 scenario must not bound its run with a ctx timeout; the duration is a settle window after Run returns")
		}

		if runDuration != 5*time.Second {
			t.Errorf("a v2 scenario must route --duration into RunConfig.Duration, got %v", runDuration)
		}
	})

	t.Run("duration routing v1 takes a ctx timeout", func(t *testing.T) {
		runDuration, applyCtxTimeout := routeDuration(false, 5*time.Second)
		if !applyCtxTimeout {
			t.Error("a v1 scenario must bound the whole run via a ctx timeout")
		}

		if runDuration != 0 {
			t.Errorf("a v1 scenario must not set RunConfig.Duration, got %v", runDuration)
		}
	})

	t.Run("duration routing zero stays endless on both paths", func(t *testing.T) {
		v1Duration, v1Timeout := routeDuration(false, 0)
		if v1Timeout || v1Duration != 0 {
			t.Errorf("v1 --duration 0 must stay endless: timeout=%t duration=%v", v1Timeout, v1Duration)
		}

		v2Duration, v2Timeout := routeDuration(true, 0)
		if v2Timeout || v2Duration != 0 {
			t.Errorf("v2 --duration 0 must stay endless: timeout=%t duration=%v", v2Timeout, v2Duration)
		}
	})

	t.Run("duration default: v2 without --duration settles 1s", func(t *testing.T) {
		got, defaulted := defaultDuration(true, false, 0)
		if got != defaultSettle {
			t.Errorf("a v2 scenario given no --duration must settle %s after Run returns, got %v", defaultSettle, got)
		}

		if !defaulted {
			t.Error("a v2 scenario given no --duration must report the default as applied")
		}
	})

	t.Run("duration default: explicit --duration 0 stays endless", func(t *testing.T) {
		got, defaulted := defaultDuration(true, true, 0)
		if got != 0 {
			t.Errorf("an explicit --duration 0 must stay endless, got %v", got)
		}

		if defaulted {
			t.Error("an explicit --duration 0 must not report the default as applied")
		}
	})

	t.Run("duration default: explicit --duration is kept", func(t *testing.T) {
		got, defaulted := defaultDuration(true, true, 5*time.Second)
		if got != 5*time.Second {
			t.Errorf("an explicit --duration must be kept as given, got %v", got)
		}

		if defaulted {
			t.Error("an explicit --duration must not report the default as applied")
		}
	})

	t.Run("duration default: v1 is unchanged", func(t *testing.T) {
		got, defaulted := defaultDuration(false, false, 0)
		if got != 0 {
			t.Errorf("a v1 scenario given no --duration must stay endless, got %v", got)
		}

		if defaulted {
			t.Error("a v1 scenario must never report the default as applied")
		}
	})

	t.Run("duration default and routing compose: a defaulted v2 run settles via RunConfig, explicit values pass through", func(t *testing.T) {
		effective, _ := defaultDuration(true, false, 0)

		runDuration, applyCtxTimeout := routeDuration(true, effective)
		if runDuration != defaultSettle || applyCtxTimeout {
			t.Errorf("a v2 run without --duration must settle %s via RunConfig.Duration with no ctx timeout, got duration=%v timeout=%t", defaultSettle, runDuration, applyCtxTimeout)
		}

		effective, _ = defaultDuration(true, true, 5*time.Second)

		runDuration, applyCtxTimeout = routeDuration(true, effective)
		if runDuration != 5*time.Second || applyCtxTimeout {
			t.Errorf("an explicit --duration 5s must reach RunConfig.Duration with no ctx timeout, got duration=%v timeout=%t", runDuration, applyCtxTimeout)
		}

		effective, _ = defaultDuration(true, true, 0)

		runDuration, applyCtxTimeout = routeDuration(true, effective)
		if runDuration != 0 || applyCtxTimeout {
			t.Errorf("an explicit --duration 0 must stay endless after routing, got duration=%v timeout=%t", runDuration, applyCtxTimeout)
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
		runErr := errors.New("conflicting configuration: both Scenario and ScenarioV2 are set")

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

		// A busy scenario emits far more than five identical info lines in one
		// second (state transitions, passed waits); every one must reach the
		// output, so none may be sampled away.
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
