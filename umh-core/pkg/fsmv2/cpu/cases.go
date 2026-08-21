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
// Two callers read it. The spec beside it drives every entry through the real
// sampler and the real engine. The cpuhealth scenario in pkg/fsmv2/examples
// renders every entry for a human at the command line, which is why this is
// ordinary code rather than a _test.go file. That choice has a cost worth
// stating: cases.go and that renderer are the only non-test importers of
// fakebox, so the fixture sits in the shipped binary's dependency graph.

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
	// ComposeMessage goes to composeHealthy unless the verdict is degraded AND
	// names at least one cause. buildVerdict sets degraded exactly when there
	// is a cause, so through Decide the cause half of that test never decides
	// anything the state half has not already decided. Either way a degraded
	// message is the headline, the separator and the cause paragraphs, and
	// nothing else — whatever the cause kind. The limited-visibility note, the
	// pinned-CPUs sentence and every budget line live inside composeHealthy and
	// no degraded case can pick them up.
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

	// PollError is the text the read's error must contain, on a machine whose
	// sample cannot be read at all. It exists because cpu.stat is the primary
	// read: when it fails the whole sample fails, so Poll returns a zero
	// CPUStatus and a non-nil error and there is no verdict, no message and no
	// counts to state. Without this field the situation cannot be written down
	// here, because every other field would have to state the zero value as if
	// it were an answer.
	//
	// Empty on every case that can be read, and the five answer fields above —
	// Verdict, Message, the two counts and RefusingAdmission — are then the
	// whole answer. Non-empty replaces those five: the spec asserts the error
	// and asserts nothing else, because a failed read produced nothing else.
	// Ticks is not one of the five and still applies, so a case can state both
	// an error and how many reads have to produce it.
	PollError string
}

