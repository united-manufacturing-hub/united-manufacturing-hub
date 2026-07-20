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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

type renderFixture struct {
	A string `yaml:"a"`
	B string `yaml:"b"`
}

type typedRenderFixture struct {
	A     string `yaml:"a"`
	Port  int    `yaml:"port,omitempty"`
	Count int    `yaml:"count,omitempty"`
}

var _ = Describe("RenderTemplate", func() {
	It("renders template directives in string fields", func() {
		in := renderFixture{A: "{{ .v }}", B: "static"}

		out, err := config.RenderTemplate(in, map[string]any{"v": "rendered"})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.A).To(Equal("rendered"))
		Expect(out.B).To(Equal("static"))
	})

	Describe("unmarshal failure of the rendered output", func() {
		var (
			in    renderFixture
			scope map[string]any
		)

		BeforeEach(func() {
			in = renderFixture{A: "line1\n{{ .inject }}", B: "tail"}
			scope = map[string]any{"inject": "x\nbroken: ["}
		})

		It("fails at the unmarshal step", func() {
			_, err := config.RenderTemplate(in, scope)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to render template as valid YAML"))
		})

		It("includes the failing region of the rendered output in the error", func() {
			_, err := config.RenderTemplate(in, scope)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("broken: ["))
			Expect(err.Error()).To(MatchRegexp(`(?m)^\s*>?\s*\d+ \|`))
		})

		It("marks the yaml-reported line, one above the malformed content for this error class", func() {
			_, err := config.RenderTemplate(in, scope)
			Expect(err).To(HaveOccurred())
			// yaml.v3 parser problem marks are 0-based: `broken: [` is on
			// rendered line 4, but the error says "line 3".
			Expect(err.Error()).To(ContainSubstring("yaml: line 3"))
			Expect(err.Error()).To(MatchRegexp(`(?m)^> +3 \|`))
		})

		It("shows one region per failing line for type errors, capped", func() {
			typedIn := typedRenderFixture{A: "line1\n{{ .inject }}"}
			typedScope := map[string]any{"inject": "x\nport: notanumber\ncount: alsonot"}

			_, err := config.RenderTemplate(typedIn, typedScope)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot unmarshal"))
			Expect(err.Error()).To(ContainSubstring("port: notanumber"))
			Expect(err.Error()).To(ContainSubstring("count: alsonot"))
			Expect(err.Error()).To(MatchRegexp(`(?m)^> +\d+ \| port: notanumber`))
			Expect(err.Error()).To(MatchRegexp(`(?m)^> +\d+ \| count: alsonot`))
		})
	})

	Describe("secret redaction in the failing snippet", func() {
		var (
			in    renderFixture
			scope map[string]any
		)

		BeforeEach(func() {
			in = renderFixture{A: "line1\n{{ .inject }}", B: "tail"}
			scope = map[string]any{"inject": "s3cr3t-pw\nbroken: ["}
		})

		It("leaks the value into the snippet when no secrets are supplied", func() {
			_, err := config.RenderTemplate(in, scope)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("s3cr3t-pw"))
		})

		It("masks the value when it is supplied as a secret", func() {
			_, err := config.RenderTemplate(in, scope, "s3cr3t-pw")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("s3cr3t-pw"))
			Expect(err.Error()).To(ContainSubstring("[REDACTED]"))
		})

		It("ignores empty secrets", func() {
			_, err := config.RenderTemplate(in, scope, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("s3cr3t-pw"))
		})
	})

	Describe("errors before the unmarshal step", func() {
		It("does not attach a snippet to template execution errors", func() {
			in := renderFixture{A: "{{ .missing }}", B: "static"}

			_, err := config.RenderTemplate(in, map[string]any{"v": "x"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to execute template"))
			Expect(err.Error()).ToNot(ContainSubstring("rendered output"))
		})
	})
})
