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
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cpuMachineSecond is one second of machine time, the amount a read-driven box
// advances per sampler read. One read therefore lands one machine second after
// the last, the cadence the worker reads at in production.
const cpuMachineSecond = time.Second

// cpuMachine is a scenario's fake CPU machine: a tickingBox wrapped in a
// HangingFS, so reads of a chosen path can be held up mid-poll. It is a
// filesystem.Service through the embedded wrapper, and holds the box so Run
// can read machine time and change the condition.
type cpuMachine struct {
	*fakebox.HangingFS
	box *tickingBox
}

// cpuMachineFromDeps reads the scenario's fake machine back out of the
// dependency map, so Run can change the condition or hang a read. The map is
// the same one the supervisor builds workers from, so the machine Run holds is
// the machine the worker reads.
func cpuMachineFromDeps(env Env) (*cpuMachine, error) {
	raw, ok := config.LookupDependency(env.Dependencies, fsmv2cpu.FilesystemKey)
	if !ok {
		return nil, errors.New("the cpu scenario's dependency map holds no filesystem under fsmv2cpu.FilesystemKey")
	}

	machine, ok := raw.(*cpuMachine)
	if !ok {
		return nil, errors.New("the filesystem under fsmv2cpu.FilesystemKey is not the scenario's cpuMachine")
	}

	return machine, nil
}

// cpuReading reads the CPU worker's last observation through the monitor's
// exact call: GetFresh under the shared staleness limit, so a scenario asserts
// the same freshness the container monitor depends on.
func cpuReading(ctx context.Context, env Env) (simple.Status[fsmv2cpu.CPUStatus], fsmv2client.Freshness, error) {
	return fsmv2client.GetFresh[simple.Status[fsmv2cpu.CPUStatus]](ctx, env.Client, fsmv2cpu.Ref, fsmv2cpu.MaxObservationAge)
}

// freshnessName prints a freshness without its numeric value: the numbers are
// an enum, and a failure line that reads fresh=4 hides which freshness it was.
func freshnessName(f fsmv2client.Freshness) string {
	switch f {
	case fsmv2client.Fresh:
		return "fresh"
	case fsmv2client.Unregistered:
		return "unregistered"
	case fsmv2client.NeverObserved:
		return "never-observed"
	case fsmv2client.Stale:
		return "stale"
	default:
		return "unknown"
	}
}

// firstLine cuts a message at its first newline, so a wait's seen value stays
// one line and names the headline the worker composed.
func firstLine(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return msg[:i]
	}

	return msg
}

// cpuStatusSeen renders the parts of a reading a wait names: the degraded
// flag, the verdict's state, and the first line of the message. A poll that
// failed composes no message, so the worker's reason stands in for it: that
// is where the poll error is written.
func cpuStatusSeen(st simple.Status[fsmv2cpu.CPUStatus]) string {
	msg := st.Result.Message
	if msg == "" {
		msg = st.Reason
	}

	return fmt.Sprintf("degraded=%t state=%q message=%q",
		st.Degraded, st.Result.Verdict.State, firstLine(msg))
}

// waitCPUFirstReading waits for the worker's first Fresh reading and the
// condition pass. NeverObserved means the first poll has not completed yet, so
// it is not done rather than a failure. Unregistered and Stale are errors:
// Upsert registers the worker synchronously, and no reading goes stale before
// the first one exists.
func waitCPUFirstReading(ctx context.Context, env Env, check string, pass func(simple.Status[fsmv2cpu.CPUStatus]) (bool, string)) error {
	return env.WaitFor(ctx, check, func(ctx context.Context) (bool, string, error) {
		st, fresh, err := cpuReading(ctx, env)
		if err != nil {
			return false, "", err
		}

		switch fresh {
		case fsmv2client.NeverObserved:
			return false, "never-observed", nil
		case fsmv2client.Unregistered:
			return false, "", fmt.Errorf("the cpu worker is not registered; the Upsert should have registered it synchronously")
		case fsmv2client.Stale:
			return false, "", fmt.Errorf("the cpu worker's reading is stale before the scenario's first wait passed")
		case fsmv2client.Fresh:
			done, seen := pass(st)

			return done, fmt.Sprintf("fresh=%s %s", freshnessName(fresh), seen), nil
		default:
			return false, "", fmt.Errorf("the cpu worker's freshness is %s", freshnessName(fresh))
		}
	})
}

// waitCPUFresh waits for pass to hold on a Fresh reading. Any freshness other
// than Fresh fails the wait at once, naming it and the stage: through a normal
// hold the reading stays Fresh, and a gap long enough to stale it would also
// make the container monitor refuse bridges.
func waitCPUFresh(ctx context.Context, env Env, check string, pass func(simple.Status[fsmv2cpu.CPUStatus]) (bool, string)) error {
	return env.WaitFor(ctx, check, func(ctx context.Context) (bool, string, error) {
		st, fresh, err := cpuReading(ctx, env)
		if err != nil {
			return false, "", err
		}

		if fresh != fsmv2client.Fresh {
			return false, "", fmt.Errorf("the CPU reading is %s, not Fresh, at stage %q", freshnessName(fresh), check)
		}

		done, seen := pass(st)

		return done, fmt.Sprintf("fresh=%s %s", freshnessName(fresh), seen), nil
	})
}

