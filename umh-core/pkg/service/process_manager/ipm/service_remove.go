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

package ipm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// removeService orchestrates the complete removal of a service from the process manager.
// This is a critical operation that must handle both running and stopped Services gracefully.
// The function first attempts to terminate any running process associated with the service,
// then removes all service files and directories. The operation is designed to be robust -
// even if process termination fails, the cleanup continues to ensure the service is fully
// removed from the system. This prevents orphaned Services that could consume resources
// or cause confusion in service management operations.
func (pm *ProcessManager) removeService(ctx context.Context, identifier ServiceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Info("Removing service", zap.String("identifier", string(identifier)))

	servicePath := filepath.Join(pm.ServiceDirectory, string(identifier))

	// Close the log file for this service to prevent file handle leaks
	if pm.logManager != nil {
		pm.logManager.UnregisterService(identifier)
	} else {
		pm.Logger.Warn("LogManager is nil - skipping service unregistration from log rotation",
			zap.String("identifier", string(identifier)))
	}

	// Attempt to terminate any running process gracefully
	if err := pm.terminateServiceProcess(ctx, identifier, servicePath, fsService); err != nil {
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

// stopService stops a running service process without removing the service directory.
// This function gracefully terminates the service process and removes the PID file while
// preserving the service configuration and logs. It uses the same termination logic as
// removeService but only cleans up the process state. This allows Services to be stopped
// temporarily without losing their configuration or historical data, enabling easy restart
// operations later.
func (pm *ProcessManager) stopService(ctx context.Context, identifier ServiceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Info("Stopping service", zap.String("identifier", string(identifier)))

	// Check if the service exists in our services map
	if _, exists := pm.Services[identifier]; !exists {
		return fmt.Errorf("service %s not found", string(identifier))
	}

	servicePath := filepath.Join(pm.ServiceDirectory, string(identifier))
	pidFile := filepath.Join(servicePath, PidFileName)

	// Attempt to terminate the running process gracefully
	if err := pm.terminateServiceProcess(ctx, identifier, servicePath, fsService); err != nil {
		// For stop operations, we're more forgiving about termination errors
		// If the process is already dead, that's fine - we just want it stopped
		pm.Logger.Debug("Process termination had issues during stop, but continuing", zap.Error(err))
	}

	// Remove the PID file after termination attempt
	if err := fsService.Remove(ctx, pidFile); err != nil {
		// If the PID file doesn't exist, that's fine - the process might have cleaned up itself
		if !os.IsNotExist(err) {
			pm.Logger.Error("Error removing PID file", zap.Error(err))
			return fmt.Errorf("error removing PID file: %w", err)
		}
	}

	pm.Logger.Info("Service stopped successfully", zap.String("identifier", string(identifier)))
	return nil
}

// terminateServiceProcess attempts to gracefully terminate a running service process.
// This function handles the complex task of finding and stopping a service process that
// may or may not be running. It first checks for the existence of a PID file, which
// indicates that a process was started. If a process is found, it attempts graceful
// termination. This approach ensures that running Services are stopped cleanly during
// service removal, preventing resource leaks and allowing processes to perform cleanup
// operations before shutting down.
func (pm *ProcessManager) terminateServiceProcess(ctx context.Context, identifier ServiceIdentifier, servicePath string, fsService filesystem.Service) error {
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

	return pm.terminateProcess(ctx, identifier, process, pid)
}

// readProcessPid reads and parses the process ID from the service's PID file.
// The PID file is created when a service process starts and contains the process ID
// that the operating system assigns to the running service. This function is essential
// for process management because it provides the link between the service configuration
// and the actual running process. If the PID file doesn't exist or is corrupted,
// it indicates that the service is not running, which is valuable information for
// service management operations.
func (pm *ProcessManager) readProcessPid(ctx context.Context, servicePath string, fsService filesystem.Service) (int, error) {
	pidFile := filepath.Join(servicePath, PidFileName)
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
// Additionally, it attempts to terminate the entire process group to ensure child processes are cleaned up.
func (pm *ProcessManager) terminateProcess(ctx context.Context, identifier ServiceIdentifier, process *os.Process, pid int) error {
	// First attempt: Send SIGTERM (graceful shutdown)
	// Try to terminate the process group first (negative PID), then fall back to individual process
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		pm.Logger.Debug("Failed to send SIGTERM to process group, trying individual process", zap.Int("pid", pid), zap.Error(err))
		if err := process.Signal(syscall.SIGTERM); err != nil {
			pm.Logger.Error("Error sending SIGTERM, trying SIGKILL", zap.Int("pid", pid), zap.Error(err))
			return pm.forceKillProcess(identifier, process, pid)
		}
	} else {
		pm.Logger.Debug("Sent SIGTERM to process group", zap.Int("pid", pid))
	}

	// Wait for graceful shutdown with timeout
	processState, err := pm.waitForProcessExit(ctx, process, pid)
	if err != nil {
		pm.Logger.Error("Process did not exit gracefully, forcing termination", zap.Int("pid", pid), zap.Error(err))
		return pm.forceKillProcess(identifier, process, pid)
	}

	// Record the exit event if we have a service identifier and process state
	if identifier != "" && processState != nil {
		exitCode := 0
		signal := int(syscall.SIGTERM) // Process was terminated by SIGTERM

		if processState.Exited() {
			exitCode = processState.ExitCode()
			signal = 0 // Normal exit, no signal
		} else if ws, ok := processState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			exitCode = -1 // Process was killed by signal
			signal = int(ws.Signal())
		}

		pm.RecordExitEvent(identifier, exitCode, signal)
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
// Returns the exit status if the process exits normally, or an error if it times out.
func (pm *ProcessManager) waitForProcessExit(ctx context.Context, process *os.Process, pid int) (*os.ProcessState, error) {
	waitCtx, cancel, err := GenerateContext(ctx, cleanupTimeReserve)
	if err != nil {
		pm.Logger.Error("Error generating wait context", zap.Error(err))
		return nil, err
	}
	defer cancel()

	// Wait for either process exit or timeout
	errGroup, _ := errgroup.WithContext(waitCtx)
	var processState *os.ProcessState

	errGroup.Go(func() error {
		state, err := process.Wait()
		if err != nil {
			return err
		}
		processState = state
		return nil
	})

	errGroup.Go(func() error {
		<-waitCtx.Done()
		return waitCtx.Err()
	})

	// Await the process to exit or the context to be cancelled
	err = errGroup.Wait()
	return processState, err
}

// forceKillProcess forcefully terminates a process using SIGKILL when graceful shutdown fails.
// This function serves as the final step in process termination when graceful shutdown fails.
// SIGKILL cannot be caught or ignored by the process, ensuring immediate termination. This is
// necessary to prevent hung processes from blocking service management operations or consuming
// system resources indefinitely. The function is designed to be used only after graceful
// termination attempts have failed.
// Additionally, it attempts to terminate the entire process group to ensure child processes are cleaned up.
func (pm *ProcessManager) forceKillProcess(identifier ServiceIdentifier, process *os.Process, pid int) error {
	// Try to kill the process group first (negative PID), then fall back to individual process
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		pm.Logger.Debug("Failed to send SIGKILL to process group, trying individual process", zap.Int("pid", pid), zap.Error(err))
		if err := process.Signal(syscall.SIGKILL); err != nil {
			pm.Logger.Error("Error sending SIGKILL to process", zap.Int("pid", pid), zap.Error(err))
			return err
		}
	} else {
		pm.Logger.Debug("Sent SIGKILL to process group", zap.Int("pid", pid))
	}

	// Record the exit event for force kill if we have a service identifier
	if identifier != "" {
		// For force kill, exit code is -1 and signal is SIGKILL
		pm.RecordExitEvent(identifier, -1, int(syscall.SIGKILL))
	}

	pm.Logger.Info("Process force-killed", zap.Int("pid", pid))
	return nil
}

// cleanupServiceDirectory removes the service directory and all its contents from the filesystem.
// This function performs the final cleanup step in service removal, ensuring that all service
// files, logs, configuration, and other data are completely removed from the system. This is
// essential for preventing disk space accumulation and ensuring that removed Services don't
// leave behind artifacts that could cause confusion or conflicts if a service with the same
// name is created later. The function uses RemoveAll to ensure complete directory removal
// regardless of the contents or subdirectory structure.
func (pm *ProcessManager) cleanupServiceDirectory(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	if err := fsService.RemoveAll(ctx, servicePath); err != nil {
		return fmt.Errorf("error removing pid file: %w", err)
	}
	return nil
}

// RecordExitEvent records an exit event in the service history
func (pm *ProcessManager) RecordExitEvent(identifier ServiceIdentifier, exitCode int, signal int) {
	// Check if service exists in our registry
	service, exists := pm.Services[identifier]
	if !exists {
		pm.Logger.Debug("Service not found when recording exit event", zap.String("identifier", string(identifier)))
		return
	}

	// Create exit event
	exitEvent := process_shared.ExitEvent{
		Timestamp: time.Now().UTC(),
		ExitCode:  exitCode,
		Signal:    signal,
	}

	// Add to exit history
	service.History.ExitHistory = append(service.History.ExitHistory, exitEvent)

	// Keep only the last N exit events to prevent unbounded memory growth
	if len(service.History.ExitHistory) > MaxExitEvents {
		service.History.ExitHistory = service.History.ExitHistory[len(service.History.ExitHistory)-MaxExitEvents:]
	}

	// Update service in the registry
	pm.Services[identifier] = service

	pm.Logger.Debug("Recorded exit event",
		zap.String("identifier", string(identifier)),
		zap.Int("exitCode", exitCode),
		zap.Int("signal", signal),
		zap.Time("timestamp", exitEvent.Timestamp))
}
