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

// BoundDiff caps a config-diff string at capRunes runes, rune-safe, appending a
// truncation marker when the diff exceeds the cap. capRunes values <= 0 are
// treated as 0.
func BoundDiff(diff string, capRunes int) string {
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
