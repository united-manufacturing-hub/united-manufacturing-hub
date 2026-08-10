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

// This file scores and orders what the latches fired: Severity puts every fired
// cause on one 0..1 scale whatever its unit and direction, and Rank orders them.

package diagnosis

import (
	"math"
	"sort"
)

// clamp01 bounds a ratio to 0..1, mapping NaN to 0 so Rank stays a total order.
// NaN arrives only from a 0/0: no headroom, read exactly on the fire mark.
func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Severity normalises a fired cause onto a 0..1 scale against its own marks, so
// causes in different units and directions compare. The value is frozen at fire.
//
//	rising:   clamp01( (value − fire) / (capacity − fire) )
//	falling:  clamp01( (fire − value) / (fire + capacity) )
//
// Capacity left at zero leaves the fire mark as the whole denominator, negated
// for a rising pair. Wherever that is negative every cause clamps to 0 and ties
// at the bottom: a rising pair over a positive fire mark, a falling pair over a
// negative one. validate accepts both. A falling cause reaches 1 only at
// value == −capacity, so for a quantity that cannot go negative the worst
// reachable score is fire/(fire+capacity), at value 0.
func (f Fired) Severity() float64 {
	m := f.Marks
	if m.Polarity == LowerIsWorse {
		return clamp01((m.Fire.At - f.Value) / (m.Fire.At - (-m.Capacity)))
	}
	return clamp01((f.Value - m.Fire.At) / (m.Capacity - m.Fire.At))
}

// Rank orders fired causes lexicographically on four keys, sorting in place and
// returning the same backing slice:
//
//	tier ascending, so a lower tier outranks a higher one
//	severity descending within the tier
//	external attribution first
//	table index ascending
func Rank(fired []Fired) []Fired {
	sort.Slice(fired, func(i, j int) bool {
		a, b := fired[i], fired[j]
		if a.Identity.Tier != b.Identity.Tier {
			return a.Identity.Tier < b.Identity.Tier
		}
		if sa, sb := a.Severity(), b.Severity(); sa != sb {
			return sa > sb
		}
		if a.Identity.External != b.Identity.External {
			return a.Identity.External
		}
		return a.Identity.Index < b.Identity.Index
	})
	return fired
}
