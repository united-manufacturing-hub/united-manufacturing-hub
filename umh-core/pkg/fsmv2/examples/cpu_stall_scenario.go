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

	"github.com/benbjohnson/clock"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// CPUStallScenarioV2 drives the real CPU monitor over a fake machine whose
// cpu.stat read hangs mid-poll, and watches the reading go stale and then
// recover, through the same GetFresh call the container monitor reads with.
//
// The story is the case that matters most in the field: the worker stops
// producing readings, the reading ages past the shared staleness limit, and
// the monitor sees it. A missing file never does this (cpu-blind checks
// that); a read that blocks does.
//
// The hang must end well before 20 seconds. At 10 seconds the supervisor logs
// data_stale, which every run allows; at 20 seconds it logs the timeout and
// restart warnings this scenario does not expect. The stale wait ends at about
// three seconds and the release step follows it at once.
//
// On every change of the worker's message the state_transition line's reason
// field carries it at info.
var CPUStallScenarioV2 = ScenarioV2{
	Name:        "cpu-stall",
	Description: "Hangs the cpu.stat read mid-poll, and watches the reading go stale and then recover (v2)",

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(cpuBlindBase, cpuBlindMachine())
		machine := &cpuMachine{HangingFS: fakebox.NewHangingFS(box.fs()), box: box}

		m := map[string]any{}

		var fs filesystem.Service = machine
		config.SetDependency(m, fsmv2cpu.FilesystemKey, fs)

		var clk clock.Clock = box.box.Clock()
		config.SetDependency(m, fsmv2cpu.ClockKey, clk)

		// The box advances once per read, when the sampler opens
		// cpu.pressure. While the cpu.stat read hangs no new read starts, so
		// machine time stops.
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

		// The headline names the machine's usage, so the waits after the hang
		// can tell the recovered reading from the startup reading and from a
		// machine the worker cannot measure.
		if err := waitCPUFirstReading(ctx, env, "first reading healthy", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy
			done := healthy && strings.Contains(st.Result.Message, "The machine is using 1.2 of 4 cores")

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("hang reads of cpu.stat")

		// The deferred release is the backstop that lets teardown finish if a
		// wait fails with the read still hung; the release step below is the
		// one the story needs.
		release := machine.Hang(cpuBlindCgroupStat)
		defer release()

		// Staleness is decided on the wall clock: the collector stamps
		// CollectedAt with time.Now() when it wraps each observation
		// (wrapNewObservation, pkg/fsmv2/supervisor/internal/collection), and
		// the box's clock reaches only the sample's own Timestamp. Machine time
		// stops while the read hangs, so a wait on it would run out its 30
		// seconds; this wait is plain wall time. The collector goroutine blocks
		// inside Poll and holds collectionMu, and its ticker drops ticks, so
		// nothing is saved and the reading stays Stale for as long as the read
		// hangs.
		if err := env.WaitFor(ctx, "reading stale while the read hangs", func(ctx context.Context) (bool, string, error) {
			st, fresh, err := cpuReading(ctx, env)
			if err != nil {
				return false, "", err
			}

			switch fresh {
			case fsmv2client.Stale:
				return true, "stale", nil
			case fsmv2client.Fresh:
				// A Fresh reading means the collector's last save is still
				// inside the staleness limit: not done yet.
				return false, fmt.Sprintf("fresh %s", cpuStatusSeen(st)), nil
			default:
				return false, "", fmt.Errorf("the CPU reading is %s while the read hangs", freshnessName(fresh))
			}
		}); err != nil {
			return err
		}

		env.Step("release cpu.stat")
		release()

		// The first polls after the release still see the Stale reading,
		// because the collector has not saved yet, so Stale means not done
		// here rather than a failure. A Fresh reading that is not healthy is
		// the hung tick's own observation, whose context expired mid-hang;
		// that one is also not done. The wait is done on the first Fresh and
		// healthy reading, and once a Fresh reading has been seen, a reading
		// that is not Fresh fails it at once.
		seenFresh := false

		return env.WaitFor(ctx, "fresh and healthy again", func(ctx context.Context) (bool, string, error) {
			st, fresh, err := cpuReading(ctx, env)
			if err != nil {
				return false, "", err
			}

			if fresh == fsmv2client.Stale && !seenFresh {
				return false, "stale, waiting for the collector's first save", nil
			}

			if fresh != fsmv2client.Fresh {
				return false, "", fmt.Errorf("the CPU reading is %s, not Fresh, after the release", freshnessName(fresh))
			}

			seenFresh = true

			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy &&
				strings.Contains(st.Result.Message, "The machine is using 1.2 of 4 cores")

			return healthy, fmt.Sprintf("fresh %s", cpuStatusSeen(st)), nil
		})
	},
}
