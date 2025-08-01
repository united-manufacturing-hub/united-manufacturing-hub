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

var _ = Describe("DataContract Configuration", func() {
	var (
		mockFS        *filesystem.MockFileSystem
		configManager *FileConfigManager
		ctx           context.Context
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		configManager = NewFileConfigManager(ctx)
		configManager.WithFileSystemService(mockFS)
		ctx = context.Background()
	})

	Describe("AtomicAddDataContract", func() {
		var (
			validYAMLWithoutDataContracts = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
`
			validYAMLWithDataContracts = `
internal:
  services:
    - name: service1
      desiredState: running
agent:
  metricsPort: 8080
dataContracts:
  - name: existing-contract
    model:
      name: existing-model
      version: v1
`
		)

		Context("when adding a data contract to an empty config", func() {
			var writtenData []byte

			BeforeEach(func() {
				writtenData = nil

				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithoutDataContracts), nil
				})

				mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
					writtenData = data
					return nil
				})

				mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
					return mockFS.NewMockFileInfo("config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
				})
			})

			It("should add the data contract successfully", func() {
				dataContract := DataContractsConfig{
					Name: "test-contract",
					Model: &ModelRef{
						Name:    "test-model",
						Version: "v1",
					},
				}

				err := configManager.AtomicAddDataContract(ctx, dataContract)
				Expect(err).NotTo(HaveOccurred())

				// Verify the written data
				Expect(writtenData).NotTo(BeEmpty())

				// Parse the written data to verify it contains the data contract
				writtenConfig, err := ParseConfig(writtenData, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(writtenConfig.DataContracts).To(HaveLen(1))
				Expect(writtenConfig.DataContracts[0].Name).To(Equal("test-contract"))
				Expect(writtenConfig.DataContracts[0].Model.Name).To(Equal("test-model"))
				Expect(writtenConfig.DataContracts[0].Model.Version).To(Equal("v1"))
			})
		})

		Context("when adding a data contract with duplicate name", func() {
			BeforeEach(func() {
				mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
					return nil
				})

				mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
					return true, nil
				})

				mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
					return []byte(validYAMLWithDataContracts), nil
				})
			})

			It("should return an error", func() {
				dataContract := DataContractsConfig{
					Name: "existing-contract",
					Model: &ModelRef{
						Name:    "different-model",
						Version: "v1",
					},
				}

				err := configManager.AtomicAddDataContract(ctx, dataContract)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("another data contract with name \"existing-contract\" already exists"))
			})
		})
	})
})
