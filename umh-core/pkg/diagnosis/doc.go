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

// Package diagnosis turns a stream of snapshots into a ranked list of causes.
//
// A caller declares a table saying what to measure and where the thresholds
// sit, ticks the engine with its own snapshot type, and gets back whatever
// crossed a threshold, worst first. The package holds machinery and no
// vocabulary of its own: it never learns what a CPU or a queue is.
//
// # One tick
//
// Each stage narrows what is known. The name on the left performs the step; it
// is not a return type.
//
//	snapshot S
//	  Instrument.Extract  reads     Reading       a float64, or an absence
//	  Window.Observe      stores    Point         into a sliding window
//	  Window.Reduce       reduces   Reduced       a number, and whether to trust it
//	  the engine          resolves  Availability  what one signal can say now
//	  Latch.Update        judges    Fired         a signal that crossed its mark
//	  Rank                orders    []Fired       worst first
//
// # Reading order
//
// The files build in one direction. Each assumes the ones above it and nothing
// below, so reading them in this order never needs a term that has not been
// introduced yet.
//
//	measure.go   a reading, and the calculations that summarise a series of them
//	window.go    where readings accumulate and age out
//	declare.go   what a caller writes: capabilities, instruments, signals, the table
//	judge.go     thresholds, the latch that fires on them, and how causes are ordered
//	engine.go    builds all of it from the table, and runs one tick
//	suite.go     generates a conformance suite for a caller's table
//
// # Using it
//
//	engine, err := diagnosis.NewEngine(table)
//	env := diagnosis.NewEnvironment(caps...)
//
//	// once per interval:
//	fired, readiness := engine.Observe(snapshot, env, now)
//	causes := diagnosis.Rank(fired)
//
// Observe returns two things: what fired, and one readiness row per signal, so
// a caller can tell "measured, and fine" from "could not measure at all".
package diagnosis
