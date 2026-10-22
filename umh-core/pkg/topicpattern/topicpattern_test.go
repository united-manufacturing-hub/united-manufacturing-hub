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

package topicpattern_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/topicpattern"
)

// containsUpperX is a grammar whose state reports whether the topic read so far contains an X.
type containsUpperX struct{}

func (containsUpperX) Start() bool { return false }

func (containsUpperX) Step(seenX bool, r rune) bool { return seenX || r == 'X' }

func (containsUpperX) Significant() []rune { return []rune{'X'} }

var _ = Describe("Analyze", func() {
	analyze := func(pattern string) topicpattern.Analysis[bool] {
		analysis, err := topicpattern.Analyze[bool](pattern, containsUpperX{})
		Expect(err).NotTo(HaveOccurred())

		return analysis
	}

	DescribeTable("returns the grammar state of every topic prefix the pattern matches",
		func(pattern string, expected []bool) {
			Expect(analyze(pattern).EndStates).To(ConsistOf(expected))
		},
		Entry("literal without the significant rune", `^ab`, []bool{false}),
		Entry("literal with the significant rune", `^aX`, []bool{true}),
		Entry("class covering the significant rune and others", `^a[W-Y]`, []bool{true, false}),
		Entry("negated class excluding the significant rune", `^a[^X]`, []bool{false}),
		Entry("any rune", `^a.`, []bool{true, false}),
		Entry("case-insensitive significant rune also accepting an insignificant variant", `^(?i)x`, []bool{true, false}),
		Entry("alternation", `^(?:ab|aX)`, []bool{true, false}),
		Entry("runes after the end of text", `^a$X`, []bool(nil)),
	)

	DescribeTable("reports whether the pattern also matches past the first rune of a topic",
		func(pattern string, expected bool) {
			Expect(analyze(pattern).MatchesMidTopic).To(Equal(expected))
		},
		Entry("anchored", `^ab`, false),
		Entry("anchored with \\A", `\Aab`, false),
		Entry("anchored in multi-line mode", `(?m)^ab`, false),
		Entry("unanchored", `ab`, true),
		Entry("one unanchored alternative", `^ab|cd`, true),
	)

	It("rejects an invalid regular expression", func() {
		_, err := topicpattern.Analyze[bool](`^a(`, containsUpperX{})
		Expect(err).To(HaveOccurred())
	})
})
