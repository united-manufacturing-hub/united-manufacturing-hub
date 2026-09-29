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
	// cpuPressureBase is where the fake machine serves its cgroup files. It
	// must equal the unexported cgroupBase in pkg/fsmv2/cpu. Nothing checks
	// that, and a mismatch makes every read fail.
	cpuPressureBase = "/sys/fs/cgroup"

	// cpuPressureCalm is one point under the pressure signal's 0.20 fire mark,
	// so the crossing later is a one-point change.
	cpuPressureCalm = 0.19

	// cpuPressureFiring is over that fire mark, so the signal fires.
	cpuPressureFiring = 0.25

	// cpuPressureCores is the fake machine's CPU count, and cpuPressureHostBusy
	// how much of it the whole machine is using.
	//
	// The capacity signal reads headroom as cores minus busy minus a one-core
	// reserve, averaged over 60 seconds, and calls the machine full below zero.
	// Four cores at 60% busy is 4 - 2.4 - 1.0 = 0.6 cores from the first
	// reading on: cpu.NewDeps takes a startup snapshot through the same
	// sampler, so the worker's first poll already carries a full rate.
	cpuPressureCores    = 4
	cpuPressureHostBusy = 0.60

	// cpuPressureUsageCores is what this instance itself uses, 0.5 of the 2.4
	// busy cores. Nothing in the story turns on that split.
	cpuPressureUsageCores = 0.5

	// cpuPressureMachineTick is how much machine time one tick of the ticker
	// advances. Its ratio to the worker's one-second poll is a correctness
	// bound, and a tenth keeps this machine's capacity signal quiet.
	//
	// tickingBox locks per file, so a tick can land between the sampler's
	// stamp and its counter reads. It adds counters the stamp does not cover,
	// and that one reading overstates its rate by tick over poll. At a tenth, host busy reads 2.64 against a stated 2.4 and
	// headroom bottoms out at 0.36, still clear of the mark at 0, and a
	// 60-second mean damps even that. At a tick equal to the poll it reads
	// 4.80, headroom is -1.80, and the machine is reported full.
	//
	// So a scenario parking a signal near its mark has to check this ratio
	// against its own margin rather than inherit the number.
	cpuPressureMachineTick = 100 * time.Millisecond
)

// CPUPressureScenarioV2 drives the real CPU monitor over a fake machine that is
// busy but not full, and steps its PSI pressure across the mark at which the
// pressure signal fires.
//
// The story is that a machine can be degraded with cores to spare, because
// tasks are queueing rather than because capacity ran out. Sixty percent of
// four cores leaves the capacity signal clear throughout; pressure alone moves,
// from one point under its fire mark to over it.
var CPUPressureScenarioV2 = ScenarioV2{
	Name:        "cpu-pressure",
	Description: "Steps a fake machine's CPU pressure over its fire mark while capacity stays clear (v2)",

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(cpuPressureBase, cpuPressureMachine(cpuPressureCalm))
		machine := &cpuMachine{HangingFS: fakebox.NewHangingFS(box.fs()), box: box}

		m := map[string]any{}

		var fs filesystem.Service = machine
		config.SetDependency(m, fsmv2cpu.FilesystemKey, fs)

		var clk clock.Clock = box.box.Clock()
		config.SetDependency(m, fsmv2cpu.ClockKey, clk)

		box.Start(cpuPressureMachineTick)

		return m, box.Stop, nil
	},

	Run: func(ctx context.Context, env Env) error {
		machine, err := cpuMachineFromDeps(env)
		if err != nil {
			return err
		}

		env.Step("create the cpu monitor on a busy machine with pressure one point under its fire mark")

		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		// The pressure figure is a level the kernel reports directly, so the
		// wall ticker cannot move it and this text holds while the machine
		// stays at 0.19. The usage-headroom line beside it is a rate, which the
		// ticker does move, so the wait pins the level and not the rate.
		if err := waitCPUFirstReading(ctx, env, "first reading healthy under the fire mark", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy
			done := healthy && strings.Contains(st.Result.Message, "Pressure 19% (degrades above 20%)")

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("raise the machine's pressure over its fire mark")
		machine.box.Set(cpuPressureMachine(cpuPressureFiring))

		// PSI is a level, so the next reading fires the signal, and the latch
		// holds it while pressure stays over the 0.12 clear mark: this
		// condition lasts to the end of the run.
		return waitCPUFresh(ctx, env, "degraded by pressure over the fire mark", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			degraded := st.Degraded && st.Result.Verdict.State == cpuhealth.StateDegraded
			done := degraded && strings.Contains(st.Result.Message, "spent 25% of the last minute waiting for a free CPU core")

			return done, cpuStatusSeen(st)
		})
	},
}

// cpuPressureMachine is the machine this scenario runs on, at the given PSI
// pressure.
func cpuPressureMachine(pressure float64) fakebox.Condition {
	return fakebox.Condition{
		Cores:      cpuPressureCores,
		HostBusy:   cpuPressureHostBusy,
		UsageCores: cpuPressureUsageCores,
		Pressure:   pressure,
		PsiPresent: true,
	}
}
