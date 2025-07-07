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
		// Clean up resources
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
				config, err := ParseConfig([]byte(validYAML), false)
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
				config, err := ParseConfig([]byte(complexYAML), false)
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
				config, err := ParseConfig([]byte(multiVersionYAML), false)
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

				err := configManager.AtomicAddDataModel(ctx, "temperature", dmVersion)
				Expect(err).NotTo(HaveOccurred())

				// Verify the written data
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify it contains the data model
				writtenConfig, err := ParseConfig(writtenData, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenConfig.DataModels).To(HaveLen(1))
				Expect(writtenConfig.DataModels[0].Name).To(Equal("temperature"))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveKey("v1"))
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

				err := configManager.AtomicAddDataModel(ctx, "existing-model", dmVersion)
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

				err := configManager.AtomicAddDataModel(ctx, "test-model", dmVersion)
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

				err := configManager.AtomicEditDataModel(ctx, "temperature", dmVersion)
				Expect(err).NotTo(HaveOccurred())

				// Verify the written data
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify it contains both versions
				writtenConfig, err := ParseConfig(writtenData, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenConfig.DataModels).To(HaveLen(1))
				Expect(writtenConfig.DataModels[0].Name).To(Equal("temperature"))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveLen(2))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveKey("v1"))
				Expect(writtenConfig.DataModels[0].Versions).To(HaveKey("v2"))

				// Verify v2 has the new structure
				v2 := writtenConfig.DataModels[0].Versions["v2"]
				Expect(v2.Structure).To(HaveLen(3))
				Expect(v2.Structure).To(HaveKey("humidity"))
				Expect(v2.Structure).To(HaveKey("unit"))
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

				err := configManager.AtomicEditDataModel(ctx, "non-existent", dmVersion)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("data model with name \"non-existent\" not found"))
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
				err := configManager.AtomicDeleteDataModel(ctx, "temperature")
				Expect(err).NotTo(HaveOccurred())

				// Verify the written data
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify the correct model was removed
				writtenConfig, err := ParseConfig(writtenData, false)
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
				err := configManager.AtomicDeleteDataModel(ctx, "non-existent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("data model with name \"non-existent\" not found"))
			})
		})
	})
})
