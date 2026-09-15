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
//	message   ComposeMessage(verdict, details) renders the customer-facing text;
//	          BlockReason renders the line refusing a new bridge: a data
//	          pipeline the customer adds to this instance, and one more
//	          consumer of the CPU judged above.
//
// # What it measures
//
// Five questions, asked each tick. The verdict is degraded when any of them
// fires. A question fires when its number crosses the threshold declared
// beside it, and stays fired until the number comes back past a second
// threshold on the healthier side; pkg/diagnosis holds that state and drives
// the crossings. Each question's thresholds live in its own signal_*.go file;
// this doc names none of them.
//
//	steal                 Something outside this box is taking CPU we were
//	                      scheduled to get. Asked only on a virtualized host.
//	pressure              Our own tasks were ready to run and did not get a
//	                      CPU. Asked only where the kernel reports pressure
//	                      statistics.
//	throttling            The kernel is cutting us off at our own CPU limit.
//	                      Asked only where a CPU limit is set.
//	host-cpu-full         There is not enough CPU left on the machine.
//	                      Declared only where the count of CPUs this container
//	                      may use was readable.
//	container-limit-full  Our own usage has come close to our own limit.
//	                      Declared only where a positive quota exists.
//
// "Asked only" and "Declared only" are different restrictions. Asked only
// means the row is in the table on every box and its instrument is gated on a
// capability, so where the box lacks that capability the question is asked and
// nothing answers it. Declared only means the table leaves the row out
// altogether, so on such a box there is no question. "Answers only", in the
// pair below, gates nothing: it picks which of one question's two instruments
// supplies the number.
//
// The host-cpu-full signal is measured two ways, and the first that can
// answer does:
//
//	host-headroom   How many cores are free on the machine, less a reserve:
//	                capacity held back for the other software on the box, so
//	                host-cpu-full fires while some cores are still free.
//	                Answers only where the sample covers the whole machine.
//	usage-fraction  How much of the CPUs we may run on we are using.
//	                Answers only where there is no CPU limit and no
//	                pressure statistics, so nothing better can answer.
//
// Only the first of those measures the machine. The second measures us, and
// stands in for the machine. It answers where there is no limit to overrun and
// no pressure statistics to read the harm off. On such a box, a container using
// most of the CPUs it may run on is the last evidence left that the machine is
// filling up. It is a stand-in and not the same quantity, which is why it
// answers only where the instrument that does measure the machine cannot.
//
// # Who is to blame
//
// A degraded verdict carries an Attribution. It is declared in the table
// beside the signal that ranked first, or beside the refinement narrowing it,
// and nothing after the verdict recomputes it. Ranking puts starvation —
// something taking CPU away from us — ahead of saturation, the CPU merely being
// used up. Steal, pressure and throttling are starvation; host-cpu-full and
// container-limit-full are saturation. Throttling belongs to starvation even
// though the limit it enforces is our own, because the kernel actively
// withholds the CPU rather than the CPU running out. Within a class, ranking
// orders by how far past its fire threshold a signal went, on one common
// scale, because the signals are measured in different units; a tie goes to
// whichever the table declares first.
//
// Steal is the host by definition: a hypervisor took the CPU. Throttling is
// the container by definition: the limit is ours. Pressure is unknown — tasks
// can wait because the machine is busy, or because we asked for more CPU than
// we may use, and the pressure number alone does not separate those.
// Container-limit-full is the container, because it is our own limit.
//
// The host-cpu-full signal says nothing by itself about whose load filled the
// machine, so two refinements narrow it. The host-share refinement fires when
// the rest of the box accounts for most of the busy time. The container-share
// refinement fires when we account for most of it. Where neither fires,
// nothing narrows the machine to a side and the blame is unknown. The header
// of signal_saturation.go says when that happens.
//
// A refinement is an ordinary signal declared under a parent signal, with its
// own instruments and thresholds, so any signal may carry them. host-cpu-full
// is the only one that does; pressure declares none, and the ambiguity above is
// left standing rather than narrowed. A refinement is judged every tick whether
// or not its parent has fired, but it is reported only while the parent is
// fired, so a refinement can never degrade the verdict on its own.
//
// The advice moves with the blame: in the refusal line, and in the paragraph
// only where a limit is in force. A machine filled from outside is answered
// with "reduce other software running on it"; a machine this instance filled
// is answered with the load the reader controls, and an unattributed one names
// nobody. Two cases opt out of that. Where this container is at its own limit
// too, one blended paragraph and the line beside it both carry the machine's
// remedy whatever the blame says. Where no limit is in force, the paragraph
// names no side and only the refusal line still does.
//
// # Sample and Reading
//
// A Reading is one optional number: a value or an absence, no third state. A
// Sample is one tick's worth of them for one cgroup, plus the facts that are not
// numbers — the timestamp, whether the host is virtualized, the CPU scope, and
// whether the kernel has ever reported pressure stats. So: a Sample is the whole
// tick, a Reading is one field of it. The distinction carries the package's main
// rule — a source that could not be read yields an absent Reading, never a
// measured zero.
//
// Why Sample and Details are flat structs: see the note above each declaration.
package cpuhealth
