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

package diagnosis

import "time"

// Polarity records which direction is worse. Both saturation instruments fire
// on a quantity where LOWER is worse, and two marks are in absolute cores
// rather than a fraction, so a severity normaliser needs unit and polarity.
type Polarity int

const (
	HigherIsWorse Polarity = iota
	LowerIsWorse
)

// Mark is a threshold together with its inclusivity. Inclusivity is part of the
// mark: decide.go:875 is the only inclusive fire arm among seventeen, and a
// generic comparison silently moves that boundary at exactly 0.70.
type Mark struct {
	At        float64
	Inclusive bool
}

// Marks is a two-mark Schmitt pair. Clear must be strictly less severe than
// Fire, judged under Polarity — NewEngine refuses a pair that is not.
type Marks struct {
	Fire     Mark
	Clear    Mark
	Polarity Polarity
	Unit     string
	// Capacity normalises severity across instruments. It is the value at which
	// severity reaches 1, stated positively here; Severity signs it by polarity,
	// or the worst case clamps to zero.
	Capacity float64
}

// Identity is what a signal is called and where it ranks — the facts S1 R6 needs
// and a latch cannot learn from a reduction. The engine stamps it at
// construction from the table, so a Fired carries a tier, an external flag and a
// table position without the latch ever consulting the table.
type Identity struct {
	// Signal names the question.
	Signal string
	// Tier is the rank class; lower ranks first, and every cause in a lower tier
	// outranks every cause in a higher one regardless of severity. The numbers
	// are the caller's vocabulary, as Marks.Unit is — this package holds
	// machinery, not words.
	Tier int
	// External marks a cause attributed outside this box. It is R6's third
	// tie-break.
	External bool
	// Index is the signal's position in the table, and R6's last tie-break. It
	// is declared rather than incidental, because Causes[0] picks the customer's
	// refusal string and sort.SliceStable would otherwise decide it.
	Index int
}

// Fired is what a fired latch contributes to a verdict. It carries enough for
// severity ranking without exposing the latch itself.
type Fired struct {
	Identity
	Value float64
	Marks Marks
	// Since is stamped on the transition, never on every tick a latch stays
	// fired. It is in-process only and must not become a Cause field.
	Since time.Time
}

// Latch is a two-mark Schmitt latch, keyed per SIGNAL, not per instrument. A
// per-instrument latch is by construction never fired on the tick its instrument
// is selected, which makes F7 unimplementable.
type Latch struct{}

// NewLatch builds an unfired latch for one signal.
func NewLatch(id Identity) *Latch { return &Latch{} }

// Update judges a trustworthy reduction against the marks. It stamps the since
// time only on a transition, never on every tick it stays fired.
//
// Coverage is the window's extent and it is what the clear arm and the re-fire
// arm are gated on. ⚠️ There is no readability parameter here and there must
// never be one: that absence is how S1 R5 spec 6 closes F1's reintroduction site
// by signature, which outranks any generated test.
func (l *Latch) Update(r Reduced, c Coverage, m Marks, now time.Time) {}

// Reset releases the latch immediately, whatever its coverage. Used when every
// capable instrument's window is absent and the signal declares
// release-on-absent.
//
// ⚠️ The coverage gate on the CLEAR arm does not apply here. An emptied window
// never reports full coverage, so a Reset routed through the clear arm can
// never release — and every AllAbsent release in the design stops working,
// silently. That is S1 R5 spec 4.
//
// The latch has no ReleaseOnAbsent field and no Signal: whether to call this is
// Observe's decision, and S1 R7b spec 6 is where the condition is asserted,
// against two signals that declare it differently.
func (l *Latch) Reset() {}

// ReleaseAfter releases the latch once span has elapsed with nothing able to
// answer the question. This is the time bound that stops a held latch outliving
// its evidence forever.
//
// The clock runs from the last Update — the most recent tick this latch was
// handed a trustworthy value — or from the tick it fired if there has been
// none since. ⚠️ NOT from the first ReleaseAfter call: a signal alternating
// between this arm and any other would restart the clock on every alternation
// and never release.
func (l *Latch) ReleaseAfter(span time.Duration, now time.Time) {}

// Fired reports whether the latch is fired and what it contributes.
func (l *Latch) Fired() (Fired, bool) { return Fired{}, false }
