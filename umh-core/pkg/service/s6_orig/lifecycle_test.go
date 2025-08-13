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

package s6_orig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
)

var _ = Describe("LifecycleManager", func() {
	var (
		ctx       context.Context
		service   *DefaultService
		mockFS    *filesystem.MockFileSystem
		artifacts *s6_shared.ServiceArtifacts
	)

	BeforeEach(func() {
		ctx = context.Background()
		service = &DefaultService{logger: logger.For("test")}
		mockFS = filesystem.NewMockFileSystem()
		artifacts = &s6_shared.ServiceArtifacts{
			ServiceDir: filepath.Join(constants.S6BaseDir, "test-service"),
			LogDir:     filepath.Join(constants.S6LogBaseDir, "test-service"),
		}
	})

	// Note: ensureArtifacts function was removed as we now always use tracked files
	// created during service creation. Services without tracked files are considered
	// to be in an inconsistent state and should be recreated.

	Describe("CreateArtifacts", func() {
		var (
			config      s6serviceconfig.S6ServiceConfig
			servicePath string
		)

		BeforeEach(func() {
			servicePath = filepath.Join(constants.S6BaseDir, "test-service")
			config = s6serviceconfig.S6ServiceConfig{
				Command: []string{"echo", "hello world"},
				Env: map[string]string{
					"TEST_VAR": "test_value",
				},
				MemoryLimit: 1024,
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
				LogFilesize: 2048,
			}

			// Mock filesystem operations
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return nil
			})
			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				return nil
			})
			mockFS.WithRenameFunc(func(ctx context.Context, oldPath, newPath string) error {
				return nil
			})
			mockFS.WithChmodFunc(func(ctx context.Context, path string, mode os.FileMode) error {
				return nil
			})
		})

		It("should create artifacts with all required components", func() {
			result, err := service.CreateArtifacts(ctx, servicePath, config, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.ServiceDir).To(Equal(servicePath))
			Expect(result.LogDir).To(Equal(filepath.Join(constants.S6LogBaseDir, "test-service")))
			Expect(result.TempDir).To(BeEmpty()) // Cleared after successful creation
		})

		// It("should handle EXDEV errors gracefully", func() {
		// 	// Simulate EXDEV error on rename
		// 	mockFS.WithRenameFunc(func(ctx context.Context, oldPath, newPath string) error {
		// 		return fmt.Errorf("invalid cross-device link: %w", os.ErrPermission)
		// 	})

		// 	result, err := service.CreateArtifacts(ctx, servicePath, config, mockFS)

		// 	// Currently the implementation doesn't have EXDEV fallback, so expect error
		// 	Expect(err).To(HaveOccurred())
		// 	Expect(err.Error()).To(ContainSubstring("failed to atomically create service"))
		// 	Expect(result).To(BeNil())
		// })

		It("should handle context cancellation", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			result, err := service.CreateArtifacts(cancelCtx, servicePath, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
			Expect(result).To(BeNil())
		})

		It("should validate config files for path traversal", func() {
			maliciousConfig := config
			maliciousConfig.ConfigFiles = map[string]string{
				"../../../etc/passwd": "malicious content",
			}

			result, err := service.CreateArtifacts(ctx, servicePath, maliciousConfig, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid config filename"))
			Expect(result).To(BeNil())
		})
	})

	Describe("RemoveArtifacts", func() {
		var (
			existingPaths  sync.Map
			renameCalls    []string
			removeAllCalls []string
		)

		BeforeEach(func() {
			existingPaths = sync.Map{}
			renameCalls = []string{}
			removeAllCalls = []string{}

			// Add tracked files to artifacts for removal operations
			artifacts.CreatedFiles = []string{
				filepath.Join(artifacts.ServiceDir, "down"),
				filepath.Join(artifacts.ServiceDir, "type"),
				filepath.Join(artifacts.ServiceDir, "log", "type"),
				filepath.Join(artifacts.ServiceDir, "log", "down"),
				filepath.Join(artifacts.ServiceDir, "log", "run"),
				filepath.Join(artifacts.ServiceDir, "run"),
				filepath.Join(artifacts.ServiceDir, "dependencies.d", "base"),
				filepath.Join(artifacts.ServiceDir, ".complete"),
			}

			// Setup paths as existing
			existingPaths.Store(artifacts.ServiceDir, true)
			existingPaths.Store(artifacts.LogDir, true)

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				val, ok := existingPaths.Load(path)
				if !ok {
					return false, nil
				}
				boolVal, ok := val.(bool)

				return ok && boolVal, nil
			})

			mockFS.WithRenameFunc(func(ctx context.Context, oldPath, newPath string) error {
				renameCalls = append(renameCalls, oldPath+"->"+newPath)
				existingPaths.Delete(oldPath)
				existingPaths.Store(newPath, true)

				return nil
			})

			mockFS.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				removeAllCalls = append(removeAllCalls, path)
				existingPaths.Delete(path)

				return nil
			})
		})

		It("should directly remove directories when using tracked files", func() {
			err := service.RemoveArtifacts(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(removeAllCalls).ToNot(BeEmpty())

			// Check that original paths are no longer visible
			exists, _ := mockFS.PathExists(ctx, artifacts.ServiceDir)
			Expect(exists).To(BeFalse())
			exists, _ = mockFS.PathExists(ctx, artifacts.LogDir)
			Expect(exists).To(BeFalse())
		})

		It("should be idempotent when paths don't exist", func() {
			// Clear existing paths
			existingPaths = sync.Map{}

			err := service.RemoveArtifacts(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(renameCalls).To(BeEmpty())
			Expect(removeAllCalls).To(BeEmpty())
		})

		It("should handle context deadline exceeded", func() {
			deadlineCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
			defer cancel()
			time.Sleep(2 * time.Nanosecond) // Ensure timeout

			err := service.RemoveArtifacts(deadlineCtx, artifacts, mockFS)

			Expect(err).To(Equal(context.DeadlineExceeded))
		})

		It("should handle nil artifacts", func() {
			err := service.RemoveArtifacts(ctx, nil, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("artifacts is nil"))
		})

		It("should handle nil lifecycle manager", func() {
			var nilService *DefaultService
			err := nilService.RemoveArtifacts(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("lifecycle manager is nil"))
		})
	})

	Describe("ForceCleanup", func() {
		var (
			existingPaths sync.Map
			processCalls  []string
		)

		BeforeEach(func() {
			existingPaths = sync.Map{}
			processCalls = []string{}

			// Setup paths as existing
			existingPaths.Store(artifacts.ServiceDir, true)
			existingPaths.Store(artifacts.LogDir, true)

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				val, ok := existingPaths.Load(path)
				if !ok {
					return false, nil
				}
				boolVal, ok := val.(bool)

				return ok && boolVal, nil
			})

			mockFS.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				existingPaths.Delete(path)

				return nil
			})

			mockFS.WithExecuteCommandFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
				processCalls = append(processCalls, name+" "+strings.Join(args, " "))

				return []byte{}, nil
			})

			mockFS.WithRenameFunc(func(ctx context.Context, oldPath, newPath string) error {
				existingPaths.Delete(oldPath)
				existingPaths.Store(newPath, true)

				return nil
			})
		})

		It("should perform aggressive cleanup with process termination", func() {
			err := service.ForceCleanup(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())

			// Check that paths are removed
			exists, _ := mockFS.PathExists(ctx, artifacts.ServiceDir)
			Expect(exists).To(BeFalse())
			exists, _ = mockFS.PathExists(ctx, artifacts.LogDir)
			Expect(exists).To(BeFalse())
		})

		It("should handle missing paths gracefully", func() {
			// Clear existing paths
			existingPaths = sync.Map{}

			err := service.ForceCleanup(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle nil artifacts", func() {
			err := service.ForceCleanup(ctx, nil, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("artifacts is nil"))
		})
	})

	Describe("CheckArtifactsHealth", func() {
		BeforeEach(func() {
			// Add tracked files to artifacts for health checks
			artifacts.CreatedFiles = []string{
				filepath.Join(artifacts.ServiceDir, "down"),
				filepath.Join(artifacts.ServiceDir, "type"),
				filepath.Join(artifacts.ServiceDir, "log", "type"),
				filepath.Join(artifacts.ServiceDir, "log", "down"),
				filepath.Join(artifacts.ServiceDir, "log", "run"),
				filepath.Join(artifacts.ServiceDir, "run"),
				filepath.Join(artifacts.ServiceDir, "dependencies.d", "base"),
				filepath.Join(artifacts.ServiceDir, ".complete"),
			}

			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return true, nil
			})
		})

		It("should return HealthOK for healthy artifacts", func() {
			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(s6_shared.HealthOK))
		})

		It("should return HealthBad for missing service directory", func() {
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				// Mock missing run script to make service unhealthy
				return !strings.HasSuffix(path, "/run"), nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(s6_shared.HealthBad))
		})

		It("should return HealthBad for missing completion sentinel", func() {
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return !strings.HasSuffix(path, ".complete"), nil
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).NotTo(HaveOccurred())
			Expect(health).To(Equal(s6_shared.HealthBad))
		})

		It("should return HealthUnknown for filesystem errors", func() {
			mockFS.WithPathExistsFunc(func(ctx context.Context, path string) (bool, error) {
				return false, errors.New("filesystem error")
			})

			health, err := service.CheckArtifactsHealth(ctx, artifacts, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(health).To(Equal(s6_shared.HealthUnknown))
		})

		It("should handle nil artifacts", func() {
			health, err := service.CheckArtifactsHealth(ctx, nil, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("artifacts is nil"))
			Expect(health).To(Equal(s6_shared.HealthBad))
		})
	})

	// Note: Edge cases for ensureArtifacts removed since we now always use tracked files.
	// Services without tracked files are considered inconsistent and should be recreated.

	Describe("Edge Cases and Error Handling", func() {
		It("should handle filesystem permission errors", func() {
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return errors.New("permission denied")
			})

			config := s6serviceconfig.S6ServiceConfig{
				Command: []string{"echo", "test"},
			}

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("permission denied"))
			Expect(result).To(BeNil())
		})
	})

	Describe("Resource Management", func() {
		It("should clean up repository directory on symlink failure", func() {
			repositoryDirCreated := false
			repositoryDirRemoved := false

			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				if strings.Contains(path, constants.S6RepositoryBaseDir) {
					repositoryDirCreated = true
				}

				return nil
			})

			mockFS.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				if strings.Contains(path, constants.S6RepositoryBaseDir) {
					repositoryDirRemoved = true
				}

				return nil
			})

			mockFS.WithSymlinkFunc(func(ctx context.Context, oldPath, newPath string) error {
				return errors.New("simulated symlink failure")
			})

			config := s6serviceconfig.S6ServiceConfig{
				Command: []string{"echo", "test"},
			}

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create scan directory symlink"))
			Expect(result).To(BeNil())
			Expect(repositoryDirCreated).To(BeTrue())
			Expect(repositoryDirRemoved).To(BeTrue())
		})

		It("should handle resource exhaustion gracefully", func() {
			// Simulate resource exhaustion
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return errors.New("no space left on device")
			})

			config := s6serviceconfig.S6ServiceConfig{
				Command: []string{"echo", "test"},
			}

			result, err := service.CreateArtifacts(ctx, artifacts.ServiceDir, config, mockFS)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no space left on device"))
			Expect(result).To(BeNil())
		})
	})
})
