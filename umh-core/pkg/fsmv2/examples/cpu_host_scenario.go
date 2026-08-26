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
)

// CPUHostScenarioV2 runs the CPU monitor against the machine it is running
// on, unmodified. Every other CPU scenario publishes a fake machine before
// the upsert; this one publishes nothing, so the CPU worker reads the host's
// own cgroup v2 and /proc/stat files, and whatever the log says about the
// machine is what the real machine deserves. It is for watching the monitor
// work, not for judging it: nothing in the suite asserts its output, and its
// result changes with the machine it runs on.
//
// # CLI Usage
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario=cpu-host --duration=60s --log-level=debug
//
// Debug is the level to watch at: the CPU worker's verdict never moves its
// FSM state, so the collector's observed_changed line, which is a debug
// line, is where each reading shows up.
//
// On a machine with no cgroup v2 CPU files, such as every developer Mac, no
// reading can ever land, and the driver refuses rather than spend its whole
// duration on a machine it cannot read. The refusal names tools/cpu-host,
// the wrapper that runs this scenario in a Linux container where those
// files exist.
var CPUHostScenarioV2 = ScenarioV2{
	Name:        "cpu-host",
	Description: "Runs the CPU monitor against the machine it is running on, unmodified (v2)",
	Driver: func(ctx context.Context, env Env) error {
		// cpu.stat is the sampler's primary file: a failure there fails the whole
		// sample, so its absence means every poll errors and no reading ever lands.
		// Refuse here rather than leaving the reader to infer that from a silent
		// stream of read errors.
		if _, err := os.Stat("/sys/fs/cgroup/cpu.stat"); err != nil {
			return fmt.Errorf("this host publishes no cgroup v2 CPU files, so no reading can land: %w; run it under tools/cpu-host, which puts it in a Linux container", err)
		}

		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		return awaitFirstCPUReading(ctx, env.Client)
	},
}
