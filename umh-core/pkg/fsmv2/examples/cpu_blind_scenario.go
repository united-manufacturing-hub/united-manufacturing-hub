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
	cpuBlindHostStat   = "/proc/stat"
	cpuBlindCgroupStat = fsmv2cpu.CgroupBase + "/cpu.stat"

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

// CPUBlindScenario drives the real CPU monitor over a fake machine and then
// takes away, one at a time, the two files it reads its numbers from.
//
// The story is that neither outage changes the worker's state. It stays
// running, healthy and Fresh through both, and its message becomes "CPU
// monitoring unavailable". The worker still reports a machine it cannot
// measure as healthy. ENG-6319 (unreadable cgroup files are reported as
// healthy) will report it as degraded, and this scenario will then expect
// degraded.
//
// The second outage keeps the first, so the story tests the two failures
// rather than a recovery.
var CPUBlindScenario = Scenario{
	Name:        "cpu-blind",
	Description: "Takes away the two files the CPU monitor reads, one at a time",

	ExpectedWarnings: []string{"cpu::read_failed::error"},

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(fsmv2cpu.CgroupBase, cpuBlindMachine())
		m := cpuMachineDeps(box)

		// The box advances on the sampler's read of cpu.pressure, which this
		// scenario never takes away, so machine time keeps moving through both
		// outages.
		box.StartPerRead(fsmv2cpu.PollInterval)

		return m, box.Stop, nil
	},

	Run: func(ctx context.Context, env Env) error {
		machine, err := cpuMachineFromDeps(env)
		if err != nil {
			return err
		}

		env.Step("create the cpu monitor on a quiet machine")

		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		// A headline naming the machine's usage proves the worker described the
		// readable machine, so the unavailable message the later waits look
		// for is a change.
		if err := waitCPUFirstReading(ctx, env, "first reading healthy", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy
			done := healthy && strings.Contains(st.Result.Message, "The machine is using 1.2 of 4 cores")

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("take /proc/stat away; expect one cpu::read_failed warning, and the worker stays running and healthy")
		machine.box.Set(cpuBlindMachine(cpuBlindHostStat))

		if err := waitCPUFresh(ctx, env, "healthy with \"CPU monitoring unavailable\" without /proc/stat", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Message == cpuBlindUnavailableMessage

			return healthy, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("take cpu.stat away as well; expect a second cpu::read_failed warning, and the worker stays running and healthy")
		setAt := machine.box.MachineNow()
		machine.box.Set(cpuBlindMachine(cpuBlindHostStat, cpuBlindCgroupStat))

		return waitCPUFresh(ctx, env, "healthy without cpu.stat for 5 seconds of machine time", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Message == cpuBlindUnavailableMessage
			held := machine.box.MachineNow().Sub(setAt) >= cpuBlindHold

			seen := fmt.Sprintf("%s held=%s", cpuStatusSeen(st), machine.box.MachineNow().Sub(setAt))

			return healthy && held, seen
		})
	},
}

// cpuBlindMachine is this scenario's machine with the named files unreadable.
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
