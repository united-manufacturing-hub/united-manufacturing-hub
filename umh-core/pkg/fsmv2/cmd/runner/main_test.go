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
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
)

// TestShutdownExitCode locks the exit-code mapping for a completed scenario
// run. A run whose supervisor did not drain cleanly within its budget
// (ShutdownClean=false) must exit non-zero so an outer harness/CI can detect a
// degraded shutdown; every other case (nil result, or a clean drain) exits 0.
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

// TestRunnerCLIRouting locks the three routing seams the runner must expose so
// that duration routing and signal/exit routing are decidable without os.Exit
// or real OS signals:
//
//   - routeDuration:      a v2 scenario gets RunConfig.Duration (a post-driver
//     settle window) while a v1 scenario routes its --duration into a ctx
//     timeout that bounds the whole run; --duration 0 stays endless on both.
//   - isCleanInterruptExit: a run error that wraps an interrupt-induced
//     ctx.Err() is a clean exit, not a fatal "Failed to start scenario" exit-1.
//   - handleSignals:       the first SIGINT triggers teardown (the runner
//     cancels and tears down gracefully); a second SIGINT force-exits.
func TestRunnerCLIRouting(t *testing.T) {
	t.Run("duration routing v2 takes RunConfig.Duration", func(t *testing.T) {
		runDuration, applyCtxTimeout := routeDuration(true, 5*time.Second)
		if applyCtxTimeout {
			t.Error("a v2 scenario must not bound its run with a ctx timeout; the duration is a post-driver settle window")
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

	t.Run("interrupt-induced ctx.Err is a clean exit", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// The runner wraps Run's error; an interrupt that cancelled ctx
		// surfaces as a ctx.Err()-wrapping error and must read as clean, not
		// as a fatal startup failure.
		runErr := fmt.Errorf("scenario %q driver failed: %w", "noop", ctx.Err())
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
