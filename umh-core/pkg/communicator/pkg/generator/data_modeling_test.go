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

package generator_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// pumpContract is a two-version data contract used across the specs below.
func pumpContract() config.DataContractV2Config {
	return config.DataContractV2Config{
		Name:        "pump",
		Description: "pump from vendor ABC",
		Versions: map[string]config.DataModelVersion{
			"v1": {Structure: map[string]config.Field{
				"pressure": {PayloadShape: "timeseries-number"},
			}},
			"v2": {Structure: map[string]config.Field{
				"pressure":    {PayloadShape: "timeseries-number"},
				"temperature": {PayloadShape: "timeseries-number"},
			}},
		},
	}
}

var _ = Describe("DataContractsV2FromConfig", func() {
	var logger *zap.SugaredLogger

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
	})

	build := func(cfg config.FullConfig) []models.DataContractV2 {
		manager := config.NewMockConfigManager().WithConfig(cfg)

		contracts, err := generator.DataContractsV2FromConfig(context.Background(), manager, logger)
		Expect(err).NotTo(HaveOccurred())

		return contracts
	}

	It("reports a contract with every version and its address", func() {
		contracts := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{pumpContract()},
		})

		Expect(contracts).To(HaveLen(1))
		Expect(contracts[0].Name).To(Equal("pump"))
		Expect(contracts[0].Description).To(Equal("pump from vendor ABC"))
		Expect(contracts[0].LatestVersion).To(Equal("v2"))
		Expect(contracts[0].Hash).NotTo(BeEmpty())
		Expect(contracts[0].Versions).To(HaveLen(2))
		Expect(contracts[0].Versions[0].Version).To(Equal("v1"))
		Expect(contracts[0].Versions[0].Contract).To(Equal("_pump_v1"))
		Expect(contracts[0].Versions[1].Version).To(Equal("v2"))
		Expect(contracts[0].Versions[1].Contract).To(Equal("_pump_v2"))
	})

	It("ignores the older dataModels and dataContracts sections entirely", func() {
		contracts := build(config.FullConfig{
			DataModels: []config.DataModelsConfig{{
				Name: "legacy",
				Versions: map[string]config.DataModelVersion{
					"v1": {Structure: map[string]config.Field{
						"value": {PayloadShape: "timeseries-number"},
					}},
				},
			}},
			DataContracts: []config.DataContractsConfig{{
				Name:  "_legacy_v1",
				Model: &config.ModelRef{Name: "legacy", Version: "v1"},
			}},
		})

		Expect(contracts).To(BeEmpty())
	})

	It("reports only what the data contracts section declares, alongside the old sections", func() {
		contracts := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{pumpContract()},
			DataModels: []config.DataModelsConfig{{
				Name:     "legacy",
				Versions: map[string]config.DataModelVersion{"v1": {}},
			}},
			DataContracts: []config.DataContractsConfig{{
				Name:  "_legacy_v1",
				Model: &config.ModelRef{Name: "legacy", Version: "v1"},
			}},
		})

		Expect(contracts).To(HaveLen(1))
		Expect(contracts[0].Name).To(Equal("pump"))
	})

	It("orders versions by number rather than lexically", func() {
		contracts := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{{
				Name: "press",
				Versions: map[string]config.DataModelVersion{
					"v1":  {},
					"v2":  {},
					"v10": {},
				},
			}},
		})

		Expect(contracts[0].LatestVersion).To(Equal("v10"))

		versions := []string{}
		for _, version := range contracts[0].Versions {
			versions = append(versions, version.Version)
		}

		Expect(versions).To(Equal([]string{"v1", "v2", "v10"}))
	})

	It("sorts entries by name", func() {
		contracts := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{
				{Name: "zebra"},
				{Name: "alpha"},
				{Name: "middle"},
			},
		})

		names := []string{}
		for _, contract := range contracts {
			names = append(names, contract.Name)
		}

		Expect(names).To(Equal([]string{"alpha", "middle", "zebra"}))
	})

	It("gives a contract with no versions an empty hash and latest version", func() {
		contracts := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{{Name: "empty"}},
		})

		Expect(contracts).To(HaveLen(1))
		Expect(contracts[0].Hash).To(BeEmpty())
		Expect(contracts[0].LatestVersion).To(BeEmpty())
		Expect(contracts[0].Versions).To(BeEmpty())
	})

	It("gives the same definition the same hash, and a changed one a different hash", func() {
		first := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{pumpContract()},
		})
		same := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{pumpContract()},
		})

		drifted := pumpContract()
		drifted.Versions["v2"] = config.DataModelVersion{
			Structure: map[string]config.Field{
				"pressure": {PayloadShape: "timeseries-number"},
			},
		}
		changed := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{drifted},
		})

		Expect(first[0].Hash).To(Equal(same[0].Hash))
		Expect(first[0].Hash).NotTo(Equal(changed[0].Hash))
	})

	It("does not hash two different names to the same value", func() {
		renamed := pumpContract()
		renamed.Name = "compressor"

		first := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{pumpContract()},
		})
		second := build(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{renamed},
		})

		Expect(first[0].Hash).NotTo(Equal(second[0].Hash))
	})

	Describe("hash collisions", func() {
		hashOf := func(structure map[string]config.Field) string {
			contracts := build(config.FullConfig{
				DataContractsV2: []config.DataContractV2Config{{
					Name:     "pump",
					Versions: map[string]config.DataModelVersion{"v1": {Structure: structure}},
				}},
			})

			return contracts[0].Hash
		}

		It("tells a folder holding a field apart from one field with the joined name", func() {
			folder := hashOf(map[string]config.Field{
				"a": {Subfields: map[string]config.Field{"b": {PayloadShape: "timeseries-number"}}},
			})
			joined := hashOf(map[string]config.Field{
				"ab": {PayloadShape: "timeseries-number"},
			})

			Expect(folder).NotTo(Equal(joined))
		})

		It("tells a nested field apart from a sibling field", func() {
			nested := hashOf(map[string]config.Field{
				"a": {Subfields: map[string]config.Field{"b": {PayloadShape: "timeseries-number"}}},
			})
			siblings := hashOf(map[string]config.Field{
				"a": {},
				"b": {PayloadShape: "timeseries-number"},
			})

			Expect(nested).NotTo(Equal(siblings))
		})

		It("changes when a relational definition changes", func() {
			relational := func(fieldType string) map[string]config.Field {
				return map[string]config.Field{
					"order": {Relational: &config.PayloadShape{
						Fields: map[string]config.PayloadField{"id": {Type: fieldType}},
					}},
				}
			}

			Expect(hashOf(relational("string"))).To(Equal(hashOf(relational("string"))))
			Expect(hashOf(relational("string"))).NotTo(Equal(hashOf(relational("number"))))
			Expect(hashOf(relational("string"))).NotTo(Equal(hashOf(map[string]config.Field{"order": {}})))
		})

		It("ignores the relational description, which does not change validation", func() {
			described := map[string]config.Field{
				"order": {Relational: &config.PayloadShape{
					Description: "Work order",
					Fields:      map[string]config.PayloadField{"id": {Type: "string"}},
				}},
			}
			plain := map[string]config.Field{
				"order": {Relational: &config.PayloadShape{
					Fields: map[string]config.PayloadField{"id": {Type: "string"}},
				}},
			}

			Expect(hashOf(described)).To(Equal(hashOf(plain)))
		})
	})

	It("returns an empty slice for an empty config", func() {
		Expect(build(config.FullConfig{})).To(BeEmpty())
	})

	It("propagates a config read failure", func() {
		manager := config.NewMockConfigManager().WithConfigError(errors.New("boom"))

		contracts, err := generator.DataContractsV2FromConfig(context.Background(), manager, logger)
		Expect(err).To(HaveOccurred())
		Expect(contracts).To(BeEmpty())
	})
})
