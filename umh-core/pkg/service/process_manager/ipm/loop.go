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
	"strconv"
	"syscall"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	// Directory structure constants
	logDirectoryName    = "log"
	configDirectoryName = "config"
	pidFileName         = "run.pid"

	// File permissions
	configFilePermission = 0644

	// Process termination constants
	cleanupTimeReserve = 20 * time.Millisecond
	stepTimeThreshold  = 10 * time.Millisecond
)

// step assumes that the mutex is already locked
func (pm *ProcessManager) step(ctx context.Context, fsService filesystem.Service) error {
	// Check if we still got time to run (from the context)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if there are any services to be created
	if len(pm.toBeCreated) > 0 {
		pm.Logger.Info("Creating services", zap.Int("count", len(pm.toBeCreated)))
		err := pm.createService(ctx, pm.toBeCreated[0], fsService)
		if err != nil {
			pm.Logger.Error("Error creating service", zap.Error(err))
			return err
		}
		// Remove the service from the list (this is safe as we hold the mutex)
		pm.toBeCreated = pm.toBeCreated[1:]
	}

	// Check if there are any services to be removed
	if len(pm.toBeRemoved) > 0 {
		pm.Logger.Info("Removing services", zap.Int("count", len(pm.toBeRemoved)))
		err := pm.removeService(ctx, pm.toBeRemoved[0], fsService)
		if err != nil {
			pm.Logger.Error("Error removing service", zap.Error(err))
			return err
		}
		// Remove the service from the list (this is safe as we hold the mutex)
		pm.toBeRemoved = pm.toBeRemoved[1:]
	}

	// If we have more time, we will step again
	deadline, ok := ctx.Deadline()
	if !ok {
		pm.Logger.Error("Context has no deadline, this should never happen")
	} else if time.Until(deadline) > stepTimeThreshold {
		pm.Logger.Info("Stepping again", zap.Duration("timeUntilDeadline", time.Until(deadline)))
		return pm.step(ctx, fsService)
	} else {
		pm.Logger.Info("No time left, stopping")
	}

	return nil
}

func (pm *ProcessManager) createService(ctx context.Context, identifier serviceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Info("Creating service", zap.String("identifier", string(identifier)))

	// Validate service exists in our registry
	service, err := pm.getServiceConfig(identifier)
	if err != nil {
		return err
	}

	// Create necessary directory structure
	if err := pm.createServiceDirectories(ctx, identifier, fsService); err != nil {
		return err
	}

	// Write all configuration files
	if err := pm.writeServiceConfigFiles(ctx, identifier, service.config, fsService); err != nil {
		return err
	}

	pm.Logger.Info("Service created successfully", zap.String("identifier", string(identifier)))
	return nil
}

// getServiceConfig retrieves the service configuration from the internal registry
func (pm *ProcessManager) getServiceConfig(identifier serviceIdentifier) (service, error) {
	service, exists := pm.services[identifier]
	if !exists {
		// This should never happen as we only add services to the list that exist
		return service, fmt.Errorf("service %s not found", identifier)
	}
	return service, nil
}

// createServiceDirectories creates the required directory structure for a service
func (pm *ProcessManager) createServiceDirectories(ctx context.Context, identifier serviceIdentifier, fsService filesystem.Service) error {
	servicePath := filepath.Join(IPM_SERVICE_DIRECTORY, string(identifier))

	directories := []string{
		filepath.Join(servicePath, logDirectoryName),
		filepath.Join(servicePath, configDirectoryName),
	}

	for _, dir := range directories {
		if err := fsService.EnsureDirectory(ctx, dir); err != nil {
			return fmt.Errorf("error creating directory %s: %w", dir, err)
		}
	}

	return nil
}

// writeServiceConfigFiles writes all configuration files for a service
func (pm *ProcessManager) writeServiceConfigFiles(ctx context.Context, identifier serviceIdentifier, config process_manager_serviceconfig.ProcessManagerServiceConfig, fsService filesystem.Service) error {
	configDirectory := filepath.Join(IPM_SERVICE_DIRECTORY, string(identifier), configDirectoryName)

	for configFileName, configFileContent := range config.ConfigFiles {
		configFilePath := filepath.Join(configDirectory, configFileName)
		if err := fsService.WriteFile(ctx, configFilePath, []byte(configFileContent), configFilePermission); err != nil {
			return fmt.Errorf("error writing config file %s: %w", configFileName, err)
		}
	}

	return nil
}

