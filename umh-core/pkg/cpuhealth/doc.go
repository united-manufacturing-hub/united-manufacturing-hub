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

// Package cpuhealth reads a cgroup's CPU counters, judges them, and renders the
// judgement as text a customer can act on.
//
// The judging machinery is NOT here. This package declares a table — what to
// read, over how long, against which thresholds — and hands snapshots to the
// engine in pkg/diagnosis, which evaluates it. What lives here is the CPU
// vocabulary: the sources, the thresholds, the names of the causes and the
// sentences.
//
// # Where the types come from
//
// Most types this package's declarations are built from are declared in
// pkg/diagnosis and are not searchable in this directory. Run
// "go doc ./pkg/diagnosis" for all of them; these are the ones used here:
//
//	Reading, Known, Unknown       an optional float64: a value, or an absence
//	Table, Signal, Instrument     the declaration a caller writes
//	Track                         a quantity measured every tick but never judged
//	Marks, Mark                   the thresholds a number is judged against
//	HigherIsWorse, LowerIsWorse   which side of a mark is the bad side
//	Mean, Last, P95, DeltaRatio   reductions: how a window folds to one number
//	Capability, Environment       what this box supports; NewEnvironment builds one
//	Engine, NewEngine             owns the windows and one latch per signal
//	Fired, Rank                   what crossed a threshold, and their order
//	StateValue                    the reduced number is trustworthy
//	Ready                         an Availability: this signal is readable now
//	Run, Outcome                  the generated readability suite
//
// What this package declares for itself is the CPU vocabulary those are built
// from: Sample and Scope (the readings), Verdict, Cause, CauseKind, State,
// Attribution and Unit (the judgement), and Signals (the facts the sentences
// interpolate).
//
// # How it fits together
//
// Five stages, each with a named entry point:
//
//	read      NewLinuxSampler(fs, base) returns a Sampler.
//	sample    Sampler.Read yields one Sample: every reading of one tick.
//	table     Table(cores, quota) declares the signals; NewEngine builds the
//	          engine from that same table.
//	verdict   Decide(engine, sample, env) calls Engine.Observe and returns a
//	          Verdict and a Signals. DeriveEnvironment(sample) builds the env.
//	message   ComposeMessage(verdict, signals) renders the customer-facing text;
//	          BlockReason renders the line refusing a new bridge.
//
// Decide is the only caller of Engine.Observe in this package and the only
// producer of Signals.
//
// # Sample and Reading
//
// A Reading is one optional number: a value or an absence, no third state. A
// Sample is one tick's worth of them for one cgroup, plus the facts that are not
// numbers — the timestamp, whether the host is virtualized, the CPU scope, and
// whether the kernel has ever reported pressure stats. So: a Sample is the whole
// tick, a Reading is one field of it. The distinction carries the package's main
// rule — a source that could not be read yields an absent Reading, never a
// confident zero.
//
// Why Sample and Signals are flat structs: see the note above each declaration.
//
// # What the sampler reads
//
// The Linux sampler reads eight paths. Four are under the cgroup base it was
// constructed with:
//
//	<base>/cpu.stat               usage_usec, nr_periods, nr_throttled
//	<base>/cpu.max                the quota
//	<base>/cpu.pressure           the "some" avg60 pressure figure
//	<base>/cpuset.cpus.effective  how many CPUs this process may run on
//
// Four are machine-wide:
//
//	/proc/stat                     host busy and steal jiffies, and the machine's CPU count
//	/proc/cpuinfo                  the x86 virtualization evidence
//	/sys/class/dmi/id/product_name the ARM64 virtualization evidence
//	/sys/class/dmi/id/sys_vendor   the second ARM64 virtualization source
//
// A cpu.stat failure fails the whole sample; every other failure leaves its own
// field absent and the sample usable.
//
// # Track and signal
//
// A signal is a question with thresholds: it is judged, it can fire, and firing
// is what degrades the verdict. A track is a quantity reduced every tick with no
// thresholds at all — measured so it can be published, never judged.
//
// The table declares five signals — throttling, pressure, steal, saturation and
// limit-saturation — and two tracks, host-busy and usage-cores. Two of the
// signals are conditional: saturation is declared only when the core count is
// positive, limit-saturation only when the quota is.
//
// The two tracks exist because attribution needs both 60-second means on every
// box, and no instrument holds either series in the form attribution needs.
//
// # Cause and signal
//
// Not the same thing, and not one-to-one. A signal is what fires; a Cause is
// what the customer is shown, and Decide derives one from the other. Two signals
// (saturation and limit-saturation) both produce CauseKindSaturation, and Decide
// folds them into a single Cause before ranking so the same paragraph is not
// printed twice. CauseKindHostContention is declared but no signal produces it.
//
// # Scope
//
// Scope answers one question: does this process's logical CPU count describe the
// whole machine (ScopeHost) or only the subset it is pinned to (ScopeAffinity)?
// ScopeUnknown means the machine's count could not be read — never assumed to be
// the host.
//
// Two things change when it is not ScopeHost. The host-headroom instrument
// withholds its reading, so saturation falls through to the arm computed from
// our own usage instead. And Signals.HostHeadroomAvailable goes false, which is
// how the message layer tells "we did not measure it" from "we measured it and
// it was fine".
//
// # What makes the verdict degraded
//
// Exactly one rule: the verdict is degraded when at least one signal is fired
// this tick, and healthy when none is. There is no severity floor and no second
// condition. Decide ranks the fired set through diagnosis.Rank — that order is
// Verdict.Causes, and no other code in this package sorts it — then reads the
// attribution off the first cause. Nothing else in this package can set the
// state.
//
// A verdict can still carry an annotation. LimitedVisibility says this box has
// no positive quota and has never reported pressure stats, which is the case
// where the fewest signals can be judged at all; it rides Signals and never
// becomes a state.
//
// # Availability and Readiness
//
// Both are pkg/diagnosis types, and they answer "could we measure this at all?"
// — a different question from "did it fire?".
//
// Availability is what one signal can say this tick, in four ascending values:
// no capable instrument on this box, every window empty, a window that has
// readings but not enough to trust, and Ready. Readiness pairs one signal name
// with its Availability, and Engine.Observe returns one row per signal, always —
// which is the only route to that answer, because the fired set reports only
// what fired.
//
// Decide turns those rows into the three ...SignalReady booleans on Signals, and
// the healthy message's budget lines print a throttle, pressure or steal figure
// only when its own flag is set. The separate ...Applies booleans are a
// different question again: whether the rule applies to this box at all, which
// is not evidence that anything was read.
package cpuhealth
