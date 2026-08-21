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

// The named machine situations CPU health is judged on: for each one, the
// machine an operator would describe out loud, and the whole answer the worker
// gives about it.
//
// Each entry states its expected answer as literal text and literal counts,
// not as a rule to re-derive. A reader comparing two entries sees the wording
// difference directly, and a change in wording shows up as a diff on the line
// that carries it.
//
// One caller reads it today: the spec beside it drives every entry through the
// real sampler and the real engine. A command-line renderer that prints them
// for a human is the intended second caller and is not written yet, which is
// why this is ordinary code rather than a _test.go file. That choice has a
// cost worth stating: cases.go is the only non-test importer of fakebox, so
// the fixture now sits in the shipped binary's dependency graph.

package fsmv2cpu

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth/fakebox"

// Case is one named machine situation and what CPU health says about it.
type Case struct {
	// Name identifies the case, in words an operator would use for the
	// machine — "pressure-at-sixty", not "case 3".
	Name string

	// Why is the one line saying what this case exists to show. It is what a
	// reader deleting the case would have to argue against.
	Why string

	// Box is the machine, in the operator units fakebox.Condition takes.
	Box fakebox.Condition

	// Ticks is how many one-second Tick calls happen before the read whose
	// answer the case states. Zero judges the first read. Each tick is one
	// poll: the worker polls once, then once more after every tick.
	Ticks int

	// Verdict is the expected cpuhealth.State as a string, "healthy" or
	// "degraded".
	Verdict string

	// Message is the whole customer-visible text expected for that verdict,
	// headline and Technical Details together.
	//
	// ComposeMessage branches once, on the state: anything not degraded goes to
	// composeHealthy. So a degraded message is the headline, the separator and
	// the cause paragraphs, and nothing else — whatever the cause kind. The
	// limited-visibility note, the pinned-CPUs sentence and every budget line
	// live inside composeHealthy and no degraded case can pick them up.
	Message string

	// SignalsCapable is how many CPU signals this machine can answer at all.
	SignalsCapable int

	// SignalsMeasured is how many of those have produced a first measurement
	// by the judged read.
	SignalsMeasured int

	// RefusingAdmission is whether the worker still refuses to admit new work,
	// which it does while a capable signal has not first-measured and the
	// admission window has not run out.
	RefusingAdmission bool
}

// Cases are the situations, in the order a reader should meet them.
var Cases = []Case{
	{
		Name: "pressure-at-sixty",
		Why: "Sixty percent busy with spare cores is still degraded: tasks are " +
			"queueing for a core, so pressure fires while capacity stays quiet.",
		Box: fakebox.Condition{Cores: 4, HostBusy: 0.60, UsageCores: 1.2, Pressure: 0.25, PsiPresent: true},
		// Two ticks, which is the earliest settled read on this machine.
		// Pressure alone would answer at tick 0, because PSI publishes its own
		// 60-second average and the signal takes the last one. Capacity is the
		// mean of a rate, and a rate needs two reads a second apart, so its
		// first two rates land on ticks 1 and 2. Judging earlier than tick 2
		// would report a machine still refusing admission, which says more
		// about the worker's age than about the machine.
		Ticks:   2,
		Verdict: "degraded",
		Message: "CPU contention\nTechnical Details: Tasks in this instance spent 25% of the last minute waiting for a free CPU core. Reduce the load on this instance, or give it more CPU. If other workloads share this server they may be competing for it.",
		// Pressure and host-cpu-full. Two different mechanisms keep the rest
		// out. Throttling and steal ARE in the table, but their instruments
		// require a CPU limit and a hypervisor, so on this machine nothing can
		// answer them. container-limit-full is not in the table at all —
		// cpuTable appends it only for a positive quota. The count walks
		// top-level signals, so the two saturation refinements never reach it.
		SignalsCapable:    2,
		SignalsMeasured:   2,
		RefusingAdmission: false,
	},
}
