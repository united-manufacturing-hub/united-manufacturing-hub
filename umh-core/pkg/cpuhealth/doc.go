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
// Most of the types this package's declarations are built from are declared in
// pkg/diagnosis and are not searchable in this directory; run
// "go doc ./pkg/diagnosis" for all of them. What this package declares for
// itself is the CPU vocabulary those are built from: Sample and Scope (the
// readings), Verdict, Cause, CauseKind, State, Attribution and Unit (the
// judgement), and Details (the facts the sentences interpolate).
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
//	          Verdict and a Details. DeriveEnvironment(sample) builds the env.
//	message   ComposeMessage(verdict, signals) renders the customer-facing text;
//	          BlockReason renders the line refusing a new bridge.
//
// # What it measures
//
// Five questions, asked each tick. The verdict is degraded when any of them
// fires. Each question's thresholds are declared beside it in a signal_*.go
// file; this doc names none of them.
//
//	steal                 Something outside this box is taking CPU we were
//	                      scheduled to get. Asked only on a virtualized host.
//	pressure              Our own tasks were ready to run and did not get a
//	                      CPU. Asked only where the kernel reports pressure
//	                      statistics.
//	throttling            The kernel is cutting us off at our own CPU limit.
//	                      Asked only where a CPU limit is set.
//	host-cpu-full         There is not enough CPU left on the machine.
//	                      Declared only where the core count was readable.
//	container-limit-full  Our own usage has come close to our own limit.
//	                      Declared only where a positive quota exists.
//
// The host-cpu-full signal is measured two ways, and the first that can
// answer does:
//
//	host-headroom   How many cores are free on the machine, less a
//	                reserve. Answers only where the sample covers the
//	                whole machine.
//	usage-fraction  How much of the CPUs we may run on we are using.
//	                Answers only where there is no CPU limit and no
//	                pressure statistics, so nothing better can answer.
//
// # Who is to blame
//
// A degraded verdict carries an Attribution. It is declared in the table
// beside the signal that ranked first, or beside the refinement narrowing it,
// and nothing after the verdict recomputes it.
//
// Steal is the host by definition: a hypervisor took the CPU. Throttling is
// the container by definition: the limit is ours. Pressure is unknown — tasks
// can wait because the machine is busy, or because we asked for more CPU than
// we may use, and the pressure number alone does not separate those.
// Container-limit-full is the container, because it is our own limit.
//
// The host-cpu-full signal says nothing by itself about whose load filled the
// machine, so two refinements narrow it: host-share when the rest of the box
// accounts for most of the busy time, container-share when we account for most
// of it. Both read our usage over the machine's busy time, and a share in the
// narrow band around one half fires neither, which leaves the side already
// blamed in place. That number needs the machine's busy time and our own usage
// over the same CPUs, so a box whose /proc/stat is unreadable and a container
// pinned to a subset of the CPUs both come out unknown.
//
// The advice moves with the blame. Only a machine filled from outside is
// answered with "reduce other software running on it"; a machine this instance
// filled is answered with the load the reader controls, and an unattributed one
// names nobody.
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
// Why Sample and Details are flat structs: see the note above each declaration.
package cpuhealth
