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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

const (
	// cpuFillingCores is the machine's CPU count and cpuFillingQuotaCores this
	// instance's own CPU limit, the figure docker run --cpus sets.
	//
	// The limit is needed. The message names who filled the machine only when
	// a limit applies (Details.LimitApplies in message.go). Without one, both
	// filled conditions render the same sentence.
	cpuFillingCores      = 4
	cpuFillingQuotaCores = 3
)

// CPUFillingScenario drives the real CPU monitor over a fake machine that
// fills up from outside, and then over the same machine filled by this
// instance's own load.
//
// The story is that "the machine is full" is not the whole answer, because
// the remedy depends on whose load filled it. Filled from outside, the advice
// is to reduce the other software on the machine. Filled by this instance, the
// advice is to reduce this instance's load. The machine is 80% busy in both,
// its pressure is the same, and this instance stays under its own limit in
// both. The one difference is how much of the busy time is ours.
var CPUFillingScenario = Scenario{
	Name:        "cpu-filling",
	Description: "Fills a fake machine from outside, then from this instance, and shows the remedy change",

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(fsmv2cpu.CgroupBase, cpuFillingQuiet())
		m := cpuMachineDeps(box)

		box.StartPerRead(cpuMachineReadAdvance)

		return m, box.Stop, nil
	},

	Run: func(ctx context.Context, env Env) error {
		machine, err := cpuMachineFromDeps(env)
		if err != nil {
			return err
		}

		env.Step("create the cpu monitor on a quiet machine: 20% of 4 cores busy, this instance using 0.5 of its 3-core limit")

		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		if err := waitCPUFirstReading(ctx, env, "first reading healthy on the quiet machine", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy
			done := healthy && strings.Contains(st.Result.Message, "This instance is using 0.5 of 3 cores (17% of its limit)")

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("fill the machine from outside: 80% busy, this instance still using 0.64 cores")
		machine.box.Set(cpuFillingHostFilled())

		// The machine reads full once every point in the headroom window is a
		// filled one, and our share stays at 0.2, so the blame on the host lasts
		// while this condition holds.
		if err := waitCPUFresh(ctx, env, "degraded, advising to reduce the other software", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			degraded := st.Degraded && st.Result.Verdict.State == cpuhealth.StateDegraded
			done := degraded && strings.Contains(st.Result.Message, "reduce other software running on it")

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("fill the machine with this instance's own load: still 80% busy, this instance now using 2.56 cores")
		machine.box.Set(cpuFillingOursFilled())

		return waitCPUFresh(ctx, env, "degraded, advising to reduce this instance's load", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			degraded := st.Degraded && st.Result.Verdict.State == cpuhealth.StateDegraded
			done := degraded && strings.Contains(st.Result.Message, "this instance is using most of it")

			return done, cpuStatusSeen(st)
		})
	},
}

// The three conditions, in the order Run walks them. Host busy is the fraction
// times the four cores. Host headroom is cores less host busy less the
// one-core reserve, and the machine is full below zero. Limit headroom is the
// quota less our usage less a tenth of the quota, and our limit is reached
// below zero. Our share is our usage over the machine's busy time. Under a full
// machine, a share below 0.49 blames the host and one above 0.51 blames us.
//
//	condition        host busy   host headroom   limit headroom   our share
//	quiet               0.8           2.2            2.20           0.625
//	filled by them      3.2          -0.2            2.06           0.20
//	filled by us        3.2          -0.2            0.14           0.80
//
// Limit headroom stays positive in all three, so this instance never reaches
// its own limit. Pressure stays under the 0.20 mark, so it never takes the
// headline away from capacity.
func cpuFillingQuiet() fakebox.Condition { return cpuFillingMachine(0.20, 0.5, 0.02) }

func cpuFillingHostFilled() fakebox.Condition { return cpuFillingMachine(0.80, 0.64, 0.05) }

func cpuFillingOursFilled() fakebox.Condition { return cpuFillingMachine(0.80, 2.56, 0.05) }

// cpuFillingMachine is this scenario's machine at one condition.
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
