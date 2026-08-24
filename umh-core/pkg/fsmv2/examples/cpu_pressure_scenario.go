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

package examples

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	// cpuPressureBase is where the fake machine serves its cgroup files. It has
	// to equal the base the production sampler reads, which is the unexported
	// cgroupBase in pkg/fsmv2/cpu. Nothing checks that they match: they are kept
	// equal by hand, and a mismatch leaves every read failing.
	cpuPressureBase = "/sys/fs/cgroup"

	// cpuPressureCalm is one point under the pressure signal's 0.20 fire mark,
	// so the machine starts healthy on a number that is nearly the threshold
	// rather than nowhere near it. A start far below the mark would leave the
	// later crossing provable by a much cruder change than the one this
	// scenario makes.
	cpuPressureCalm = 0.19

	// cpuPressureFiring is over that fire mark, so the signal fires.
	cpuPressureFiring = 0.25

	// cpuPressureCores is the fake machine's CPU count, and cpuPressureHostBusy
	// how much of it the whole machine is using. Four cores at 60% busy leaves
	// 4 - 2.4 - 1.0 = 0.6 cores of headroom over the one-core reserve the
	// capacity signal keeps, which is above that signal's clear mark. So
	// capacity stays quiet at both pressures, and the verdict can only move for
	// the reason this scenario is about.
	cpuPressureCores    = 4
	cpuPressureHostBusy = 0.60

	// cpuPressureUsageCores is what this instance itself is using, well under
	// the machine's own busy time. It is the smaller half of the split, so the
	// machine's load is mostly somebody else's.
	cpuPressureUsageCores = 0.5

	// cpuPressureMachineTick is how much fake machine time one real tick
	// advances. Counters and the real clock have to move together: every rate
	// the sampler publishes is a counter delta over the gap between two real
	// sample timestamps, so a box ticking slower than real time serves a
	// machine idler than the one stated. At a tenth of the worker's own
	// one-second poll cadence, a whole tick landing on either side of a poll
	// moves the rate that poll reads by a tenth, which is far inside the margin
	// the headroom figure above leaves.
	cpuPressureMachineTick = 100 * time.Millisecond

	// cpuPressureHold is how long each condition is held before the story moves
	// on. The CPU worker polls once a second, so five seconds is five readings:
	// one that can catch the change and four more that have to agree with it. A
	// shorter hold would leave the verdict resting on a single poll landing at
	// the right moment, which is indistinguishable from a verdict that settled.
	cpuPressureHold = 5 * time.Second

	// cpuPressureReadyPoll is how often the driver asks whether the worker has
	// read the machine yet. It is well under the worker's own poll cadence, so
	// the wait costs at most one worker poll rather than one of these.
	cpuPressureReadyPoll = 50 * time.Millisecond
)

// CPUPressureScenarioV2 drives the real CPU monitor over a fake machine that is
// busy but not full, and steps its PSI pressure across the mark at which the
// pressure signal fires.
//
// The story is that a machine can be degraded with cores to spare, because
// tasks are queueing rather than because capacity ran out. Sixty percent of
// four cores leaves the capacity signal clear throughout; pressure alone moves,
// from one point under its fire mark to over it.
//
// # CLI Usage
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario=cpu-pressure --duration=30s --log-level=debug
//
// Debug is what makes the answer visible. The CPU worker's MonitorSpec declares
// no Health function, so its verdict never moves the worker's FSM state and
// nothing about the verdict is logged at info. The collector's observed_changed
// line, which is a debug line, is where the verdict flip shows up. Run it at
// info and the page carries the two machine conditions and the supervisor's
// spawn and teardown, with nothing in between.
var CPUPressureScenarioV2 = ScenarioV2{
	Name:        "cpu-pressure",
	Description: "Steps a fake machine's CPU pressure over its fire mark while capacity stays clear (v2)",
	Driver:      driveCPUPressure,
}

// driveCPUPressure publishes a fake machine, spawns the CPU monitor onto it
// through the migration-API client, holds the calm condition, then raises the
// pressure and holds again.
//
// It judges nothing. What the worker made of either condition is read back
// after the run, from the store, by the spec beside this file.
func driveCPUPressure(ctx context.Context, env Env) error {
	calm := cpuPressureMachine(cpuPressureCalm)
	machine := newTickingBox(cpuPressureBase, calm)

	// Published before the Upsert below, because cpu.NewDeps reads this key
	// once, at the moment the supervisor spawns the worker; a filesystem
	// published after that spawn reaches nothing. Cleared on the way out
	// because the key is process-global, so a driver that left it set would
	// hand the next scenario in this process its fake machine.
	register.SetDeps[filesystem.Service](fsmv2cpu.FilesystemDepsKey, machine.FS())
	defer register.ClearDeps(fsmv2cpu.FilesystemDepsKey)

	machine.Start(cpuPressureMachineTick)
	defer machine.Stop()

	announceMachine(calm)

	// Nil config: CPUConfig is an empty struct, and this is the same call the
	// config worker makes in production.
	if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
		return fmt.Errorf("upsert cpu monitor: %w", err)
	}

	if err := awaitFirstCPUReading(ctx, env.Client); err != nil {
		return fmt.Errorf("wait for the cpu monitor's first reading: %w", err)
	}

	if err := holdFor(ctx, cpuPressureHold); err != nil {
		return err
	}

	firing := cpuPressureMachine(cpuPressureFiring)
	machine.Set(firing)
	announceMachine(firing)

	return holdFor(ctx, cpuPressureHold)
}

