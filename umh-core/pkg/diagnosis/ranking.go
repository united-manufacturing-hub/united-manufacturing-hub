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

import (
	"math"
	"sort"
)

// clamp01 bounds a ratio to the severity scale. A NaN can reach it when a
// degenerate mark has no headroom and the value sits exactly at the fire mark
// (0/0); it falls back to the lowest severity so Rank stays a total order.
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

// Severity normalises a fired cause against its own marks and scale, so a
// falling mark in cores compares against a rising one in a ratio:
//
//	rising:   clamp01( (value − fire) / (capacity − fire) )
//	falling:  clamp01( (fire − value) / (fire − (−capacity)) )
//
// The falling scale is MINUS the capacity. With a positive one a 4-core box two
// cores past its mark gives 2/−4, which clamps to zero, the lowest severity for
// the worst case, so the dominant cause ranks last and every saturation cause
// ties.
//
// Severity is a snapshot of the value frozen at the fire transition: it does not
// track post-fire deterioration, and Rank compares these frozen severities.
func (f Fired) Severity() float64 {
	m := f.Marks
	if m.Polarity == LowerIsWorse {
		return clamp01((m.Fire.At - f.Value) / (m.Fire.At - (-m.Capacity)))
	}
	return clamp01((f.Value - m.Fire.At) / (m.Capacity - m.Fire.At))
}

// Rank sorts causes in place and returns the same backing slice, so a caller
// that holds another reference to that slice sees the reordered data. Order is
// by tier (lower first), then by severity descending within the tier, then by
// the externally-attributed cause, then by declared table position. Four
// levels, because three are not total (held causes all clamp to severity 0 and
// External is true for one signal, so a two-cause tie would otherwise be
// resolved by the order of appends).
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
