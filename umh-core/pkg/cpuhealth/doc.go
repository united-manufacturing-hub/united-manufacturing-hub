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
// judgement), and Signals (the facts the sentences interpolate).
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
package cpuhealth
