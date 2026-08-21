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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// cpuScenarioBase is where the fake machine serves its cgroup files and where
// the sampler looks for them, matching the production sampler's
// "/sys/fs/cgroup". One constant so the two cannot drift apart and leave every
// read failing.
const cpuScenarioBase = "/sys/fs/cgroup"

// cpuCaseHeader opens every rendered block. It is the reader's eye-catch and
// the spec's handle on where one situation ends and the next begins, so a
// block that stops being rendered stops being counted.
const cpuCaseHeader = "=== situation: "

// cpuScenarioPreamble introduces the blocks and glosses the three words a
// reader cannot guess: capable, measured and warm-up. It says nothing about
// how many situations follow, because the reader can see them and a stated
// number would go stale the moment one is added.
const cpuScenarioPreamble = `CPU health: one block per named machine situation.

Each block is one situation from pkg/fsmv2/cpu/cases.go, driven through the
real sampler and the real engine over a fake machine, in the order a reader
should meet them. "why" is what the situation exists to show.

"capable" counts the CPU signals this machine can answer at all, and
"measured" how many of those have ever produced a reading. Measured is
sticky: a signal that measured once stays measured, so on a single read it
means "has ever measured", not "measured just now".

"warm-up" is this worker's own refusal of new work while it has not measured
anything yet. It is NOT the gate that admits bridges: a degraded verdict
blocks admission whatever the warm-up line says, so a block reading
"degraded" and "not refusing" together is not the product accepting work.

"message" is the text as the customer reads it.
`

// renderCPUHealthCases drives every entry of fsmv2cpu.Cases through the real
// sampler and the real engine over a fake machine, and renders what the worker
// answered. It returns the whole rendering rather than printing it, so a spec
// can assert on the text the command line shows.
//
// The answer printed is the one the worker gave on the read, never the one the
// case states. A case states its expected answer for the spec beside it to
// assert; a renderer that printed that instead would print the same page
// whatever the code did.
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario=cpuhealth
func renderCPUHealthCases(ctx context.Context) string {
	var out strings.Builder

	out.WriteString(cpuScenarioPreamble)

	for _, c := range fsmv2cpu.Cases {
		out.WriteString("\n" + cpuCaseHeader + c.Name + " ===\n")
		out.WriteString("why        " + c.Why + "\n")
		out.WriteString("machine    " + describeBox(c.Box) + "\n")
		out.WriteString("reads      " + describeReads(c.Ticks) + "\n\n")

		status, err := pollCase(ctx, c)
		if err != nil {
			// A failed read produced no verdict, no message and no counts, so
			// the error is the whole answer. Saying so keeps a reader from
			// reading the absent fields as an omission.
			out.WriteString("read failed, so there is no verdict, no message and no counts\n")
			out.WriteString(err.Error() + "\n")

			continue
		}

		out.WriteString("verdict    " + status.Verdict + "\n")
		out.WriteString(fmt.Sprintf("signals    %d capable, %d measured\n", status.SignalsCapable, status.SignalsMeasured))
		out.WriteString("warm-up    " + describeWarmUp(status.RefusingAdmission) + "\n\n")

		// The message goes out with no label in front of it and no indent, on
		// its own lines. It is the one thing here a customer reads, so what is
		// on the screen is the bytes the customer gets, newline included.
		out.WriteString("message    (as the customer reads it)\n")
		out.WriteString(status.Message + "\n")
	}

	return out.String()
}

// pollCase drives one case the way the spec beside Cases drives it: a fresh
// machine and fresh dependencies, one read, then one more read after each
// tick. Fresh matters — the engine holds each signal's 60-second window and
// its latch, so a shared one would let an earlier case's history decide this
// answer.
func pollCase(ctx context.Context, c fsmv2cpu.Case) (fsmv2cpu.CPUStatus, error) {
	box := fakebox.NewBox(cpuScenarioBase, c.Box)
	identity := deps.Identity{ID: "cpu-cases", WorkerType: fsmv2cpu.WorkerType}
	d := fsmv2cpu.NewDepsWithSampler(
		identity,
		deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity),
		cpuhealth.NewLinuxSamplerWithClock(box.FS(), cpuScenarioBase, box.Clock()),
	)

	status, err := fsmv2cpu.Poll(ctx, d, fsmv2cpu.CPUConfig{})

	for i := 0; i < c.Ticks; i++ {
		box.Tick(time.Second)
		status, err = fsmv2cpu.Poll(ctx, d, fsmv2cpu.CPUConfig{})
	}

	return status, err
}

