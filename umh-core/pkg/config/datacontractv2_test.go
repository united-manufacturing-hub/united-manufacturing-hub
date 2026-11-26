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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("DataContractV2 address clashes", func() {
	Describe("CheckLegacyAddressFree", func() {
		It("rejects a name a data model already uses", func() {
			cfg := FullConfig{DataModels: []DataModelsConfig{{Name: "pump"}}}

			err := CheckLegacyAddressFree(cfg, "pump")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("a data model named \"pump\" already exists"))
		})

		It("rejects a name whose address a dataContracts entry already uses", func() {
			cfg := FullConfig{DataContracts: []DataContractsConfig{{Name: "_pump_v3"}}}

			err := CheckLegacyAddressFree(cfg, "pump")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("\"_pump_v3\" is already in use"))
		})

		It("accepts names that only share a prefix", func() {
			cfg := FullConfig{
				DataModels:    []DataModelsConfig{{Name: "pump-station"}},
				DataContracts: []DataContractsConfig{{Name: "_pump-station_v1"}, {Name: "_pump_v1_extra"}},
			}

			Expect(CheckLegacyAddressFree(cfg, "pump")).To(Succeed())
		})
	})

	Describe("CheckDataContractV2NameFree", func() {
		It("rejects a data model name a v2 contract already uses", func() {
			cfg := FullConfig{DataContractsV2: []DataContractV2Config{{Name: "pump"}}}

			err := CheckDataContractV2NameFree(cfg, "pump")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("a data contract named \"pump\" already exists"))
		})

		It("accepts a name no v2 contract uses", func() {
			cfg := FullConfig{DataContractsV2: []DataContractV2Config{{Name: "pump"}}}

			Expect(CheckDataContractV2NameFree(cfg, "valve")).To(Succeed())
		})
	})

	Describe("NextDataContractV2VersionKey", func() {
		It("starts at v1", func() {
			Expect(NextDataContractV2VersionKey(nil)).To(Equal("v1"))
		})

		It("goes one above the highest number, not the count", func() {
			versions := map[string]DataModelVersion{"v1": {}, "v2": {}, "v10": {}}
			Expect(NextDataContractV2VersionKey(versions)).To(Equal("v11"))
		})

		It("ignores keys that are not v<number>", func() {
			versions := map[string]DataModelVersion{"v2": {}, "draft": {}, "v3-beta": {}}
			Expect(NextDataContractV2VersionKey(versions)).To(Equal("v3"))
		})
	})

	Describe("addDataContractV2Version", func() {
		It("adds the next version without changing the others", func() {
			cfg := FullConfig{DataContractsV2: []DataContractV2Config{{
				Name:     "pump",
				Versions: map[string]DataModelVersion{"v1": {Structure: map[string]Field{"a": {PayloadShape: "timeseries-number"}}}},
			}}}
			original := cfg.DataContractsV2[0].Versions

			key, err := addDataContractV2Version(&cfg, "pump", DataModelVersion{Structure: map[string]Field{"b": {PayloadShape: "timeseries-string"}}})
			Expect(err).ToNot(HaveOccurred())
			Expect(key).To(Equal("v2"))
			Expect(cfg.DataContractsV2[0].Versions).To(HaveLen(2))
			Expect(cfg.DataContractsV2[0].Versions["v1"].Structure).To(HaveKey("a"))
			Expect(original).To(HaveLen(1))
		})

		It("fails for a contract that does not exist", func() {
			_, err := addDataContractV2Version(&FullConfig{}, "pump", DataModelVersion{})
			Expect(err).To(MatchError(ContainSubstring(`data contract with name "pump" not found`)))
		})
	})

	Describe("ContractNameClashes", func() {
		It("lists a v2 contract that shares its name with a data model or a contract address", func() {
			cfg := FullConfig{
				DataModels:      []DataModelsConfig{{Name: "pump"}},
				DataContracts:   []DataContractsConfig{{Name: "_valve_v2"}},
				DataContractsV2: []DataContractV2Config{{Name: "pump"}, {Name: "valve"}, {Name: "motor"}},
			}

			clashes := ContractNameClashes(cfg)
			Expect(clashes).To(HaveLen(2))
			Expect(clashes[0]).To(ContainSubstring(`data contract "pump": a data model named "pump"`))
			Expect(clashes[1]).To(ContainSubstring(`data contract "valve": the contract address "_valve_v2"`))
		})

		It("returns nothing for a config without clashes", func() {
			cfg := FullConfig{
				DataModels:      []DataModelsConfig{{Name: "pump"}},
				DataContractsV2: []DataContractV2Config{{Name: "valve"}},
			}

			Expect(ContractNameClashes(cfg)).To(BeEmpty())
		})
	})

	Describe("WriteYAMLConfigFromString", func() {
		const pumpModel = `
agent:
  metricsPort: 8080
dataModels:
  - name: pump
    version:
      v1:
        structure:
          field1:
            _payloadshape: timeseries-string
`
		const pumpContract = `
dataContractsV2:
  - name: pump
    version:
      v1:
        structure:
          field1:
            _payloadshape: timeseries-string
`

		var (
			mockFS        *filesystem.MockFileSystem
			configManager *FileConfigManager
			ctx           context.Context
			currentYAML   string
			written       []string
		)

		setUp := func(current string) {
			currentYAML = current
			written = nil

			mockFS = filesystem.NewMockFileSystem()
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error { return nil })
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) { return true, nil })
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				return []byte(currentYAML), nil
			})
			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				if path == DefaultConfigPath {
					written = append(written, string(data))
				}

				return nil
			})

			configManager = NewFileConfigManager()
			configManager.WithFileSystemService(mockFS)
		}

		BeforeEach(func() {
			ctx = context.Background()
		})

		AfterEach(func() {
			configManager.Stop()
		})

		It("rejects a save that gives a v2 contract a data model's name", func() {
			setUp(pumpModel)

			err := configManager.WriteYAMLConfigFromString(ctx, pumpModel+pumpContract, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`data contract "pump": a data model named "pump" already exists`))
			Expect(written).To(BeEmpty())
		})

		It("keeps a clash the current config already holds", func() {
			setUp(pumpModel + pumpContract)

			err := configManager.WriteYAMLConfigFromString(ctx, pumpModel+pumpContract, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(written).To(HaveLen(1))
		})

		It("saves a config without clashes", func() {
			setUp(pumpModel)

			err := configManager.WriteYAMLConfigFromString(ctx, pumpModel, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(written).To(HaveLen(1))
		})
	})

	Describe("atomic adds", func() {
		const yamlWithBothConcepts = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
dataModels:
  - name: pump
    version:
      v1:
        structure:
          field1:
            _payloadshape: timeseries-string
dataContractsV2:
  - name: valve
    version:
      v1:
        structure:
          field1:
            _payloadshape: timeseries-string
`

		var (
			mockFS        *filesystem.MockFileSystem
			configManager *FileConfigManager
			ctx           context.Context
			dmVersion     DataModelVersion
		)

		BeforeEach(func() {
			ctx = context.Background()
			dmVersion = DataModelVersion{Structure: map[string]Field{"field": {PayloadShape: "timeseries-string"}}}

			mockFS = filesystem.NewMockFileSystem()
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error { return nil })
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) { return true, nil })
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				return []byte(yamlWithBothConcepts), nil
			})

			configManager = NewFileConfigManager()
			configManager.WithFileSystemService(mockFS)

			_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
			time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish
		})

		AfterEach(func() {
			configManager.Stop()
		})

		It("rejects a v2 contract named after an existing data model", func() {
			err := configManager.AtomicAddDataContractV2(ctx, "pump", dmVersion, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("a data model named \"pump\" already exists"))
		})

		It("rejects a data model named after an existing v2 contract", func() {
			err := configManager.AtomicAddDataModel(ctx, "valve", dmVersion, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("a data contract named \"valve\" already exists"))
		})
	})
})
