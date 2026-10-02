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
	// The usage file only the v1 reader opens, and the usage file only the v2
	// reader opens.
	cpuV1UsageFile = fsmv2cpu.CgroupBase + "/cpu,cpuacct/cpuacct.usage"
	cpuV2StatFile  = fsmv2cpu.CgroupBase + "/cpu.stat"

	cpuV1QuietUsageCores = 0.5

	// Over its 2-core limit, this instance is throttled in 10% of periods,
	// twice the 5% at which the throttling signal fires.
	cpuV1LimitedUsageCores = 1.9
	cpuV1LimitedThrottle   = 0.10

	// The worker's messages for this machine on a cgroup v2 box.
	cpuV1HealthyHeadlineAsOnV2 = "CPU healthy. This instance is using 0.5 of 2 cores (25% of its limit) and can use 1.3 more before it is marked degraded."
	cpuV1LimitedHeadlineAsOnV2 = "CPU limited\nThis instance hit its CPU limit and was paused until the next cycle"
)

// CPUV1Scenario drives the real CPU monitor over a fake machine that mounts
// cgroup v1, then pushes this instance into its CPU limit.
//
// The story is that the worker judges a v1 machine the way it judges a v2
// one. It reads the limit from cpu.cfs_quota_us and cpu.cfs_period_us, the
// usage from cpuacct.usage and the throttling from v1's cpu.stat, and
// composes the messages a v2 machine in the same condition gets. v1 has no
// cpu.pressure, so the pressure line reads "not measured" and no warning is
// raised for it.
var CPUV1Scenario = Scenario{
	Name:        "cpu-v1",
	Description: "Runs the CPU monitor over a fake cgroup v1 machine and throttles it at its CPU limit",

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(fsmv2cpu.CgroupBase, cpuV1Machine(cpuV1QuietUsageCores, 0))
		m := cpuMachineDeps(box)

		box.StartPerRead(cpuMachineReadAdvance)

		return m, box.Stop, nil
	},

	Run: func(ctx context.Context, env Env) error {
		machine, err := cpuMachineFromDeps(env)
		if err != nil {
			return err
		}

		env.Step("create the cpu monitor on a cgroup v1 machine: 4 cores, this instance using 0.5 of its 2-core limit")

		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		if err := waitCPUFirstReading(ctx, env, "first reading healthy, as on v2", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy
			done := healthy && firstLine(st.Result.Message) == cpuV1HealthyHeadlineAsOnV2

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		if machine.box.Reads(cpuV1UsageFile) == 0 || machine.box.Reads(cpuV2StatFile) > 0 {
			return fmt.Errorf("the worker did not read the machine as cgroup v1: %d reads of %s, %d reads of %s",
				machine.box.Reads(cpuV1UsageFile), cpuV1UsageFile, machine.box.Reads(cpuV2StatFile), cpuV2StatFile)
		}

		env.Step("raise this instance to 1.9 cores, throttled in 10% of periods; wait for the worker to go degraded")
		machine.box.Set(cpuV1Machine(cpuV1LimitedUsageCores, cpuV1LimitedThrottle))

		// The throttle ratio climbs towards 10% as the 60-second window fills,
		// and the signal stays fired while it is over the 3% clear mark.
		return waitCPUFresh(ctx, env, "degraded by its CPU limit, as on v2", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			degraded := st.Degraded && st.Result.Verdict.State == cpuhealth.StateDegraded
			done := degraded && strings.HasPrefix(st.Result.Message, cpuV1LimitedHeadlineAsOnV2)

			return done, cpuStatusSeen(st)
		})
	},
}

// cpuV1Machine is this scenario's machine with this instance at the given
// usage and throttle.
func cpuV1Machine(usageCores, throttle float64) fakebox.Condition {
	return fakebox.Condition{
		Cores:      4,
		QuotaCores: 2,
		HostBusy:   0.30,
		UsageCores: usageCores,
		Throttle:   throttle,
		CgroupV1:   true,
	}
}
