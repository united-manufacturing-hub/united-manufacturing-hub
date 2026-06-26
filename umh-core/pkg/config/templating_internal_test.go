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

package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// markerLineRegex matches a snippet marker at the start of a line; counting
// markers via this regex (rather than bare substring search) keeps the count
// honest when fixture content itself contains "> ".
var markerLineRegex = regexp.MustCompile(`(?m)^> `)

var _ = Describe("renderedRegionSnippet", func() {
	// Production rendered output always ends with a trailing newline.
	rendered := []byte("a: 1\nb: 2\nc: 3\nd: 4\ne: 5\n")

	It("falls back to the head of the output when the error carries no line number", func() {
		// Several yaml.v3 error classes have no position information; the
		// rendered output must still appear in the error.
		snippet := renderedRegionSnippet(rendered, errors.New("yaml: unknown anchor 'x' referenced"))
		Expect(snippet).To(ContainSubstring("reported no line number"))
		Expect(snippet).To(ContainSubstring("a: 1"))
		Expect(markerLineRegex.FindAllString(snippet, -1)).To(BeEmpty())
		Expect(snippet).NotTo(HaveSuffix("\n"))
	})

	It("limits the head fallback to its window and truncates its lines", func() {
		long := []byte("l1: " + strings.Repeat("v", 300) + "\nl2: 2\nl3: 3\nl4: 4\nl5: 5\nl6: 6\nl7: 7\nl8: 8\nl9: 9\n")
		snippet := renderedRegionSnippet(long, errors.New("yaml: mapping values are not allowed in this context"))
		Expect(snippet).To(ContainSubstring("…(truncated)"))
		Expect(snippet).To(ContainSubstring("l7: 7"))
		Expect(snippet).NotTo(ContainSubstring("l8: 8"))
	})

	It("treats a line number embedded in a longer word as no line number", func() {
		snippet := renderedRegionSnippet(rendered, errors.New("yaml: deadline 5: oops"))
		Expect(snippet).To(ContainSubstring("reported no line number"))
	})

	It("falls back to the head of the output when the line number overflows int", func() {
		snippet := renderedRegionSnippet(rendered, errors.New("yaml: line 99999999999999999999: oops"))
		Expect(snippet).To(ContainSubstring("reported no line number"))
	})

	It("clamps out-of-range reported lines to the nearest line", func() {
		// Parser errors are 0-based, so a first-line failure reports
		// "yaml: line 0"; dropping the snippet there would recreate the
		// undiagnosable error this function exists to eliminate. The high
		// clamp must land on the real last line, not the phantom empty
		// line after the trailing newline.
		low := renderedRegionSnippet(rendered, errors.New("yaml: line 0: oops"))
		Expect(low).To(MatchRegexp(`(?m)^> +1 \| a: 1`))
		Expect(low).To(ContainSubstring("rendered output around the reported line"))

		high := renderedRegionSnippet(rendered, errors.New("yaml: line 99: oops"))
		Expect(high).To(MatchRegexp(`(?m)^> +5 \| e: 5`))
		Expect(high).NotTo(MatchRegexp(`(?m)^> +6 \|`))
		Expect(high).NotTo(HaveSuffix("\n"))
	})

	It("clamps past the double trailing newline of a keep-chomped block scalar", func() {
		// yaml.Marshal emits two trailing newlines when the final value is
		// a |+ block scalar; both must be trimmed or the clamp marks a
		// phantom empty line.
		doubled := []byte("a: 1\nb: 2\n\n")
		snippet := renderedRegionSnippet(doubled, errors.New("yaml: line 99: oops"))
		Expect(snippet).To(MatchRegexp(`(?m)^> +2 \| b: 2`))
		Expect(snippet).NotTo(MatchRegexp(`(?m)^> +3 \|`))
	})

	It("truncates overlong lines on a rune boundary", func() {
		long := "k: " + strings.Repeat("ä", 300)
		snippet := renderedRegionSnippet([]byte(long+"\n"), errors.New("yaml: line 1: oops"))
		Expect(snippet).To(ContainSubstring("…(truncated)"))
		Expect(len(snippet)).To(BeNumerically("<", 400))
		Expect(utf8.ValidString(snippet)).To(BeTrue())
	})

	It("dedupes repeated type-error lines and caps the regions", func() {
		typeErr := &yaml.TypeError{Errors: []string{
			"line 1: cannot unmarshal one",
			"line 1: cannot unmarshal one again",
			"line 2: cannot unmarshal two",
			"line 3: cannot unmarshal three",
			"line 4: cannot unmarshal four",
		}}
		snippet := renderedRegionSnippet(rendered, typeErr)

		// Without dedup the cap would spend two slots on line 1 and line 3
		// would get no marker; with it, lines 1-3 are marked and line 4
		// falls past the cap.
		Expect(snippet).To(MatchRegexp(`(?m)^> +1 \| a: 1`))
		Expect(snippet).To(MatchRegexp(`(?m)^> +2 \| b: 2`))
		Expect(snippet).To(MatchRegexp(`(?m)^> +3 \| c: 3`))
		Expect(markerLineRegex.FindAllString(snippet, -1)).To(HaveLen(maxSnippetRegions))
		Expect(strings.Count(snippet, "...\n")).To(Equal(maxSnippetRegions - 1))
	})

	It("skips type-error entries without a usable line number", func() {
		typeErr := &yaml.TypeError{Errors: []string{
			"cannot unmarshal something without a position",
			"line 99999999999999999999: overflowing",
			"line 2: cannot unmarshal two",
		}}
		snippet := renderedRegionSnippet(rendered, typeErr)
		Expect(snippet).To(MatchRegexp(`(?m)^> +2 \| b: 2`))
		Expect(markerLineRegex.FindAllString(snippet, -1)).To(HaveLen(1))
	})

	It("unwraps a wrapped type error", func() {
		wrapped := fmt.Errorf("outer context: %w", &yaml.TypeError{Errors: []string{
			"line 2: cannot unmarshal two",
		}})
		snippet := renderedRegionSnippet(rendered, wrapped)
		Expect(snippet).To(MatchRegexp(`(?m)^> +2 \| b: 2`))
	})
})
