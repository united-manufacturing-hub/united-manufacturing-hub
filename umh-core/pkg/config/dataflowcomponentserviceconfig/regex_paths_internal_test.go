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

package dataflowcomponentserviceconfig

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("isAnchored", func() {
	DescribeTable("reports whether every match starts at the start of the text",
		func(pattern string, expected bool) {
			program, err := compileRegex(pattern)
			Expect(err).NotTo(HaveOccurred())
			Expect(isAnchored(program)).To(Equal(expected))
		},
		Entry("anchored", `^ab`, true),
		Entry("anchored with \\A", `\Aab`, true),
		Entry("anchored in multi-line mode", `(?m)^ab`, true),
		Entry("anchored in front of an alternation", `^(?:ab|cd)`, true),
		Entry("unanchored", `ab`, false),
		Entry("one unanchored alternative", `^ab|cd`, false),
		Entry("anchored inside every alternative", `^ab|^cd`, false),
		Entry("optional anchor", `(?:^)?ab`, false),
	)

	It("rejects an invalid regular expression", func() {
		_, err := compileRegex(`^a(`)
		Expect(err).To(HaveOccurred())
	})
})
