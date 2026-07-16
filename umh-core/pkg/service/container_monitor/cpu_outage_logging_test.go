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

//go:build test

// In-package white-box tests for the sampler-outage log discipline: the
// outage Warn carries the sampler error and fires once per transition into
// the outage (not on every failure tick), recovery is logged at Info, and the
// wasThrottled transition flag is not reset by a sampler-failure blip. These
// observe the private logger and the private wasThrottled field, so they live
// in package container_monitor.
package container_monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// newOutageTestService builds a ContainerMonitorService over a scripted
// cpu.stat mock plus an observed logger. The returned closures script the
// throttle counters and toggle sampler failure.
func newOutageTestService(t *testing.T) (svc *ContainerMonitorService, logs *observer.ObservedLogs, setCounters func(nrPeriods, nrThrottled, usageUsec int64), setFails func(bool)) {
	t.Helper()

	var (
		nrPeriods, nrThrottled, usageUsec int64
		samplerFails                      bool
	)

	mockFS := filesystem.NewMockFileSystem().WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
		switch path {
		case "/sys/fs/cgroup/cpu.max":
			return []byte("200000 100000\n"), nil // quota 2.0 cores
		case "/sys/fs/cgroup/cpu.stat":
			if samplerFails {
				return nil, errors.New("cpu.stat transient read error")
			}

			return []byte(fmt.Sprintf(
				"usage_usec %d\nnr_periods %d\nnr_throttled %d\nthrottled_usec 0\n",
				usageUsec, nrPeriods, nrThrottled,
			)), nil
		default:
			return nil, errors.New("file not found: " + path)
		}
	})

	svc = NewContainerMonitorServiceWithPath(mockFS, t.TempDir())

	core, logs := observer.New(zapcore.DebugLevel)
	svc.logger = zap.New(core).Sugar()

	setCounters = func(p, th, u int64) { nrPeriods, nrThrottled, usageUsec = p, th, u }
	setFails = func(f bool) { samplerFails = f }

	return svc, logs, setCounters, setFails
}

// countLogs returns how many observed entries at the given level contain
// substr.
func countLogs(logs *observer.ObservedLogs, level zapcore.Level, substr string) int {
	n := 0

	for _, e := range logs.All() {
		if e.Level == level && strings.Contains(e.Message, substr) {
			n++
		}
	}

	return n
}

// TestOutageWarn_OncePerTransition_WithCause pins the sampler-outage log
// discipline: the Warn fires ONCE on the transition into the outage and
// carries the sampler error as the cause; repeated failure ticks stay silent;
// the first successful tick after the outage logs recovery at Info.
//
// RED assertions: three consecutive failure ticks must produce exactly 1 Warn
// (not 3), the Warn must contain "cpu.stat transient read error", and the
// recovery tick must produce an Info mentioning the recovery.
func TestOutageWarn_OncePerTransition_WithCause(t *testing.T) {
	ctx := context.Background()
	svc, logs, setCounters, setFails := newOutageTestService(t)

	// Tick 1: healthy baseline.
	setCounters(1000, 0, 1_000_000)

	if _, err := svc.GetStatus(ctx); err != nil {
		t.Fatalf("baseline GetStatus: unexpected error: %v", err)
	}

	// Ticks 2-4: sustained sampler outage.
	setFails(true)

	for i := range 3 {
		if _, err := svc.GetStatus(ctx); err != nil {
			t.Fatalf("failure tick %d GetStatus: unexpected error: %v", i, err)
		}
	}

	warns := countLogs(logs, zapcore.WarnLevel, "cgroup cpu usage unavailable")
	if warns != 1 {
		t.Fatalf("outage Warn count across 3 failure ticks: got %d, want 1 (log once on the transition INTO the outage, not every tick)", warns)
	}

	withCause := countLogs(logs, zapcore.WarnLevel, "cpu.stat transient read error")
	if withCause != 1 {
		t.Fatalf("outage Warn with the sampler error as cause: got %d, want 1 (the Warn must say WHY the sampler failed)", withCause)
	}

	// Tick 5: sampler recovers; recovery is logged at Info.
	setFails(false)
	setCounters(2000, 0, 2_000_000)

	if _, err := svc.GetStatus(ctx); err != nil {
		t.Fatalf("recovery GetStatus: unexpected error: %v", err)
	}

	infos := countLogs(logs, zapcore.InfoLevel, "cgroup cpu usage readable again")
	if infos != 1 {
		t.Fatalf("recovery Info count: got %d, want 1 (the end of the outage is logged once at Info)", infos)
	}

	// A second outage transitions again: one more Warn.
	setFails(true)

	if _, err := svc.GetStatus(ctx); err != nil {
		t.Fatalf("second outage GetStatus: unexpected error: %v", err)
	}

	warns = countLogs(logs, zapcore.WarnLevel, "cgroup cpu usage unavailable")
	if warns != 2 {
		t.Fatalf("outage Warn count after a second transition: got %d, want 2 (once per transition)", warns)
	}
}

// TestWasThrottled_SurvivesSamplerFailureBlip pins that the throttle-onset
// transition flag is updated only on successful sampler ticks: a
// sampler-failure tick in the middle of a sustained throttle must not reset
// wasThrottled, or the next successful still-throttled tick re-fires the
// onset Warn for an unchanged condition.
//
// Sequence: throttled tick (onset Warn fires), failure tick, throttled tick.
// RED assertions: wasThrottled is still true after the failure tick, and the
// onset Warn fired exactly once across the whole sequence.
func TestWasThrottled_SurvivesSamplerFailureBlip(t *testing.T) {
	ctx := context.Background()
	svc, logs, setCounters, setFails := newOutageTestService(t)

	// Tick 1: baseline the throttle ring.
	setCounters(1000, 0, 1_000_000)

	if _, err := svc.GetStatus(ctx); err != nil {
		t.Fatalf("baseline GetStatus: unexpected error: %v", err)
	}

	// Tick 2: throttle ratio 0.10 > 0.05 fires the latch; onset Warn fires.
	setCounters(2000, 100, 2_000_000)
	time.Sleep(1 * time.Second)

	if _, err := svc.GetStatus(ctx); err != nil {
		t.Fatalf("throttled tick GetStatus: unexpected error: %v", err)
	}

	if !svc.wasThrottled {
		t.Fatalf("precondition: wasThrottled must be true after the throttle fires")
	}

	// Tick 3: sampler fails mid-throttle. The flag must survive the blip.
	setFails(true)

	if _, err := svc.GetStatus(ctx); err != nil {
		t.Fatalf("failure tick GetStatus: unexpected error: %v", err)
	}

	if !svc.wasThrottled {
		t.Fatalf("wasThrottled after a sampler-failure tick: got false, want true (the flag must only update on successful sampler ticks; a blip reset re-fires the onset Warn)")
	}

	// Tick 4: sampler recovers, still throttled (ratio stays above the
	// recover mark). No second onset Warn.
	setFails(false)
	setCounters(3000, 200, 3_000_000)

	if _, err := svc.GetStatus(ctx); err != nil {
		t.Fatalf("recovered throttled tick GetStatus: unexpected error: %v", err)
	}

	onsetWarns := countLogs(logs, zapcore.WarnLevel, "CPU throttling detected")
	if onsetWarns != 1 {
		t.Fatalf("throttle onset Warn count: got %d, want 1 (a sampler blip must not re-fire the onset Warn for an unchanged throttle condition)", onsetWarns)
	}
}