// tickingBox is a fakebox.Box that runs, and that can be read while it runs.
//
// A Box does neither on its own, and a worker under a supervisor needs both.
// Its counters and its clock move only when someone calls Tick, so a Box
// handed straight to such a worker serves a machine frozen at zero. And a Box
// is not safe for concurrent use, while the collector reads it from its own
// goroutine.
//
// Both the filesystem and the clock go into the dependency map together, in
// the scenario's Dependencies. Tick moves the counters and the clock by the
// same amount, so once the sampler stamps from that clock, the time it divides
// by and the counters it divides are the same quantity, and the rate that
// comes out is the rate the condition states. Publishing only one of the two
// fails quietly: a filesystem with no clock leaves the sampler stamping wall
// time while the counters accrue on the box's clock, and a clock nothing
// advances leaves every rate withheld, because the sampler's elapsed time is
// never positive.
//
// Everything that touches the Box's counters goes through the one mutex here:
// the reads the collector makes, the ticks, and Set. The clock is not covered
// by it and does not need to be, because clock.Mock synchronises itself. What
// that leaves is an ordering gap rather than a race. The sampler stamps once
// at the top of a read and then opens the files, so a tick landing after the
// stamp adds counters the stamp does not account for, and whichever source is
// read after that tick reports a rate too high by one tick's worth. Only one
// tick can fit, so the overstatement is bounded.
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

// fs returns a filesystem service serving this box, safe to read while the box
// advances. It wraps the Box's own service rather than replacing it, so what a
// reader gets back is whatever the Box would have served.
func (t *tickingBox) fs() filesystem.Service {
	inner := t.box.FS()

	guarded := filesystem.NewMockFileSystem()
	guarded.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
		t.mu.Lock()
		defer t.mu.Unlock()

		// The read-driven advance. cpu.pressure is opened once per read,
		// before anything that can fail the read early, so seeing it is
		// seeing a read begin. It ticks even when the condition makes that
		// file unreadable, because the box serves the failure rather than
		// skipping the open.
		//
		// The tick lands after the sampler has stamped the read and before
		// any file is served, so the stamp trails the counters by one tick,
		// the same one tick on every read. StartPerRead carries what that
		// exactness buys a scenario.
		if t.perRead > 0 && path == t.base+"/cpu.pressure" {
			t.box.Tick(t.perRead)
		}

		return inner.ReadFile(ctx, path)
	}

	return guarded
}

// MachineNow reads the box's own clock: the instant the sampler stamps its
// samples from, and the one every window span and release rule downstream is
// denominated in. A scenario whose story is measured in machine time reads
// this rather than the wall clock.
//
// clock.Mock synchronises itself, so this is safe to call while the box ticks.
func (t *tickingBox) MachineNow() time.Time {
	return t.box.Clock().Now()
}

// Set changes the condition later ticks accrue at, and takes effect on the
// next read for anything the box states directly rather than accrues. PSI
// pressure is one of those: the kernel reports it as a level, so a Box writes
// it rather than accumulating it.
func (t *tickingBox) Set(c fakebox.Condition) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.box.Set(c)
}

// Start uses every as both the ticker's wall-clock interval and the machine
// time each tick advances, so machine time keeps pace with the wall clock.
// Call it once.
//
// A ticker drops ticks under load rather than queueing them, and on the box's
// own clock a drop withholds a tick's counters and a tick's clock together,
// so no rate it reports is wrong.
//
// It does not lengthen the run either. Both the scenario's waits and the
// worker's polls are on the wall clock, so a drop does not buy back the time:
// it thins the run, leaving less machine time inside each wall-clock second.
// The visible effect is fewer sample-seconds per reading, not a longer story.
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
// It is exact rather than merely repeatable. Every reading covers one tick of
// counters over one tick of clock, so the rate is the one the condition
// states, with none of the straddling a wall-clock ticker has to bound.
//
// The price is that machine time now advances only while the worker is
// reading. A scenario waiting on this clock waits out its ctx if the worker
// stops polling, and how much wall time a machine second costs is the
// collector's cadence rather than a ticker's.
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
// worker keeps making no longer move machine time.
//
// Stop runs in the scenario's Dependencies cleanup, after the supervisor has
// stopped (the ScenarioV2.Dependencies doc), so the box keeps advancing
// through the whole settle window and the readings of that window keep their
// rates.
//
// A stopped box has stopped its clock too, so a read the worker makes after
// Stop finds no elapsed time to divide by and serves no rate at all.
func (t *tickingBox) Stop() {
	t.mu.Lock()
	t.perRead = 0
	t.mu.Unlock()

	if t.ticking {
		close(t.stop)
		<-t.done
	}
}
