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
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

// CPUStallScenarioV2 drives the real CPU monitor over a fake machine whose
// cpu.stat read hangs mid-poll, and watches the reading go stale and then
// recover, through the same GetFresh call the container monitor reads with.
//
// The story is that the worker stops producing readings, the reading ages
// past the shared staleness limit, and the monitor sees it. A missing file never does this (cpu-blind checks
// that); a read that blocks does.
//
// The worker stays running through the hang, so no state_transition line
// appears and the log is silent from the hang step to the release step. The
// staleness shows only in the freshness GetFresh returns with the reading.
//
// The hang must end well before 20 seconds. At 10 seconds the supervisor logs
// data_stale, which every run allows; at 20 seconds it logs the timeout and
// restart warnings this scenario does not expect. The stale wait ends at about
// three seconds and the release step follows it at once.
var CPUStallScenarioV2 = ScenarioV2{
	Name:        "cpu-stall",
	Description: "Hangs the cpu.stat read mid-poll, and watches the reading go stale and then recover (v2)",

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(cpuBlindBase, cpuBlindMachine())
		m := cpuMachineDeps(box)

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

		env.Step("hang reads of cpu.stat; the worker stays running, so wait for GetFresh to call the reading stale (3 seconds)")

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

		env.Step("let the hung cpu.stat read return; wait for a fresh, healthy reading")
		release()

		// Until the collector saves again the reading is still Stale, so Stale
		// before the first Fresh is not done. The first Fresh reading may be
		// the hung tick's own, whose context expired mid-hang, so the wait
		// also needs it healthy.
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