// Cases are the situations, in the order a reader should meet them.
var Cases = []Case{
	{
		Name: "quiet-box",
		Why: "A four-core machine at a fifth of its capacity: nothing fires, and " +
			"the healthy message prints the budget it measured and only that.",
		Box: fakebox.Condition{Cores: 4, HostBusy: 0.20, UsageCores: 0.5, Pressure: 0.02, PsiPresent: true},
		// Two, for the reason pressure-at-sixty settles at two: the healthy
		// headline is withheld until the host-busy mean has its two rates, and
		// before that the machine reports that it is starting up. starting-up
		// below is that same read, judged early on purpose.
		Ticks:   2,
		Verdict: "healthy",
		Message: "CPU healthy. The machine is using 0.8 of 4 cores and can use 2.2 more before it is marked degraded.\nTechnical Details: Headroom 2.2 cores = 4 total - 0.8 used - 1.0 reserved (degraded below 0). Pressure 2% (degraded above 20%).",
		// Pressure and host-cpu-full, as on pressure-at-sixty and for the same
		// two reasons: no CPU limit leaves throttling's instrument unanswerable
		// and keeps container-limit-full out of the table, and bare metal leaves
		// steal's unanswerable.
		SignalsCapable:    2,
		SignalsMeasured:   2,
		RefusingAdmission: false,
	},
	{
		Name: "starting-up",
		Why: "quiet-box judged on its first read. A worker with no rate yet says " +
			"so in one line and refuses admission, rather than reporting a " +
			"budget it has not measured.",
		Box: fakebox.Condition{Cores: 4, HostBusy: 0.20, UsageCores: 0.5, Pressure: 0.02, PsiPresent: true},
		// Pinned at zero, and the one case where that is the point rather than a
		// cost. Letting it settle would turn it into a second copy of quiet-box
		// and delete the only statement here about an unsettled worker.
		Ticks:   0,
		Verdict: "healthy",
		Message: "CPU: starting up.",
		// The same two capable signals as quiet-box — capability is a fact about
		// the machine and does not move with the worker's age. Only pressure has
		// measured: PSI publishes its own 60-second average, so the signal takes
		// the last one and answers on the first read, while host-cpu-full is
		// still waiting for its second rate. That gap is exactly what the
		// refusal reports.
		SignalsCapable:    2,
		SignalsMeasured:   1,
		RefusingAdmission: true,
	},
	{
		Name: "host-full-not-us",
		Why: "The machine is full and a fifth of its busy time is ours. The share " +
			"refinement blames the host, and the paragraph sends the operator " +
			"to the other software on the machine. host-full-because-us is the " +
			"same machine with the load the other way round.",
		// The CPU limit is what makes this pair say anything. It is generous —
		// 3 cores against 0.64 used — so the container is nowhere near it and
		// container-limit-full stays quiet. What the limit buys is the branch:
		// the machine-full paragraph only reads the blame when a limit applies,
		// so without one both halves of this pair print the same sentence and
		// the refinement's answer never reaches the customer.
		Box: fakebox.Condition{Cores: 4, QuotaCores: 3, HostBusy: 0.80, UsageCores: 0.64, Pressure: 0.05, PsiPresent: true},
		// Host headroom is the mean of a rate, so the earliest it can fire is
		// its second rate, and the share refinement that blames the host is a
		// mean of a rate too. Ticks 0 and 1 report a machine starting up.
		Ticks:   2,
		Verdict: "degraded",
		Message: "CPU running near full\nTechnical Details: The machine is full. Add CPU to the machine, or reduce other software running on it.",
		// Four. A positive quota does two things at once: it makes throttling's
		// instrument answerable, and it is the condition on which cpuTable
		// appends container-limit-full to the table at all. Steal is still
		// unanswerable on bare metal, and the two share refinements are not
		// counted — the walk is over top-level signals.
		SignalsCapable:    4,
		SignalsMeasured:   4,
		RefusingAdmission: false,
	},
	{
		Name: "host-full-because-us",
		Why: "host-full-not-us with four fifths of the busy time ours. The blame " +
			"moves to this instance and the remedy moves with it: the operator " +
			"is sent to this instance's own load instead of to other software " +
			"that is not the problem. The pair is the whole point — one machine " +
			"state, two remedies, chosen by who filled it.",
		// Same 3-core limit as host-full-not-us, and still not reached: 2.56
		// used leaves 0.44 of the limit spare, and the signal fires only once
		// the spare falls under its 0.3-core reserve. Only the machine is full
		// here, so this stays the one-cause case and the blended both-at-limit
		// paragraph belongs to machine-and-limit-full.
		Box: fakebox.Condition{Cores: 4, QuotaCores: 3, HostBusy: 0.80, UsageCores: 2.56, Pressure: 0.05, PsiPresent: true},
		// Two, as host-full-not-us: same signal, same mean, same second rate.
		Ticks:   2,
		Verdict: "degraded",
		Message: "CPU running near full\nTechnical Details: The machine is full, and this instance is using most of it. Reduce the load on this instance, or add CPU to the machine.",
		// Four, as host-full-not-us and for the same reason.
		SignalsCapable:    4,
		SignalsMeasured:   4,
		RefusingAdmission: false,
	},
	{
		Name: "plain-host-no-psi",
		Why: "A full machine with no CPU limit and a kernel publishing no pressure " +
			"stats. That pair is what earns the trailing sentence asking for " +
			"psi=1, which no other case here prints.",
		Box: fakebox.Condition{Cores: 4, HostBusy: 0.80, UsageCores: 1.0, QuotaCores: 0, PsiPresent: false, Virtualized: true},
		// Two. Both capable signals are means of a rate here — there is no PSI
		// to answer at tick 0 — so nothing at all has measured before tick 2.
		Ticks:   2,
		Verdict: "degraded",
		Message: "CPU running near full\nTechnical Details: CPU averaged 80% of the machine over the last minute and this instance has little headroom left. Add CPU capacity, or reduce the load on it. Pressure stats are unavailable; enable Linux pressure stats (boot with psi=1) for richer detail.",
		// Steal and host-cpu-full. The hypervisor makes steal answerable, where
		// the bare-metal cases above cannot answer it; no PSI makes pressure
		// unanswerable, where they can. Steal is measured by tick 2 on its mean
		// arm: the percentile arm needs twenty samples and has two, and the two
		// arms are one signal.
		SignalsCapable:    2,
		SignalsMeasured:   2,
		RefusingAdmission: false,
	},
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
	{
		Name: "at-the-baseline",
		Why: "Pressure one point under its 20% mark, on an eight-core machine with " +
			"headroom to spare. The verdict is healthy and the budget line still " +
			"prints the number that did not fire, so a reader can see how close " +
			"it came.",
		Box: fakebox.Condition{Cores: 8, HostBusy: 0.60, UsageCores: 4.5, Pressure: 0.19, PsiPresent: true},
		// Two, as quiet-box: the healthy headline waits for the host-busy mean's
		// second rate.
		Ticks:   2,
		Verdict: "healthy",
		Message: "CPU healthy. The machine is using 4.8 of 8 cores and can use 2.2 more before it is marked degraded.\nTechnical Details: Headroom 2.2 cores = 8 total - 4.8 used - 1.0 reserved (degraded below 0). Pressure 19% (degraded above 20%).",
		// Pressure and host-cpu-full, as on quiet-box.
		SignalsCapable:    2,
		SignalsMeasured:   2,
		RefusingAdmission: false,
	},
	{
		Name: "throttled",
		Why: "A container the kernel is pausing at its own CPU limit, and which is " +
			"also out of budget. Throttling is starvation and outranks the " +
			"saturation beside it, so it takes the headline; both paragraphs are " +
			"printed, dominant first.",
		Box: fakebox.Condition{Cores: 4, QuotaCores: 2, HostBusy: 0.55, UsageCores: 1.9, Throttle: 0.12, PsiPresent: true},
		// Two, and the two signals get there by different routes. Throttling is a
		// ratio of two counter deltas and has one by tick 1; the limit's headroom
		// is a mean of a rate and needs its second. Judged at tick 1 this machine
		// would print the throttling paragraph alone.
		Ticks:   2,
		Verdict: "degraded",
		Message: "CPU limited\nTechnical Details: This instance hit its CPU limit and was paused until the next cycle, in 12% of CPU scheduling periods over the last minute. Work is being delayed. Raise this instance's CPU limit, or reduce the load on it.\n\nCPU averaged 95% of its limit over the last minute and this instance has little headroom left. Raise its CPU limit, or reduce the load on it.",
		// Four, for the reason host-full-not-us states: the quota makes
		// throttling answerable and puts container-limit-full in the table.
		SignalsCapable:    4,
		SignalsMeasured:   4,
		RefusingAdmission: false,
	},
	{
		Name: "limit-full",
		Why: "throttled's container out of its budget without being paused for it. " +
			"The limit-saturation paragraph then speaks alone, under the " +
			"saturation headline rather than the throttling one.",
		Box: fakebox.Condition{Cores: 4, QuotaCores: 2, HostBusy: 0.50, UsageCores: 1.85, Throttle: 0, PsiPresent: true},
		// Two: the limit's headroom is a mean of a rate, so its second rate is
		// the earliest it can fire.
		Ticks:   2,
		Verdict: "degraded",
		Message: "CPU running near full\nTechnical Details: CPU averaged 93% of its limit over the last minute and this instance has little headroom left. Raise its CPU limit, or reduce the load on it.",
		// Four, as throttled: the quota is what puts them there, and a throttle
		// of zero is a signal that answered, not one that cannot.
		SignalsCapable:    4,
		SignalsMeasured:   4,
		RefusingAdmission: false,
	},
	{
		Name: "machine-and-limit-full",
		Why: "The machine is full AND this instance is out of its own CPU limit. " +
			"The two remedies contradict each other — raising a limit cannot " +
			"help on a machine with nothing left to give — so the message blends " +
			"them into one paragraph instead of handing the operator both.",
		// A tighter limit than the host-full pair: 2 cores against 1.9 used,
		// which is inside the 0.2-core reserve, so container-limit-full fires
		// here where it stays quiet there. The machine is fuller too, at 3.6 of
		// its 4 cores busy.
		Box: fakebox.Condition{Cores: 4, QuotaCores: 2, HostBusy: 0.90, UsageCores: 1.9, PsiPresent: true},
		// Two: both capacity signals are means of a rate, and both reach their
		// second rate together.
		Ticks:   2,
		Verdict: "degraded",
		// One paragraph, not two, and it names the limit rather than acting on
		// it. Both capacity causes fired, and speaksForCapacity lets only one of
		// them speak: /proc/stat was readable, so the machine's own reading
		// speaks and the limit rides along in the trailing clause.
		Message: "CPU running near full\nTechnical Details: The machine is full and this instance's CPU limit cannot help. Add CPU to the machine, or reduce other software running on it. (This instance is also at its 2-core limit.)",
		// Four, as on every case with a quota.
		SignalsCapable:    4,
		SignalsMeasured:   4,
		RefusingAdmission: false,
	},
	{
		Name: "noisy-neighbour",
		Why: "A guest whose hypervisor is taking 15% of the CPU it asked for. " +
			"Steal is the only cause here whose remedy is on someone else's " +
			"platform, and the message says so.",
		Box: fakebox.Condition{Cores: 4, HostBusy: 0.50, UsageCores: 1.0, Steal: 0.15, Virtualized: true, PsiPresent: true},
		// Two, and steal reaches it on its mean arm rather than on the
		// percentile arm listed ahead of it. The percentile instrument needs
		// twenty samples; the mean needs two and shares the percentile's mark,
		// so the mean fires at tick 2 and the episode reports the mean for as
		// long as it holds.
		Ticks:   2,
		Verdict: "degraded",
		Message: "CPU taken by the server\nTechnical Details: Other virtual machines on the same physical server took 15% of the CPU this instance needed over the last minute. This is outside UMH's control. On your virtualization platform, give this VM more guaranteed CPU, or reduce the other VMs sharing the server.",
		// Three: the hypervisor adds steal to quiet-box's pair. Still no quota,
		// so throttling stays unanswerable and container-limit-full stays out of
		// the table.
		SignalsCapable:    3,
		SignalsMeasured:   3,
		RefusingAdmission: false,
	},
	{
		Name: "cannot-measure",
		Why: "cpu.stat is the primary read: losing it fails the whole sample, and " +
			"the worker reports that it could not measure rather than a healthy " +
			"zero. It is the only case whose answer is an error.",
		Box: fakebox.Condition{Cores: 4, HostBusy: 0.50, UsageCores: 1.0, Unreadable: []string{"/sys/fs/cgroup/cpu.stat"}},
		// Two, and no tick changes the answer — which is why they are here.
		// Every read fails the same way, and a single read cannot show that:
		// three failing reads run, each asserting the error and the empty
		// status, so the claim is guarded rather than merely written down.
		Ticks: 2,
		// The five answer fields are left unstated: see PollError. The read
		// produced no status, so stating a verdict or a count would be inventing
		// one.
		PollError: "read /sys/fs/cgroup/cpu.stat: fakebox: /sys/fs/cgroup/cpu.stat: no such file or directory",
	},
	{
		Name: "no-host-stats",
		Why: "An unreadable /proc/stat, and a KNOWN DEFECT rather than an " +
			"intended answer. Losing that one host file takes the core count " +
			"with it, so every signal drops out of the table and CPU monitoring " +
			"goes dark on a machine with four readable cores — which is why the " +
			"answer below reports four cores, no signals at all and a healthy " +
			"verdict at the same time. The customer text is wrong twice over: it " +
			"blames the cgroup, which read fine, and it says healthy about a " +
			"machine it cannot see. That wording lives in " +
			"pkg/cpuhealth/message.go and is out of scope here. Do not take this " +
			"answer as what UMH should show when /proc/stat cannot be read; the " +
			"case pins what the code does today, so a fix shows up as a diff on " +
			"this entry.",
		Box: fakebox.Condition{Cores: 4, HostBusy: 0.50, UsageCores: 1.0, QuotaCores: 0, PsiPresent: false, Unreadable: []string{"/proc/stat"}},
		// Zero. Nothing here is derived from a rate or a window, so the first
		// read is already the settled one.
		Ticks:   0,
		Verdict: "healthy",
		// The capacity this message reports as unreadable is a SAMPLER DEFECT,
		// not this machine. The stated machine has four CPUs and a readable
		// cpuset. read.go nests the cpuset read inside the /proc/stat success
		// branch, so losing /proc/stat also loses LogicalCpus, the capacity
		// collapses to zero and the healthy message takes the zero-capacity
		// early return.
		//
		// The cost is larger than one wrong figure: with no core count the
		// table drops host-cpu-full too, so CPU monitoring on this machine goes
		// entirely dark rather than merely losing a capacity number. It is a
		// known defect in read.go and deliberately out of scope here. The case
		// pins what the code does today, so a fix shows up as a change to this
		// line and to the count below.
		Message: "CPU monitoring unavailable: cgroup read failed. Defaulting to healthy.",
		// Zero, and the zero is load-bearing. The lost core count keeps
		// host-cpu-full out of the table entirely, and no quota, no PSI and no
		// hypervisor leave the other three with no instrument this machine can
		// answer. A worker with nothing capable has nothing to wait for, so it
		// does not refuse admission.
		SignalsCapable:    0,
		SignalsMeasured:   0,
		RefusingAdmission: false,
	},
}
