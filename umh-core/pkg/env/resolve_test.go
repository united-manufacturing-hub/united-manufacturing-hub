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

package env_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
)

const (
	setVariable   = "UMH_TEST_ENV_EXPAND"
	unsetVariable = "UMH_TEST_ENV_EXPAND_UNSET"
	variableValue = "from-environment"
)

var _ = Describe("ResolveVariables", func() {
	BeforeEach(func() {
		GinkgoT().Setenv(setVariable, variableValue)
	})

	It("keeps a value without a reference unchanged", func() {
		Expect(env.ResolveVariables("pa$word{x}")).To(Equal("pa$word{x}"))
	})

	It("replaces ${VAR} with the environment variable", func() {
		Expect(env.ResolveVariables("${" + setVariable + "}")).To(Equal(variableValue))
	})

	It("replaces a reference inside a longer value", func() {
		Expect(env.ResolveVariables("prefix-${" + setVariable + "}-suffix")).
			To(Equal("prefix-" + variableValue + "-suffix"))
	})

	It("uses the default of ${VAR:default} when the variable is unset", func() {
		Expect(env.ResolveVariables("${" + unsetVariable + ":fallback}")).To(Equal("fallback"))
	})

	It("uses the default of ${VAR:default} when the variable is empty", func() {
		GinkgoT().Setenv(setVariable, "")

		Expect(env.ResolveVariables("${" + setVariable + ":fallback}")).To(Equal("fallback"))
	})

	It("prefers the variable over the default when it is set", func() {
		Expect(env.ResolveVariables("${" + setVariable + ":fallback}")).To(Equal(variableValue))
	})

	It("accepts an empty default", func() {
		Expect(env.ResolveVariables("${" + unsetVariable + ":}")).To(Equal(""))
	})

	It("fails and names every variable that is unset and has no default", func() {
		_, err := env.ResolveVariables("${" + unsetVariable + "}${" + unsetVariable + "_SECOND}")

		Expect(err).To(MatchError(And(
			ContainSubstring(unsetVariable),
			ContainSubstring(unsetVariable+"_SECOND"))))
	})

	DescribeTable("rejects reference forms other than ${VAR} and ${VAR:default}, quoting only the reference",
		func(value, reference string) {
			_, err := env.ResolveVariables("secret-prefix-" + value)

			Expect(err).To(MatchError(ContainSubstring(reference)))
			Expect(err).NotTo(MatchError(ContainSubstring("secret-prefix")))
		},
		Entry("a transform", "${"+setVariable+"|base64decode}", "${"+setVariable+"|base64decode}"),
		Entry("a transform after a default", "${"+setVariable+":fallback|base64decode}", "${"+setVariable+":fallback|base64decode}"),
		Entry("the escape", "${{"+setVariable+"}}", "${{"+setVariable+"}"),
		Entry("a nested reference in the default", "${"+unsetVariable+":${"+setVariable+"}}", "${"+unsetVariable+":${"+setVariable+"}"),
		Entry("an empty name", "${}", "${}"),
	)
})

var _ = Describe("ValidateReferences", func() {
	It("accepts ${VAR} and ${VAR:default} without reading the environment", func() {
		Expect(env.ValidateReferences("${" + unsetVariable + "}:${" + unsetVariable + ":fallback}")).To(Succeed())
	})

	It("rejects an unsupported form", func() {
		Expect(env.ValidateReferences("${" + setVariable + "|base64decode}")).
			To(MatchError(ContainSubstring("|base64decode")))
	})
})
