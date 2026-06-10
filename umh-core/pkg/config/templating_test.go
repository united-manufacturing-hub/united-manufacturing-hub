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

// renderFixture is a minimal struct whose string fields can carry template
// directives, mirroring how tag_processor code blocks are stored in DFC configs.
type renderFixture struct {
	A string `yaml:"a"`
	B string `yaml:"b"`
}

// typedRenderFixture adds int fields so a rendered output with string values
// in their place produces a yaml.TypeError (valid YAML, failing decode).
// omitempty keeps the zero-valued fields out of the marshalled template so
// the injected keys do not collide with them.
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
		// A directive inside a multi-line string (stored as a YAML block
		// scalar) whose expansion contains a column-0 line. The column-0
		// line terminates the block scalar and becomes a malformed
		// top-level node, so yaml.Marshal and template execution succeed
		// but unmarshalling the rendered output fails. This is the failure
		// mode seen in ENG-5103 (`yaml: line 25: did not find expected key`,
		// where the logged error omitted the rendered output).
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
			Expect(err.Error()).To(ContainSubstring("failed to unmarshal rendered template"))
		})

		It("includes the failing region of the rendered output in the error", func() {
			_, err := config.RenderTemplate(in, scope)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("broken: ["))
			// Line numbers let the reader correlate with the yaml error's
			// "line N" reference.
			Expect(err.Error()).To(MatchRegexp(`(?m)^\s*>?\s*\d+ \|`))
		})

		It("marks the yaml-reported line, one above the malformed content for this error class", func() {
			_, err := config.RenderTemplate(in, scope)
			Expect(err).To(HaveOccurred())
			// The fixture renders `broken: [` on line 4 and yaml.v3 reports
			// "yaml: line 3" (parser problem marks are 0-based). The marker
			// follows the reported line; pinning the exact number catches a
			// regression where the marker drifts to any other line in the
			// window.
			Expect(err.Error()).To(ContainSubstring("yaml: line 3"))
			Expect(err.Error()).To(MatchRegexp(`(?m)^> +3 \|`))
		})

		It("shows one region per failing line for type errors, capped", func() {
			// Valid YAML whose values cannot decode into the typed fields:
			// yaml.TypeError aggregates one "line N:" entry per field.
			typedIn := typedRenderFixture{A: "line1\n{{ .inject }}"}
			typedScope := map[string]any{"inject": "x\nport: notanumber\ncount: alsonot"}

			_, err := config.RenderTemplate(typedIn, typedScope)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot unmarshal"))
			Expect(err.Error()).To(ContainSubstring("port: notanumber"))
			Expect(err.Error()).To(ContainSubstring("count: alsonot"))
			// One marker per reported line.
			Expect(err.Error()).To(MatchRegexp(`(?m)^> +\d+ \| port: notanumber`))
			Expect(err.Error()).To(MatchRegexp(`(?m)^> +\d+ \| count: alsonot`))
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
