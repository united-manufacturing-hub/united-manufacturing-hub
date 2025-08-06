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

package constants

import "time"

const (
	ExpectedMaxP95ExecutionTimePerEvent     = 1 * time.Millisecond
	DefaultMaxTicksToRemainInTransientState = 600 // If a FSM remains in a transient state for more than this number of ticks, it will be considered stuck and will trigger a remove / force remove
)

const (
	// ─────────────────────────────────────────────────────────────────────────────
	// Rate‑limiting parameters for a single‑threaded control loop
	//
	// After an *expensive* operation (add / update / remove / state‑change) the
	// manager schedules the next time it is allowed to perform the *same* kind of
	// operation.  The cooldown is expressed in control‑loop *ticks* – a tick
	// corresponds to **one pass** through `BaseFSMManager.Reconcile`.
	//
	// The raw cooldown is `TicksBeforeNext*`, but a small amount of *jitter*
	// (see `JitterFraction` below) is added so that, when many instances are
	// created at once, their subsequent updates are spread out instead of all
	// happening on the same future tick.
	//
	// Example (with the defaults below):
	//   • create "A" on tick 100  →  next add allowed earliest at tick 100+15±25%
	//   • update "B" on tick 105  →  next update allowed earliest at tick 105+15±25%
	//   • normal per‑tick reconciliation is **not** affected – only the *rate‑limited*
	//     operations below respect the barrier.
	//
	// If you lengthen these values you reduce peak load but also slow down
	// convergence; if you shorten them the system reacts faster but risks
	// starving later managers in the loop.
	// ─────────────────────────────────────────────────────────────────────────────
	TicksBeforeNextAdd    = 50 // base cooldown (in ticks) after adding an instance
	TicksBeforeNextUpdate = 5  // base cooldown after changing an instance configuration
	TicksBeforeNextRemove = 10 // base cooldown after starting a removal
	TicksBeforeNextState  = 3  // base cooldown after changing desired FSM state

	// JitterFraction defines how much randomness we mix into the cooldown.
	//
	// The effective delay δ is picked *uniformly* from
	//     [ base * (1‑j) , base * (1+j) ]
	// where j == JitterFraction.
	//
	//   j = 0    →  no jitter, always exactly `base` ticks
	//   j = 0.25 →  ±25 % spread (default): base * 0.75  …  base * 1.25
	//   j = 1    →  full 0 … 2*base range (rarely useful)
	//
	// A small jitter is enough to avoid "thundering herd" effects while keeping
	// the expected delay equal to `base`.  Make sure `math/rand.Seed` is called
	// once at program start so runs do not share the same pseudo‑random sequence.
	JitterFraction = 0.25
)
