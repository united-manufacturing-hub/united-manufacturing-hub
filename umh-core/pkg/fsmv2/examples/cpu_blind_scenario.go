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
	"fmt"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
)

const (
	// cpuBlindBase is where this scenario's fake machine serves its cgroup
	// files, and has to equal the unexported cgroupBase in pkg/fsmv2/cpu.
	// cpuPressureBase carries the full note on why nothing checks that.
	cpuBlindBase = "/sys/fs/cgroup"

	// The files this scenario takes away, in the order it takes them.
	//
	// /proc/stat is the machine's own CPU accounting, a HOST file outside the
	// cgroup. cpu.stat is the cgroup's, and the sampler treats it as primary:
	// a cgroup that cannot be read at all is not a machine to reason about.
	cpuBlindHostStat   = "/proc/stat"
	cpuBlindCgroupStat = cpuBlindBase + "/cpu.stat"

	// The machine this scenario runs on, while it can still be read. It is a
	// quiet four-core box with no CPU limit: nothing here is near any mark, so
	// every verdict in this story comes from what could and could not be read
	// rather than from a number crossing a threshold.
	cpuBlindCores      = 4
	cpuBlindHostBusy   = 0.30
	cpuBlindUsageCores = 0.5
	cpuBlindPressure   = 0.02

	// How much machine time each condition is held for. This is machine time,
	// not wall time: see holdMachine.
	//
	// Nothing here waits on a window, because nothing here is a rate crossing a
	// mark: the sampler either reads a file or does not, and the verdict moves
	// on the next reading. The holds are long enough that a reader sees the
	// machine sit in each condition, and no longer. In particular none of them
	// reaches the 60 readings at which a window with nothing new to store
	// empties, so the readings this story shows are the ones the outage itself
	// produces rather than the ones its aftermath does.
	cpuBlindHold = 15 * time.Second
)

// CPUBlindScenarioV2 drives the real CPU monitor over a fake machine and then
// takes away, one at a time, the two files it reads its numbers from.
//
// The story is that the two failures do not behave alike, and the difference
// matters more than either on its own.
//
// Losing /proc/stat is a fail-open. The poll SUCCEEDS, the worker reports
// healthy, and the message says CPU monitoring is unavailable — so a machine
// nothing can measure is published as a healthy one, and everything downstream
// that gates on the verdict, admission of new bridges included, sees an open
// door. That is a KNOWN DEFECT and not the intended answer. The message is
// wrong twice over: it blames a cgroup read for a HOST file, and it announces
// its own default rather than refusing to answer. This scenario pins what the
// code does today so the behaviour is visible instead of inferred. The fix is in
// pkg/cpuhealth, out of this package's scope, where healthy-on-unavailable is
// currently the documented contract rather than a recorded defect.
// TODO(ENG-XXXX): file the cpuhealth fix.
//
// Losing cpu.stat is the other shape. The read fails, the poll returns an
// error, and the framework marks the worker degraded with the error as its
// reason. Nothing is defaulted and nothing is guessed.
//
// # CLI Usage
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario=cpu-blind --duration=200ms --log-level=debug
//
// On every completed poll the cpu_reading debug line carries the verdict and the
// composed message; a failed poll never reaches it. When the worker's state
// changes, the message also appears at info in the state_transition line's
// reason field.
var CPUBlindScenarioV2 = ScenarioV2{
	Name:        "cpu-blind",
	Description: "Takes away the two files the CPU monitor reads, and shows that one fails open and the other does not (v2)",
	Driver:      driveCPUBlind,
}

// driveCPUBlind publishes a fake machine, spawns the CPU monitor onto it
// through the migration-API client, then makes /proc/stat unreadable and then
// cpu.stat as well.
//
// The second outage keeps the first, because a machine losing sight of itself
// does not usually get one file back as it loses another, and because a story
// that restored /proc/stat would be testing recovery rather than the two
// failures. cpu.stat failing is what decides that condition either way: the
// sampler gives up on it before it reaches anything /proc/stat would answer.
//
// It judges nothing. What the worker made of each condition is read back after
// the run, out of the store's own delta history, by the spec beside this file.
func driveCPUBlind(ctx context.Context, env Env) error {
	readable := cpuBlindMachine()
	machine := newTickingBox(cpuBlindBase, readable)

	// Published before the Upsert below, because cpu.NewDeps reads both keys
	// once, at the moment the supervisor spawns the worker; anything published
	// after that spawn reaches nothing. The poll cadence has the same deadline:
	// the collector reads it when the worker spawns.
	clearDeps := machine.Deps()
	defer clearDeps()

	restorePoll := useFastCPUPoll()
	defer restorePoll()

	// The box advances on the sampler's read of cpu.pressure, which this
	// scenario never takes away, so machine time keeps moving through both
	// outages — including the last one, where the read fails before it reaches
	// most of the files.
	machine.StartPerRead(cpuMachineSecond)
	defer machine.Stop()

	announceBlindMachine(readable)

	// Nil config: CPUConfig is an empty struct, and this is the same call the
	// config worker makes in production.
	if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
		return fmt.Errorf("upsert cpu monitor: %w", err)
	}

	if err := awaitFirstCPUReading(ctx, env.Client); err != nil {
		return fmt.Errorf("wait for the cpu monitor's first reading: %w", err)
	}

	if err := holdMachine(ctx, machine, cpuBlindHold); err != nil {
		return err
	}

	for _, gone := range [][]string{
		{cpuBlindHostStat},
		{cpuBlindHostStat, cpuBlindCgroupStat},
	} {
		condition := cpuBlindMachine(gone...)
		machine.Set(condition)
		announceBlindMachine(condition)

		if err := holdMachine(ctx, machine, cpuBlindHold); err != nil {
			return err
		}
	}

	return nil
}

// cpuBlindMachine is this scenario's machine with the named files unreadable.
// Everything else is the same in all three conditions, so the three calls
// differ by exactly what the machine can no longer see.
//
// fakebox rejects a path it does not serve rather than ignoring it, so a
// mistyped name here fails the run instead of leaving the spec asserting
// against the readable machine it was written to rule out.
func cpuBlindMachine(unreadable ...string) fakebox.Condition {
	return fakebox.Condition{
		Cores:      cpuBlindCores,
		HostBusy:   cpuBlindHostBusy,
		UsageCores: cpuBlindUsageCores,
		Pressure:   cpuBlindPressure,
		PsiPresent: true,
		Unreadable: unreadable,
	}
}

// announceBlindMachine prints the fake machine's condition, and is the only
// thing this scenario prints. One line goes out each time the mock changes, the
// opening condition included, and nothing else at all: every other line on the
// page is the supervisor's own output, so a reader can tell a stimulus from a
// response without being told which is which.
//
// It names what cannot be read, which announceMachine does not, because that is
// the only thing this scenario changes.
func announceBlindMachine(c fakebox.Condition) {
	unreadable := "nothing"
	if len(c.Unreadable) > 0 {
		unreadable = strings.Join(c.Unreadable, ", ")
	}

	fmt.Printf("fake machine: %d cores, host %.0f%% busy, this instance %g cores, pressure %.0f%%, unreadable: %s\n",
		c.Cores, c.HostBusy*100, c.UsageCores, c.Pressure*100, unreadable)
}
