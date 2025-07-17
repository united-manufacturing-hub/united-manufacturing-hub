//go:build internal_process_manager
// +build internal_process_manager

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

package ipm_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
	"go.uber.org/zap"

	"syscall"

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
		pm        *ipm.ProcessManager
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

		pm = ipm.NewProcessManager(logger)
	})

	Describe("Create", func() {
		It("should create a new service successfully", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "Hello World"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'",
				},
			}

			// First, add service to the PM's service map
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was added to services map
			identifier := constants.ServicePathToIdentifier(servicePath)
			service, exists := pm.Services.Load(identifier)
			Expect(exists).To(BeTrue())
			ipmService := service.(ipm.IpmService)
			Expect(ipmService.Config).To(Equal(config))
			Expect(ipmService.History.Status).To(Equal(process_shared.ServiceUnknown))
			Expect(ipmService.History.ExitHistory).To(HaveLen(0))

			// Verify task was queued
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationCreate))

			// Process the queued task
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was processed (should be empty after Reconcile() execution)
			Expect(pm.TaskQueue).To(HaveLen(0))

			// Verify directories were created
			logDir := filepath.Join(ipm.DefaultServiceDirectory, "logs", servicePath)
			configDir := filepath.Join(ipm.DefaultServiceDirectory, "services", servicePath)

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
				Command: []string{"/bin/echo", "test"},
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
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create operation should succeed (just queues task)
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// But Reconcile with cancelled context should fail
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			err = pm.Reconcile(cancelCtx, fsService)
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
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create should succeed (just queues task)
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// But Reconcile should fail due to filesystem error
			err = pm.Reconcile(ctx, mockFS)
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
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create should succeed (just queues task)
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// But Reconcile should fail due to filesystem error
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error writing config file"))
		})

		It("should cleanup old PID file and retry creation", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
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

			// Create should succeed (just queues task)
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// First Reconcile attempt should fail because of cleanup
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cleaned up old service instance"))

			// Service should still be in the services map
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
			Expect(exists).To(BeTrue())

			// Simulate PID file being removed after cleanup
			pidFileExists = false

			// Second attempt should succeed by calling Reconcile() again manually
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was processed (should be empty after Reconcile() execution)
			Expect(pm.TaskQueue).To(HaveLen(0))
		})
	})

	Describe("Remove", func() {
		It("should remove a service successfully when no process is running", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the create task first
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was created
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
			Expect(exists).To(BeTrue())

			// Setup mock filesystem to simulate no .pid file (process not running)
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return nil, os.ErrNotExist
				}
				return []byte{}, nil
			})

			// Remove the service
			err = pm.Remove(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the queued remove task
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was removed from services map
			_, exists = pm.Services.Load(identifier)
			Expect(exists).To(BeFalse())

			// Verify service was processed (should be empty after Reconcile() execution)
			Expect(pm.TaskQueue).To(HaveLen(0))
		})

		It("should remove a service successfully when process is not running but .pid file exists", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the create task first
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was created
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
			Expect(exists).To(BeTrue())

			// Setup mock filesystem to simulate .pid file with non-existent process
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("99999"), nil // Non-existent PID
				}
				return []byte{}, nil
			})

			// Remove the service
			err = pm.Remove(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the queued remove task
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was removed from services map
			_, exists = pm.Services.Load(identifier)
			Expect(exists).To(BeFalse())
		})

		It("should handle corrupted .pid file gracefully", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock filesystem to simulate corrupted .pid file
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return []byte("not-a-number"), nil // Corrupted PID
				}
				return []byte{}, nil
			})

			// Remove the service
			err = pm.Remove(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the remove task - should succeed despite corrupted PID because we continue with cleanup
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was removed from services map
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
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
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the create task
			err = pm.Reconcile(ctx, fsService)
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

			err = pm.Remove(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Reconcile should fail due to filesystem error
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error removing services directory"))
		})

		It("should handle context cancellation during removal", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// First create the service
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Remove should succeed (just queues task)
			err = pm.Remove(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// But Reconcile with cancelled context should fail
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			err = pm.Reconcile(cancelCtx, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("Start", func() {
		BeforeEach(func() {
			// Create a service first for start tests
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/bash", "-c", "echo 'Hello World'; sleep 10"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'\nsleep 10", // Long-running script for testing
				},
			}
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the create task to actually create the service
			err = pm.Reconcile(ctx, fsService)
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

			// Start should succeed (just queues task)
			err := pm.Start(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify task was queued
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationStart))

			// Make EnsureDirectory fail to simulate filesystem issues during startup
			mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error {
				return fmt.Errorf("simulated filesystem error during directory creation")
			})

			// Reconcile should fail because EnsureDirectory fails
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error ensuring log directory"))
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

			// Start should succeed (just queues task)
			err := pm.Start(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify task was queued
			identifier := constants.ServicePathToIdentifier(servicePath)
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Identifier).To(Equal(identifier))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationStart))

			// First Reconcile attempt should fail because of existing process termination
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error terminating existing process"))
		})

		It("should handle filesystem errors during PID file writing", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate no existing PID file
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			})

			// Mock WriteFile to fail when writing PID file
			mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
				if filepath.Base(path) == "run.pid" {
					return fmt.Errorf("mock filesystem error during PID file writing")
				}
				return nil // Allow other file writes to succeed
			})

			// Start should succeed (just queues task)
			err := pm.Start(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Reconcile should fail during PID file writing
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error writing PID file"))
		})

		It("should handle context cancellation during start", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"

			// Start should succeed (just queues task)
			err := pm.Start(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Reconcile with canceled context should fail
			err = pm.Reconcile(cancelCtx, fsService)
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
				Command: []string{"/bin/bash", "-c", "echo 'Hello World'; sleep 10"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'\nsleep 10",
				},
			}
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the create task to actually create the service
			err = pm.Reconcile(ctx, fsService)
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
			err := pm.Stop(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify task was queued
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationStop))

			// Process the queued task
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was processed (should be empty after Reconcile() execution)
			Expect(pm.TaskQueue).To(HaveLen(0))

			// Verify service still exists in services map (only process is stopped, not removed)
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
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
			err := pm.Stop(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify task was queued
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationStop))

			// Process the queued task
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service was processed (should be empty after Reconcile() execution)
			Expect(pm.TaskQueue).To(HaveLen(0))

			// Verify service still exists in services map (only process is stopped, not removed)
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
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

			// Stop the service - should succeed (just queues task)
			err := pm.Stop(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify task was queued
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationStop))

			// Process the queued task - should succeed despite corrupted PID
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Verify service still exists in services map
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
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

			// Stop should succeed (just queues task)
			err := pm.Stop(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify task was queued
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationStop))

			// Reconcile should fail due to filesystem error
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error removing PID file"))
		})

		It("should handle context cancellation during stop", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"

			// Stop should succeed (just queues task)
			err := pm.Stop(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Reconcile with canceled context should fail
			err = pm.Reconcile(cancelCtx, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("Restart", func() {
		BeforeEach(func() {
			// Create a service first for restart tests
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/bash", "-c", "echo 'Hello World'; sleep 10"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'\nsleep 10",
				},
			}
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the create task to actually create the service
			err = pm.Reconcile(ctx, fsService)
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

			// Process the restart task (should complete both stop and start)
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// After successful restart, queue should be empty (both stop and start completed)
			Expect(pm.TaskQueue).To(HaveLen(0))

			// Verify service still exists in services map
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
			Expect(exists).To(BeTrue())
		})

		It("should restart a service successfully when process is running", func() {
			servicePath := "test-service"

			// Setup mock filesystem to simulate running process with PID file that gets cleaned up
			mockFS := filesystem.NewMockFileSystem()
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if filepath.Base(path) == "run.pid" {
					return nil, os.ErrNotExist // No PID file exists (simulating clean state)
				}
				return []byte{}, nil
			})

			// Restart the service
			err := pm.Restart(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// Process the restart task (should complete both stop and start)
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).ToNot(HaveOccurred())

			// After successful restart, queue should be empty (both stop and start completed)
			Expect(pm.TaskQueue).To(HaveLen(0))

			// Verify service still exists in services map
			identifier := constants.ServicePathToIdentifier(servicePath)
			_, exists := pm.Services.Load(identifier)
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

			// Restart should succeed (just queues task)
			err := pm.Restart(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify restart task was queued
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationRestart))

			// Reconcile should fail during stop part of restart
			err = pm.Reconcile(ctx, mockFS)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error removing PID file"))

			// Verify NO start operation was queued due to stop failure during restart
			// Task queue should still have the failed restart task
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationRestart))
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
			Expect(pm.TaskQueue).To(HaveLen(1))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationRestart))

			// Second restart (should add another restart task)
			err = pm.Restart(ctx, servicePath, mockFS)
			Expect(err).ToNot(HaveOccurred())
			Expect(pm.TaskQueue).To(HaveLen(2))
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationRestart))
			Expect(pm.TaskQueue[1].Operation).To(Equal(ipm.OperationRestart))
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

			// Restart should succeed (just queues task)
			err := pm.Restart(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Reconcile with canceled context should fail
			err = pm.Reconcile(cancelCtx, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("Status", func() {
		It("should return service status for existing service with no process running", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "Hello World"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Status should return ServiceDown since no process is running
			status, err := pm.Status(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(status.Status).To(Equal(process_shared.ServiceDown))
			Expect(status.Pid).To(Equal(0))
			Expect(status.Pgid).To(Equal(0))
			Expect(status.Uptime).To(Equal(int64(0)))
		})

		It("should return service status for existing service with process running", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "Hello World"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"run.sh":      "#!/bin/bash\necho 'Hello World'",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock to simulate a running process
			identifier := constants.ServicePathToIdentifier(servicePath)
			pidFile := filepath.Join(pm.ServiceDirectory, string(identifier), constants.PidFileName)
			mockPid := "1234"

			// Mock filesystem to return PID file content
			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if path == pidFile {
					return []byte(mockPid), nil
				}
				// Default mock behavior for other files
				if filepath.Base(path) == "config.yaml" {
					return []byte("test: value"), nil
				}
				if filepath.Base(path) == "run.sh" {
					return []byte("#!/bin/bash\necho 'Hello World'"), nil
				}
				return []byte{}, nil
			})

			// Mock Stat to simulate PID file exists
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == pidFile {
					return &mockFileInfo{name: "run.pid"}, nil
				}
				return nil, os.ErrNotExist
			})

			// Note: In a real test, the process checking with os.FindProcess and signal would fail
			// for a mock PID, so the service would show as down. This is expected behavior
			// since we can't easily mock the OS process checking functions.
			status, err := pm.Status(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			// Will be ServiceDown because mock PID doesn't correspond to real process
			Expect(status.Status).To(Equal(process_shared.ServiceDown))
		})

		It("should return error for non-existent service", func() {
			servicePath := "non-existent-service"

			status, err := pm.Status(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(process_shared.ErrServiceNotExist))
			Expect(status).To(Equal(process_shared.ServiceInfo{}))
		})

		It("should handle corrupted PID file gracefully", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock to simulate corrupted PID file
			identifier := constants.ServicePathToIdentifier(servicePath)
			pidFile := filepath.Join(pm.ServiceDirectory, string(identifier), constants.PidFileName)

			// Mock filesystem to return invalid PID content
			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if path == pidFile {
					return []byte("not-a-number"), nil
				}
				return []byte{}, nil
			})

			// Mock Stat to simulate PID file exists
			mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
				if path == pidFile {
					return &mockFileInfo{name: "run.pid"}, nil
				}
				return nil, os.ErrNotExist
			})

			status, err := pm.Status(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(status.Status).To(Equal(process_shared.ServiceDown))
			Expect(status.Pid).To(Equal(0))
		})

		It("should handle filesystem error when reading PID file", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock to simulate filesystem error when reading PID file
			identifier := constants.ServicePathToIdentifier(servicePath)
			pidFile := filepath.Join(pm.ServiceDirectory, string(identifier), constants.PidFileName)

			// Mock filesystem to return error
			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if path == pidFile {
					return nil, fmt.Errorf("filesystem error")
				}
				return []byte{}, nil
			})

			status, err := pm.Status(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(status.Status).To(Equal(process_shared.ServiceDown))
			Expect(status.Pid).To(Equal(0))
		})
	})

	Describe("ServiceExists", func() {
		It("should return true for existing service", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Check if service exists
			exists, err := pm.ServiceExists(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should return false for non-existent service", func() {
			servicePath := "non-existent-service"

			// Check if service exists
			exists, err := pm.ServiceExists(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("should return false after service is removed", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify service exists
			exists, err := pm.ServiceExists(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())

			// Remove service
			err = pm.Remove(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Verify service no longer exists
			exists, err = pm.ServiceExists(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("should handle multiple service existence checks", func() {
			service1 := "service-1"
			service2 := "service-2"
			service3 := "non-existent-service"

			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create two services
			err := pm.Create(ctx, service1, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			err = pm.Create(ctx, service2, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Check all services
			exists1, err := pm.ServiceExists(ctx, service1, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists1).To(BeTrue())

			exists2, err := pm.ServiceExists(ctx, service2, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists2).To(BeTrue())

			exists3, err := pm.ServiceExists(ctx, service3, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists3).To(BeFalse())
		})
	})

	Describe("CleanServiceDirectory", func() {
		It("should handle directory cleaning operation successfully", func() {
			path := "/some/path"

			err := pm.CleanServiceDirectory(ctx, path, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle context cancellation", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			path := "/some/path"

			// CleanServiceDirectory should succeed even with canceled context
			// since it's a no-op for IPM
			err := pm.CleanServiceDirectory(cancelCtx, path, fsService)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("ForceRemove", func() {
		It("should handle force remove operation successfully", func() {
			servicePath := "test-service"

			err := pm.ForceRemove(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle non-existent service", func() {
			servicePath := "non-existent-service"

			err := pm.ForceRemove(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle context cancellation", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"

			// ForceRemove should succeed even with canceled context
			// since it's a no-op for IPM
			err := pm.ForceRemove(cancelCtx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("ExitHistory", func() {
		It("should return empty exit history for new service", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// ExitHistory should return empty list for new service
			exitHistory, err := pm.ExitHistory(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exitHistory).To(BeEmpty())
		})

		It("should return error for non-existent service", func() {
			servicePath := "non-existent-service"

			exitHistory, err := pm.ExitHistory(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(process_shared.ErrServiceNotExist))
			Expect(exitHistory).To(BeNil())
		})

		It("should record exit events when process terminates", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Manually record an exit event to simulate process termination
			identifier := constants.ServicePathToIdentifier(servicePath)
			pm.RecordExitEvent(identifier, 1, 0) // Normal exit with code 1

			// Check exit history
			exitHistory, err := pm.ExitHistory(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exitHistory).To(HaveLen(1))
			Expect(exitHistory[0].ExitCode).To(Equal(1))
			Expect(exitHistory[0].Signal).To(Equal(0))
			Expect(exitHistory[0].Timestamp).To(BeTemporally("~", time.Now(), time.Second))
		})

		It("should record signal-based exit events", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Manually record a signal-based exit event
			identifier := constants.ServicePathToIdentifier(servicePath)
			pm.RecordExitEvent(identifier, -1, int(syscall.SIGTERM)) // Killed by SIGTERM

			// Check exit history
			exitHistory, err := pm.ExitHistory(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exitHistory).To(HaveLen(1))
			Expect(exitHistory[0].ExitCode).To(Equal(-1))
			Expect(exitHistory[0].Signal).To(Equal(int(syscall.SIGTERM)))
		})

		It("should maintain multiple exit events in chronological order", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Record multiple exit events
			identifier := constants.ServicePathToIdentifier(servicePath)
			pm.RecordExitEvent(identifier, 0, 0)                     // Normal exit
			pm.RecordExitEvent(identifier, 1, 0)                     // Error exit
			pm.RecordExitEvent(identifier, -1, int(syscall.SIGKILL)) // Killed

			// Check exit history
			exitHistory, err := pm.ExitHistory(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exitHistory).To(HaveLen(3))

			// Verify events are in order
			Expect(exitHistory[0].ExitCode).To(Equal(0))
			Expect(exitHistory[0].Signal).To(Equal(0))

			Expect(exitHistory[1].ExitCode).To(Equal(1))
			Expect(exitHistory[1].Signal).To(Equal(0))

			Expect(exitHistory[2].ExitCode).To(Equal(-1))
			Expect(exitHistory[2].Signal).To(Equal(int(syscall.SIGKILL)))

			// Verify timestamps are in chronological order
			Expect(exitHistory[0].Timestamp).To(BeTemporally("<=", exitHistory[1].Timestamp))
			Expect(exitHistory[1].Timestamp).To(BeTemporally("<=", exitHistory[2].Timestamp))
		})

		It("should limit exit history to prevent unbounded growth", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Record more than 100 exit events to test the limit
			identifier := constants.ServicePathToIdentifier(servicePath)
			for i := 0; i < 105; i++ {
				pm.RecordExitEvent(identifier, i, 0)
			}

			// Check exit history is limited to 100 events
			exitHistory, err := pm.ExitHistory(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(exitHistory).To(HaveLen(100))

			// Verify the first 5 events were dropped and we have the last 100
			Expect(exitHistory[0].ExitCode).To(Equal(5))    // First kept event
			Expect(exitHistory[99].ExitCode).To(Equal(104)) // Last event
		})

		It("should handle recordExitEvent for non-existent service gracefully", func() {
			identifier := constants.ServicePathToIdentifier("non-existent-service")

			// This should not panic or cause errors
			pm.RecordExitEvent(identifier, 1, 0)

			// Try to get exit history for the non-existent service
			exitHistory, err := pm.ExitHistory(ctx, "non-existent-service", fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(process_shared.ErrServiceNotExist))
			Expect(exitHistory).To(BeNil())
		})
	})

	Describe("GetConfig", func() {
		It("should return config for existing service", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/usr/bin/my-app", "--config", "app.conf"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
					"app.conf":    "setting=production",
				},
				MemoryLimit: 512 * 1024 * 1024, // 512MB
				LogFilesize: 10 * 1024 * 1024,  // 10MB
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// GetConfig should return the stored configuration
			retrievedConfig, err := pm.GetConfig(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedConfig.ConfigFiles).To(Equal(config.ConfigFiles))
			Expect(retrievedConfig.MemoryLimit).To(Equal(config.MemoryLimit))
			Expect(retrievedConfig.LogFilesize).To(Equal(config.LogFilesize))
		})

		It("should return error for non-existent service", func() {
			servicePath := "non-existent-service"

			retrievedConfig, err := pm.GetConfig(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(process_shared.ErrServiceNotExist))
			Expect(retrievedConfig).To(Equal(process_manager_serviceconfig.ProcessManagerServiceConfig{}))
		})

		It("should handle service with no config files", func() {
			servicePath := "simple-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command:     []string{"/bin/echo", "simple"},
				ConfigFiles: map[string]string{}, // Empty config files
				MemoryLimit: 0,
				LogFilesize: 0,
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// GetConfig should return the stored configuration
			retrievedConfig, err := pm.GetConfig(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedConfig.ConfigFiles).To(BeEmpty())
			Expect(retrievedConfig.MemoryLimit).To(Equal(int64(0)))
			Expect(retrievedConfig.LogFilesize).To(Equal(int64(0)))
		})

		It("should handle context cancellation", func() {
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"

			retrievedConfig, err := pm.GetConfig(cancelledCtx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(retrievedConfig).To(Equal(process_manager_serviceconfig.ProcessManagerServiceConfig{}))
		})
	})

	Describe("GetConfigFile", func() {
		It("should return config file content for existing service and file", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/usr/bin/database-server", "--port", "5432"},
				ConfigFiles: map[string]string{
					"config.yaml": "database:\n  host: localhost\n  port: 5432",
					"app.conf":    "debug=true\nport=8080",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// GetConfigFile should return the specific file content
			content, err := pm.GetConfigFile(ctx, servicePath, "config.yaml", fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(content)).To(Equal("database:\n  host: localhost\n  port: 5432"))

			// Test the other file too
			content, err = pm.GetConfigFile(ctx, servicePath, "app.conf", fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(content)).To(Equal("debug=true\nport=8080"))
		})

		It("should return error for non-existent service", func() {
			servicePath := "non-existent-service"

			content, err := pm.GetConfigFile(ctx, servicePath, "config.yaml", fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(process_shared.ErrServiceNotExist))
			Expect(content).To(BeNil())
		})

		It("should return error for non-existent config file", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Try to get a non-existent config file
			content, err := pm.GetConfigFile(ctx, servicePath, "non-existent.conf", fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))
			Expect(content).To(BeNil())
		})

		It("should handle empty config file content", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"empty.conf":  "", // Empty file
					"normal.conf": "content=value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// GetConfigFile should return empty content for empty file
			content, err := pm.GetConfigFile(ctx, servicePath, "empty.conf", fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(content).To(BeEmpty())

			// Verify normal file still works
			content, err = pm.GetConfigFile(ctx, servicePath, "normal.conf", fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(content)).To(Equal("content=value"))
		})

		It("should handle context cancellation", func() {
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			servicePath := "test-service"

			content, err := pm.GetConfigFile(cancelledCtx, servicePath, "config.yaml", fsService)
			Expect(err).To(HaveOccurred())
			Expect(content).To(BeNil())
		})
	})

	Describe("GetLogs", func() {
		It("should return empty logs for service with no log file", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// GetLogs should return empty slice when no log file exists
			logs, err := pm.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(logs).To(HaveLen(0))
		})

		It("should return error for non-existent service", func() {
			servicePath := "non-existent-service"

			logs, err := pm.GetLogs(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(process_shared.ErrServiceNotExist))
			Expect(logs).To(BeNil())
		})

		It("should return empty logs for empty log file", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock to simulate empty log file
			identifier := constants.ServicePathToIdentifier(servicePath)
			logFile := filepath.Join(pm.ServiceDirectory, string(identifier), constants.LogDirectoryName, constants.CurrentLogFileName)

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if path == logFile {
					return true, nil
				}
				return false, nil
			})
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if path == logFile {
					return []byte(""), nil // Empty log file
				}
				return []byte{}, nil
			})

			logs, err := pm.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(logs).To(HaveLen(0))
		})

		It("should parse log entries correctly", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock to simulate log file with content
			identifier := constants.ServicePathToIdentifier(servicePath)
			logFile := filepath.Join(pm.ServiceDirectory, string(identifier), constants.LogDirectoryName, constants.CurrentLogFileName)

			// Create sample log content in the format expected by the parser
			logContent := `2025-01-15 10:30:45.123456789  Starting application
2025-01-15 10:30:45.987654321  Configuration loaded successfully
2025-01-15 10:30:46.555555555  Application ready
`

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if path == logFile {
					return true, nil
				}
				return false, nil
			})
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if path == logFile {
					return []byte(logContent), nil
				}
				return []byte{}, nil
			})

			logs, err := pm.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(logs).To(HaveLen(3))

			// Verify the parsed entries
			Expect(logs[0].Content).To(Equal("Starting application"))
			Expect(logs[0].Timestamp.Year()).To(Equal(2025))
			Expect(logs[0].Timestamp.Month()).To(Equal(time.January))
			Expect(logs[0].Timestamp.Day()).To(Equal(15))

			Expect(logs[1].Content).To(Equal("Configuration loaded successfully"))
			Expect(logs[2].Content).To(Equal("Application ready"))
		})

		It("should handle malformed log entries gracefully", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock to simulate log file with mixed content (some valid, some malformed)
			identifier := constants.ServicePathToIdentifier(servicePath)
			logFile := filepath.Join(pm.ServiceDirectory, string(identifier), constants.LogDirectoryName, constants.CurrentLogFileName)

			logContent := `2025-01-15 10:30:45.123456789  Valid log entry
malformed log entry without timestamp
2025-01-15 10:30:46.555555555  Another valid entry
just plain text
`

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if path == logFile {
					return true, nil
				}
				return false, nil
			})
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if path == logFile {
					return []byte(logContent), nil
				}
				return []byte{}, nil
			})

			logs, err := pm.GetLogs(ctx, servicePath, fsService)
			Expect(err).ToNot(HaveOccurred())
			Expect(logs).To(HaveLen(4)) // Should include all lines, even malformed ones

			// Verify valid entries have parsed timestamps
			Expect(logs[0].Content).To(Equal("Valid log entry"))
			Expect(logs[0].Timestamp.IsZero()).To(BeFalse())

			// Verify malformed entries are preserved as content-only
			Expect(logs[1].Content).To(Equal("malformed log entry without timestamp"))
			Expect(logs[1].Timestamp.IsZero()).To(BeTrue()) // No valid timestamp

			Expect(logs[2].Content).To(Equal("Another valid entry"))
			Expect(logs[2].Timestamp.IsZero()).To(BeFalse())

			Expect(logs[3].Content).To(Equal("just plain text"))
			Expect(logs[3].Timestamp.IsZero()).To(BeTrue()) // No valid timestamp
		})

		It("should handle filesystem errors when reading log file", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock to simulate filesystem error when reading log file
			identifier := constants.ServicePathToIdentifier(servicePath)
			logFile := filepath.Join(pm.ServiceDirectory, string(identifier), constants.LogDirectoryName, constants.CurrentLogFileName)

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if path == logFile {
					return true, nil
				}
				return false, nil
			})
			mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
				if path == logFile {
					return nil, fmt.Errorf("filesystem read error")
				}
				return []byte{}, nil
			})

			logs, err := pm.GetLogs(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read log file"))
			Expect(err.Error()).To(ContainSubstring("filesystem read error"))
			Expect(logs).To(BeNil())
		})

		It("should handle filesystem errors when checking log file existence", func() {
			servicePath := "test-service"
			config := process_manager_serviceconfig.ProcessManagerServiceConfig{
				Command: []string{"/bin/echo", "test"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Create service first
			err := pm.Create(ctx, servicePath, config, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Process the creation
			err = pm.Reconcile(ctx, fsService)
			Expect(err).ToNot(HaveOccurred())

			// Setup mock to simulate filesystem error when checking file existence
			identifier := constants.ServicePathToIdentifier(servicePath)
			logFile := filepath.Join(pm.ServiceDirectory, string(identifier), constants.LogDirectoryName, constants.CurrentLogFileName)

			mockFS := fsService.(*filesystem.MockFileSystem)
			mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
				if path == logFile {
					return false, fmt.Errorf("filesystem error checking file existence")
				}
				return false, nil
			})

			logs, err := pm.GetLogs(ctx, servicePath, fsService)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check if log file exists"))
			Expect(err.Error()).To(ContainSubstring("filesystem error checking file existence"))
			Expect(logs).To(BeNil())
		})
	})

	Describe("generateContext", func() {
		It("should create a context with proper deadline", func() {
			timeout := 100 * time.Millisecond
			parentCtx, parentCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer parentCancel()

			childCtx, childCancel, err := ipm.GenerateContext(parentCtx, timeout)
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

			_, _, err := ipm.GenerateContext(parentCtx, timeout)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context has no deadline"))
		})

		It("should return error when parent context doesn't have enough time", func() {
			timeout := 200 * time.Millisecond
			parentCtx, parentCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer parentCancel()

			_, _, err := ipm.GenerateContext(parentCtx, timeout)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context has no enough time"))
		})

		It("should return error when parent context is already cancelled", func() {
			parentCtx, parentCancel := context.WithCancel(context.Background())
			parentCancel() // Cancel immediately

			timeout := 100 * time.Millisecond

			_, _, err := ipm.GenerateContext(parentCtx, timeout)
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
			pm = ipm.NewProcessManager(logger, ipm.WithServiceDirectory(tempDir))

			// Debug: Check if ProcessManager has correct service directory
			GinkgoT().Logf("ProcessManager service directory: %s", pm.ServiceDirectory)
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
				Command: []string{"/bin/echo", "Hello World"},
				ConfigFiles: map[string]string{
					"config.yaml": "test: value",
				},
			}

			// Debug: Check task queue before Create
			Expect(pm.TaskQueue).To(HaveLen(0), "Task queue should be empty before Create")

			err := pm.Create(ctx, servicePath, config, realFS)

			// Debug: Check what error occurred, if any
			if err != nil {
				GinkgoT().Logf("Create returned error: %v", err)
			}

			Expect(err).ToNot(HaveOccurred())

			// Verify task was queued
			GinkgoT().Logf("Task queue length after Create: %d", len(pm.TaskQueue))
			if len(pm.TaskQueue) > 0 {
				GinkgoT().Logf("First task in queue: %+v", pm.TaskQueue[0])
			}
			Expect(pm.TaskQueue).To(HaveLen(1), "Task queue should have one task after Create")
			Expect(pm.TaskQueue[0].Operation).To(Equal(ipm.OperationCreate))

			// Process the queued task
			err = pm.Reconcile(ctx, realFS)
			Expect(err).ToNot(HaveOccurred())

			// Debug: Check if task queue is empty (meaning Reconcile() processed the task)
			Expect(pm.TaskQueue).To(HaveLen(0), "Task queue should be empty after Reconcile")

			// Verify service was added to services map
			identifier := constants.ServicePathToIdentifier(servicePath)
			service, exists := pm.Services.Load(identifier)
			Expect(exists).To(BeTrue())
			ipmService := service.(ipm.IpmService)
			Expect(ipmService.Config).To(Equal(config))

			// Verify directories were created on real filesystem
			// Note: We use the identifier (not servicePath) because that's what ProcessManager uses internally
			logDir := filepath.Join(tempDir, string(identifier), constants.LogDirectoryName)
			configDir := filepath.Join(tempDir, string(identifier), constants.ServiceDirectoryName)

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
			Expect(string(content)).To(Equal("#!/bin/bash\n# Auto-generated run script for service\nset -e\n\n# Execute the service command\nexec '/bin/echo' 'Hello World'\n"))
		})

		It("should use custom service directory", func() {
			// Verify ProcessManager is using the custom directory
			Expect(pm.ServiceDirectory).To(Equal(tempDir))
		})
	})
})
