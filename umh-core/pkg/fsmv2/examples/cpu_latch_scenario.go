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
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
)

const (
	// The pressure levels this scenario stages, against the pressure signal's
	// 0.20 fire mark and 0.12 clear mark (pressureMarks in pkg/cpuhealth).
	// cpuLatchNoise is under the fire mark and over the clear mark, so a fired
	// signal holds on it and an unfired one would not fire.
	cpuLatchFiring = 0.25
	cpuLatchNoise  = 0.15
	cpuLatchCalm   = 0.05

	// cpuLatchWindow is the pressure window's span. The latch releases only
	// once the window has collected this long, and a released signal cannot
	// fire again for this long (Latch.Update in pkg/diagnosis).
	cpuLatchWindow = 60 * time.Second
)

// CPULatchScenarioV2 drives the real CPU monitor over a fake machine whose PSI
// pressure crosses its fire mark, falls into the band between the two marks,
// drops under the clear mark, and rises again.
//
// The story is what the two marks buy and what they cost. A degraded machine
// stays degraded while its pressure wanders below the mark that fired it. It
// is released when the pressure drops under the clear mark. When the pressure
// comes back, the report is late: a released signal cannot fire again for 60
// machine seconds, so the machine sits over its fire mark while reported
// healthy. The last wait checks that the new verdict came no earlier than that.
var CPULatchScenarioV2 = ScenarioV2{
	Name:        "cpu-latch",
	Description: "Holds a fake machine's CPU verdict through noise, releases it on recovery, and shows the bar on re-firing (v2)",

	Dependencies: func() (map[string]any, func(), error) {
		box := newTickingBox(cpuPressureBase, cpuPressureMachine(cpuLatchFiring))
		m := cpuMachineDeps(box)

		box.StartPerRead(cpuMachineReadAdvance)

		return m, box.Stop, nil
	},

	Run: func(ctx context.Context, env Env) error {
		machine, err := cpuMachineFromDeps(env)
		if err != nil {
			return err
		}

		env.Step("create the cpu monitor on a machine with CPU pressure at 25%, over the 20% at which the monitor degrades")

		if err := env.Client.Upsert(fsmv2cpu.Ref, nil); err != nil {
			return fmt.Errorf("upsert cpu monitor: %w", err)
		}

		// PSI is a level and the pressure reduction is the newest reading, so
		// the first reading fires, and the verdict holds while pressure stays
		// over the 0.12 clear mark.
		if err := waitCPUFirstReading(ctx, env, "first reading degraded by pressure at 25%", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			return cpuLatchDegradedAt(st, 25)
		}); err != nil {
			return err
		}

		// The latch cannot release until the window covers 60 machine seconds.
		// Before that, the noise below would hold the verdict whatever the clear
		// mark, and the release would come late. Machine time only rises, so
		// this lasts.
		firstAt := machine.box.MachineNow()
		coveredAt := firstAt.Add(cpuLatchWindow)

		if err := env.WaitFor(ctx, "the pressure window covers 60 machine seconds", func(context.Context) (bool, string, error) {
			now := machine.box.MachineNow()

			return !now.Before(coveredAt), fmt.Sprintf("machine time %s of %s", now.Sub(firstAt), cpuLatchWindow), nil
		}); err != nil {
			return err
		}

		env.Step("drop CPU pressure to 15%, under the fire mark but over the 12% clear mark; the verdict must hold")
		machine.box.Set(cpuPressureMachine(cpuLatchNoise))

		if err := waitCPUFresh(ctx, env, "still degraded with pressure at 15%", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			return cpuLatchDegradedAt(st, 15)
		}); err != nil {
			return err
		}

		env.Step("drop CPU pressure to 5%, under the clear mark; the verdict must release")

		calmAt := machine.box.MachineNow()
		machine.box.Set(cpuPressureMachine(cpuLatchCalm))

		// The window is covered, so the first reading at 5% releases. The
		// healthy verdict then lasts while pressure stays at 5%.
		if err := waitCPUFresh(ctx, env, "healthy with pressure at 5%", func(st simple.Status[fsmv2cpu.CPUStatus]) (bool, string) {
			healthy := !st.Degraded && st.Result.Verdict.State == cpuhealth.StateHealthy
			done := healthy && strings.Contains(st.Result.Message, "Pressure 5% (degrades above 20%)")

			return done, cpuStatusSeen(st)
		}); err != nil {
			return err
		}

		env.Step("raise CPU pressure back to 25%; the verdict may fire again only 60 machine seconds after the release")

		fireAt := machine.box.MachineNow()
		machine.box.Set(cpuPressureMachine(cpuLatchFiring))

		// The healthy stretch before the re-fire is bounded, so no wait accepts
		// it. The check below reads the end of that stretch instead, which lasts.
		// Without the bar the signal fires on the first reading after fireAt,
		// at most one advance later. The check tells that apart from the bar
		// only if that reading is still inside the bar. A slower machine makes
		// this guard trip, never pass wrongly.
		if gap := fireAt.Sub(calmAt); gap+cpuMachineReadAdvance >= cpuLatchWindow {
			return fmt.Errorf("pressure went back to 25%% %s after the 5%% step; with one %s reading more that reaches the %s re-fire bar, so the re-fire check would pass on any code",
				gap, cpuMachineReadAdvance, cpuLatchWindow)
		}

		barEnd := calmAt.Add(cpuLatchWindow)

		// One observation carries both the verdict and its sample's machine
		// time, so a later observation cannot lend this one a later time.
		return env.WaitFor(ctx, "degraded again at 25%, no earlier than 60 machine seconds after the release", func(ctx context.Context) (bool, string, error) {
			obs, err := fsmv2client.Get[simple.Status[fsmv2cpu.CPUStatus]](ctx, env.Client, fsmv2cpu.Ref)
			if err != nil {
				if errors.Is(err, fsmv2client.ErrNotObserved) {
					return false, "", fmt.Errorf("the cpu worker has no observation after it was observed: %w", err)
				}

				return false, "", err
			}

			if age := time.Since(obs.CollectedAt); age > fsmv2cpu.MaxObservationAge {
				return false, "", fmt.Errorf("the CPU reading is %s old, not Fresh, while waiting for the re-fire", age)
			}

			done, seen := cpuLatchDegradedAt(obs.Status, 25)
			if !done {
				return false, seen, nil
			}

			sampledAt := obs.Metrics.Worker.Gauges[string(deps.GaugeCPULastSampleUnix)]
			if sampledAt < float64(barEnd.Unix()) {
				return false, "", fmt.Errorf("the verdict fired again on the reading sampled at machine second %.0f, before the re-fire bar ends at %d",
					sampledAt, barEnd.Unix())
			}

			return true, seen, nil
		})
	},
}

// cpuLatchDegradedAt reports whether a reading is degraded by pressure at the
// given percentage.
func cpuLatchDegradedAt(st simple.Status[fsmv2cpu.CPUStatus], percent int) (bool, string) {
	degraded := st.Degraded && st.Result.Verdict.State == cpuhealth.StateDegraded
	done := degraded && strings.Contains(st.Result.Message, fmt.Sprintf("spent %d%% of the last minute", percent))

	return done, cpuStatusSeen(st)
}
