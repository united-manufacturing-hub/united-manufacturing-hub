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
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("Container Monitor Service", func() {
	var (
		service      *container_monitor.ContainerMonitorService
		mockFS       *filesystem.MockFileSystem
		ctx          context.Context
		defaultHWID  = "hwid-12345"
		testDataPath string
		err          error
	)

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		ctx = context.Background()

		// Create a temporary directory for the test
		testDataPath, err = os.MkdirTemp("", "container-monitor-test")
		Expect(err).NotTo(HaveOccurred())

		// Create the service with the test data path
		service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)
	})

	AfterEach(func() {
		// Clean up the temporary directory
		err := os.RemoveAll(testDataPath)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("GetStatus", func() {
		Context("when hardware ID file exists", func() {
			It("should return container with the hardware ID from file", func() {
				// Setup the mock to say the file exists
				hwidPath := filepath.Join(testDataPath, "hwid")
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == hwidPath {
						return true, nil
					}
					return false, nil
				})
				mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
					if path == hwidPath {
						return []byte("test-hwid-123"), nil
					}
					return nil, errors.New("file not found")
				})

				// Call the service
				container, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(container).NotTo(BeNil())
				Expect(container.Hwid).To(Equal("test-hwid-123"))
			})

			It("should contain CPU metrics", func() {
				status, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(status.CPU).ToNot(BeNil())
				Expect(status.CPU.CoreCount).To(BeNumerically(">", 0))
			})

			It("should contain memory metrics", func() {
				status, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(status.Memory).ToNot(BeNil())
				Expect(status.Memory.CGroupTotalBytes).To(BeNumerically(">", 0))
			})

			It("should contain disk metrics", func() {
				Skip("Skipping disk metrics test as this cannot be mocked, gopsutil cannot be used with a mock")
				status, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(status.Disk).ToNot(BeNil())
				Expect(status.Disk.DataPartitionTotalBytes).To(BeNumerically(">", 0))
			})

			It("should set architecture from runtime", func() {
				Skip("Skipping architecture test")
				status, err := service.GetStatus(ctx)
				Expect(err).ToNot(HaveOccurred())
				expectedArch := models.ContainerArchitecture(os.Getenv("GOARCH"))
				if expectedArch == "" {
					expectedArch = models.ArchitectureAmd64 // Default in most test environments
				}
				Expect(status.Architecture).To(Equal(expectedArch))
			})
		})

		Context("when hardware ID file does not exist", func() {
			It("should generate a new hardware ID", func() {
				// Setup the mock to say the file doesn't exist
				hwidPath := filepath.Join(testDataPath, "hwid")
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == hwidPath {
						return false, nil
					}
					return false, nil
				})
				mockFS.WithEnsureDirectoryFunc(func(_ context.Context, path string) error {
					if path == testDataPath {
						return nil
					}
					return errors.New("failed to ensure directory")
				})
				mockFS.WithWriteFileFunc(func(_ context.Context, path string, data []byte, perm os.FileMode) error {
					if path == hwidPath {
						return nil
					}
					return errors.New("failed to write file")
				})

				// Call the service
				container, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(container).NotTo(BeNil())
				Expect(container.Hwid).NotTo(BeEmpty())
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
				// The file exists but reading fails
				hwidPath := filepath.Join(testDataPath, "hwid")
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == hwidPath {
						return true, nil
					}
					return false, nil
				})

				mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
					if path == hwidPath {
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

			It("should return valid health status", func() {
				// Health status depends on current system resource usage
				// and can be either Active (normal usage) or Degraded (high resource usage)
				Eventually(func() models.HealthCategory {
					status, err := service.GetStatus(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(status).ToNot(BeNil())
					GinkgoWriter.Printf("GetStatus: %+v\n", status)
					return status.OverallHealth
				}).Should(Or(
					Equal(models.Active),
					Equal(models.Degraded),
				))
			})
		})

		// Additional tests for critical conditions would be added here
		// using a real implementation that can simulate high load
	})

	// This test is designed to be run manually to check real system metrics
	Describe("MANUAL_TEST: Real System Container Status", func() {
		BeforeEach(func() {
			Skip("This test is meant to be run manually. Remove this Skip() to run.")
		})

		It("should print real container metrics for manual inspection", func() {
			// Use the real filesystem implementation instead of mocks
			realFS := filesystem.NewDefaultService()
			realService := container_monitor.NewContainerMonitorService(realFS)
			realService.SetDataPath("/workspaces/united-manufacturing-hub")

			container, err := realService.GetStatus(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(container).ToNot(BeNil())

			// Print all metrics for manual inspection
			GinkgoWriter.Printf("\n=== CONTAINER MONITOR REAL METRICS ===\n")
			GinkgoWriter.Printf("Hardware ID: %s\n", container.Hwid)
			GinkgoWriter.Printf("Architecture: %s\n", container.Architecture)

			GinkgoWriter.Printf("\n--- CPU Metrics ---\n")
			if container.CPU != nil {
				GinkgoWriter.Printf("Core Count: %d\n", container.CPU.CoreCount)
				GinkgoWriter.Printf("Total Usage (mCPU): %.2f\n", container.CPU.TotalUsageMCpu)
				GinkgoWriter.Printf("Usage Per Core: %.2f%%\n", (container.CPU.TotalUsageMCpu/1000.0)/float64(container.CPU.CoreCount)*100.0)
			} else {
				GinkgoWriter.Printf("CPU metrics unavailable\n")
			}

			GinkgoWriter.Printf("\n--- Memory Metrics ---\n")
			if container.Memory != nil {
				GinkgoWriter.Printf("CGroup Total: %d bytes (%.2f GB)\n",
					container.Memory.CGroupTotalBytes,
					float64(container.Memory.CGroupTotalBytes)/1024/1024/1024)
				GinkgoWriter.Printf("CGroup Used: %d bytes (%.2f GB)\n",
					container.Memory.CGroupUsedBytes,
					float64(container.Memory.CGroupUsedBytes)/1024/1024/1024)
				GinkgoWriter.Printf("Memory Usage: %.2f%%\n",
					float64(container.Memory.CGroupUsedBytes)/float64(container.Memory.CGroupTotalBytes)*100)
			} else {
				GinkgoWriter.Printf("Memory metrics unavailable\n")
			}

			GinkgoWriter.Printf("\n--- Disk Metrics ---\n")
			if container.Disk != nil {
				GinkgoWriter.Printf("Data Partition Total: %d bytes (%.2f GB)\n",
					container.Disk.DataPartitionTotalBytes,
					float64(container.Disk.DataPartitionTotalBytes)/1024/1024/1024)
				GinkgoWriter.Printf("Data Partition Used: %d bytes (%.2f GB)\n",
					container.Disk.DataPartitionUsedBytes,
					float64(container.Disk.DataPartitionUsedBytes)/1024/1024/1024)
				GinkgoWriter.Printf("Disk Usage: %.2f%%\n",
					float64(container.Disk.DataPartitionUsedBytes)/float64(container.Disk.DataPartitionTotalBytes)*100)
			} else {
				GinkgoWriter.Printf("Disk metrics unavailable - /data directory may not exist\n")
			}

			// Get health info too
			health, err := realService.GetHealth(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(health).ToNot(BeNil())

			GinkgoWriter.Printf("\n--- Health Status ---\n")
			GinkgoWriter.Printf("Category: %d\n", health.Category)
			GinkgoWriter.Printf("Observed State: %s\n", health.ObservedState)
			GinkgoWriter.Printf("Message: %s\n", health.Message)
			GinkgoWriter.Printf("=================================\n")
		})
	})
})
