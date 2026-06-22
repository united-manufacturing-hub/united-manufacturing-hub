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

//go:build test

// BoundDiff has no production caller yet; the test build tag above keeps it
// out of production binaries until the first non-test caller is added.

package protocolconverterserviceconfig

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// EmptyDiffSentinel is the exact string BoundDiff returns when the diff is empty
// but the configs are known to differ — the silent-mismatch class that hid the
// ENG-5090 re-apply cause. Surfaced verbatim in StatusReason and the WARN line.
const EmptyDiffSentinel = "configs differ but field diff is empty (divergence outside compared fields)"

// BoundDiff caps the prefix of a config-diff string at capRunes runes,
// rune-safe. capRunes values <= 0 are treated as 0.
//
// When the diff exceeds the cap, BoundDiff returns the first capRunes runes
// followed by a truncation marker carrying the total rune count; the marker is
// additional and exempt from the cap. When the diff is empty, BoundDiff returns
// EmptyDiffSentinel regardless of capRunes; the sentinel is also exempt from
// the cap. An empty diff is surfaced explicitly rather than collapsing to an
// empty string that is indistinguishable from "nothing to report".
func BoundDiff(diff string, capRunes int) string {
	if diff == "" {
		return EmptyDiffSentinel
	}
	if capRunes < 0 {
		capRunes = 0
	}

	total := utf8.RuneCountInString(diff)
	if total <= capRunes {
		return diff
	}

	var b strings.Builder
	i := 0
	for _, r := range diff {
		if i >= capRunes {
			break
		}
		b.WriteRune(r)
		i++
	}
	b.WriteString(" …[truncated, ")
	b.WriteString(strconv.Itoa(total))
	b.WriteString(" chars total]")
	return b.String()
}
