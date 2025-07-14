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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// removeService orchestrates the complete removal of a service from the process manager.
// This is a critical operation that must handle both running and stopped services gracefully.
// The function first attempts to terminate any running process associated with the service,
// then removes all service files and directories. The operation is designed to be robust -
// even if process termination fails, the cleanup continues to ensure the service is fully
// removed from the system. This prevents orphaned services that could consume resources
// or cause confusion in service management operations.
func (pm *ProcessManager) removeService(ctx context.Context, identifier serviceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Info("Removing service", zap.String("identifier", string(identifier)))

	servicePath := filepath.Join(pm.serviceDirectory, string(identifier))

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

// terminateServiceProcess attempts to gracefully terminate a running service process.
// This function handles the complex task of finding and stopping a service process that
// may or may not be running. It first checks for the existence of a PID file, which
// indicates that a process was started. If a process is found, it attempts graceful
// termination. This approach ensures that running services are stopped cleanly during
// service removal, preventing resource leaks and allowing processes to perform cleanup
// operations before shutting down.
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

// readProcessPid reads and parses the process ID from the service's PID file.
// The PID file is created when a service process starts and contains the process ID
// that the operating system assigns to the running service. This function is essential
// for process management because it provides the link between the service configuration
// and the actual running process. If the PID file doesn't exist or is corrupted,
// it indicates that the service is not running, which is valuable information for
// service management operations.
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

// terminateProcess handles the graceful termination of a process with fallback to force termination.
// This function implements a two-stage shutdown process that respects the service's need to clean up
// resources while ensuring that the termination completes within reasonable time bounds. It first
// sends SIGTERM to allow the process to shut down gracefully, then waits for the process to exit.
// If the process doesn't respond to SIGTERM or takes too long to exit, it falls back to SIGKILL
// for immediate termination. This approach balances clean shutdown with system reliability.
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

// waitForProcessExit waits for a process to exit within a specified timeout period.
// This function is crucial for implementing graceful shutdown behavior because it allows
// processes time to complete their cleanup operations after receiving SIGTERM. The function
// uses an error group to race between process exit and context timeout, ensuring that
// the termination process doesn't hang indefinitely. The timeout is calculated to leave
// sufficient time for the overall service removal operation to complete, maintaining
// system responsiveness while allowing reasonable cleanup time.
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

// forceKillProcess sends SIGKILL to forcefully terminate a process that didn't respond to SIGTERM.
// This function serves as the final step in process termination when graceful shutdown fails.
// SIGKILL cannot be ignored or caught by processes, making it the ultimate tool for ensuring
// that processes are terminated. While this approach doesn't allow for clean shutdown, it's
// necessary to prevent hung processes from blocking service management operations or consuming
// system resources indefinitely. The function is designed to be used only after graceful
// termination attempts have failed.
func (pm *ProcessManager) forceKillProcess(process *os.Process, pid int) error {
	if err := process.Signal(syscall.SIGKILL); err != nil {
		pm.Logger.Error("Error sending SIGKILL", zap.Int("pid", pid), zap.Error(err))
		return err
	}

	pm.Logger.Info("Process force killed", zap.Int("pid", pid))
	return nil
}

// cleanupServiceDirectory removes the service directory and all its contents from the filesystem.
// This function performs the final cleanup step in service removal, ensuring that all service
// files, logs, configuration, and other data are completely removed from the system. This is
// essential for preventing disk space accumulation and ensuring that removed services don't
// leave behind artifacts that could cause confusion or conflicts if a service with the same
// name is created later. The function uses RemoveAll to ensure complete directory removal
// regardless of the contents or subdirectory structure.
func (pm *ProcessManager) cleanupServiceDirectory(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	if err := fsService.RemoveAll(ctx, servicePath); err != nil {
		return fmt.Errorf("error removing pid file: %w", err)
	}
	return nil
}