func (pm *ProcessManager) removeService(ctx context.Context, identifier serviceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Info("Removing service", zap.String("identifier", string(identifier)))

	servicePath := filepath.Join(IPM_SERVICE_DIRECTORY, string(identifier))

	// Attempt to terminate any running process gracefully
	if err := pm.terminateServiceProcess(ctx, servicePath, fsService); err != nil {
		// Log error but continue with cleanup - we want to remove the service directory regardless
		pm.Logger.Error("Error terminating process, continuing with cleanup", zap.Error(err))
	}

	// Clean up the service directory
	if err := pm.cleanupServiceDirectory(ctx, servicePath, fsService); err != nil {
		return err
	}

	pm.Logger.Info("Service removed successfully", zap.String("identifier", string(identifier)))
	return nil
}

// terminateServiceProcess attempts to gracefully terminate a running service process
func (pm *ProcessManager) terminateServiceProcess(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	// Read the process PID from the pid file
	pid, err := pm.readProcessPid(ctx, servicePath, fsService)
	if err != nil {
		// No process to terminate
		return nil
	}

	// Find and terminate the process
	process, err := os.FindProcess(pid)
	if err != nil {
		pm.Logger.Info("Process not running anymore, skipping termination", zap.Int("pid", pid))
		return nil
	}

	return pm.terminateProcess(ctx, process, pid)
}

// readProcessPid reads and parses the process PID from the pid file
func (pm *ProcessManager) readProcessPid(ctx context.Context, servicePath string, fsService filesystem.Service) (int, error) {
	pidFile := filepath.Join(servicePath, pidFileName)
	pidBytes, err := fsService.ReadFile(ctx, pidFile)
	if err != nil {
		pm.Logger.Info("Process not running anymore, skipping termination", zap.String("pidFile", pidFile))
		return 0, err
	}

	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		return 0, fmt.Errorf("error parsing pid: %w", err)
	}

	return pid, nil
}

// terminateProcess handles the graceful termination of a process with fallback to force kill
func (pm *ProcessManager) terminateProcess(ctx context.Context, process *os.Process, pid int) error {
	// First attempt: Send SIGTERM (graceful shutdown)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		pm.Logger.Error("Error sending SIGTERM, trying SIGKILL", zap.Int("pid", pid), zap.Error(err))
		return pm.forceKillProcess(process, pid)
	}

	// Wait for graceful shutdown with timeout
	if err := pm.waitForProcessExit(ctx, process, pid); err != nil {
		pm.Logger.Error("Process did not exit gracefully, forcing termination", zap.Int("pid", pid), zap.Error(err))
		return pm.forceKillProcess(process, pid)
	}

	pm.Logger.Info("Process terminated gracefully", zap.Int("pid", pid))
	return nil
}

// waitForProcessExit waits for a process to exit with a timeout
func (pm *ProcessManager) waitForProcessExit(ctx context.Context, process *os.Process, pid int) error {
	waitCtx, cancel, err := generateContext(ctx, cleanupTimeReserve)
	if err != nil {
		pm.Logger.Error("Error generating wait context", zap.Error(err))
		return err
	}
	defer cancel()

	// Wait for either process exit or timeout
	errGroup, _ := errgroup.WithContext(waitCtx)

	errGroup.Go(func() error {
		_, err := process.Wait()
		return err
	})

	errGroup.Go(func() error {
		<-waitCtx.Done()
		return waitCtx.Err()
	})

	// Await the process to exit or the context to be cancelled
	return errGroup.Wait()
}

// forceKillProcess sends SIGKILL to forcefully terminate a process
func (pm *ProcessManager) forceKillProcess(process *os.Process, pid int) error {
	if err := process.Signal(syscall.SIGKILL); err != nil {
		pm.Logger.Error("Error sending SIGKILL", zap.Int("pid", pid), zap.Error(err))
		return err
	}

	pm.Logger.Info("Process force killed", zap.Int("pid", pid))
	return nil
}

// cleanupServiceDirectory removes the service directory and all its contents
func (pm *ProcessManager) cleanupServiceDirectory(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	if err := fsService.RemoveAll(ctx, servicePath); err != nil {
		return fmt.Errorf("error removing pid file: %w", err)
	}
	return nil
}

// generateContext generates a new context that is so guaranteed to leave timeout in the original context
func generateContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	// Check if we have enough time on the ctx (if not, we will return an error)
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, nil, fmt.Errorf("context has no deadline")
	}

	// Check if we have enough time on the ctx (if not, we will return an error)
	if time.Until(deadline) < timeout {
		return nil, nil, fmt.Errorf("context has no enough time")
	}

	// Create a new context with deadline-timeout
	ctx, cancel := context.WithDeadline(ctx, deadline.Add(-timeout))
	return ctx, cancel, nil
}