// describeBox says what the machine is doing, in the operator units the
// condition is written in. It prints only what is set: a bare-metal machine
// says nothing about a hypervisor, and an unlimited cgroup says nothing about
// a limit. The core count, the two load figures and a pressure clause print
// on every machine, set or not, so the line is not a list of what the
// situation turns on.
//
// Nothing enforces that a field added to fakebox.Condition reaches this
// function. A new field is simply absent from the page and every test still
// passes, so adding one means editing here by hand.
func describeBox(c fakebox.Condition) string {
	parts := []string{fmt.Sprintf("%d cores", c.Cores)}

	if c.QuotaCores > 0 {
		parts = append(parts, "limit "+coresText(c.QuotaCores))
	}

	parts = append(parts,
		fmt.Sprintf("host %s busy", percentText(c.HostBusy)),
		"this instance "+coresText(c.UsageCores),
	)

	if c.PsiPresent {
		parts = append(parts, "pressure "+percentText(c.Pressure))
	} else {
		parts = append(parts, "no pressure stats")
	}

	if c.Throttle > 0 {
		parts = append(parts, "throttled in "+percentText(c.Throttle)+" of periods")
	}

	if c.Steal > 0 {
		parts = append(parts, "steal "+percentText(c.Steal))
	}

	if c.Virtualized {
		parts = append(parts, "on a hypervisor")
	}

	if c.Affinity > 0 {
		parts = append(parts, fmt.Sprintf("pinned to %d CPUs", c.Affinity))
	}

	if len(c.Unreadable) > 0 {
		parts = append(parts, "cannot read "+strings.Join(c.Unreadable, " or "))
	}

	return strings.Join(parts, ", ")
}

// describeReads says how many reads ran and which one the block reports.
// It says reads rather than judgements: a machine whose sample cannot be read
// is read just as often and judged not at all, so a line about judging would
// contradict the read failure printed under it.
//
// Ticks is how many one-second ticks pass before the reported read, and one
// read runs before the first tick, so the count is always one more than the
// ticks.
func describeReads(ticks int) string {
	if ticks == 0 {
		return "one read, and the answer below is that read"
	}

	return fmt.Sprintf("%d reads a second apart, and the answer below is the last", ticks+1)
}

// describeWarmUp spells out RefusingAdmission, naming it for the warm-up it
// reports. An earlier wording said "new work accepted", which read as the
// product's answer on admission and was backwards on the eight degraded
// situations: the bridge-admission gate refuses on a degraded verdict, and
// this bit is only the worker's own refusal while nothing has measured yet.
// The wording here has to keep those two apart on its own, because the line
// is read next to the verdict.
func describeWarmUp(refusing bool) string {
	if refusing {
		return "refusing new work until a capable signal first measures"
	}

	return "not refusing: this worker's own start-up adds no block"
}

// coresText writes a core count the way an operator says it, with the plural
// that reads right at exactly one.
func coresText(v float64) string {
	if v == 1 {
		return "1 core"
	}

	return fmt.Sprintf("%g cores", v)
}

// percentText writes a 0-to-1 fraction as whole percent, the unit the
// customer-visible messages use for the same figures.
func percentText(v float64) string {
	return fmt.Sprintf("%.0f%%", v*100)
}

// CPUHealthScenarioEntry registers the cpuhealth scenario for CLI access.
//
// It uses a CustomRunner with YAMLConfig "" — a YAML-spawned worker would go
// through the production NewDeps and get a real filesystem, which is exactly
// what a fake-machine scenario cannot use.
//
// # CLI Usage
//
//	go run pkg/fsmv2/cmd/runner/main.go --scenario cpuhealth
//
// What it drives: the fsmv2 CPU monitor worker's Poll over every named machine
// situation in pkg/fsmv2/cpu/cases.go, printing what the worker answered about
// each one.
var CPUHealthScenarioEntry = Scenario{
	Name:        "cpuhealth",
	Description: "Prints what the CPU monitor worker answers about every named machine situation",
	YAMLConfig:  "", // worker built directly with a fake machine's sampler
	CustomRunner: func(ctx context.Context, _ RunConfig) (*RunResult, error) {
		out := renderCPUHealthCases(ctx)
		fmt.Print(out)

		done := make(chan struct{})
		close(done)

		// ShutdownClean is true: this scenario drives Poll directly over a fake
		// machine and has no supervisor, so there is nothing that could drain
		// uncleanly. The CLI exits 0 when it is true.
		return &RunResult{Output: out, Done: done, ShutdownClean: true}, nil
	},
}
