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
	"context"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// Downgrading is the only way back to a release from before the merge, and the
// failure mode if it is wrong is the worst one available: the older release reads
// what it cannot decode, caches an empty config, and reports nothing while every
// bridge is torn down. So the property here is not "it produces plausible YAML" but
// "the pre-merge structs read it and get the same contracts back".
var _ = Describe("downgrade-config", func() {
	ctx := context.Background()

	// preMergeSections is what an older release decodes with: dataContracts entries
	// carry a model *mapping*, and structures live in dataModels. Declared here rather
	// than reused from the decoder so the assertion does not depend on the type whose
	// output it is checking.
	type preMergeSections struct {
		DataModels    []DataModelsConfig    `yaml:"dataModels"`
		DataContracts []DataContractsConfig `yaml:"dataContracts"`
	}

	Describe("every corpus file", func() {
		check := func(path string) {
			original, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			before, err := ParseConfig(original, ctx, true)
			Expect(err).NotTo(HaveOccurred())

			downgraded, err := DowngradeConfigYAML(ctx, original)
			Expect(err).NotTo(HaveOccurred(), "corpus files must all downgrade")

			// The merged section must be gone, or an older release would reject it.
			Expect(string(downgraded)).NotTo(ContainSubstring("versions:"))

			// Decoded through the pre-merge structs, which is what the older release
			// does -- not through the union type this package uses.
			var sections preMergeSections
			Expect(yaml.Unmarshal(downgraded, &sections)).To(Succeed())

			after, notices := AbsorbConfig(sections.DataModels, LegacyEntries(sections.DataContracts))
			Expect(FirstDrop(notices)).To(BeNil())
			Expect(ContractsEqual(before.Contracts, after)).To(BeTrue(),
				"downgrade lost information:\nbefore %+v\nafter  %+v", before.Contracts, after)
		}

		DescribeTable("survives the pre-merge structs", migrationTableArgs(check)...)
	})

	Describe("what it emits", func() {
		It("writes the model as a {name, version} pointer, not a label", func() {
			downgraded, err := DowngradeConfigYAML(ctx, []byte(`
agent:
  metricsPort: 8080
dataContracts:
  - model: pump
    versions:
      v1:
        name: _pump_v1
        structure:
          temperature:
            _payloadshape: timeseries-number
`))
			Expect(err).NotTo(HaveOccurred())

			text := string(downgraded)
			Expect(text).To(ContainSubstring("dataModels:"))
			Expect(text).To(ContainSubstring("name: pump"))
			Expect(text).To(ContainSubstring("version: v1"))
			Expect(text).To(ContainSubstring("name: _pump_v1"))

			// A label would be read as a name by the older decoder and fail.
			Expect(text).NotTo(ContainSubstring("model: pump"))
		})

		It("keeps a bare address as a contract with no model", func() {
			downgraded, err := DowngradeConfigYAML(ctx, []byte(`
agent:
  metricsPort: 8080
dataContracts:
  - name: _raw
`))
			Expect(err).NotTo(HaveOccurred())

			var sections preMergeSections
			Expect(yaml.Unmarshal(downgraded, &sections)).To(Succeed())
			Expect(sections.DataContracts).To(HaveLen(1))
			Expect(sections.DataContracts[0].Name).To(Equal("_raw"))
			Expect(sections.DataContracts[0].Model).To(BeNil())
		})

		It("leaves an unaddressed version as a data model with no contract", func() {
			// Which is what a definition already was before the merge, so the older
			// release sees exactly what it used to.
			downgraded, err := DowngradeConfigYAML(ctx, []byte(`
agent:
  metricsPort: 8080
dataContracts:
  - model: motor
    versions:
      v1:
        structure:
          rpm:
            _payloadshape: timeseries-number
`))
			Expect(err).NotTo(HaveOccurred())

			var sections preMergeSections
			Expect(yaml.Unmarshal(downgraded, &sections)).To(Succeed())
			Expect(sections.DataModels).To(HaveLen(1))
			Expect(sections.DataModels[0].Name).To(Equal("motor"))
			Expect(sections.DataContracts).To(BeEmpty())
		})

		It("passes everything else through", func() {
			downgraded, err := DowngradeConfigYAML(ctx, []byte(`
agent:
  metricsPort: 8080
  location:
    0: factory-A
payloadShapes:
  timeseries-number:
    fields:
      value:
        _type: number
dataContracts:
  - name: _raw
`))
			Expect(err).NotTo(HaveOccurred())

			text := string(downgraded)
			Expect(text).To(ContainSubstring("factory-A"))
			Expect(text).To(ContainSubstring("timeseries-number"))
		})
	})

	Describe("when it cannot be done", func() {
		// Refusing is the only safe answer. A partial downgrade would be read
		// successfully by the older release, which would then tear down the bridges of
		// the contracts that went missing without reporting anything.
		It("refuses rather than writing a partial result", func() {
			unconvertible := []DataContract{{
				Model:          "pump",
				Version:        "v1",
				DefaultBridges: []map[string]interface{}{{"name": "lost"}},
			}}

			Expect(ContractsAreLossless(unconvertible)).To(BeFalse())
		})

		It("reports a parse failure rather than writing anything", func() {
			_, err := DowngradeConfigYAML(ctx, []byte("dataContracts:\n  - model:\n      - a list\n"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("model"))
		})
	})

	Describe("running it twice", func() {
		It("is a no-op the second time", func() {
			original := []byte(`
agent:
  metricsPort: 8080
dataContracts:
  - model: pump
    versions:
      v1:
        name: _pump_v1
        structure:
          temperature:
            _payloadshape: timeseries-number
`)

			once, err := DowngradeConfigYAML(ctx, original)
			Expect(err).NotTo(HaveOccurred())

			twice, err := DowngradeConfigYAML(ctx, once)
			Expect(err).NotTo(HaveOccurred())

			// An operator who runs it again -- unsure whether the first one took -- must
			// not end up with something different.
			Expect(strings.TrimSpace(string(twice))).To(Equal(strings.TrimSpace(string(once))))
		})
	})
})
