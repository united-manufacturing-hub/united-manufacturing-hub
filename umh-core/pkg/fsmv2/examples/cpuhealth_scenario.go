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
	"errors"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// cpuScenarioBase is the cgroup path the mock sampler reads under, matching the
// production sampler's "/sys/fs/cgroup".
const cpuScenarioBase = "/sys/fs/cgroup"

// runCPUHealthScenario drives the fsmv2 CPU monitor worker over a MOCKED
// filesystem through two paths: a healthy one (a quiet, present cgroup judges a
// healthy verdict with its capable/measured counts) and an unhappy one (an
// unreadable cpu.stat — a whole-sample failure — reports could-not-measure).
//
// It is a dev scenario, not a substitute for the rung specs: it exists so you
// can watch the worker execute before the P4 seam mounts it.
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario=cpuhealth
func runCPUHealthScenario(ctx context.Context) string {
	var out string
	appendf := func(format string, a ...any) {
		line := fmt.Sprintf(format, a...)
		out += line + "\n"
		fmt.Println(line)
	}

	appendf("--- cpuhealth scenario: healthy box ---")

	// Happy path: a readable cgroup. cpu.stat is primary; cpu.max names a
	// 2-quota limit; /proc/stat and the cpuset make the host and affinity
	// signals readable too.
	healthyS := cpuhealth.NewLinuxSampler(cpuMockFS(false), cpuScenarioBase)
	healthyD := fsmv2cpu.NewDepsWithSampler(
		deps.Identity{ID: "cpu", WorkerType: fsmv2cpu.WorkerType},
		deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, deps.Identity{ID: "cpu", WorkerType: fsmv2cpu.WorkerType}),
		healthyS,
	)

	appendf("setup: mock filesystem serving a readable 2-quota cgroup")

	// A Poll is a faithful single tick (the collector calls Poll each interval).
	status, err := fsmv2cpu.Poll(ctx, healthyD, fsmv2cpu.CPUConfig{})
	if err != nil {
		appendf("healthy tick errored: %v", err)
	} else {
		appendf("tick: verdict=%q message=%q capable=%d measured=%d", status.Verdict, status.Message, status.SignalsCapable, status.SignalsMeasured)
	}

	appendf("--- cpuhealth scenario: failing read box ---")

	// Unhappy path: cpu.stat unreadable. This is a whole-sample failure — the
	// worker reports could-not-measure, never a healthy zero.
	failingS := cpuhealth.NewLinuxSampler(cpuMockFS(true), cpuScenarioBase)
	failingD := fsmv2cpu.NewDepsWithSampler(
		deps.Identity{ID: "cpu", WorkerType: fsmv2cpu.WorkerType},
		deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, deps.Identity{ID: "cpu", WorkerType: fsmv2cpu.WorkerType}),
		failingS,
	)

	status, err = fsmv2cpu.Poll(ctx, failingD, fsmv2cpu.CPUConfig{})
	if err != nil {
		appendf("failing tick: could not measure (%v)", err)
	} else {
		appendf("failing tick: verdict=%q (a healthy zero here would be the bug)", status.Verdict)
	}

	return out
}

// CPUHealthScenarioEntry registers the cpuhealth scenario for CLI access.
//
// It uses a CustomRunner with YAMLConfig "" — a YAML-spawned worker would go
// through the production NewDeps and get a real filesystem, which is exactly
// what a mocked-filesystem scenario cannot use.
//
// # CLI Usage
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario cpuhealth
//
// What it drives: the fsmv2 CPU monitor worker's Poll over a mocked filesystem,
// through a healthy (quiet present cgroup judges healthy with capable/measured
// counts) and an unhappy (unreadable cpu.stat reports could-not-measure) path.
var CPUHealthScenarioEntry = Scenario{
	Name:        "cpuhealth",
	Description: "Drives the CPU monitor worker over a mocked filesystem (healthy + failing read)",
	YAMLConfig:  "", // worker built directly with a mock-backed sampler
	CustomRunner: func(ctx context.Context, _ RunConfig) (*RunResult, error) {
		runCPUHealthScenario(ctx)
		done := make(chan struct{})
		close(done)

		// ShutdownClean is true: this scenario drives Poll directly over a
		// mocked filesystem and has no supervisor, so there is nothing that
		// could drain uncleanly. The CLI exits 0 when it is true.
		return &RunResult{Done: done, ShutdownClean: true}, nil
	},
}

// cpuMockFS returns a mocked filesystem.Service. When failCPUStat is true,
// cpu.stat is unreadable (a whole-sample failure); otherwise it serves a quiet,
// readable, 2-quota cgroup so the healthy path judges healthy.
func cpuMockFS(failCPUStat bool) filesystem.Service {
	fs := filesystem.NewMockFileSystem()
	fs.ReadFileFunc = func(_ context.Context, path string) ([]byte, error) {
		switch path {
		case cpuScenarioBase + "/cpu.stat":
			if failCPUStat {
				return nil, errors.New("permission denied")
			}
			return []byte("usage_usec 5000000\nuser_usec 4000000\nsystem_usec 1000000\nnr_periods 100\nnr_throttled 2\n"), nil
		case cpuScenarioBase + "/cpu.max":
			return []byte("200000 100000\n"), nil
		case cpuScenarioBase + "/cpu.pressure":
			// PSI "some" avg60=10 (10% pressure, quiet). Serves a readable
			// pressure so the healthy box's first tick judges it measured.
			return []byte("some avg10=0.10 avg60=10.00 avg300=20.00 total=100000\n"), nil
		case "/proc/stat":
			return []byte("cpu  100 0 50 1000 0 10 5 0 0 0\ncpu0 50 0 25 500 0 5 2 0 0 0\n"), nil
		case cpuScenarioBase + "/cpuset.cpus.effective":
			return []byte("0\n"), nil
		case "/proc/cpuinfo":
			return []byte("processor\t: 0\n"), nil
		default:
			return nil, errors.New("unreadable")
		}
	}

	return fs
}
