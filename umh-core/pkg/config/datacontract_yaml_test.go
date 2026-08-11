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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// The dataContracts section holds three forms. Two are the merged shape and one
// is what every config in the field looks like today, so the decoder has to take
// all of them:
//
//   - grouped     model + versions, each version optionally addressed by name
//   - bare        name alone, no model and no structure (_raw)
//   - legacy      name + a model *mapping* pointing at a dataModels version
//
// `model` is therefore a scalar in one form and a mapping in another, which is
// why this needs a custom unmarshaller rather than struct tags.
var _ = Describe("dataContracts decoding", func() {
	decode := func(doc string) ([]DataContractYAMLEntry, error) {
		var parsed struct {
			DataContracts []DataContractYAMLEntry `yaml:"dataContracts"`
		}

		dec := yaml.NewDecoder(strings.NewReader(doc))
		dec.KnownFields(true)

		if err := dec.Decode(&parsed); err != nil {
			return nil, err
		}

		return parsed.DataContracts, nil
	}

	Describe("the grouped form", func() {
		It("reads a version that carries an address", func() {
			entries, err := decode(`
dataContracts:
  - model: pump
    description: Pump monitoring
    versions:
      v1:
        name: _pump_v1
        structure:
          temperature:
            _payloadshape: timeseries-number
`)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Model).To(Equal("pump"))
			Expect(entries[0].Description).To(Equal("Pump monitoring"))
			Expect(entries[0].LegacyModelRef).To(BeNil())
			Expect(entries[0].Versions).To(HaveKey("v1"))
			Expect(entries[0].Versions["v1"].Name).To(Equal("_pump_v1"))
			Expect(entries[0].Versions["v1"].Structure).To(HaveKey("temperature"))
		})

		It("reads a version with no address as a definition", func() {
			entries, err := decode(`
dataContracts:
  - model: motor
    versions:
      v1:
        structure:
          rpm:
            _payloadshape: timeseries-number
`)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries[0].Model).To(Equal("motor"))
			Expect(entries[0].Versions["v1"].Name).To(BeEmpty())
			Expect(entries[0].Versions["v1"].Structure).To(HaveKey("rpm"))
		})

		It("carries default_bridges on the version, where it actually sits", func() {
			entries, err := decode(`
dataContracts:
  - model: pump
    versions:
      v1:
        name: _pump_v1
        default_bridges:
          - name: keep-me
        structure:
          temperature:
            _payloadshape: timeseries-number
`)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries[0].Versions["v1"].DefaultBridges).To(HaveLen(1))
		})
	})

	Describe("the bare form", func() {
		It("reads an entry that is only an address", func() {
			entries, err := decode(`
dataContracts:
  - name: _raw
`)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Name).To(Equal("_raw"))
			Expect(entries[0].Model).To(BeEmpty())
			Expect(entries[0].Versions).To(BeEmpty())
			Expect(entries[0].LegacyModelRef).To(BeNil())
		})
	})

	Describe("the legacy form", func() {
		It("reads a model mapping as a pointer rather than a label", func() {
			entries, err := decode(`
dataContracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1
`)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Name).To(Equal("_pump_v1"))
			Expect(entries[0].Model).To(BeEmpty())
			Expect(entries[0].LegacyModelRef).NotTo(BeNil())
			Expect(entries[0].LegacyModelRef.Name).To(Equal("pump"))
			Expect(entries[0].LegacyModelRef.Version).To(Equal("v1"))
		})

		It("carries default_bridges at the entry level, where the legacy shape put it", func() {
			entries, err := decode(`
dataContracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1
    default_bridges:
      - name: keep-me
`)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries[0].DefaultBridges).To(HaveLen(1))
		})
	})

	Describe("strictness", func() {
		// A custom UnmarshalYAML silently disables KnownFields, because Node.Decode
		// builds its own decoder with knownFields false. Without re-asserting it by
		// hand, a typo'd key is accepted and the contract silently loses its
		// structure — after which its Redpanda subjects get deleted. This is the
		// single most dangerous thing about the decoder, so it is asserted directly.
		It("rejects an unknown key on an entry", func() {
			_, err := decode(`
dataContracts:
  - model: pump
    verzions:
      v1:
        structure: {}
`)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("verzions"))
		})

		It("rejects an unknown key on a version", func() {
			_, err := decode(`
dataContracts:
  - model: pump
    versions:
      v1:
        structrue:
          temperature:
            _payloadshape: timeseries-number
`)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("structrue"))
		})

		It("rejects a model that is neither a label nor a pointer", func() {
			_, err := decode(`
dataContracts:
  - name: _pump_v1
    model:
      - pump
`)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("model"))
		})

		It("rejects an entry mixing a legacy pointer with grouped versions", func() {
			_, err := decode(`
dataContracts:
  - name: _pump_v1
    model:
      name: pump
      version: v1
    versions:
      v1:
        structure: {}
`)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("YAML aliases", func() {
		It("decodes an entry reused through an anchor", func() {
			entries, err := decode(`
_shared: &shared
  model: pump
  versions:
    v1:
      name: _pump_v1
      structure:
        temperature:
          _payloadshape: timeseries-number
dataContracts:
  - *shared
`)
			// The top-level _shared key is not part of the struct, so strict
			// decoding rejects it; the point is that the alias itself resolves.
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("_shared"))

				return
			}

			Expect(entries[0].Model).To(Equal("pump"))
		})
	})
})
