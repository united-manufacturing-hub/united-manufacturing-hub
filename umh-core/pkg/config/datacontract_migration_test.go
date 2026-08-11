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
	"path/filepath"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// corpusPaths enumerates the corpus at tree-construction time so DescribeTable can
// generate one entry per file.
func corpusPaths() []string {
	paths, err := filepath.Glob("testdata/corpus/*.yaml")
	if err != nil || len(paths) == 0 {
		panic("corpus is missing; the migration property has nothing to run against")
	}

	sort.Strings(paths)

	return paths
}

func migrationTableArgs(body func(string)) []any {
	paths := corpusPaths()
	args := make([]any, 0, len(paths)+1)
	args = append(args, body)

	for _, p := range paths {
		args = append(args, Entry(filepath.Base(p), p))
	}

	return args
}

// Migration is what happens the first time a pre-merge config is written back: the
// two sections collapse into one. It has to be a fixed point -- reading the result
// must produce the same contracts as reading the original, or a restart would see
// a different config than the one that was running.
var _ = Describe("data contract migration", func() {
	ctx := context.Background()

	Describe("writing the merged shape", func() {
		check := func(path string) {
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			before, err := ParseConfig(data, ctx, false)
			Expect(err).NotTo(HaveOccurred(), "corpus file must parse")

			written, err := yaml.Marshal(before.withContractsProjected())
			Expect(err).NotTo(HaveOccurred())

			after, err := ParseConfig(written, ctx, false)
			Expect(err).NotTo(HaveOccurred(), "the shape we write must be a shape we read")

			Expect(ContractsEqual(before.Contracts, after.Contracts)).To(BeTrue(),
				"migration changed the contracts:\nbefore %+v\nafter  %+v",
				before.Contracts, after.Contracts)
		}

		DescribeTable("re-parses to the same contracts", migrationTableArgs(check)...)

		It("drops the pre-merge models section", func() {
			data, err := os.ReadFile("testdata/corpus/conventional.yaml")
			Expect(err).NotTo(HaveOccurred())

			parsed, err := ParseConfig(data, ctx, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed.DataModels).NotTo(BeEmpty(), "the fixture is meant to be pre-merge")

			written, err := yaml.Marshal(parsed.withContractsProjected())
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written)).NotTo(ContainSubstring("dataModels:"))
			Expect(string(written)).To(ContainSubstring("dataContracts:"))
		})

		It("nests the structure under the model it belongs to", func() {
			data, err := os.ReadFile("testdata/corpus/conventional.yaml")
			Expect(err).NotTo(HaveOccurred())

			parsed, err := ParseConfig(data, ctx, false)
			Expect(err).NotTo(HaveOccurred())

			written, err := yaml.Marshal(parsed.withContractsProjected())
			Expect(err).NotTo(HaveOccurred())

			// The point of the merge: one entry carrying both the label and the
			// structure that used to live in a separate section.
			Expect(string(written)).To(ContainSubstring("model: pump"))
			Expect(string(written)).To(ContainSubstring("structure:"))
		})
	})

	Describe("the self-check", func() {
		check := func(path string) {
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())

			parsed, err := ParseConfig(data, ctx, false)
			Expect(err).NotTo(HaveOccurred())

			Expect(parsed.ContractsDegraded).To(BeFalse(),
				"no corpus file should be unconvertible; contracts were %+v", parsed.Contracts)
		}

		DescribeTable("accepts every corpus file", migrationTableArgs(check)...)

		// The check exists to catch our own conversion bugs, so it has to be shown
		// failing on something. A definition carrying default_bridges is the real
		// class: ToLegacyConfig emits no contract entry for an unaddressed version,
		// so the bridges have nowhere to go and vanish. AbsorbConfig already refuses
		// to produce this, which is why it has to be built by hand here.
		It("rejects a contract set that cannot survive the downgrade", func() {
			unconvertible := []DataContract{{
				Model:          "pump",
				Version:        "v1",
				Structure:      map[string]Field{"temperature": {PayloadShape: "timeseries-number"}},
				DefaultBridges: []map[string]interface{}{{"name": "lost-on-downgrade"}},
			}}

			Expect(ContractsAreLossless(unconvertible)).To(BeFalse())
		})

		It("accepts the same contract set once it has an address to carry them", func() {
			convertible := []DataContract{{
				Name:           "_pump_v1",
				Model:          "pump",
				Version:        "v1",
				Structure:      map[string]Field{"temperature": {PayloadShape: "timeseries-number"}},
				DefaultBridges: []map[string]interface{}{{"name": "kept"}},
			}}

			Expect(ContractsAreLossless(convertible)).To(BeTrue())
		})

		It("leaves a degraded config's sections alone rather than rewriting them", func() {
			// Whatever is on disk is the only trustworthy record at this point, so
			// projection must be a no-op instead of emitting contracts we could not
			// verify.
			degraded := FullConfig{
				ContractsDegraded: true,
				DataModels: []DataModelsConfig{{
					Name:     "pump",
					Versions: map[string]DataModelVersion{"v1": {}},
				}},
				DataContracts: []DataContractYAMLEntry{{
					Name:           "_pump_v1",
					LegacyModelRef: &ModelRef{Name: "pump", Version: "v1"},
				}},
			}

			projected := degraded.withContractsProjected()
			Expect(projected.DataModels).To(HaveLen(1))
			Expect(projected.DataContracts).To(HaveLen(1))
			Expect(projected.DataContracts[0].LegacyModelRef).NotTo(BeNil())
		})
	})

	Describe("notices", func() {
		It("reports what it dropped, with enough detail to find it", func() {
			data, err := os.ReadFile("testdata/corpus/orphan.yaml")
			Expect(err).NotTo(HaveOccurred())

			_, notices, err := ParseConfigWithNotices(data, ctx, false)
			Expect(err).NotTo(HaveOccurred())

			warning := FirstWarning(notices)
			Expect(warning).NotTo(BeNil(), "an orphan contract must be reported")
			Expect(warning.Reason).To(ContainSubstring("does not exist"))
		})

		It("says nothing about a config that converts cleanly", func() {
			data, err := os.ReadFile("testdata/corpus/conventional.yaml")
			Expect(err).NotTo(HaveOccurred())

			_, notices, err := ParseConfigWithNotices(data, ctx, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(FirstWarning(notices)).To(BeNil())
		})
	})

	Describe("empty input", func() {
		// manager.go uses reflect.DeepEqual against FullConfig{} to mean "nothing
		// loaded yet". A non-nil empty Contracts slice would make a valid empty
		// config look loaded, and a cloned one look different from its original.
		It("leaves Contracts nil so the empty-config guards keep working", func() {
			parsed, err := ParseConfig([]byte("agent:\n  metricsPort: 8080\n"), ctx, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed.Contracts).To(BeNil())
		})

		It("survives Clone with its nil-ness intact", func() {
			Expect(FullConfig{}.Clone().Contracts).To(BeNil())
		})
	})

	Describe("the emitted YAML", func() {
		It("writes a single address as a scalar, not a one-element list", func() {
			contracts := []DataContract{{
				Name: "_pump_v1", Model: "pump", Version: "v1",
				Structure: map[string]Field{"t": {PayloadShape: "timeseries-number"}},
			}}

			written, err := yaml.Marshal(map[string]any{
				"dataContracts": ToYAMLEntries(contracts),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written)).To(ContainSubstring("name: _pump_v1"))
			Expect(strings.Contains(string(written), "- _pump_v1")).To(BeFalse(),
				"a single address should stay readable")
		})

		It("writes several addresses as a list", func() {
			contracts := []DataContract{
				{Name: "_a_v1", Model: "m", Version: "v1"},
				{Name: "_b_v1", Model: "m", Version: "v1"},
			}

			written, err := yaml.Marshal(map[string]any{
				"dataContracts": ToYAMLEntries(contracts),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written)).To(ContainSubstring("- _a_v1"))
			Expect(string(written)).To(ContainSubstring("- _b_v1"))
		})
	})
})
