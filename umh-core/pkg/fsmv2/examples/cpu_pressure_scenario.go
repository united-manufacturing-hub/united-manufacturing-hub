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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

const (
	// cpuPressureCalm is one point under the 0.20 fire mark.
	cpuPressureCalm = 0.19

	cpuPressureFiring = 0.25

	// The capacity signal reads headroom as cores minus busy minus a one-core
	// reserve, averaged over 60 seconds, and calls the machine full below zero.
	// Four cores at 60% busy is 4 - 2.4 - 1.0 = 0.6 cores from the first
	// reading on: cpu.NewDeps takes a startup snapshot through the same
	// sampler, so the worker's first poll already carries a full rate.
	cpuPressureCores    = 4
	cpuPressureHostBusy = 0.60

	// Nothing in the story turns on this instance's own share of the busy cores.
	cpuPressureUsageCores = 0.5

	// cpuPressureMachineTick is how much machine time one tick of the ticker
	// advances. Its ratio to the worker's one-second poll is a correctness
	// bound, and a tenth keeps this machine's capacity signal quiet.
	//
	// tickingBox locks per file, so a tick can land between the sampler's
	// stamp and its counter reads. It adds counters the stamp does not cover,
	// and that one reading overstates its rate by tick over poll. At a tenth,
	// host busy reads 2.64 against a stated 2.4 and headroom bottoms out at
	// 0.36, still clear of the mark at 0, and a 60-second mean damps even
	// that. At a tick equal to the poll it reads 4.80, headroom is -1.80, and
	// the machine is reported full.
	//
	// So a scenario parking a signal near its mark has to check this ratio
	// against its own margin rather than inherit the number.
	cpuPressureMachineTick = 100 * time.Millisecond
)

// CPUPressureScenarioV2 drives the real CPU monitor over a fake machine that is
// busy but not full, and steps its PSI pressure across the mark at which the
// pressure signal fires.
//
// The story is that pressure alone degrades the machine: tasks are queueing
// for a free core. The scenario does not check the capacity signal. It
// averages over 60 seconds and the run is a few seconds long, so its line
// reads "Machine headroom not available (measuring)" throughout.
var CPUPressureScenarioV2 = ScenarioV2{
	Name:        "cpu-pressure",
	Description: "Raises a fake machine's CPU pressure from 19% to 25%, over the 20% at which the monitor degrades (v2)",

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(fsmv2cpu.CgroupBase, cpuPressureMachine(cpuPressureCalm))
		m := cpuMachineDeps(box)

		box.Start(cpuPressureMachineTick)

		return m, box.Stop, nil
	},

	Run: func(ctx context.Context, env Env) error {
		machine, err := cpuMachineFromDeps(env)
		if err != nil {
			return err
		}

		env.Step("create the cpu monitor on a machine using 60% of 4 cores, with CPU pressure at 19%, under the 20% at which the monitor degrades")

		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		// The pressure figure is a level the kernel reports directly, so the
		// wall ticker cannot move it and this text holds while the machine
		// stays at 0.19. The usage-headroom line beside it is a rate, which the
		// ticker does move, so the wait pins the level and not the rate.
		if err := waitCPUFirstReading(ctx, env, "first reading healthy with pressure at 19%", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy
			done := healthy && strings.Contains(st.Result.Message, "Pressure 19% (degrades above 20%)")

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("raise CPU pressure to 25%; wait for the worker to go degraded")
		machine.box.Set(cpuPressureMachine(cpuPressureFiring))

		// PSI is a level, so the next reading fires, and the latch holds it while
		// pressure stays over the 0.12 clear mark.
		return waitCPUFresh(ctx, env, "degraded by pressure at 25%", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
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
