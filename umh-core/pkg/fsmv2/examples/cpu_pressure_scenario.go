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

	"github.com/benbjohnson/clock"

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
	// how much of it the whole machine is using.
	//
	// The capacity signal reads headroom as cores - busy - a one-core reserve,
	// averaged over 60 seconds, and calls the machine full below zero. Four
	// cores at 60% busy is 4 - 2.4 - 1.0 = 0.6 cores, and that is what the run
	// reports from its first measured reading onward, with no ramp into it: the
	// worker's very first read has no baseline to rate against, and because the
	// clock it is stamped from is the box's own, no machine time has passed
	// either, so the reading is withheld rather than counted as a zero that the
	// average would then have to climb out of.
	//
	// So the run sits 0.6 cores above the mark at which this machine would be
	// called full, and stays there. That is the story: it is a busy machine
	// with capacity to spare, and what makes it degraded later has nothing to
	// do with capacity.
	cpuPressureCores    = 4
	cpuPressureHostBusy = 0.60

	// cpuPressureUsageCores is what this instance itself is using: 0.5 cores of
	// the 2.4 the machine is busy with, so about a fifth of the load is ours and
	// the rest is somebody else's. Nothing in this story turns on that split; it
	// is set to a plausible figure rather than left at zero, which would be a
	// machine busy with nothing.
	cpuPressureUsageCores = 0.5

	// cpuPressureMachineTick is how much machine time one tick of the ticker
	// advances. Its ratio to the worker's one-second poll cadence is a
	// correctness bound, not a matter of taste, and a tenth is what keeps this
	// machine's capacity signal quiet.
	//
	// A tick that lands inside the sampler's read adds counters the stamp does
	// not cover (see tickingBox), so that one reading overstates its rate by
	// tick over poll. At a tenth, host busy reads 2.64 against a stated 2.4 and
	// headroom bottoms out at 0.36, still clear of the mark at 0, and a
	// 60-second mean damps even that. At a tick equal to the poll it reads
	// 4.80, headroom is -1.80, and the machine is reported full.
	//
	// So a scenario parking a signal near its mark has to check this ratio
	// against its own margin rather than inherit the number.
	cpuPressureMachineTick = 100 * time.Millisecond

	// cpuPressureHold is how long each condition is held before the story moves
	// on. The verdict does not need it: pressure reduces by Last, so the first
	// reading after a change already carries the new one. The page does. Two
	// seconds is two of the worker's one-second readings, which is a short
	// stretch at each condition rather than a single line, and a reader can see
	// that the machine sat there rather than passed through.
	cpuPressureHold = 2 * time.Second

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
// info and the page still carries the supervisor spawning its children between
// the two machine conditions, but nothing about what the worker made of either.
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

	// Published before the Upsert below, because cpu.NewDeps reads both keys
	// once, at the moment the supervisor spawns the worker; anything published
	// after that spawn reaches nothing.
	clearDeps := machine.Deps()
	defer clearDeps()

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
// Verdict is empty on every tick that could not measure, so a machine the
// worker cannot read at all never satisfies this and the driver waits out its
// ctx. That is the honest outcome: there is no story to tell on a machine
// nobody can read.
func awaitFirstCPUReading(ctx context.Context, client *fsmv2client.FSMv2Client) error {
	ticker := time.NewTicker(cpuPressureReadyPoll)
	defer ticker.Stop()

	for {
		obs, err := fsmv2client.Get[fsmv2cpu.CPUStatus](ctx, client, fsmv2cpu.Ref)
		switch {
		case err == nil:
			if obs.Status.Verdict != "" {
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

// tickingBox is a fakebox.Box that runs, and that can be read while it runs.
//
// A Box does neither on its own, and a worker under a supervisor needs both.
// Its counters and its clock move only when someone calls Tick, so a Box handed
// straight to such a worker serves a machine frozen at zero. And a Box is not
// safe for concurrent use, while the collector reads it from its own goroutine.
//
// Publish FS and Clock together, which Deps does. Tick moves the counters and
// the clock by the same amount, so once the sampler stamps from that clock, the
// time it divides by and the counters it divides are the same quantity, and the
// rate that comes out is the rate the condition states. Stamping from the wall
// clock instead divides a tick's worth of counter by however long the two reads
// happened to be apart, which at spawn can be under a millisecond: a hundredfold
// overstatement that then sits in a 60-second mean for a minute.
//
// Everything that touches the Box's counters goes through the one mutex here:
// the reads the collector makes, the ticks, and Set. The clock is not covered by
// it and does not need to be, because clock.Mock synchronises itself. What that
// leaves is an ordering gap rather than a race. The sampler stamps once at the
// top of a read and then opens the files (read.go, ts := s.clock.Now()), so a
// tick landing after the stamp adds counters the stamp does not account for, and
// whichever source is read after that tick reports a rate too high by one tick's
// worth. Only one tick can fit, so the overstatement is bounded; the wall-clock
// version divided by a gap that could be arbitrarily short and was not.
type tickingBox struct {
	box  *fakebox.Box
	stop chan struct{}
	done chan struct{}
	// base is the cgroup mount this box serves, so the read-driven mode below
	// can recognise the file the sampler opens once per read.
	base string
	// perRead is how much machine time one sampler read advances the box, and
	// zero when the box is driven by a wall-clock ticker instead. StartPerRead
	// sets it and Stop clears it.
	perRead time.Duration
	// ticking records that a wall-clock ticker was started, so Stop knows
	// whether there is a goroutine to join.
	ticking bool
	mu      sync.Mutex
}

// newTickingBox returns a box serving base in the condition initial describes.
// It does not advance until Start is called.
func newTickingBox(base string, initial fakebox.Condition) *tickingBox {
	return &tickingBox{
		box:  fakebox.NewBox(base, initial),
		base: base,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Deps publishes this box's filesystem and clock for the next CPU worker the
// supervisor spawns, and returns the function that takes both back out. Both
// keys are process-global, so a driver that left either set would hand the next
// scenario in this process its fake machine.
//
// Publishing the pair together is the point of this method, because one of the
// halves fails silently on its own.
//
// A filesystem with no clock is that half. Nothing errors. The sampler stamps
// from the wall clock while the counters accrue on this box's, so it divides a
// tick's worth of counter by however long two reads happened to be apart, which
// at spawn is on the order of a hundred microseconds. In this scenario that
// reads as a machine thousands of percent busy, and the 60-second mean carries
// it for a minute. It is also exactly what a scenario copied from a pre-clock
// one does.
//
// A clock nothing advances is the loud half. Machine time stands still, the
// sampler's elapsed is never positive, every rate is withheld, and a driver
// waiting for the worker's first reading waits out its ctx instead.
//
// Publishing neither is production: the real filesystem and the real clock.
func (t *tickingBox) Deps() (clear func()) {
	register.SetDeps[filesystem.Service](fsmv2cpu.FilesystemDepsKey, t.fs())
	register.SetDeps[clock.Clock](fsmv2cpu.ClockDepsKey, t.box.Clock())

	return func() {
		register.ClearDeps(fsmv2cpu.FilesystemDepsKey)
		register.ClearDeps(fsmv2cpu.ClockDepsKey)
	}
}

// fs returns a filesystem service serving this box, safe to read while the box
// advances. It wraps the Box's own service rather than replacing it, so what a
// reader gets back is whatever the Box would have served.
func (t *tickingBox) fs() filesystem.Service {
	inner := t.box.FS()

	guarded := filesystem.NewMockFileSystem()
	guarded.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
		t.mu.Lock()
		defer t.mu.Unlock()

		// The read-driven advance. cpu.pressure is the file the sampler opens
		// first and exactly once per read, before anything that can fail the
		// read early, so seeing it is seeing a read begin. It ticks even when
		// the condition makes that file unreadable, because the box serves the
		// failure rather than skipping the open.
		//
		// The tick lands after the sampler has stamped the read and before any
		// file is served, so the stamp trails the counters by one tick — the
		// SAME one tick on every read, which is what makes the deltas exact:
		// one tick of counters over one tick of clock is the rate the condition
		// states, with none of the straddling cpuPressureMachineTick has to
		// bound.
		if t.perRead > 0 && path == t.base+"/cpu.pressure" {
			t.box.Tick(t.perRead)
		}

		return inner.ReadFile(ctx, path)
	}

	return guarded
}

// MachineNow reads the box's own clock: the instant the sampler stamps its
// samples from, and the one every window span and release rule downstream is
// denominated in.
//
// A driver whose story is measured in machine time waits against this rather
// than against the wall clock. How much machine time a wall second buys is not
// fixed: it is however many ticks the ticker actually delivered, and a run
// under debug logging delivers fewer of them than a run with a discarding
// logger. A wall-clock hold therefore covers a different stretch of the story
// on each, while a machine-time hold covers the same stretch on both and pays
// the difference in wall seconds instead.
//
// clock.Mock synchronises itself, so this is safe to call while the box ticks.
func (t *tickingBox) MachineNow() time.Time {
	return t.box.Clock().Now()
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

// Start advances the box by every, every, so machine time keeps pace with the
// wall clock. Call it once.
//
// A ticker drops ticks under load rather than queueing them, and on the box's
// own clock a drop withholds a tick's counters and a tick's clock together, so
// no rate it reports is wrong. The wall-clock version had to argue that a drop
// erred in the harmless direction; this one does not err at all.
//
// It does not lengthen the run either. Both the driver's holds and the worker's
// polls are on the wall clock, so a drop does not buy back the time: it thins
// the run, leaving less machine time inside each wall-clock second. The visible
// effect is fewer sample-seconds per reading, not a longer story.
//
// Enough consecutive drops to span a whole poll is the case that is not merely
// thinner. Machine time then does not advance between two reads at all, the
// sampler's elapsed is zero, and that reading is withheld rather than served
// low.
func (t *tickingBox) Start(every time.Duration) {
	t.startTicker(every, every)
}

// StartPerRead advances the box by advance once per sampler read, rather than
// on a wall-clock ticker. Call it once, and not beside Start.
//
// A scenario reaches for this when it has to reach a RELEASE. A latch releases
// only on a window whose Coverage is Full, and Coverage is the distance from
// the oldest stored reading to the newest AFTER everything older than the span
// has been pruned. Pruning keeps a reading landing exactly on the cutoff and
// drops everything before it, so that distance reaches the span only when a
// stored reading sits exactly one span before the newest. One read, one tick
// puts every reading a whole tick from every other, so a span that is a whole
// number of ticks is covered from the moment enough readings exist. Under a
// wall-clock ticker the readings land wherever the two cadences happen to
// cross, and Coverage is Full only by coincidence: measured at ten ticks to the
// poll, on 17 readings out of 200, first at the 69th rather than the 61st.
//
// It is also exact rather than merely repeatable. Every reading covers one
// tick of counters over one tick of clock, so the rate is the one the condition
// states, with none of the straddling cpuPressureMachineTick has to bound.
//
// The price is that machine time now advances only while the worker is reading.
// A driver waiting on this clock waits out its ctx if the worker stops polling,
// and how much wall time a machine second costs is the collector's cadence
// rather than a ticker's.
func (t *tickingBox) StartPerRead(advance time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.perRead = advance
}

// startTicker advances the box by advance, every interval of wall time.
func (t *tickingBox) startTicker(interval, advance time.Duration) {
	t.ticking = true

	go func() {
		defer close(t.done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-t.stop:
				return
			case <-ticker.C:
				t.mu.Lock()
				t.box.Tick(advance)
				t.mu.Unlock()
			}
		}
	}()
}

// Stop halts the advancing and waits for it to have halted, so no tick lands
// after Stop returns. It ends both modes: it joins the ticker goroutine when
// there is one, and it stops a read-driven box advancing, so the reads the
// worker keeps making during the settle window no longer move machine time.
//
// A driver's defer fires this when the driver returns, which is BEFORE the
// runner's settle window. Everything the worker reads during that window comes
// from a stopped box — and a stopped box has stopped its clock too, so the
// sampler's elapsed time is zero, every rate is withheld rather than recomputed,
// and no window ages.
//
// This scenario would survive the freeze without any of that, because what
// carries its verdict is PSI pressure, which the kernel reports as a level and
// a Box writes rather than accrues. A frozen box keeps serving 25%.
//
// What the stopped clock protects is the scenario that copies this one and
// hangs its verdict on a rate. Freeze the counters while the wall clock runs and
// its rates do not merely fall: every one of them converges on a confident zero,
// so the page ends on an idle machine with full headroom that nothing flags as
// unmeasured. Stopping the clock turns that into no reading at all.
func (t *tickingBox) Stop() {
	t.mu.Lock()
	t.perRead = 0
	t.mu.Unlock()

	if t.ticking {
		close(t.stop)
		<-t.done
	}
}
