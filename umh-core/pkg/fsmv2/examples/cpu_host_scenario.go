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
	"os"

	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

// CPUHostScenarioV2 runs the CPU monitor against the machine it is running
// on. It publishes no fake machine, so the CPU worker reads the host's own
// cgroup v2 and /proc/stat files. It is for watching the monitor work: the
// wait asserts only that a fresh reading arrived, nothing about the verdict.
//
// On every change of the worker's message the state_transition line's
// reason field carries it at info.
//
// On a machine without /sys/fs/cgroup/cpu.stat, such as a developer Mac, Run
// refuses rather than watch readings that cannot describe the cgroup. The
// refusal names tools/cpu-host, which runs this scenario in a Linux container.
var CPUHostScenarioV2 = ScenarioV2{
	Name:        "cpu-host",
	Description: "Runs the CPU monitor against the machine it is running on, unmodified (v2)",
	Run: func(ctx context.Context, env Env) error {
		if _, err := os.Stat("/sys/fs/cgroup/cpu.stat"); err != nil {
			return fmt.Errorf("this host publishes no cgroup v2 CPU files, so no reading can describe the machine: %w; run it under tools/cpu-host, which puts it in a Linux container", err)
		}

		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		return waitCPUFirstReading(ctx, env, "first fresh reading", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			return true, cpuStatusSeen(st)
		})
	},
}
