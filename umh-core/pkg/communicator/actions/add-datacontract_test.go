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
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"gopkg.in/yaml.v3"
)

// contractPayload builds the wire payload the router hands to Parse.
func contractPayload(name, description string, structure map[string]models.Field) map[string]interface{} {
	yamlData, err := yaml.Marshal(structure)
	Expect(err).ToNot(HaveOccurred())

	return map[string]interface{}{
		"encodedStructure": base64.StdEncoding.EncodeToString(yamlData),
		"description":      description,
		"name":             name,
	}
}

func timeseriesStructure() map[string]models.Field {
	return map[string]models.Field{
		"pressure": {PayloadShape: "timeseries-number"},
	}
}

var _ = Describe("AddDataContractAction", func() {
	var (
		action          *actions.AddDataContractAction
		mockConfigMgr   *config.MockConfigManager
		outboundChannel chan *models.UMHMessage
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
	)

	BeforeEach(func() {
		userEmail = "test@umh.app"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 100)
		mockConfigMgr = config.NewMockConfigManager().WithConfig(config.FullConfig{})

		action = actions.NewAddDataContractAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfigMgr)
	})

	AfterEach(func() {
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("decodes the base64 YAML structure", func() {
			err := action.Parse(contractPayload("pump", "Pump telemetry", timeseriesStructure()))
			Expect(err).ToNot(HaveOccurred())
		})

		It("rejects a structure that is not valid base64", func() {
			err := action.Parse(map[string]interface{}{
				"name":             "pump",
				"encodedStructure": "not-base64!!",
			})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Validate", func() {
		It("accepts a contract with a name and a structure", func() {
			Expect(action.Parse(contractPayload("pump", "", timeseriesStructure()))).To(Succeed())
			Expect(action.Validate()).To(Succeed())
		})

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

		It("rejects a name longer than the table-name ceiling", func() {
			Expect(action.Parse(contractPayload(strings.Repeat("a", 54), "", timeseriesStructure()))).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("54 characters"))
		})

		It("rejects a name already used by a data model", func() {
			mockConfigMgr.WithConfig(config.FullConfig{
				DataModels: []config.DataModelsConfig{{
					Name:     "pump",
					Versions: map[string]config.DataModelVersion{"v1": {}},
				}},
			})
			Expect(action.Parse(contractPayload("pump", "", timeseriesStructure()))).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`a data model named "pump" already exists`))
		})

		It("rejects a name whose address a v1 data contract already uses", func() {
			mockConfigMgr.WithConfig(config.FullConfig{
				DataContracts: []config.DataContractsConfig{{Name: "_pump_v3"}},
			})
			Expect(action.Parse(contractPayload("pump", "", timeseriesStructure()))).To(Succeed())

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`"_pump_v3" is already in use`))
		})
	})

	Describe("Execute", func() {
		It("writes only to the data contracts section", func() {
			Expect(action.Parse(contractPayload("pump", "Pump telemetry", timeseriesStructure()))).To(Succeed())
			Expect(action.Validate()).To(Succeed())

			_, _, err := action.Execute()
			Expect(err).ToNot(HaveOccurred())

			cfg, err := mockConfigMgr.GetConfig(GinkgoT().Context(), 0)
			Expect(err).ToNot(HaveOccurred())

			Expect(cfg.DataContractsV2).To(HaveLen(1))
			Expect(cfg.DataContractsV2[0].Name).To(Equal("pump"))
			Expect(cfg.DataContractsV2[0].Description).To(Equal("Pump telemetry"))
			Expect(cfg.DataContractsV2[0].Versions).To(HaveKey("v1"))

			// The older sections must stay untouched: the two concepts never
			// write into each other.
			Expect(cfg.DataModels).To(BeEmpty())
			Expect(cfg.DataContracts).To(BeEmpty())
		})

		It("refuses a name that is already taken in the data contracts section", func() {
			mockConfigMgr.WithConfig(config.FullConfig{
				DataContractsV2: []config.DataContractV2Config{{
					Name:     "pump",
					Versions: map[string]config.DataModelVersion{"v1": {}},
				}},
			})

			Expect(action.Parse(contractPayload("pump", "", timeseriesStructure()))).To(Succeed())
			Expect(action.Validate()).To(Succeed())

			_, _, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})
	})
})
