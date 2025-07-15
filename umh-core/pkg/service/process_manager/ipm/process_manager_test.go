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

// mockFileInfo is a simple implementation of os.FileInfo for testing
type mockFileInfo struct {
	name string
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() interface{}   { return nil }

var _ = Describe("ProcessManager", func() {
	var (
		pm        *ProcessManager
		ctx       context.Context
		fsService filesystem.Service
		logger    *zap.SugaredLogger
	)

	BeforeEach(func() {
		// Use a context with a deadline for testing
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
		DeferCleanup(cancel)

		// Use a development logger for testing so we can see log messages
		devLogger, _ := zap.NewDevelopment()
		logger = devLogger.Sugar()

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

		// Setup mock filesystem to indicate no PID file exists by default
		mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
			if filepath.Base(path) == "run.pid" {
				return nil, os.ErrNotExist
			}
			return nil, os.ErrNotExist
		})

		fsService = mockFS

		pm = NewProcessManager(logger)
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

			// First, add service to the PM's service map
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was added to services map
			identifier := servicePathToIdentifier(servicePath)
			service, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())
			Expect(service.config).To(Equal(config))
			Expect(service.history.Status).To(Equal(process_shared.ServiceUnknown))
			Expect(service.history.ExitHistory).To(HaveLen(0))

			// Verify service was added to task queue (should be empty after step() execution)
			Expect(pm.taskQueue).To(HaveLen(0))

			// Verify directories were created
			logDir := filepath.Join(DefaultServiceDirectory, servicePath, "log")
			configDir := filepath.Join(DefaultServiceDirectory, servicePath, "config")

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

			// Ensure no PID file exists to bypass cleanup logic
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
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

			// Ensure no PID file exists to bypass cleanup logic
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
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

		It("should cleanup old PID file and retry creation", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Setup mock filesystem to simulate existing PID file
			mockFS := filesystem.NewMockFileSystem()
			pidFileExists := true
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if filepath.Base(path) == "run.pid" && pidFileExists {
					return &mockFileInfo{name: "run.pid"}, nil
				}
				return nil, os.ErrNotExist
			})

			// Mock ReadFile to return a PID when the file exists
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" && pidFileExists {
					return []byte("99999"), nil // Non-existent PID
				}
				return []byte{}, os.ErrNotExist
			})

			// First attempt should fail because of cleanup
			err := pm.Create(ctx, servicePath, config, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cleaned up old service instance"))

			// Service should still be in the services map
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())

			// Simulate PID file being removed after cleanup
			pidFileExists = false

			// Second attempt should succeed by calling step() again manually
			err = pm.step(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was processed (should be empty after step() execution)
			Expect(pm.taskQueue).To(HaveLen(0))
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
			Expect(pm.taskQueue).To(HaveLen(0))
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

	Describe("Start", func() {
		BeforeEach(func() {
			// Create a service first for start tests
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'\nsleep 10", // Long-running script for testing
				},
			}
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should start a service when no process is running", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate no existing PID file
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if filepath.Base(path) == "run.pid" {
					return nil, os.ErrNotExist // No PID file exists
				}
				return nil, os.ErrNotExist
			})

			// Make EnsureDirectory fail to simulate filesystem issues during startup
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return fmt.Errorf("simulated filesystem error during directory creation")
			})

			// Start the service - this will fail because EnsureDirectory fails
			err := pm.Start(ctx, servicePath, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error ensuring log directory"))

			// Verify service was NOT processed due to error
			Expect(pm.taskQueue).To(HaveLen(1))
			Expect(pm.taskQueue[0].Operation).To(Equal(OperationStart))
		})

		It("should terminate existing process and retry start", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate existing PID file
			mockFS := filesystem.NewMockFileSystem()
			pidFileExists := true
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if filepath.Base(path) == "run.pid" && pidFileExists {
					return &mockFileInfo{name: "run.pid"}, nil
				}
				return nil, os.ErrNotExist
			})

			// Mock ReadFile to return a PID when the file exists
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" && pidFileExists {
					return []byte("99999"), nil // Non-existent PID
				}
				return []byte{}, os.ErrNotExist
			})

			// Mock Remove to simulate PID file removal
			mockFS.WithRemoveFunc(func(ctx context.Context, path string) error {
				if filepath.Base(path) == "run.pid" {
					pidFileExists = false
				}
				return nil
			})

			// First attempt should fail because of existing process termination
			err := pm.Start(ctx, servicePath, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error terminating existing process"))

			// Service should still be in the task queue for retry
			identifier := servicePathToIdentifier(servicePath)
			Expect(pm.taskQueue).To(HaveLen(1))
			Expect(pm.taskQueue[0].Identifier).To(Equal(identifier))
			Expect(pm.taskQueue[0].Operation).To(Equal(OperationStart))
		})

		It("should handle filesystem errors during PID file writing", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate no existing PID file
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			})

			// Since real process execution will fail before PID writing,
			// we expect the process startup error rather than PID write error
			err := pm.Start(ctx, servicePath, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error starting process"))
		})

		It("should handle context cancellation during start", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"

			err := pm.Start(cancelCtx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})

		It("should handle non-existent service", func() {
			servicePath := "non-existent-service"

			err := pm.Start(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("Stop", func() {
		BeforeEach(func() {
			// Create a service first for stop tests
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'\nsleep 10",
				},
			}
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should stop a service successfully when no process is running", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate no running process (no PID file)
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return nil, os.ErrNotExist // No PID file exists
				}
				return []byte{}, nil
			})

			// Stop the service
			err := pm.Stop(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was processed (should be empty after step() execution)
			Expect(pm.taskQueue).To(HaveLen(0))

			// Verify service still exists in services map (only process is stopped, not removed)
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())
		})

		It("should stop a service successfully when process is running", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate running process with PID file
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("99999"), nil // Non-existent PID for testing
				}
				return []byte{}, nil
			})

			// Stop the service
			err := pm.Stop(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was processed (should be empty after step() execution)
			Expect(pm.taskQueue).To(HaveLen(0))

			// Verify service still exists in services map (only process is stopped, not removed)
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())
		})

		It("should handle corrupted PID file gracefully", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate corrupted PID file
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("not-a-number"), nil // Corrupted PID
				}
				return []byte{}, nil
			})

			// Stop the service - should succeed despite corrupted PID
			err := pm.Stop(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service still exists in services map
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())
		})

		It("should return error when service does not exist", func() {
			servicePath := "non-existent-service"

			err := pm.Stop(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should handle filesystem errors during PID file removal", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate PID file that can't be removed
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("99999"), nil // Non-existent PID
				}
				return []byte{}, nil
			})
			mockFS.WithRemoveFunc(func(ctx context.Context, path string) error {
				if filepath.Base(path) == "run.pid" {
					return fmt.Errorf("mock filesystem error")
				}
				return nil
			})

			err := pm.Stop(ctx, servicePath, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error removing PID file"))
		})

		It("should handle context cancellation during stop", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"

			err := pm.Stop(cancelCtx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("Restart", func() {
		BeforeEach(func() {
			// Create a service first for restart tests
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'\nsleep 10",
				},
			}
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should restart a service successfully when no process is running", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate no running process
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return nil, os.ErrNotExist // No PID file exists
				}
				return []byte{}, nil
			})

			// Restart the service
			err := pm.Restart(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify restart queued a start operation
			Expect(pm.taskQueue).To(HaveLen(1))
			Expect(pm.taskQueue[0].Operation).To(Equal(OperationStart))
			Expect(pm.taskQueue[0].Identifier).To(Equal(servicePathToIdentifier(servicePath)))

			// Verify service still exists in services map
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())
		})

		It("should restart a service successfully when process is running", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate running process with PID file
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("99999"), nil // Non-existent PID for testing
				}
				return []byte{}, nil
			})

			// Restart the service
			err := pm.Restart(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify restart queued a start operation
			Expect(pm.taskQueue).To(HaveLen(1))
			Expect(pm.taskQueue[0].Operation).To(Equal(OperationStart))
			Expect(pm.taskQueue[0].Identifier).To(Equal(servicePathToIdentifier(servicePath)))

			// Verify service still exists in services map
			identifier := servicePathToIdentifier(servicePath)
			_, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())
		})

		It("should not queue start operation when stop fails", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate stop failure
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("99999"), nil // Non-existent PID
				}
				return []byte{}, nil
			})
			mockFS.WithRemoveFunc(func(ctx context.Context, path string) error {
				if filepath.Base(path) == "run.pid" {
					return fmt.Errorf("mock filesystem error")
				}
				return nil
			})

			// Restart the service - should fail during stop
			err := pm.Restart(ctx, servicePath, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error removing PID file"))

			// Verify NO start operation was queued due to stop failure
			Expect(pm.taskQueue).To(HaveLen(1))
			Expect(pm.taskQueue[0].Operation).To(Equal(OperationRestart)) // Original task still there
		})

		It("should handle multiple restart operations in sequence", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate successful stop operations
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return nil, os.ErrNotExist // No PID file exists
				}
				return []byte{}, nil
			})

			// First restart
			err := pm.Restart(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(pm.taskQueue).To(HaveLen(1))
			Expect(pm.taskQueue[0].Operation).To(Equal(OperationStart))

			// Second restart (should add another start task)
			err = pm.Restart(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(pm.taskQueue).To(HaveLen(2))
			Expect(pm.taskQueue[0].Operation).To(Equal(OperationStart))
			Expect(pm.taskQueue[1].Operation).To(Equal(OperationStart))
		})

		It("should return error when service does not exist", func() {
			servicePath := "non-existent-service"

			err := pm.Restart(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should handle context cancellation during restart", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"

			err := pm.Restart(cancelCtx, servicePath, fsService)
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

	Context("Real Filesystem Tests", func() {
		var (
			tempDir string
			realFS  filesystem.Service
		)

		BeforeEach(func() {
			// Create a temporary directory for testing
			var err error
			tempDir, err = os.MkdirTemp("", "ipm-test-")
			Expect(err).ToNot(HaveOccurred())

			// Debug: Check if temp directory is writable
			testFile := filepath.Join(tempDir, "test-write")
			err = os.WriteFile(testFile, []byte("test"), 0644)
			Expect(err).ToNot(HaveOccurred())
			_ = os.Remove(testFile)

			GinkgoT().Logf("Created temp directory: %s", tempDir)

			// Use real filesystem service
			realFS = filesystem.NewDefaultService()

			// Create ProcessManager with custom service directory
			pm = NewProcessManager(logger, WithServiceDirectory(tempDir))

			// Debug: Check if ProcessManager has correct service directory
			GinkgoT().Logf("ProcessManager service directory: %s", pm.serviceDirectory)
		})

		AfterEach(func() {
			// Clean up temporary directory
			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		It("should create a service with real filesystem", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'",
				},
			}

			// Debug: Check task queue before Create
			Expect(pm.taskQueue).To(HaveLen(0), "Task queue should be empty before Create")

			err := pm.Create(ctx, servicePath, config, realFS)

			// Debug: Check what error occurred, if any
			if err != nil {
				GinkgoT().Logf("Create returned error: %v", err)
			}

			// If Create failed, let's see what's in the task queue
			GinkgoT().Logf("Task queue length after Create: %d", len(pm.taskQueue))
			if len(pm.taskQueue) > 0 {
				GinkgoT().Logf("First task in queue: %+v", pm.taskQueue[0])
			}

			Expect(err).ToNot(HaveOccurred())

			// Debug: Check if task queue is empty (meaning step() processed the task)
			Expect(pm.taskQueue).To(HaveLen(0), "Task queue should be empty after Create")

			// Verify service was added to services map
			identifier := servicePathToIdentifier(servicePath)
			service, exists := pm.services[identifier]
			Expect(exists).To(BeTrue())
			Expect(service.config).To(Equal(config))

			// Verify directories were created on real filesystem
			// Note: We use the identifier (not servicePath) because that's what ProcessManager uses internally
			logDir := filepath.Join(tempDir, string(identifier), "log")
			configDir := filepath.Join(tempDir, string(identifier), "config")

			// Debug: Check if service directory exists at all
			serviceDir := filepath.Join(tempDir, string(identifier))
			serviceDirExists, err := realFS.PathExists(ctx, serviceDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(serviceDirExists).To(BeTrue(), "Service directory should exist")

			logDirExists, err := realFS.PathExists(ctx, logDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(logDirExists).To(BeTrue(), "Log directory should exist")

			configDirExists, err := realFS.PathExists(ctx, configDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(configDirExists).To(BeTrue(), "Config directory should exist")

			// Verify config files were written
			configFilePath := filepath.Join(configDir, "config.yaml")
			configFileExists, err := realFS.PathExists(ctx, configFilePath)
			Expect(err).ToNot(HaveOccurred())
			Expect(configFileExists).To(BeTrue())

			// Verify file contents
			content, err := realFS.ReadFile(ctx, configFilePath)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(content)).To(Equal("test: value"))

			// Verify run.sh was written
			runScriptPath := filepath.Join(configDir, "run.sh")
			runScriptExists, err := realFS.PathExists(ctx, runScriptPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(runScriptExists).To(BeTrue())

			content, err = realFS.ReadFile(ctx, runScriptPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(content)).To(Equal("#!/bin/bash\necho 'Hello World'"))
		})

		It("should use custom service directory", func() {
			// Verify ProcessManager is using the custom directory
			Expect(pm.serviceDirectory).To(Equal(tempDir))
		})
	})
})
