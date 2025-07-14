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

package ipm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
)

var _ = Describe("ProcessManager", func() {
	var (
		pm        *ProcessManager
		ctx       context.Context
		fsService filesystem.Service
		logger    *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()
		mockFS := filesystem.NewMockFileSystem()

		// Setup mock filesystem to return the content that was written
		mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
			if filepath.Base(path) == "config.yaml" {
				return []byte("test: value"), nil
			}
			if filepath.Base(path) == "run.sh" {
				return []byte("#!/bin/bash\necho 'Hello World'"), nil
			}
			return []byte{}, nil
		})

		fsService = mockFS

		pm = &ProcessManager{
			Logger:        logger,
			services:      make(map[serviceIdentifier]service),
			toBeCreated:   make([]serviceIdentifier, 0),
			toBeRemoved:   make([]serviceIdentifier, 0),
			toBeRestarted: make([]serviceIdentifier, 0),
			toBeStarted:   make([]serviceIdentifier, 0),
			toBeStopped:   make([]serviceIdentifier, 0),
		}
	})

	Describe("Create", func() {
		It("should create a new service successfully", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'",
				},
			}

			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was added to services map
			identifier := servicePathToIdentifier(servicePath)
			service, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())
			Expect(service.config).To(Equal(config))
			Expect(service.history.Status).To(Equal(process_shared.ServiceUnknown))
			Expect(service.history.ExitHistory).To(HaveLen(0))

			// Verify service was added to toBeCreated (should be empty after step() execution)
			Expect(pm.toBeCreated).To(HaveLen(0))

			// Verify directories were created
			logDir := filepath.Join(IPM_SERVICE_DIRECTORY, servicePath, "log")
			configDir := filepath.Join(IPM_SERVICE_DIRECTORY, servicePath, "config")

			logDirExists, err := fsService.PathExists(ctx, logDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(logDirExists).To(BeTrue())

			configDirExists, err := fsService.PathExists(ctx, configDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(configDirExists).To(BeTrue())

			// Verify config files were written
			configContent, err := fsService.ReadFile(ctx, filepath.Join(configDir, "config.yaml"))
			Expect(err).ToNot(HaveOccurred())
			Expect(string(configContent)).To(Equal("test: value"))

			runScript, err := fsService.ReadFile(ctx, filepath.Join(configDir, "run.sh"))
			Expect(err).ToNot(HaveOccurred())
			Expect(string(runScript)).To(Equal("#!/bin/bash\necho 'Hello World'"))
		})

		It("should return error when service already exists", func() {
			servicePath := "existing-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first time
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Try to create the same service again
			err = pm.Create(ctx, servicePath, config, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already exists"))
		})

		It("should handle context cancellation", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			err := pm.Create(cancelCtx, servicePath, config, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})

		It("should handle filesystem errors during directory creation", func() {
			// Create a filesystem service that will fail on directory creation
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return fmt.Errorf("mock filesystem error")
			})

			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			err := pm.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error creating directory"))
		})

		It("should handle filesystem errors during config file writing", func() {
			// Create a filesystem service that will fail on file writing
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				return fmt.Errorf("mock filesystem error")
			})

			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			err := pm.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error writing config file"))
		})
	})

	Describe("Remove", func() {
		It("should remove a service successfully when no process is running", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock filesystem to simulate no .pid file (process not running)
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return nil, os.ErrNotExist
				}
				return []byte{}, nil
			})

			// Remove the service
			err = pm.Remove(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was removed from services map
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeFalse())

			// Verify service was processed (should be empty after step() execution)
			Expect(pm.toBeRemoved).To(HaveLen(0))
		})

		It("should remove a service successfully when process is not running but .pid file exists", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock filesystem to simulate .pid file with non-existent process
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("99999"), nil // Non-existent PID
				}
				return []byte{}, nil
			})

			// Remove the service
			err = pm.Remove(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was removed from services map
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeFalse())
		})

		It("should handle corrupted .pid file gracefully", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock filesystem to simulate corrupted .pid file
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("not-a-number"), nil // Corrupted PID
				}
				return []byte{}, nil
			})

			// Remove the service - should succeed despite corrupted PID because we continue with cleanup
			err = pm.Remove(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was removed from services map
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeFalse())
		})

		It("should return error when service does not exist", func() {
			servicePath := "non-existent-service"

			err := pm.Remove(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))
		})

		It("should handle filesystem errors during directory removal", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock filesystem to fail on RemoveAll
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return nil, os.ErrNotExist // No pid file exists
				}
				return []byte{}, nil
			})
			mockFS.WithRemoveAllFunc(func(ctx context.Context, path string) error {
				return fmt.Errorf("mock filesystem error")
			})

			err = pm.Remove(ctx, servicePath, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error removing pid file"))
		})

		It("should handle context cancellation during removal", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			err = pm.Remove(cancelCtx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("generateContext", func() {
		It("should create a context with proper deadline", func() {
			timeout := 100 * time.Millisecond
			parentCtx, parentCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer parentCancel()

			childCtx, childCancel, err := generateContext(parentCtx, timeout)
			Expect(err).ToNot(HaveOccurred())
			defer childCancel()

			parentDeadline, _ := parentCtx.Deadline()
			childDeadline, _ := childCtx.Deadline()

			// Child deadline should be 100ms before parent deadline
			expectedChildDeadline := parentDeadline.Add(-timeout)
			Expect(childDeadline).To(BeTemporally("~", expectedChildDeadline, time.Millisecond))
		})

		It("should return error when parent context has no deadline", func() {
			parentCtx := context.Background()
			timeout := 100 * time.Millisecond

			_, _, err := generateContext(parentCtx, timeout)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context has no deadline"))
		})

		It("should return error when parent context doesn't have enough time", func() {
			timeout := 200 * time.Millisecond
			parentCtx, parentCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer parentCancel()

			_, _, err := generateContext(parentCtx, timeout)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context has no enough time"))
		})

		It("should return error when parent context is already cancelled", func() {
			parentCtx, parentCancel := context.WithCancel(context.Background())
			parentCancel() // Cancel immediately

			timeout := 100 * time.Millisecond

			_, _, err := generateContext(parentCtx, timeout)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})
	})
})
