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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
)

const (
	// cpuLatchBase is where this scenario's fake machine serves its cgroup
	// files, and has to equal the unexported cgroupBase in pkg/fsmv2/cpu.
	// cpuPressureBase carries the full note on why nothing checks that.
	cpuLatchBase = "/sys/fs/cgroup"

	// The pressure levels this scenario stages, against the pressure signal's
	// 0.20 fire mark and 0.12 clear mark (pressureMarks in pkg/cpuhealth).
	//
	// cpuLatchNoise is the one the story turns on. It is UNDER the fire mark,
	// so a fresh signal would not fire on it, and OVER the clear mark, so a
	// fired one does not release. That gap between the two marks is what stops
	// a reading hovering at the threshold from flapping the verdict, and this
	// scenario is the gap being used.
	cpuLatchFiring = 0.25
	cpuLatchNoise  = 0.15
	cpuLatchCalm   = 0.05

	// cpuLatchCores is the machine's CPU count, cpuLatchHostBusy how much of it
	// the whole machine is using, and cpuLatchUsageCores what this instance
	// itself uses. There is no CPU limit.
	//
	// Capacity has to stay clear throughout, because this story is about one
	// signal and a second fired signal would take the headline. Four cores at
	// 30% busy leaves 4 - 1.2 - 1.0 = 1.8 cores of headroom against a fire mark
	// at zero, where the 1.0 is the core cpuhealth holds back as reserve
	// (cpuReserveCores). The machine never moves off that headroom.
	cpuLatchCores      = 4
	cpuLatchHostBusy   = 0.30
	cpuLatchUsageCores = 0.5

	// How much machine time each condition is held for. This is machine time,
	// not wall time: see holdMachine.
	//
	// The lengths here are the two release rules, not taste. A fired signal
	// releases only on a window whose Coverage is Full, which takes 60 readings
	// (see StartPerRead), and a released signal cannot fire again until a whole
	// span has passed since it released — another 60. So the shortest story
	// that shows a release AND the re-fire behind it is about 120 readings
	// long, and these holds are that plus room to sit in each condition.
	//
	// Measured:
	//
	//	The signal releases 1 reading after the calm condition starts. PSI is a
	//	level the kernel reports rather than a rate this package averages, so
	//	the reduction is the newest reading and the crossing needs no window of
	//	its own; the window only had to be covered, which it was long before.
	//
	//	The signal fires again exactly 60 readings after that release, the span
	//	the re-fire rule above requires. By then the machine has been over its
	//	fire mark for 40 readings, reported healthy, with the technical details
	//	printing the number that would have fired it.
	cpuLatchFiringHold = 40 * time.Second
	cpuLatchNoiseHold  = 40 * time.Second
	cpuLatchCalmHold   = 20 * time.Second
	cpuLatchRefireHold = 60 * time.Second
)

// CPULatchScenarioV2 drives the real CPU monitor over a fake machine whose PSI
// pressure crosses its fire mark, falls back into the band between the two
// marks, drops under the clear mark, and rises again.
//
// The story is what the two marks buy and what they cost. A machine that has
// been called degraded stays degraded while its pressure wanders below the mark
// that fired it, so an operator is not told the trouble ended and started again
// every few seconds. It lets go when the machine genuinely recovers. And when
// the trouble comes back, the report is late: a released signal cannot fire
// again for a whole window, so the machine spends that stretch over its fire
// mark while the verdict still reads healthy and the technical details print
// the number that would have fired it.
//
// # CLI Usage
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario=cpu-latch --duration=200ms --log-level=debug
//
// On every completed poll the cpu_reading debug line carries the verdict and the
// composed message; a failed poll never reaches it. When the worker's state
// changes, the message also appears at info in the state_transition line's
// reason field.
var CPULatchScenarioV2 = ScenarioV2{
	Name:        "cpu-latch",
	Description: "Holds a fake machine's CPU verdict through noise, releases it on recovery, and shows the bar on re-firing (v2)",
	Driver:      driveCPULatch,
}

// driveCPULatch publishes a fake machine, spawns the CPU monitor onto it
// through the migration-API client, and walks its pressure through the levels
// the consts above stage.
//
// It judges nothing. What the worker made of each level is read back after the
// run, out of the store's own delta history, by the spec beside this file.
func driveCPULatch(ctx context.Context, env Env) error {
	firing := cpuLatchMachine(cpuLatchFiring)
	machine := newTickingBox(cpuLatchBase, firing)

	// Published before the Upsert below, because cpu.NewDeps reads both keys
	// once, at the moment the supervisor spawns the worker; anything published
	// after that spawn reaches nothing. The poll cadence has the same deadline:
	// the collector reads it when the worker spawns.
	clearDeps := machine.Deps()
	defer clearDeps()

	restorePoll := useFastCPUPoll()
	defer restorePoll()

	machine.StartPerRead(cpuMachineSecond)
	defer machine.Stop()

	announceMachine(firing)

	// Nil config: CPUConfig is an empty struct, and this is the same call the
	// config worker makes in production.
	if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
		return fmt.Errorf("upsert cpu monitor: %w", err)
	}

	if err := awaitFirstCPUReading(ctx, env.Client); err != nil {
		return fmt.Errorf("wait for the cpu monitor's first reading: %w", err)
	}

	if err := holdMachine(ctx, machine, cpuLatchFiringHold); err != nil {
		return err
	}

	for _, step := range []struct {
		pressure float64
		hold     time.Duration
	}{
		{cpuLatchNoise, cpuLatchNoiseHold},
		{cpuLatchCalm, cpuLatchCalmHold},
		{cpuLatchFiring, cpuLatchRefireHold},
	} {
		condition := cpuLatchMachine(step.pressure)
		machine.Set(condition)
		announceMachine(condition)

		if err := holdMachine(ctx, machine, step.hold); err != nil {
			return err
		}
	}

	return nil
}

// cpuLatchMachine is this scenario's machine at the given PSI pressure.
// Everything except the pressure is the same in every condition, so the calls
// differ by exactly the one number the story turns on.
func cpuLatchMachine(pressure float64) fakebox.Condition {
	return fakebox.Condition{
		Cores:      cpuLatchCores,
		HostBusy:   cpuLatchHostBusy,
		UsageCores: cpuLatchUsageCores,
		Pressure:   pressure,
		PsiPresent: true,
	}
}
