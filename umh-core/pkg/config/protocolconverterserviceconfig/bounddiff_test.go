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

package protocolconverterserviceconfig

import (
	"strings"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BoundDiff", func() {
	It("passes a short diff through unchanged", func() {
		short := "hello world"
		Expect(BoundDiff(short, 400)).To(Equal(short),
			"short diff should pass through unchanged")
	})

	It("passes a diff of exactly capRunes through unchanged with no marker", func() {
		exactCap := strings.Repeat("a", 400)
		Expect(BoundDiff(exactCap, 400)).To(Equal(exactCap),
			"diff of exactly capRunes runes should pass through unchanged with no marker")
	})

	It("truncates a long diff to the first capRunes runes plus a marker with the total rune count", func() {
		longRunes := []rune(strings.Repeat("b", 1000))
		longDiff := string(longRunes)
		got := BoundDiff(longDiff, 400)
		expectedPrefix := string(longRunes[:400])
		expectedMarker := " …[truncated, 1000 chars total]"
		Expect(got).To(Equal(expectedPrefix+expectedMarker),
			"long diff should be truncated to first capRunes runes plus marker with total rune count")
	})

	It("truncates multibyte input on a rune boundary keeping the output valid UTF-8", func() {
		multibyte := strings.Repeat("ü", 500) // ü is 2 bytes, 1 rune
		gotMB := BoundDiff(multibyte, 400)
		Expect(utf8.ValidString(gotMB)).To(BeTrue(),
			"truncated multibyte output must stay valid UTF-8")
		Expect(gotMB).To(HavePrefix(strings.Repeat("ü", 400)),
			"truncated multibyte output must prefix with the first capRunes runes")
		Expect(gotMB).To(Equal(strings.Repeat("ü", 400)+" …[truncated, 500 chars total]"),
			"multibyte truncation must use rune-indexed cut and correct total count")
	})

	It("treats capRunes of 0 as marker-only output", func() {
		// Use a non-empty diff so the truncation path is taken; the empty-diff
		// case is handled separately.
		nonEmpty := "x"
		Expect(BoundDiff(nonEmpty, 0)).To(Equal(" …[truncated, 1 chars total]"),
			"capRunes 0 should yield marker-only output")
	})

	It("treats negative capRunes as 0", func() {
		nonEmpty := "x"
		Expect(BoundDiff(nonEmpty, -5)).To(Equal(" …[truncated, 1 chars total]"),
			"negative capRunes should be treated as 0 and yield marker-only output")
	})

	It("passes an empty diff through unchanged", func() {
		// Empty diff is not special-cased: len([]rune("")) == 0 <= capRunes, so
		// it is returned unchanged.
		Expect(BoundDiff("", 400)).To(Equal(""),
			"empty diff should pass through unchanged")
	})
})
