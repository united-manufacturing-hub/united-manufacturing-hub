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

	"github.com/benbjohnson/clock"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

const (
	// cpuBlindBase is where this scenario's fake machine serves its cgroup
	// files, and has to equal the unexported cgroupBase in pkg/fsmv2/cpu.
	// cpuPressureBase carries the full note on why nothing checks that.
	cpuBlindBase = "/sys/fs/cgroup"

	// The files this scenario takes away, in the order it takes them.
	// /proc/stat is the machine's CPU accounting, outside the cgroup; cpu.stat
	// is the cgroup's. The sampler reads either one, when it cannot be opened,
	// as absent: losing it costs its readings and does not fail the sample.
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

	// cpuBlindHold is how much machine time the second outage must last. The
	// box advances one machine second per read, so the wait sees that many
	// readings without cpu.stat, not only the one the Set produced.
	cpuBlindHold = 5 * time.Second

	// cpuBlindUnavailableMessage is what the worker reports once no machine
	// CPU count can be read: CapacityCores is zero, and composeHealthy renders
	// this instead of a budget headline.
	cpuBlindUnavailableMessage = "CPU monitoring unavailable: cgroup read failed. Defaulting to healthy."
)

// CPUBlindScenarioV2 drives the real CPU monitor over a fake machine and then
// takes away, one at a time, the two files it reads its numbers from.
//
// The story is that neither outage disturbs the reading. Losing /proc/stat
// leaves the poll succeeding and the worker reporting healthy, with a message
// saying CPU monitoring is unavailable, since with no machine CPU count to
// budget against there is nothing to judge. Losing cpu.stat as well reads the
// same way: the sample carries on with its readings absent, the worker stays
// healthy and Fresh through both outages, and the second one holds for several
// readings to prove it. Under the 2026-09-23 decision (ENG-5815) a machine the
// worker cannot measure is reported healthy, which is what it does.
//
// The second outage keeps the first, so the story tests the two failures
// rather than a recovery.
//
// On every change of the worker's message the state_transition line's reason
// field carries it at info.
var CPUBlindScenarioV2 = ScenarioV2{
	Name:        "cpu-blind",
	Description: "Takes away the two files the CPU monitor reads, one at a time (v2)",

	ExpectedWarnings: []string{"cpu::read_failed::error"},

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(cpuBlindBase, cpuBlindMachine())
		machine := &cpuMachine{HangingFS: fakebox.NewHangingFS(box.fs()), box: box}

		m := map[string]any{}

		var fs filesystem.Service = machine
		config.SetDependency(m, fsmv2cpu.FilesystemKey, fs)

		var clk clock.Clock = box.box.Clock()
		config.SetDependency(m, fsmv2cpu.ClockKey, clk)

		// The box advances on the sampler's read of cpu.pressure, which this
		// scenario never takes away, so machine time keeps moving through both
		// outages.
		box.StartPerRead(cpuMachineSecond)

		return m, box.Stop, nil
	},

	Run: func(ctx context.Context, env Env) error {
		machine, err := cpuMachineFromDeps(env)
		if err != nil {
			return err
		}

		env.Step("create the cpu monitor on a quiet machine")

		// Nil config: CPUConfig is an empty struct, and this is the same call
		// the config worker makes in production.
		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		// The headline names the machine's usage, which holds while both files
		// read; the later waits compare against it, so a verdict that never
		// described the readable machine fails the first wait instead.
		if err := waitCPUFirstReading(ctx, env, "first reading healthy", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy
			done := healthy && strings.Contains(st.Result.Message, "The machine is using 1.2 of 4 cores")

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("take /proc/stat away")
		machine.box.Set(cpuBlindMachine(cpuBlindHostStat))

		// No machine CPU count means no capacity to budget against, so the
		// message says monitoring is unavailable and the verdict stays healthy.
		if err := waitCPUFresh(ctx, env, "healthy without /proc/stat", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Message == cpuBlindUnavailableMessage

			return healthy, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("take cpu.stat away as well")
		setAt := machine.box.MachineNow()
		machine.box.Set(cpuBlindMachine(cpuBlindHostStat, cpuBlindCgroupStat))

		return waitCPUFresh(ctx, env, "healthy without cpu.stat for the hold", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Message == cpuBlindUnavailableMessage
			held := machine.box.MachineNow().Sub(setAt) >= cpuBlindHold

			seen := fmt.Sprintf("%s held=%s", cpuStatusSeen(st), machine.box.MachineNow().Sub(setAt))

			return healthy && held, seen
		})
	},
}

// cpuBlindMachine is this scenario's machine with the named files unreadable.
// The calls differ only in which files the machine can no longer see.
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
