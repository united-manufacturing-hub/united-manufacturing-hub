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

package container_monitor_test

import (
	"context"
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("Container Monitor Service", func() {
	var (
		mockFS      *filesystem.MockFileSystem
		service     container_monitor.Service
		ctx         context.Context
		defaultHWID = "hwid-12345"
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		service = container_monitor.NewContainerMonitorService(mockFS)
		ctx = context.Background()
	})

	Describe("GetStatus", func() {
		Context("when hardware ID file exists", func() {
			BeforeEach(func() {
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == constants.HWIDFilePath {
						return true, nil
					}
					return false, nil
				})

				mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
					if path == constants.HWIDFilePath {
						return []byte(defaultHWID), nil
					}
					return nil, errors.New("file not found")
				})
			})

			It("should return container with the hardware ID from file", func() {
				container, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(container).ToNot(BeNil())
				Expect(container.Hwid).To(Equal(defaultHWID))
			})

			It("should contain CPU metrics", func() {
				container, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(container.CPU).ToNot(BeNil())
				Expect(container.CPU.CoreCount).To(BeNumerically(">", 0))
			})

			It("should contain memory metrics", func() {
				container, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(container.Memory).ToNot(BeNil())
				Expect(container.Memory.CGroupTotalBytes).To(BeNumerically(">", 0))
			})

			It("should contain disk metrics", func() {
				container, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(container.Disk).ToNot(BeNil())
				Expect(container.Disk.DataPartitionTotalBytes).To(BeNumerically(">", 0))
			})

			It("should set architecture from runtime", func() {
				Skip("Skipping architecture test")
				container, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				expectedArch := models.ContainerArchitecture(os.Getenv("GOARCH"))
				if expectedArch == "" {
					expectedArch = models.ArchitectureAmd64 // Default in most test environments
				}
				Expect(container.Architecture).To(Equal(expectedArch))
			})
		})

		Context("when hardware ID file does not exist", func() {
			BeforeEach(func() {
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == constants.HWIDFilePath {
						return false, nil
					}
					return false, nil
				})

				// Mock EnsureDirectory to succeed
				mockFS.WithEnsureDirectoryFunc(func(_ context.Context, path string) error {
					if path == constants.DataMountPath {
						return nil
					}
					return errors.New("failed to ensure directory")
				})

				// Mock WriteFile to store the generated ID
				mockFS.WithWriteFileFunc(func(_ context.Context, path string, data []byte, perm os.FileMode) error {
					if path == constants.HWIDFilePath {
						// Successfully saved, return no error
						return nil
					}
					return errors.New("failed to write file")
				})
			})

			It("should return container with newly generated hardware ID", func() {
				container, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(container).ToNot(BeNil())
				Expect(container.Hwid).To(Not(BeEmpty()))
				Expect(container.Hwid).To(Not(Equal("")))
				Expect(len(container.Hwid)).To(BeNumerically(">=", 32)) // SHA-256 hash is at least 32 bytes when encoded to hex
			})
		})

		Context("when hardware ID file doesn't exist and creation fails", func() {
			BeforeEach(func() {
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == constants.HWIDFilePath {
						return false, nil
					}
					return false, nil
				})

				// Mock EnsureDirectory to fail, forcing fallback
				mockFS.WithEnsureDirectoryFunc(func(_ context.Context, path string) error {
					return errors.New("failed to ensure directory")
				})
			})

			It("should return container with fallback hardware ID", func() {
				container, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(container).ToNot(BeNil())
				Expect(container.Hwid).To(Equal(defaultHWID)) // Fallback value
			})
		})

		Context("when hardware ID file read fails", func() {
			BeforeEach(func() {
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == constants.HWIDFilePath {
						return true, nil
					}
					return false, nil
				})

				mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
					if path == constants.HWIDFilePath {
						return nil, errors.New("read error")
					}
					return nil, errors.New("file not found")
				})
			})

			It("should return container with empty hardware ID", func() {
				container, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(container).ToNot(BeNil())
				Expect(container.Hwid).To(Equal(""))
			})
		})
	})

	Describe("GetHealth", func() {
		Context("when all metrics are normal", func() {
			BeforeEach(func() {
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == constants.HWIDFilePath {
						return true, nil
					}
					return false, nil
				})

				mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
					if path == constants.HWIDFilePath {
						return []byte(defaultHWID), nil
					}
					return nil, errors.New("file not found")
				})
			})

			It("should return healthy status", func() {
				health, err := service.GetHealth(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(health).ToNot(BeNil())
				Expect(health.Category).To(Equal(models.Active))
				Expect(health.ObservedState).To(Equal(constants.ContainerStateRunning))
			})
		})

		// Additional tests for critical conditions would be added here
		// using a real implementation that can simulate high load
	})
})
