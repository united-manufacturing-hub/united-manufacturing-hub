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
