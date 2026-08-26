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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
)

const (
	// cpuFillingBase is where this scenario's fake machine serves its cgroup
	// files, and has to equal the unexported cgroupBase in pkg/fsmv2/cpu.
	// cpuPressureBase carries the full note on why nothing checks that.
	cpuFillingBase = "/sys/fs/cgroup"

	// cpuFillingCores is the machine's CPU count and cpuFillingQuotaCores this
	// instance's own CPU limit, the figure docker run --cpus sets.
	//
	// The quota is load-bearing, not decoration. The attribution paragraphs
	// this scenario exists to show are gated on Details.LimitApplies in
	// message.go, so on a machine with no limit BOTH of the last two conditions
	// render the same sentence and the story demonstrates nothing.
	//
	// Three cores against four is also chosen so the container's own limit
	// stays clear throughout — see the condition table below.
	cpuFillingCores      = 4
	cpuFillingQuotaCores = 3

	// How much machine time each condition is held for. This is machine time,
	// not wall time, and the difference matters: see holdMachine.
	//
	// The lengths are not cosmetic here, the way cpuPressureHold is. Every
	// number this story turns on is a 60-second sliding MEAN, so a condition
	// has to be held long enough for the mean to cross its mark, and the quiet
	// opening has to be held short enough that its readings age out of the
	// window rather than holding the mean back.
	//
	// Measured against those means, in readings after the condition BEFORE the
	// one that carries them:
	//
	//	55 readings after the quiet condition ends, the machine reads full — the
	//	quiet readings have aged far enough out of the headroom window for its
	//	mean to go under zero
	//	31 readings after the filled-from-outside condition ends, the blame moves
	//	to us — the share window's mean crosses both refinement marks, and the
	//	60-second bar on re-firing a released signal has passed
	//
	// Both are offsets rather than fixed readings, because the quiet condition
	// runs longer than the hold below asks for: the driver's readiness wait looks
	// at the store every cpuPressureReadyPoll, and machine seconds pass while it
	// does. Measured here, the quiet condition ran to reading 21 and the two
	// verdicts landed on readings 76 and 132 of a run that ends at 161 — the same
	// three numbers on every run, which is what the read-driven clock buys.
	//
	// Each hold runs well past the second it has to reach, so a reader of the
	// store sees each verdict for a stretch rather than for one reading.
	cpuFillingQuietHold = 10 * time.Second
	cpuFillingHostHold  = 80 * time.Second
	cpuFillingOursHold  = 60 * time.Second
)

// CPUFillingScenarioV2 drives the real CPU monitor over a fake machine that
// fills up, and then over the same machine filled by this instance's own load
// instead of somebody else's.
//
// The story is that "the machine is full" is not the whole answer a customer
// needs, because the remedy depends on whose load filled it. The middle
// condition is a machine filled from outside, and the advice is to reduce the
// other software on it. The last condition is the same machine, equally full,
// filled by this instance — and telling that customer to go after somebody
// else's load would send them after nothing.
//
// Nothing else moves across those two conditions: the machine is 80% busy in
// both, its pressure is the same, and its CPU limit is reached in neither. The
// one difference is how much of the busy time is ours.
//
// # CLI Usage
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario=cpu-filling --duration=200ms --log-level=debug --dump-store
//
// The verdict shows up on the collector's observed_changed line, which is a
// debug line — the CPU worker declares no Health function, so its verdict never
// moves the worker's FSM state and nothing about the verdict is logged at info.
// --dump-store prints the closing message whole.
var CPUFillingScenarioV2 = ScenarioV2{
	Name:        "cpu-filling",
	Description: "Fills a fake machine from outside, then from this instance, and shows the remedy change (v2)",
	Driver:      driveCPUFilling,
}

// driveCPUFilling publishes a fake machine, spawns the CPU monitor onto it
// through the migration-API client, and walks the machine through three
// conditions: quiet, filled from outside, filled by us.
//
// It judges nothing. What the worker made of each condition is read back after
// the run, out of the store's own delta history, by the spec beside this file.
func driveCPUFilling(ctx context.Context, env Env) error {
	quiet := cpuFillingQuiet()
	machine := newTickingBox(cpuFillingBase, quiet)

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

	announceFillingMachine(quiet)

	// Nil config: CPUConfig is an empty struct, and this is the same call the
	// config worker makes in production.
	if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
		return fmt.Errorf("upsert cpu monitor: %w", err)
	}

	if err := awaitFirstCPUReading(ctx, env.Client); err != nil {
		return fmt.Errorf("wait for the cpu monitor's first reading: %w", err)
	}

	if err := holdMachine(ctx, machine, cpuFillingQuietHold); err != nil {
		return err
	}

	hostFilled := cpuFillingHostFilled()
	machine.Set(hostFilled)
	announceFillingMachine(hostFilled)

	if err := holdMachine(ctx, machine, cpuFillingHostHold); err != nil {
		return err
	}

	oursFilled := cpuFillingOursFilled()
	machine.Set(oursFilled)
	announceFillingMachine(oursFilled)

	return holdMachine(ctx, machine, cpuFillingOursHold)
}