// cpuPressureMachine is the machine this scenario runs on, at the given PSI
// pressure. Everything except the pressure is the same in both conditions, so
// the two calls differ by exactly the one number the story turns on.
func cpuPressureMachine(pressure float64) fakebox.Condition {
	return fakebox.Condition{
		Cores:      cpuPressureCores,
		HostBusy:   cpuPressureHostBusy,
		UsageCores: cpuPressureUsageCores,
		Pressure:   pressure,
		PsiPresent: true,
	}
}

// announceMachine prints the fake machine's condition, and is the only thing
// this scenario prints. One line goes out each time the mock changes, the
// opening condition included, and nothing else at all: every other line on the
// page is the supervisor's own output, so a reader can tell a stimulus from a
// response without being told which is which.
//
// Both lines carry the whole condition rather than only the field that moved,
// so a reader sees for themselves that pressure is the only difference between
// them, instead of taking a claim that it is.
func announceMachine(c fakebox.Condition) {
	fmt.Printf("fake machine: %d cores, host %.0f%% busy, this instance %g cores, pressure %.0f%%\n",
		c.Cores, c.HostBusy*100, c.UsageCores, c.Pressure*100)
}

// awaitFirstCPUReading blocks until the CPU monitor has completed one reading
// of the machine, or ctx is cancelled.
//
// This is a start-up gate, not a check on the answer: it waits for the worker
// to exist and to have read something, and looks at no part of the verdict. A
// blind sleep in its place would sometimes start the story before the worker
// had spawned, and the calm condition would then be held for less time than it
// looks.
//
// Polls counts only readings that succeeded, so a machine the worker cannot
// read at all never satisfies this and the driver waits out its ctx. That is
// the honest outcome: there is no story to tell on a machine nobody can read.
func awaitFirstCPUReading(ctx context.Context, client *fsmv2client.FSMv2Client) error {
	ticker := time.NewTicker(cpuPressureReadyPoll)
	defer ticker.Stop()

	for {
		obs, err := fsmv2client.Get[fsmv2cpu.CPUStatus](ctx, client, fsmv2cpu.Ref)
		switch {
		case err == nil:
			if obs.Status.Polls > 0 {
				return nil
			}
		case errors.Is(err, fsmv2client.ErrNotObserved):
			// The child has published nothing yet: retry on a later tick.
		default:
			return fmt.Errorf("get cpu observation: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// holdFor waits d out, or returns early once ctx is cancelled. A cancelled ctx
// is the only stop signal a driver gets and teardown cannot begin until the
// driver returns, so every wait in here has to watch it.
func holdFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// tickingBox is a fakebox.Box that advances with real time and can be read
// while it advances.
//
// A Box does neither on its own, and a worker under a supervisor needs both. Its
// counters move only when someone calls Tick, so a Box handed straight to such a
// worker serves an idle machine whatever condition it states — every rate the
// sampler publishes is a counter delta over real elapsed time. And a Box is not
// safe for concurrent use, while the collector reads it from its own goroutine.
//
// Everything that touches the Box goes through the one mutex here: the reads the
// collector makes, the ticks, and Set.
type tickingBox struct {
	box  *fakebox.Box
	stop chan struct{}
	done chan struct{}
	mu   sync.Mutex
}

// newTickingBox returns a box serving base in the condition initial describes.
// It does not advance until Start is called.
func newTickingBox(base string, initial fakebox.Condition) *tickingBox {
	return &tickingBox{
		box:  fakebox.NewBox(base, initial),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// FS returns a filesystem service serving this box, safe to read while the box
// advances. It wraps the Box's own service rather than replacing it, so what a
// reader gets back is whatever the Box would have served.
func (t *tickingBox) FS() filesystem.Service {
	inner := t.box.FS()

	guarded := filesystem.NewMockFileSystem()
	guarded.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
		t.mu.Lock()
		defer t.mu.Unlock()

		return inner.ReadFile(ctx, path)
	}

	return guarded
}

// Set changes the condition later ticks accrue at, and takes effect on the next
// read for anything the box states directly rather than accrues. PSI pressure
// is one of those: the kernel reports it as a level, so a Box writes it rather
// than accumulating it.
func (t *tickingBox) Set(c fakebox.Condition) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.box.Set(c)
}

// Start advances the box by every, every. Call it once.
//
// A ticker drops ticks under load rather than queueing them, so a busy machine
// leaves the box running behind real time. That serves a machine idler than the
// one stated, never a busier one, which is the harmless direction: the capacity
// signal this scenario needs to keep quiet fires on too little headroom.
func (t *tickingBox) Start(every time.Duration) {
	go func() {
		defer close(t.done)

		ticker := time.NewTicker(every)
		defer ticker.Stop()

		for {
			select {
			case <-t.stop:
				return
			case <-ticker.C:
				t.mu.Lock()
				t.box.Tick(every)
				t.mu.Unlock()
			}
		}
	}()
}

// Stop halts the advancing and waits for it to have halted, so no tick lands
// after Stop returns. The box stays readable and holds whatever it last served.
func (t *tickingBox) Stop() {
	close(t.stop)
	<-t.done
}
