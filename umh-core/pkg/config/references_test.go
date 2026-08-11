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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
)

// Deleting a contract that something still depends on is the failure the pre-merge
// delete allowed: the model went away, its contract kept pointing at nothing, and
// an address that was still receiving data silently stopped being validated. These
// tests are about the refusal that replaces it, and about it being narrow enough not
// to block legitimate deletions.
var _ = Describe("finding references", func() {
	streamProcessor := func(name, modelName, modelVersion string) StreamProcessorConfig {
		sp := StreamProcessorConfig{}
		sp.Name = name
		sp.StreamProcessorServiceConfig.Config.Model = streamprocessorserviceconfig.ModelRef{
			Name:    modelName,
			Version: modelVersion,
		}

		return sp
	}

	bridgeNaming := func(name, topic string) DataFlowComponentConfig {
		df := DataFlowComponentConfig{}
		df.Name = name
		df.DataFlowComponentServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Output: map[string]interface{}{"uns": map[string]interface{}{"topic": topic}},
			},
		}

		return df
	}

	Describe("stream processors", func() {
		It("matches on the declared model", func() {
			cfg := FullConfig{
				Contracts:       []DataContract{{Name: "_pump_v1", Model: "pump", Version: "v1"}},
				StreamProcessor: []StreamProcessorConfig{streamProcessor("sp-1", "pump", "v1")},
			}

			refs := FindModelReferences(cfg, "pump")
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].Kind).To(Equal(ReferenceKindStreamProcessor))
			Expect(refs[0].Name).To(Equal("sp-1"))
			Expect(refs[0].Detail).To(ContainSubstring("declares model"))
		})

		// Benthos composes the topic from the stored pair, but a config can also name
		// the contract directly. The two can disagree, so both have to match or the
		// reference is missed in one of the two spellings.
		It("matches a config that names the composed address instead", func() {
			cfg := FullConfig{
				Contracts:       []DataContract{{Name: "_pump_v1", Model: "pump", Version: "v1"}},
				StreamProcessor: []StreamProcessorConfig{streamProcessor("sp-2", "_pump_v1", "v1")},
			}

			refs := FindModelReferences(cfg, "pump")
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].Name).To(Equal("sp-2"))
		})

		It("does not match an unrelated model", func() {
			cfg := FullConfig{
				Contracts:       []DataContract{{Name: "_pump_v1", Model: "pump", Version: "v1"}},
				StreamProcessor: []StreamProcessorConfig{streamProcessor("sp-3", "motor", "v1")},
			}

			Expect(FindModelReferences(cfg, "pump")).To(BeEmpty())
		})
	})

	Describe("_refModel pointers", func() {
		It("finds a direct pointer and names the field", func() {
			cfg := FullConfig{Contracts: []DataContract{
				{Model: "pump", Version: "v1"},
				{Name: "_plant_v1", Model: "plant", Version: "v1", Structure: map[string]Field{
					"mainPump": {ModelRef: &ModelRef{Name: "pump", Version: "v1"}},
				}},
			}}

			refs := FindModelReferences(cfg, "pump")
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].Kind).To(Equal(ReferenceKindRefModel))
			Expect(refs[0].Name).To(Equal("plant"))
			Expect(refs[0].Detail).To(ContainSubstring("mainPump"))
		})

		It("follows the pointer transitively", func() {
			// plant -> assembly -> pump. Deleting pump breaks plant too, so a refusal
			// that only looked one level deep would let it through.
			cfg := FullConfig{Contracts: []DataContract{
				{Model: "pump", Version: "v1"},
				{Model: "assembly", Version: "v1", Structure: map[string]Field{
					"pump": {ModelRef: &ModelRef{Name: "pump", Version: "v1"}},
				}},
				{Name: "_plant_v1", Model: "plant", Version: "v1", Structure: map[string]Field{
					"assembly": {ModelRef: &ModelRef{Name: "assembly", Version: "v1"}},
				}},
			}}

			refs := FindModelReferences(cfg, "pump")
			Expect(refs).To(HaveLen(2))

			names := []string{refs[0].Name, refs[1].Name}
			Expect(names).To(ConsistOf("assembly", "plant"))
		})

		It("finds a pointer nested inside subfields", func() {
			cfg := FullConfig{Contracts: []DataContract{
				{Model: "pump", Version: "v1"},
				{Model: "plant", Version: "v1", Structure: map[string]Field{
					"section": {Subfields: map[string]Field{
						"mainPump": {ModelRef: &ModelRef{Name: "pump", Version: "v1"}},
					}},
				}},
			}}

			refs := FindModelReferences(cfg, "pump")
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].Detail).To(ContainSubstring("section.mainPump"))
		})

		It("terminates on a reference cycle", func() {
			// The validator rejects cycles, but a hand-edited config can contain one and
			// the guard still has to answer rather than recurse forever.
			cfg := FullConfig{Contracts: []DataContract{
				{Model: "a", Version: "v1", Structure: map[string]Field{
					"b": {ModelRef: &ModelRef{Name: "b", Version: "v1"}},
				}},
				{Model: "b", Version: "v1", Structure: map[string]Field{
					"a": {ModelRef: &ModelRef{Name: "a", Version: "v1"}},
				}},
			}}

			Expect(FindModelReferences(cfg, "a")).NotTo(BeEmpty())
		})
	})

	Describe("bridges", func() {
		It("finds a bridge publishing to the address", func() {
			cfg := FullConfig{
				Contracts: []DataContract{{Name: "_pump_v1", Model: "pump", Version: "v1"}},
				DataFlow: []DataFlowComponentConfig{
					bridgeNaming("bridge-1", "umh.v1.plant._pump_v1.temperature"),
				},
			}

			refs := FindAddressReferences(cfg, "_pump_v1")
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].Kind).To(Equal(ReferenceKindBridge))
			Expect(refs[0].Name).To(Equal("bridge-1"))
		})

		// The failure that would get this guard switched off: refusing to delete
		// _pump_v1 because some unrelated bridge mentions _pump_v10.
		It("does not match the address inside a longer one", func() {
			cfg := FullConfig{
				Contracts: []DataContract{{Name: "_pump_v1", Model: "pump", Version: "v1"}},
				DataFlow: []DataFlowComponentConfig{
					bridgeNaming("bridge-2", "umh.v1.plant._pump_v10.temperature"),
				},
			}

			Expect(FindAddressReferences(cfg, "_pump_v1")).To(BeEmpty())
		})

		It("matches an address at the very end of the config text", func() {
			cfg := FullConfig{
				Contracts: []DataContract{{Name: "_pump_v1"}},
				DataFlow:  []DataFlowComponentConfig{bridgeNaming("bridge-3", "_pump_v1")},
			}

			Expect(FindAddressReferences(cfg, "_pump_v1")).To(HaveLen(1))
		})

		It("reports nothing for an address no bridge names", func() {
			cfg := FullConfig{
				Contracts: []DataContract{{Name: "_pump_v1"}},
				DataFlow:  []DataFlowComponentConfig{bridgeNaming("bridge-4", "_motor_v1")},
			}

			Expect(FindAddressReferences(cfg, "_pump_v1")).To(BeEmpty())
		})
	})

	Describe("the delimiter anchoring itself", func() {
		DescribeTable("containsToken",
			func(text, token string, expected bool) {
				Expect(containsToken(text, token)).To(Equal(expected))
			},
			Entry("exact match", "_pump_v1", "_pump_v1", true),
			Entry("dot-delimited", "a._pump_v1.b", "_pump_v1", true),
			Entry("quoted", `topic: "_pump_v1"`, "_pump_v1", true),
			Entry("longer version number", "_pump_v10", "_pump_v1", false),
			Entry("longer name", "_pump_v1x", "_pump_v1", false),
			Entry("prefixed by a name rune", "x_pump_v1", "_pump_v1", false),
			Entry("absent", "_motor_v1", "_pump_v1", false),
			Entry("second occurrence matches", "_pump_v10 _pump_v1", "_pump_v1", true),
			Entry("empty token never matches", "anything", "", false),
		)
	})

	Describe("what a refusal says", func() {
		It("names the referrer and the evidence", func() {
			ref := Reference{
				Kind:   ReferenceKindBridge,
				Name:   "bridge-1",
				Detail: `its configuration contains the address "_pump_v1"`,
			}

			Expect(ref.String()).To(ContainSubstring("bridge"))
			Expect(ref.String()).To(ContainSubstring("bridge-1"))
			Expect(ref.String()).To(ContainSubstring("_pump_v1"))
		})

		It("stops naming referrers past a few, so the message stays readable", func() {
			refs := []Reference{
				{Kind: ReferenceKindBridge, Name: "a"},
				{Kind: ReferenceKindBridge, Name: "b"},
				{Kind: ReferenceKindBridge, Name: "c"},
				{Kind: ReferenceKindBridge, Name: "d"},
				{Kind: ReferenceKindBridge, Name: "e"},
			}

			described := describeReferences(refs)
			Expect(described).To(ContainSubstring(`"a"`))
			Expect(described).To(ContainSubstring("and 2 more"))
			Expect(described).NotTo(ContainSubstring(`"e"`))
		})
	})
})