// The three conditions, in the order the driver walks them. Each is written as
// its own function so the call sites read as the story rather than as three
// triples of floats, and so the numbers sit under the table that explains them.
//
// What each condition produces, in the units the signals are denominated in.
// Host busy is the fraction times the four cores. Host headroom is cores less
// host busy less the one-core reserve, and the machine is called full below
// zero. Limit headroom is the quota less our usage less a tenth of the quota,
// and our own limit is reached below zero. Our share is our usage over the
// machine's busy time, and the two refinements under "the machine is full"
// blame the host below 0.49 and blame us above 0.51.
//
//	condition     host busy   host headroom   limit headroom   our share
//	quiet             0.8            2.2            2.20         0.625
//	filled by them    3.2           -0.2            2.06         0.20
//	filled by us      3.2           -0.2            0.14         0.80
//
// Limit headroom stays positive in all three, which is why the usage figures
// are 0.64 and 2.56 rather than rounder numbers: this instance never reaches
// its own limit, so the message layer renders the pure attribution paragraph
// rather than the blended sentence it writes for a container at BOTH ceilings.
//
// Pressure moves with the load, because a filling machine would show it, and
// stays well under the 0.20 mark at which it would fire and take the headline
// away from capacity.
func cpuFillingQuiet() fakebox.Condition { return cpuFillingMachine(0.20, 0.5, 0.02) }

func cpuFillingHostFilled() fakebox.Condition { return cpuFillingMachine(0.80, 0.64, 0.05) }

func cpuFillingOursFilled() fakebox.Condition { return cpuFillingMachine(0.80, 2.56, 0.05) }

// cpuFillingMachine is this scenario's machine at one condition. Everything the
// three conditions share is written here once, so the three calls above differ
// by exactly the three numbers the story turns on.
func cpuFillingMachine(hostBusy, usageCores, pressure float64) fakebox.Condition {
	return fakebox.Condition{
		Cores:      cpuFillingCores,
		QuotaCores: cpuFillingQuotaCores,
		HostBusy:   hostBusy,
		UsageCores: usageCores,
		Pressure:   pressure,
		PsiPresent: true,
	}
}

// announceFillingMachine prints the fake machine's condition, and is the only
// thing this scenario prints. One line goes out each time the mock changes, the
// opening condition included, and nothing else at all: every other line on the
// page is the supervisor's own output, so a reader can tell a stimulus from a
// response without being told which is which.
//
// It names the CPU limit, which announceMachine does not, because this
// scenario's message changes only on a machine that has one.
func announceFillingMachine(c fakebox.Condition) {
	fmt.Printf("fake machine: %d cores, limit %g cores, host %.0f%% busy, this instance %g cores, pressure %.0f%%\n",
		c.Cores, c.QuotaCores, c.HostBusy*100, c.UsageCores, c.Pressure*100)
}

const (
	// The machine clock the CPU scenarios measured in machine time run on, and
	// the collector cadence that carries it.
	//
	// The box advances one machine SECOND per sampler read (StartPerRead), so
	// readings land one second apart on an exact grid — the cadence the CPU
	// worker runs at in production, and the one thing that makes a release
	// reachable rather than a coincidence. Machine time then advances only when
	// the worker reads, so the collector's cadence is what a machine second
	// costs in wall time, and cpuFastPollWall is what makes these stories
	// affordable: they are two and a half minutes of machine time, because the
	// 60-second windows have to fill and one release rule waits a further 60,
	// and that is under a wall second here.
	cpuMachineSecond = time.Second
	cpuFastPollWall  = 5 * time.Millisecond

	// cpuMachineHoldPoll is how often holdMachine looks at the box's clock, and
	// is under the wall time one machine second costs, so a hold overshoots by
	// less than one reading.
	cpuMachineHoldPoll = time.Millisecond
)

// useFastCPUPoll speeds the collector's poll of the CPU worker up to
// cpuFastPollWall, and returns the function that puts the previous cadence
// back. Why the cadence has to outrun the box's ticker is under cpuFastPollWall.
//
// The registry behind it is process-global, exactly as the deps keys are, so a
// driver that left it set would hand the next scenario in this process a CPU
// worker polling twenty times a second. It is a plain function rather than
// anything the supervisor owns, so it can be called before the worker spawns,
// which is when the collector reads the cadence.
//
// The restore is conditional because nothing can un-register a worker type. In
// this process it always fires: simple.Register publishes every monitor's
// Interval from its own init, so the CPU worker always has a cadence to go back
// to.
func useFastCPUPoll() (restore func()) {
	previous, registered := fsmv2.ObservationIntervalFor(fsmv2cpu.WorkerType)
	fsmv2.RegisterObservationInterval(fsmv2cpu.WorkerType, cpuFastPollWall)

	return func() {
		if registered {
			fsmv2.RegisterObservationInterval(fsmv2cpu.WorkerType, previous)
		}
	}
}

// holdMachine waits until the box has advanced d of MACHINE time, or until ctx
// is cancelled.
//
// Machine time is the right unit for any hold whose length has to mean
// something to the engine, because every window span, every coverage rule and
// every re-fire bar downstream is denominated in the sample clock. Wall time is
// not a stand-in for it. The box advances by however many ticks its ticker
// actually delivered, and a run under debug logging delivers materially fewer
// than the same run with a discarding logger — measured here as a story that
// reached its second verdict on one and never reached it on the other, from the
// same holds. Holding on this clock pays that difference in wall seconds and
// leaves the story identical.
func holdMachine(ctx context.Context, machine *tickingBox, d time.Duration) error {
	deadline := machine.MachineNow().Add(d)

	ticker := time.NewTicker(cpuMachineHoldPoll)
	defer ticker.Stop()

	for machine.MachineNow().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}

	return nil
}
