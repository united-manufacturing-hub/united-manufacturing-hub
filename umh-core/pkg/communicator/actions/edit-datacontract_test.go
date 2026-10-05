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

package actions_test

import (
	"encoding/base64"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"gopkg.in/yaml.v3"
)

var _ = Describe("EditDataContractAction", func() {
	var (
		action          *actions.EditDataContractAction
		mockConfigMgr   *config.MockConfigManager
		outboundChannel chan *models.UMHMessage
	)

	existing := func() config.FullConfig {
		return config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{{
				Name:        "pump",
				Description: "Pump telemetry",
				Versions: map[string]config.DataModelVersion{
					"v1": {Structure: map[string]config.Field{"count": {PayloadShape: "timeseries-number"}}},
					"v2": {Structure: map[string]config.Field{"count": {PayloadShape: "timeseries-string"}}},
				},
			}},
		}
	}

	BeforeEach(func() {
		outboundChannel = make(chan *models.UMHMessage, 100)
		mockConfigMgr = config.NewMockConfigManager().WithConfig(existing())

		action = actions.NewEditDataContractAction("test@umh.app", uuid.New(), uuid.New(), outboundChannel, mockConfigMgr)
	})

	AfterEach(func() {
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("rejects a structure that is not valid base64", func() {
			err := action.Parse(map[string]interface{}{"name": "pump", "encodedStructure": "not-base64!!"})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Validate", func() {
		It("rejects a missing name", func() {
			Expect(action.Parse(contractPayload("", "", timeseriesStructure()))).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Name"))
		})

		It("rejects an empty structure", func() {
			Expect(action.Parse(contractPayload("pump", "", map[string]models.Field{}))).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Structure"))
		})

		It("rejects a contract that does not exist", func() {
			Expect(action.Parse(contractPayload("valve", "", timeseriesStructure()))).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`data contract with name "valve" not found`))
		})

		It("rejects a name that exists only as a data model", func() {
			mockConfigMgr.WithConfig(config.FullConfig{
				DataModels: []config.DataModelsConfig{{Name: "valve", Versions: map[string]config.DataModelVersion{"v1": {}}}},
			})
			Expect(action.Parse(contractPayload("valve", "", timeseriesStructure()))).To(Succeed())

			Expect(action.Validate()).ToNot(Succeed())
		})
	})

	Describe("Execute", func() {
		It("appends the next version and leaves the others and the description as they were", func() {
			Expect(action.Parse(contractPayload("pump", "", timeseriesStructure()))).To(Succeed())
			Expect(action.Validate()).To(Succeed())

			result, _, err := action.Execute()
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("version", "v3"))
			Expect(result).To(HaveKeyWithValue("contract", "_pump_v3"))

			cfg, err := mockConfigMgr.GetConfig(GinkgoT().Context(), 0)
			Expect(err).ToNot(HaveOccurred())

			contract := cfg.DataContractsV2[0]
			Expect(contract.Description).To(Equal("Pump telemetry"))
			Expect(contract.Versions).To(HaveLen(3))
			Expect(contract.Versions["v1"]).To(Equal(existing().DataContractsV2[0].Versions["v1"]))
			Expect(contract.Versions["v2"]).To(Equal(existing().DataContractsV2[0].Versions["v2"]))
			Expect(contract.Versions["v3"].Structure).To(HaveKey("pressure"))

			Expect(cfg.DataModels).To(BeEmpty())
			Expect(cfg.DataContracts).To(BeEmpty())
		})
	})
})

var _ = Describe("GetDataContractAction", func() {
	var (
		action          *actions.GetDataContractAction
		mockConfigMgr   *config.MockConfigManager
		outboundChannel chan *models.UMHMessage
	)

	BeforeEach(func() {
		outboundChannel = make(chan *models.UMHMessage, 100)
		mockConfigMgr = config.NewMockConfigManager().WithConfig(config.FullConfig{
			DataContractsV2: []config.DataContractV2Config{{
				Name:        "pump",
				Description: "Pump telemetry",
				Versions: map[string]config.DataModelVersion{
					"v1": {Structure: map[string]config.Field{
						"count": {PayloadShape: "timeseries-number"},
						"motor": {Subfields: map[string]config.Field{
							"rpm": {PayloadShape: "timeseries-number"},
						}},
						"order": {Relational: &config.PayloadShape{
							Fields: map[string]config.PayloadField{"id": {Type: "string"}},
						}},
					}},
				},
			}},
		})

		action = actions.NewGetDataContractAction("test@umh.app", uuid.New(), uuid.New(), outboundChannel, mockConfigMgr)
	})

	AfterEach(func() {
		close(outboundChannel)
	})

	It("rejects a missing name", func() {
		Expect(action.Parse(map[string]interface{}{"name": ""})).To(Succeed())
		Expect(action.Validate()).ToNot(Succeed())
	})

	It("returns every version with its structure as base64 YAML", func() {
		Expect(action.Parse(map[string]interface{}{"name": "pump"})).To(Succeed())
		Expect(action.Validate()).To(Succeed())

		result, _, err := action.Execute()
		Expect(err).ToNot(HaveOccurred())

		response, ok := result.(models.GetDataContractResponse)
		Expect(ok).To(BeTrue())
		Expect(response.Name).To(Equal("pump"))
		Expect(response.Description).To(Equal("Pump telemetry"))
		Expect(response.Versions).To(HaveKey("v1"))

		decoded, err := base64.StdEncoding.DecodeString(response.Versions["v1"].EncodedStructure)
		Expect(err).ToNot(HaveOccurred())

		var structure map[string]models.Field
		Expect(yaml.Unmarshal(decoded, &structure)).To(Succeed())
		Expect(structure["count"].PayloadShape).To(Equal("timeseries-number"))
		Expect(structure["motor"].Subfields["rpm"].PayloadShape).To(Equal("timeseries-number"))
		Expect(structure["order"].Relational.Fields["id"].Type).To(Equal("string"))
	})

	It("fails for a contract that does not exist", func() {
		Expect(action.Parse(map[string]interface{}{"name": "valve"})).To(Succeed())

		_, _, err := action.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})
})
