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

	"github.com/benbjohnson/clock"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cpuMachineReadAdvance is a read-driven box's machine time per sampler read
// when a scenario's story turns on 60-second windows: ten machine seconds per
// read fills one in six readings at the worker's 1s poll.
const cpuMachineReadAdvance = 10 * time.Second

// cpuMachine is a scenario's fake CPU machine: a tickingBox wrapped in a
// HangingFS, so reads of a chosen path can be held up mid-poll. The
// HangingFS wraps outside the box's mutex, so a hung read does not block Set
// or Stop.
type cpuMachine struct {
	*fakebox.HangingFS
	box *tickingBox
}

// cpuMachineDeps returns the dependency map a CPU scenario's Dependencies
// builds over box: the box's filesystem, wrapped as a cpuMachine, and its
// clock. ClockKey says what publishing the filesystem alone costs.
func cpuMachineDeps(box *tickingBox) map[string]any {
	machine := &cpuMachine{HangingFS: fakebox.NewHangingFS(box.fs()), box: box}

	m := map[string]any{}

	var fs filesystem.Service = machine
	config.SetDependency(m, fsmv2cpu.FilesystemKey, fs)

	var clk clock.Clock = box.box.Clock()
	config.SetDependency(m, fsmv2cpu.ClockKey, clk)

	return m
}

// cpuMachineFromDeps reads the scenario's fake machine back out of the
// dependency map, the same map the supervisor builds the worker from.
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

func firstLine(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return msg[:i]
	}

	return msg
}

// cpuStatusSeen renders the parts of a reading a wait names. A poll that
// failed composes no message, so the worker's reason, where the poll error is
// written, stands in for it.
func cpuStatusSeen(st simple.Status[fsmv2cpu.CPUStatus]) string {
	msg := st.Result.Message
	if msg == "" {
		msg = st.Reason
	}

	return fmt.Sprintf("degraded=%t state=%q message=%q",
		st.Degraded, st.Result.Verdict.State, firstLine(msg))
}

// waitCPUFirstReading waits for a Fresh reading on which pass holds. pass must
// name a condition that lasts once reached, as waitCPUFresh says.
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

			return done, "fresh " + seen, nil
		default:
			return false, "", fmt.Errorf("the cpu worker's freshness is %s", fresh)
		}
	})
}

// waitCPUFresh waits for pass to hold on a Fresh reading. Any other freshness
// fails the wait at once: the container monitor's judgeWorkerCPU reports any
// reading that is not Fresh as degraded. The wait polls, so pass must name a
// condition that lasts once reached; a short-lived one can be missed.
func waitCPUFresh(ctx context.Context, env Env, check string, pass func(simple.Status[fsmv2cpu.CPUStatus]) (bool, string)) error {
	return env.WaitFor(ctx, check, func(ctx context.Context) (bool, string, error) {
		st, fresh, err := cpuReading(ctx, env)
		if err != nil {
			return false, "", err
		}

		if fresh != fsmv2client.Fresh {
			return false, "", fmt.Errorf("the CPU reading is %s, not Fresh, at stage %q", fresh, check)
		}

		done, seen := pass(st)

		return done, "fresh " + seen, nil
	})
}

// tickingBox is a fakebox.Box that advances once per sampler read and can be
// read while it does. A plain Box moves only when someone calls Tick, and is
// not safe for the collector's goroutine to read.
//
// mu covers every touch of the Box's counters: reads, ticks and Set. It is
// taken per file, not per sampler read. clock.Mock synchronises itself.
type tickingBox struct {
	box *fakebox.Box
	// base is the cgroup mount this box serves, so fs can recognise the file
	// the sampler opens once per read.
	base string
	// perRead is how much machine time one sampler read advances the box, and
	// zero before StartPerRead and after Stop.
	perRead time.Duration
	mu      sync.Mutex
}

// newTickingBox returns a box serving base in the condition initial describes.
// It does not advance until StartPerRead is called.
func newTickingBox(base string, initial fakebox.Condition) *tickingBox {
	return &tickingBox{
		box:  fakebox.NewBox(base, initial),
		base: base,
	}
}

// fs returns a filesystem service serving this box, safe to read while the box
// advances.
func (t *tickingBox) fs() filesystem.Service {
	inner := t.box.FS()

	guarded := filesystem.NewMockFileSystem()
	guarded.ReadFileFunc = func(ctx context.Context, path string) ([]byte, error) {
		t.mu.Lock()
		defer t.mu.Unlock()

		// The read-driven advance. The sampler opens cpu.pressure once per
		// read, after it stamps the read and before any other counter file,
		// so each read covers exactly one tick. It ticks even when the
		// condition makes cpu.pressure unreadable.
		if t.perRead > 0 && path == t.base+"/cpu.pressure" {
			t.box.Tick(t.perRead)
		}

		return inner.ReadFile(ctx, path)
	}

	return guarded
}

// MachineNow reads the box's own clock, the one the sampler stamps its samples
// from. It is safe to call while the box ticks.
func (t *tickingBox) MachineNow() time.Time {
	return t.box.Clock().Now()
}

// Set changes the condition later ticks accrue at.
func (t *tickingBox) Set(c fakebox.Condition) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.box.Set(c)
}

// StartPerRead advances the box by advance once per sampler read. Call it
// once.
//
// Every reading then covers one tick of counters over one tick of clock, so
// the rate is exactly the one the condition states. Machine time advances
// only while the worker reads: a scenario waiting on this clock waits out its
// ctx if the worker stops polling.
func (t *tickingBox) StartPerRead(advance time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.perRead = advance
}

// Stop halts the advancing, so no tick lands after Stop returns. It runs in
// the scenario's Dependencies cleanup, after the supervisor has stopped (the
// ScenarioV2.Dependencies doc), so the box has advanced for every read the
// worker made.
func (t *tickingBox) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.perRead = 0
}
