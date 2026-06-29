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

package types

import (
	"strings"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sanitizeErrorDetail", func() {
	It("strips whitespace control chars to spaces and collapses runs", func() {
		Expect(sanitizeErrorDetail("a\r\nb\tc")).To(Equal("a b c"))
	})

	It("strips non-whitespace control chars to spaces", func() {
		Expect(sanitizeErrorDetail("a\x00b")).To(Equal("a b"))
	})

	It("trims surrounding whitespace and returns empty for empty input", func() {
		Expect(sanitizeErrorDetail("  hi  ")).To(Equal("hi"))
		Expect(sanitizeErrorDetail("")).To(Equal(""))
	})

	It("returns empty when all input is control characters", func() {
		Expect(sanitizeErrorDetail("\x00\x01\x02")).To(Equal(""))
	})

	It("caps to 256 bytes on a UTF-8 rune boundary", func() {
		// 100 Euro signs = 300 bytes; the maximal valid prefix is 85 runes = 255 bytes.
		Expect(sanitizeErrorDetail(strings.Repeat("€", 100))).To(Equal(strings.Repeat("€", 85)))
	})

	It("passes through input of exactly 256 bytes unchanged", func() {
		Expect(sanitizeErrorDetail(strings.Repeat("a", 256))).To(Equal(strings.Repeat("a", 256)))
	})

	It("caps input of 257 bytes to 256", func() {
		Expect(sanitizeErrorDetail(strings.Repeat("a", 257))).To(Equal(strings.Repeat("a", 256)))
	})

	It("trims a trailing space left by the byte cap", func() {
		// 255 single-byte 'a's + ' ' + 16 'b's: the 256-byte cut lands on the
		// inter-word space, so the capped slice would end in a space without a re-trim.
		Expect(sanitizeErrorDetail(strings.Repeat("a", 255) + " " + strings.Repeat("b", 16))).
			To(Equal(strings.Repeat("a", 255)))
	})

	It("coerces invalid UTF-8 to the replacement rune so output is always valid UTF-8", func() {
		out := sanitizeErrorDetail("a\x80b\xfe")
		Expect(utf8.ValidString(out)).To(BeTrue())
		Expect(out).To(ContainSubstring("�"))
	})

	It("caps 4-byte runes on a rune boundary without splitting a rune", func() {
		emoji := "😀" // U+1F600, 4 bytes each; 100 runes = 400 bytes, cap keeps 64 runes = 256 bytes.
		out := sanitizeErrorDetail(strings.Repeat(emoji, 100))
		Expect(utf8.ValidString(out)).To(BeTrue())
		Expect(out).To(Equal(strings.Repeat(emoji, 64)))
	})
})
