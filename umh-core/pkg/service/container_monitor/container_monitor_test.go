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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

		// Pins the absence of a raw-usage degrade override in GetStatus:
		// a high-usage NON-throttled container must report CPUHealth ==
		// Active, matching cpuStat.Health (busy is not sick). An override
		// that sets CPUHealth = Degraded purely from cpuPercent > 70
		// contradicts the Active cpuStat.Health and causes downstream
		// bridge-blocking under normal high load. OverallHealth is not
		// asserted because getMemoryMetrics and getDiskMetrics read the
		// real host via gopsutil, so a loaded CI box can flip those
		// subsystems to Degraded independently of CPU.
		Context("when cgroup CPU usage is high but not throttled", func() {
			It("reports Active CPUHealth, consistent with CPU.Health", func() {
				mockFS = filesystem.NewMockFileSystem()
				ctx = context.Background()

				testDataPath, err := os.MkdirTemp("", "rung2-cpu-high-usage")
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = os.RemoveAll(testDataPath) }()

				// cpu.max: "200000 100000" => quota 200000 / period 100000
				// = 2.0 cores. With ~1.6 cores of sustained usage this
				// yields cpuPercent ≈ 80%, comfortably above the 70%
				// CPUHighThresholdPercent that the removed override used.
				const cpuMax = "200000 100000\n"

				// cpuStatUsec is advanced between GetStatus calls so the
				// cgroup Sampler measures a known usage_usec delta over
				// wall-clock. nr_throttled stays at 0 across both
				// snapshots so the throttle ratio is 0 (well below the
				// 0.05 threshold), i.e. NOT throttled.
				cpuStatUsec := int64(0)
				nrPeriods := int64(100)

				mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
					switch path {
					case "/sys/fs/cgroup/cpu.max":
						return []byte(cpuMax), nil
					case "/sys/fs/cgroup/cpu.stat":
						return []byte(fmt.Sprintf(
							"usage_usec %d\nnr_periods %d\nnr_throttled 0\nthrottled_usec 0\n",
							cpuStatUsec, nrPeriods,
						)), nil
					default:
						return nil, errors.New("file not found: " + path)
					}
				})

				service = container_monitor.NewContainerMonitorServiceWithPath(mockFS, testDataPath)

				// First GetStatus: establishes the Sampler's baseline
				// usage_usec and the throttle window's first snapshot.
				_, err = service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Advance usage_usec by 1_600_000 (1.6 core-seconds) and
				// the period counter, then let ~1 second of wall-clock
				// elapse so the Sampler reports ~1.6 cores of usage.
				// cpuPercent = (1600 mCPU / 1000) / 2.0 * 100 = 80%.
				cpuStatUsec = 1_600_000
				nrPeriods = 200
				time.Sleep(1 * time.Second)

				status, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())
				Expect(status.CPU).NotTo(BeNil())

				// Sanity: the cgroup scenario actually produced high
				// usage (cpuPercent > 70). Without this guard the
				// regression pin below could pass vacuously.
				effectiveCores := status.CPU.CgroupCores
				if effectiveCores <= 0 {
					effectiveCores = float64(status.CPU.CoreCount)
				}
				cpuPercent := (status.CPU.TotalUsageMCpu / 1000.0) / effectiveCores * 100.0
				Expect(cpuPercent).To(BeNumerically(">", constants.CPUHighThresholdPercent),
					"test setup must produce high usage; got cpuPercent=%.1f", cpuPercent)

				// Regression pin: high-usage non-throttled container
				// must NOT be Degraded.
				Expect(status.CPUHealth).To(Equal(models.Active),
					"CPUHealth must be Active for high-usage non-throttled container (busy is not sick)")
				Expect(status.CPU.Health).NotTo(BeNil())
				Expect(status.CPU.Health.Category).To(Equal(models.Active),
					"CPU.Health.Category must be Active when not throttled")

				// The two verdicts must agree: while CPUHealth is Active
				// the health message must read as a healthy verdict, not a
				// degraded one. This is the contradiction that produced the
				// downstream "CPU degraded: CPU utilization normal"
				// false-block. The healthy budget message names the
				// degraded THRESHOLD ("before it is marked degraded",
				// "degraded below 0"), so pin the healthy headline prefix
				// and guard against a "CPU degraded" verdict rather than the
				// bare word "degraded".
				Expect(status.CPU.Health.Message).To(HavePrefix("CPU healthy"),
					"CPU.Health.Message must render the healthy headline when CPUHealth is Active")
				Expect(strings.ToLower(status.CPU.Health.Message)).NotTo(ContainSubstring("cpu degraded"),
					"CPU.Health.Message must not read as a degraded verdict when CPUHealth is Active")
			})
		})

		Context("when the cgroup is unreadable", func() {
			BeforeEach(func() {
				mockFS.WithFileExistsFunc(func(_ context.Context, path string) (bool, error) {
					if path == filepath.Join(testDataPath, "hwid") {
						return true, nil
					}

					return false, nil
				})
				mockFS.WithReadFileFunc(func(_ context.Context, path string) ([]byte, error) {
					if path == filepath.Join(testDataPath, "hwid") {
						return []byte(defaultHWID), nil
					}
					// Every cgroup/proc read fails: the cgroup-v1 /
					// non-container / transient-read-failure path.
					return nil, errors.New("file not found: " + path)
				})
			})

			It("emits State=healthy on the wire (no empty-string contract violation)", func() {
				status, err := service.GetStatus(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.CPU.State).To(Equal("healthy"),
					"a cgroup-read failure must default State to healthy, not the empty string")
				Expect(status.CPU.Attribution).To(BeEmpty(),
					"Attribution is omitempty and must be absent when not degraded")
				Expect(status.CPU.Causes).To(BeEmpty(),
					"Causes is omitempty and must be absent when not degraded")
				// Guard against a vacuous pass: the cgroup-read failure path
				// must actually have been taken, so CgroupCores (populated only
				// when cgroupErr == nil) stays zero on the wire.
				Expect(status.CPU.CgroupCores).To(BeZero(),
					"CgroupCores must be absent when the cgroup read failed")

				// Wire contract: "state" present, "attribution"/"causes" absent.
				wireJSON, err := json.Marshal(status.CPU)
				Expect(err).NotTo(HaveOccurred())
				Expect(wireJSON).To(ContainSubstring(`"state":"healthy"`),
					"the JSON wire must always carry state (no omitempty)")
				Expect(wireJSON).NotTo(ContainSubstring(`"attribution"`),
					"attribution is omitempty and must be absent when healthy")
				Expect(wireJSON).NotTo(ContainSubstring(`"causes"`),
					"causes is omitempty and must be absent when healthy")
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
