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
	"errors"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("DataModel Configuration", func() {
	var (
		mockFS            *filesystem.MockFileSystem
		configManager     *FileConfigManager
		ctx               context.Context
		ctxWithCancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()

		// Create a context with a timeout for cancellation tests
		ctx = context.Background()
		ctxWithCancelFunc = func() {}
	})

	JustBeforeEach(func() {
		configManager = NewFileConfigManager()
		configManager.WithFileSystemService(mockFS)
	})

	AfterEach(func() {
		configManager.Stop()
		ctxWithCancelFunc()
	})

	Describe("parseConfig with DataModels", func() {
		Context("with various YAML inputs containing data models", func() {
			It("should parse valid YAML with data models correctly", func() {
				validYAML := `
internal:
  services:
    - name: service1
      desiredState: running
  redpanda:
    desiredState: running
agent:
  metricsPort: 8080
  location:
    0: Enterprise
    1: Site
dataModels:
  - name: temperature
    version:
      v1:
        structure:
          temperature:
            _payloadshape: timeseries-number
          unit:
            _payloadshape: timeseries-number
`
				config, err := ParseConfig([]byte(validYAML), ctx, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(config.Internal.Services).To(HaveLen(1))
				Expect(config.Internal.Services[0].Name).To(Equal("service1"))
				Expect(config.Internal.Services[0].FSMInstanceConfig.DesiredFSMState).To(Equal("running"))
				Expect(config.Internal.Redpanda.DesiredFSMState).To(Equal("running"))
				Expect(config.Agent.MetricsPort).To(Equal(8080))
				Expect(config.Agent.Location).To(HaveLen(2))
				Expect(config.Agent.Location[0]).To(Equal("Enterprise"))
				Expect(config.Agent.Location[1]).To(Equal("Site"))

				// Test data models parsing
				Expect(config.DataModels).To(HaveLen(1))
				Expect(config.DataModels[0].Name).To(Equal("temperature"))
				Expect(config.DataModels[0].Versions).To(HaveKey("v1"))
				Expect(config.DataModels[0].Versions["v1"].Structure).To(HaveKey("temperature"))
				Expect(config.DataModels[0].Versions["v1"].Structure["temperature"].PayloadShape).To(Equal("timeseries-number"))
				Expect(config.DataModels[0].Versions["v1"].Structure).To(HaveKey("unit"))
				Expect(config.DataModels[0].Versions["v1"].Structure["unit"].PayloadShape).To(Equal("timeseries-number"))
			})

			It("should handle complex nested data model structures", func() {
				complexYAML := `
dataModels:
  - name: complex-model
    version:
      v1:
        structure:
          sensor:
            temp_reading:
              _payloadshape: timeseries-number
            temp_unit:
              _refModel: 
                name: temperature
                version: v1
          metadata:
            _refModel: 
              name: device-info
              version: v1
`
				config, err := ParseConfig([]byte(complexYAML), ctx, false)
				Expect(err).NotTo(HaveOccurred())

				// Test complex data model parsing
				Expect(config.DataModels).To(HaveLen(1))
				Expect(config.DataModels[0].Name).To(Equal("complex-model"))
				Expect(config.DataModels[0].Versions["v1"].Structure).To(HaveKey("sensor"))
				sensorField := config.DataModels[0].Versions["v1"].Structure["sensor"]
				Expect(sensorField.Subfields).To(HaveLen(2))
				Expect(sensorField.Subfields["temp_reading"].PayloadShape).To(Equal("timeseries-number"))
				Expect(sensorField.Subfields["temp_unit"].ModelRef).NotTo(BeNil())
				Expect(sensorField.Subfields["temp_unit"].ModelRef.Name).To(Equal("temperature"))
				Expect(sensorField.Subfields["temp_unit"].ModelRef.Version).To(Equal("v1"))

				Expect(config.DataModels[0].Versions["v1"].Structure).To(HaveKey("metadata"))
				metadataField := config.DataModels[0].Versions["v1"].Structure["metadata"]
				Expect(metadataField.ModelRef).NotTo(BeNil())
				Expect(metadataField.ModelRef.Name).To(Equal("device-info"))
				Expect(metadataField.ModelRef.Version).To(Equal("v1"))
			})

			It("should parse data models with multiple versions", func() {
				multiVersionYAML := `
dataModels:
  - name: sensor-data
    version:
      v1:
        structure:
          value:
            _payloadshape: timeseries-number
      v2:
        structure:
          value:
            _payloadshape: timeseries-number
          timestamp:
            _payloadshape: timeseries-string
          metadata:
            _refModel: 
              name: sensor-metadata
              version: v1
`
				config, err := ParseConfig([]byte(multiVersionYAML), ctx, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(config.DataModels).To(HaveLen(1))
				dm := config.DataModels[0]
				Expect(dm.Name).To(Equal("sensor-data"))
				Expect(dm.Versions).To(HaveLen(2))

				// Check v1
				Expect(dm.Versions).To(HaveKey("v1"))
				v1 := dm.Versions["v1"]
				Expect(v1.Structure).To(HaveLen(1))
				Expect(v1.Structure).To(HaveKey("value"))

				// Check v2
				Expect(dm.Versions).To(HaveKey("v2"))
				v2 := dm.Versions["v2"]
				Expect(v2.Structure).To(HaveLen(3))
				Expect(v2.Structure).To(HaveKey("value"))
				Expect(v2.Structure).To(HaveKey("timestamp"))
				Expect(v2.Structure).To(HaveKey("metadata"))
				Expect(v2.Structure["metadata"].ModelRef).NotTo(BeNil())
				Expect(v2.Structure["metadata"].ModelRef.Name).To(Equal("sensor-metadata"))
				Expect(v2.Structure["metadata"].ModelRef.Version).To(Equal("v1"))
			})
		})
	})

	Describe("AtomicAddDataModel", func() {
		var (
			validYAMLWithoutDataModels = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
`
			validYAMLWithDataModels = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
dataModels:
  - name: existing-model
    version:
      v1:
        structure:
          field1:
            _payloadshape: timeseries-string
`
		)

		Context("when adding a data model to an empty config", func() {
			var writtenData []byte

			BeforeEach(func() {
				writtenData = nil // Reset for each test

				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithoutDataModels), nil
				})

				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					writtenData = data

					return nil
				})

				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
				})
			})

			It("should add the data model successfully", func() {
				dmVersion := DataModelVersion{
					Structure: map[string]Field{
						"temperature": {
							PayloadShape: "timeseries-number",
						},
						"unit": {
							PayloadShape: "timeseries-string",
						},
					},
				}

				Eventually(func() error {
					err := configManager.AtomicAddDataModel(ctx, "temperature", dmVersion, "test description")

					return err
				}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

				// Verify the written data
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify it contains the data model
				writtenConfig, err := ParseConfig(writtenData, ctx, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenConfig.DataModels).To(HaveLen(1))
				Expect(writtenConfig.DataModels[0].Name).To(Equal("temperature"))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveKey("v1_0"))
			})
		})

		Context("when adding a data model with duplicate name", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithDataModels), nil
				})
			})

			It("should return an error", func() {
				dmVersion := DataModelVersion{
					Structure: map[string]Field{
						"field": {
							PayloadShape: "timeseries-string",
						},
					},
				}

				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish
				err := configManager.AtomicAddDataModel(ctx, "existing-model", dmVersion, "test description")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("another data model with name \"existing-model\" already exists"))
			})
		})

		Context("when file system operations fail", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return errors.New("directory creation failed")
				})
			})

			It("should return an error", func() {
				dmVersion := DataModelVersion{
					Structure: map[string]Field{
						"field": {
							PayloadShape: "timeseries-string",
						},
					},
				}

				err := configManager.AtomicAddDataModel(ctx, "test-model", dmVersion, "test description")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get config"))
			})
		})
	})

	Describe("AtomicEditDataModel", func() {
		var (
			validYAMLWithDataModels = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
dataModels:
  - name: temperature
    version:
      v1:
        structure:
          temperature:
            _payloadshape: timeseries-number
`
		)

		Context("when editing an existing data model", func() {
			var writtenData []byte

			BeforeEach(func() {
				writtenData = nil // Reset for each test

				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithDataModels), nil
				})

				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					writtenData = data

					return nil
				})

				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
				})
			})

			It("should add a new version to the existing data model", func() {
				dmVersion := DataModelVersion{
					Structure: map[string]Field{
						"temperature": {
							PayloadShape: "timeseries-number",
						},
						"humidity": {
							PayloadShape: "timeseries-number",
						},
						"unit": {
							PayloadShape: "timeseries-string",
						},
					},
				}

				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish
				err := configManager.AtomicEditDataModel(ctx, "temperature", dmVersion, "test description")

				Expect(err).NotTo(HaveOccurred())

				// Verify the written data
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify it contains both versions
				writtenConfig, err := ParseConfig(writtenData, ctx, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenConfig.DataModels).To(HaveLen(1))
				Expect(writtenConfig.DataModels[0].Name).To(Equal("temperature"))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveLen(2))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveKey("v1"))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveKey("v1_1"))
				Expect(writtenConfig.DataModels[0].Description).To(Equal("test description"))

				// Verify v1_1 has the new structure
				v1_1 := writtenConfig.DataModels[0].Versions["v1_1"]
				Expect(v1_1.Structure).To(HaveLen(3))
				Expect(v1_1.Structure).To(HaveKey("humidity"))
				Expect(v1_1.Structure).To(HaveKey("unit"))
			})
		})

		Context("when editing a legacy model with a single unversioned key", func() {
			var (
				legacySingleVersionYAML = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
dataModels:
  - name: legacy-model
    version:
      v1:
        structure:
          field1:
            _payloadshape: timeseries-string
`
				writtenData []byte
			)

			BeforeEach(func() {
				writtenData = nil // Reset for each test

				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(legacySingleVersionYAML), nil
				})

				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					writtenData = data

					return nil
				})

				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
				})
			})

			It("appends a minor to a legacy single-version model", func() {
				dmVersion := DataModelVersion{
					Structure: map[string]Field{
						"field2": {
							PayloadShape: "timeseries-string",
						},
					},
				}

				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish
				err := configManager.AtomicEditDataModel(ctx, "legacy-model", dmVersion, "legacy upgrade")

				Expect(err).NotTo(HaveOccurred())
				Expect(writtenData).NotTo(BeEmpty())

				writtenConfig, err := ParseConfig(writtenData, ctx, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenConfig.DataModels).To(HaveLen(1))

				versions := writtenConfig.DataModels[0].Versions
				Expect(versions).To(HaveLen(2))
				Expect(versions).To(HaveKey("v1"))
				Expect(versions).To(HaveKey("v1_1"))
				Expect(versions["v1_1"].Structure).To(HaveKey("field2"))
			})
		})

		Context("when editing a legacy model with multiple unversioned keys", func() {
			var (
				legacyMultiVersionYAML = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
dataModels:
  - name: legacy-model
    version:
      v1:
        structure:
          field1:
            _payloadshape: timeseries-string
      v2:
        structure:
          field1:
            _payloadshape: timeseries-string
          field2:
            _payloadshape: timeseries-string
`
				writtenData []byte
			)

			BeforeEach(func() {
				writtenData = nil // Reset for each test

				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(legacyMultiVersionYAML), nil
				})

				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					writtenData = data

					return nil
				})

				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
				})
			})

			It("appends to the highest major of a legacy multi-version model", func() {
				dmVersion := DataModelVersion{
					Structure: map[string]Field{
						"field3": {
							PayloadShape: "timeseries-string",
						},
					},
				}

				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish
				err := configManager.AtomicEditDataModel(ctx, "legacy-model", dmVersion, "legacy upgrade")

				Expect(err).NotTo(HaveOccurred())
				Expect(writtenData).NotTo(BeEmpty())

				writtenConfig, err := ParseConfig(writtenData, ctx, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenConfig.DataModels).To(HaveLen(1))

				versions := writtenConfig.DataModels[0].Versions
				Expect(versions).To(HaveLen(3))
				Expect(versions).To(HaveKey("v1"))
				Expect(versions).To(HaveKey("v2"))
				Expect(versions).To(HaveKey("v2_1"))
				Expect(versions["v1"].Structure).To(HaveLen(1))
				Expect(versions["v2"].Structure).To(HaveLen(2))
				Expect(versions["v2_1"].Structure).To(HaveKey("field3"))
			})
		})

		Context("when editing a non-existent data model", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithDataModels), nil
				})
			})

			It("should return an error", func() {
				dmVersion := DataModelVersion{
					Structure: map[string]Field{
						"field": {
							PayloadShape: "timeseries-string",
						},
					},
				}

				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish
				err := configManager.AtomicEditDataModel(ctx, "non-existent", dmVersion, "test description")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("data model with name \"non-existent\" not found"))
			})
		})
	})

	Describe("AtomicAddDataModelVersionWithContract", func() {
		var (
			validYAMLWithPumpV1 = `
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
`
			validYAMLWithPumpV1AndTakenContract = `
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
dataContracts:
  - name: _pump_v1_1
    model:
      name: pump
      version: v1_1
`
			newVersion = DataModelVersion{
				Structure: map[string]Field{
					"field2": {
						PayloadShape: "timeseries-string",
					},
				},
			}
		)

		Context("when the write succeeds", func() {
			var currentData []byte

			BeforeEach(func() {
				currentData = []byte(validYAMLWithPumpV1)

				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return currentData, nil
				})

				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					currentData = data

					return nil
				})

				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(currentData)), 0644, time.Now(), false), nil
				})
			})

			It("writes the version and its contract in one config write (P16)", func() {
				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish

				key, err := configManager.AtomicAddDataModelVersionWithContract(ctx, "pump", newVersion, "desc")
				Expect(err).NotTo(HaveOccurred())
				Expect(key).To(Equal("v1_1"))

				// Verify the bytes actually written, rather than GetConfig, which
				// can race a background refresh started by an earlier call and
				// briefly return a stale cache.
				Expect(currentData).NotTo(BeEmpty())

				writtenConfig, err := ParseConfig(currentData, ctx, false)
				Expect(err).NotTo(HaveOccurred())

				Expect(writtenConfig.DataModels).To(HaveLen(1))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveKey("v1_1"))
				Expect(writtenConfig.DataModels[0].Description).To(Equal("desc"))
				Expect(writtenConfig.DataContracts).To(ContainElement(HaveField("Name", "_pump_v1_1")))
			})
		})

		Context("when the write fails", func() {
			var currentData []byte

			BeforeEach(func() {
				currentData = []byte(validYAMLWithPumpV1)

				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return currentData, nil
				})

				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(currentData)), 0644, time.Now(), false), nil
				})
			})

			It("leaves neither the version nor a contract behind when the write fails (P16)", func() {
				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish

				// Only now start failing writes, so the data model above was
				// already readable through the primed cache.
				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					return errors.New("mock write failure")
				})

				_, err := configManager.AtomicAddDataModelVersionWithContract(ctx, "pump", newVersion, "desc")
				Expect(err).To(HaveOccurred())

				cfg, err := configManager.GetConfig(ctx, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.DataModels[0].Versions).NotTo(HaveKey("v1_1"))
				Expect(cfg.DataContracts).NotTo(ContainElement(HaveField("Name", "_pump_v1_1")))
			})
		})

		Context("when the contract name is already taken", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithPumpV1AndTakenContract), nil
				})
			})

			It("refuses and leaves the version map unchanged", func() {
				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish

				_, err := configManager.AtomicAddDataModelVersionWithContract(ctx, "pump", newVersion, "desc")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("data contract \"_pump_v1_1\" already exists"))

				cfg, err := configManager.GetConfig(ctx, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.DataModels[0].Versions).To(HaveLen(1))
				Expect(cfg.DataModels[0].Versions).To(HaveKey("v1"))
				Expect(cfg.DataModels[0].Versions).NotTo(HaveKey("v1_1"))
			})
		})
	})

	Describe("AtomicDeleteDataModel", func() {
		var (
			validYAMLWithMultipleDataModels = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
dataModels:
  - name: temperature
    version:
      v1:
        structure:
          temperature:
            _payloadshape: timeseries-number
  - name: pressure
    version:
      v1:
        structure:
          pressure:
            _payloadshape: timeseries-number
`
		)

		Context("when deleting an existing data model", func() {
			var writtenData []byte

			BeforeEach(func() {
				writtenData = nil // Reset for each test

				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithMultipleDataModels), nil
				})

				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					writtenData = data

					return nil
				})

				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
				})
			})

			It("should remove the specified data model", func() {
				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish
				err := configManager.AtomicDeleteDataModel(ctx, "temperature")
				Expect(err).NotTo(HaveOccurred())

				// Verify the written data
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify the correct model was removed
				writtenConfig, err := ParseConfig(writtenData, ctx, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenConfig.DataModels).To(HaveLen(1))
				Expect(writtenConfig.DataModels[0].Name).To(Equal("pressure"))
			})
		})

		Context("when deleting a non-existent data model", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithMultipleDataModels), nil
				})
			})

			It("should return an error", func() {
				_, _ = configManager.GetConfig(ctx, 0) // get the config to trigger the background refresh
				time.Sleep(100 * time.Millisecond)     // wait for the background refresh to finish
				err := configManager.AtomicDeleteDataModel(ctx, "non-existent")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("data model with name \"non-existent\" not found"))
			})
		})
	})
})
