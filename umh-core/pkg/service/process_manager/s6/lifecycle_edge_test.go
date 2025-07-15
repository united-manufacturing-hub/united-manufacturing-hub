//go:build s6_process_manager

// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package s6

import (
	"context"
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"os"
	"path/filepath"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("LifecycleManager Edge Cases", func() {
	var (
		ctx       context.Context
		service   *DefaultService
		mockFS    *filesystem.MockFileSystem
		artifacts *ServiceArtifacts
		config    process_manager_serviceconfig.ProcessManagerServiceConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		service = &DefaultService{Logger: logger.For("test")}
		mockFS = filesystem.NewMockFileSystem()
		artifacts = &ServiceArtifacts{
			ServiceDir: filepath.Join(constants.S6BaseDir, "test-service"),
			LogDir:     filepath.Join(constants.S6LogBaseDir, "test-service"),
			CreatedFiles: []string{
				filepath.Join(constants.S6BaseDir, "test-service", "down"),
				filepath.Join(constants.S6BaseDir, "test-service", "type"),
				filepath.Join(constants.S6BaseDir, "test-service", "log", "type"),
				filepath.Join(constants.S6BaseDir, "test-service", "log", "down"),
				filepath.Join(constants.S6BaseDir, "test-service", "log", "run"),
				filepath.Join(constants.S6BaseDir, "test-service", "run"),
				filepath.Join(constants.S6BaseDir, "test-service", "dependencies.d", "base"),
				filepath.Join(constants.S6BaseDir, "test-service", ".complete"),
			},
		}
		config = process_manager_serviceconfig.ProcessManagerServiceConfig{
			Command: []string{"echo", "test"},
			Env: map[string]string{
				"TEST_VAR": "test_value",
			},
		}
	})

	Describe("File Disappearance During Operations", func() {
		It("should handle files disappearing during health check", func() {
			callCount := 0
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				callCount++
				// First call succeeds, second call fails (file disappeared)
				if callCount == 1 {
					return true, nil
				}
				if strings.HasSuffix(path, ".complete") {
					return false, nil // Sentinel file disappeared
				}
				return true, nil
			})

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthBad)) // Missing completion file = bad state
		})

		It("should return HealthUnknown for I/O errors during health check", func() {
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return false, fmt.Errorf("I/O error: disk read error")
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthUnknown)) // I/O error = unknown state, retry
		})

		It("should handle partial directory structure", func() {
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// Service has main run script but missing log run script
				if strings.Contains(path, "/log/run") {
					return false, nil
				}
				return true, nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthBad)) // Partial structure = bad state
		})
	})

	Describe("Concurrent Access Scenarios", func() {
		It("should handle files being modified during creation", func() {
			var mu sync.Mutex
			writeCount := 0

			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return nil
			})

			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				mu.Lock()
				defer mu.Unlock()
				writeCount++

				// Simulate another process modifying files during creation
				// Target the log run script which is now created earlier in the process
				if writeCount == 5 && strings.HasSuffix(path, "log/run") {
					return fmt.Errorf("text file busy")
				}
				return nil
			})

			mockFS.WithRenameFunc(func(ctx context.Context, oldPath, newPath string) error {
				return nil
			})

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("text file busy"))
			Expect(result).To(BeNil())
		})

		It("should handle rename failures due to concurrent access", func() {
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return nil
			})

			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				return nil
			})

			mockFS.WithRenameFunc(func(ctx context.Context, oldPath, newPath string) error {
				return fmt.Errorf("directory not empty: another process created files")
			})

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to atomically create service"))
			Expect(result).To(BeNil())
		})
	})

	Describe("Permission and Access Issues", func() {
		It("should handle permission denied during creation", func() {
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				if strings.Contains(path, "log") {
					return fmt.Errorf("permission denied")
				}
				return nil
			})

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))
			Expect(result).To(BeNil())
		})

		It("should handle files becoming unreadable during health check", func() {
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if strings.HasSuffix(path, "run") {
					return false, fmt.Errorf("permission denied")
				}
				return true, nil
			})

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthUnknown)) // Permission error = unknown, retry
		})
	})

	Describe("Filesystem Corruption Scenarios", func() {
		It("should handle supervise directory consistency issues", func() {
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// Main supervise exists but log supervise doesn't
				if strings.Contains(path, "/log/supervise") {
					return false, nil
				}
				return true, nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthBad)) // Inconsistent supervise = bad state
		})

		It("should detect incomplete service creation", func() {
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// Missing type file indicates incomplete creation
				if strings.HasSuffix(path, "type") {
					return false, nil
				}
				return true, nil
			})

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthBad)) // Missing type file = bad state
		})
	})

	Describe("Resource Exhaustion Scenarios", func() {
		It("should handle disk space exhaustion during creation", func() {
			writeCount := 0
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return nil
			})

			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				writeCount++
				if writeCount > 3 {
					return fmt.Errorf("no space left on device")
				}
				return nil
			})

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no space left on device"))
			Expect(result).To(BeNil())
		})

		It("should handle inode exhaustion", func() {
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				if strings.Contains(path, "dependencies.d") {
					return fmt.Errorf("no space left on device: inode table full")
				}
				return nil
			})

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inode table full"))
			Expect(result).To(BeNil())
		})
	})

	Describe("Race Conditions and Timing Issues", func() {
		It("should handle context cancellation during creation", func() {
			slowCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel context immediately

			result, err := service.CreateArtifacts(slowCtx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
			Expect(result).To(BeNil())
		})

		It("should handle files created by other processes during removal", func() {
			var mu sync.Mutex
			existingFiles := make(map[string]bool)
			existingFiles[artifacts.ServiceDir] = true
			existingFiles[artifacts.LogDir] = true

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				mu.Lock()
				defer mu.Unlock()
				return existingFiles[path], nil
			})

			renameCount := 0
			mockFS.WithRenameFunc(func(ctx context.Context, oldPath, newPath string) error {
				mu.Lock()
				defer mu.Unlock()
				renameCount++

				// Simulate another process recreating files during removal
				if renameCount == 1 {
					existingFiles[oldPath+"_new"] = true
				}

				delete(existingFiles, oldPath)
				existingFiles[newPath] = true
				return nil
			})

			mockFS.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				mu.Lock()
				defer mu.Unlock()
				delete(existingFiles, path)
				return nil
			})

			err := service.RemoveArtifacts(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred()) // Should succeed despite concurrent modifications
		})
	})

	Describe("Network Filesystem Issues", func() {
		It("should handle stale NFS handles", func() {
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if strings.HasSuffix(path, ".complete") {
					return false, fmt.Errorf("stale file handle")
				}
				return true, nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthUnknown)) // Network error = unknown, retry
		})

		It("should handle network timeouts during operations", func() {
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				if strings.Contains(path, "log") {
					return fmt.Errorf("operation timed out")
				}
				return nil
			})

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operation timed out"))
			Expect(result).To(BeNil())
		})
	})

	Describe("Recovery and Cleanup Edge Cases", func() {
		It("should handle cleanup failure during failed creation", func() {
			tempDirExists := true
			cleanupFailed := false

			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return nil
			})

			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				if strings.HasSuffix(path, "run") {
					return fmt.Errorf("simulated write failure")
				}
				return nil
			})

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return strings.Contains(path, ".new-") && tempDirExists, nil
			})

			mockFS.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				if strings.Contains(path, ".new-") {
					cleanupFailed = true
					return fmt.Errorf("cleanup failed: device busy")
				}
				return nil
			})

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated write failure"))
			Expect(result).To(BeNil())
			Expect(cleanupFailed).To(BeTrue()) // Cleanup was attempted
		})

		It("should handle force cleanup with stuck processes", func() {
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				if strings.Contains(path, "service") {
					return fmt.Errorf("directory not empty: process still running")
				}
				return nil
			})

			mockFS.WithExecuteCommandFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
				// Simulate kill commands succeeding
				return []byte{}, nil
			})

			err := service.ForceCleanup(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("force cleanup incomplete"))
		})
	})

	Describe("State Consistency Validation", func() {
		It("should detect partially created services", func() {
			// Service directory exists but is missing critical files
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == artifacts.ServiceDir, nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// Only main run script exists, missing log scripts and sentinel
				return strings.HasSuffix(path, "/run") && !strings.Contains(path, "/log/"), nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthBad)) // Partial creation = bad state
		})

		It("should validate all required files for healthy state", func() {
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// All required files present (including new log service files)
				requiredFiles := []string{"/down", "/type", "/log/type", "/log/down", "/log/run", "/run", "/dependencies.d/base", "/.complete"}
				for _, required := range requiredFiles {
					if strings.HasSuffix(path, required) {
						return true, nil
					}
				}
				return false, nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthOK)) // All files present = healthy
		})
	})

	Describe("Error Classification for FSM", func() {
		It("should handle I/O errors as retryable", func() {
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return false, fmt.Errorf("input/output error")
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthUnknown))
		})

		It("should handle permission denied as retryable", func() {
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return false, fmt.Errorf("permission denied")
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthUnknown))
		})

		It("should handle network errors as retryable", func() {
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return false, fmt.Errorf("network is unreachable")
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthUnknown))
		})

		It("should handle missing files as permanent errors", func() {
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// Missing run script indicates bad state
				return !strings.HasSuffix(path, "/run"), nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthBad))
		})
	})

	Describe("Service Creation Edge Cases", func() {
		It("should handle very long service names", func() {
			longName := strings.Repeat("a", 200)
			servicePath := filepath.Join(constants.S6BaseDir, longName)
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"echo", "test"},
			}

			result, err := service.CreateArtifacts(ctx, servicePath, config, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ServiceDir).To(Equal(servicePath))
		})

		It("should handle service names with unicode characters", func() {
			unicodeName := "service-测试-नमस्ते"
			servicePath := filepath.Join(constants.S6BaseDir, unicodeName)
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"echo", "test"},
			}

			result, err := service.CreateArtifacts(ctx, servicePath, config, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ServiceDir).To(Equal(servicePath))
		})

		It("should handle creation with large config files", func() {
			largeContent := strings.Repeat("x", 10000)
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"echo", "test"},
				ConfigFiles: map[string]string{
					"large-config.txt": largeContent,
				},
			}

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.ServiceDir).To(Equal(artifacts.ServiceDir))
		})
	})

	Describe("Filesystem Edge Cases", func() {
		It("should handle disk full scenarios", func() {
			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				return fmt.Errorf("no space left on device")
			})

			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"echo", "test"},
			}

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no space left on device"))
			Expect(result).To(BeNil())
		})

		It("should handle permission denied errors", func() {
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return fmt.Errorf("permission denied")
			})

			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"echo", "test"},
			}

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))
			Expect(result).To(BeNil())
		})

		It("should handle filesystem corruption", func() {
			mockFS.WithRenameFunc(func(ctx context.Context, oldPath, newPath string) error {
				return fmt.Errorf("input/output error")
			})

			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"echo", "test"},
			}

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("input/output error"))
			Expect(result).To(BeNil())
		})

		It("should handle intermittent filesystem errors", func() {
			callCount := 0
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				callCount++
				if callCount%2 == 0 {
					return false, fmt.Errorf("intermittent error")
				}
				return true, nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthUnknown))
		})
	})

	Describe("Concurrency Edge Cases", func() {
		It("should handle concurrent creation attempts", func() {
			var wg sync.WaitGroup
			var mu sync.Mutex
			results := make([]*ServiceArtifacts, 0)
			errors := make([]error, 0)

			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"echo", "test"},
			}

			// Try to create the same service concurrently
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)
					mu.Lock()
					results = append(results, result)
					errors = append(errors, err)
					mu.Unlock()
				}()
			}

			wg.Wait()

			// At least one should succeed
			successCount := 0
			for i, err := range errors {
				if err == nil {
					successCount++
					Expect(results[i]).NotTo(BeNil())
				}
			}
			Expect(successCount).To(BeNumerically(">=", 1))
		})

		It("should handle concurrent removal operations", func() {
			var wg sync.WaitGroup
			var mu sync.Mutex
			errors := make([]error, 0)

			// Run multiple removals concurrently
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := service.RemoveArtifacts(ctx, artifacts, mockFS)
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
				}()
			}

			wg.Wait()

			// All should succeed (idempotent)
			for _, err := range errors {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})

	Describe("Health Check Edge Cases", func() {
		It("should handle missing individual files gracefully", func() {
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// Mock missing type file
				return !strings.HasSuffix(path, "/type"), nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthBad))
		})

		It("should handle partially created services", func() {
			// Mock scenario where service directory exists but some files are missing
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return path == artifacts.ServiceDir, nil
			})

			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// Only run script exists
				return strings.HasSuffix(path, "/run"), nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(process_shared.HealthBad))
		})
	})
})
