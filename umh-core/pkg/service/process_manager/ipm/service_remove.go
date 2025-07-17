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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"golang.org/x/sync/errgroup"
)

// removeService orchestrates the complete removal of a service from the process manager.
// This is a critical operation that must handle both running and stopped Services gracefully.
// The function first attempts to terminate any running process associated with the service,
// then removes all service files and directories. The operation is designed to be robust -
// even if process termination fails, the cleanup continues to ensure the service is fully
// removed from the system. This prevents orphaned Services that could consume resources
// or cause confusion in service management operations.
func (pm *ProcessManager) removeService(ctx context.Context, identifier constants.ServiceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Infof("Removing service: %s", identifier)

	servicePath := string(identifier) // Convert identifier back to servicePath

	// Close the LogLineWriter for this service before removal
	if serviceValue, exists := pm.Services.Load(identifier); exists {
		service := serviceValue.(IpmService)
		if service.LogLineWriter != nil {
			if err := service.LogLineWriter.Close(); err != nil {
				pm.Logger.Errorf("Error closing LogLineWriter during service removal for %s: %v", identifier, err)
			}
		}
	}

	// Close the log file for this service to prevent file handle leaks
	if pm.logManager != nil {
		pm.logManager.UnregisterService(identifier)
	} else {
		pm.Logger.Warnf("LogManager is nil - skipping service unregistration from log rotation for: %s", identifier)
	}

	// Attempt to terminate any running process gracefully
	if err := pm.terminateServiceProcess(ctx, identifier, servicePath, fsService); err != nil {
		// Log error but continue with cleanup - we want to remove the service directory regardless
		pm.Logger.Errorf("Error terminating process, continuing with cleanup: %v", err)
	}

	// Clean up the service directories (both logs and services)
	if err := pm.cleanupServiceDirectories(ctx, servicePath, fsService); err != nil {
		return err
	}

	pm.Logger.Infof("Service removed successfully: %s", identifier)
	return nil
}

// stopService stops a running service process without removing the service directory.
// This function gracefully terminates the service process and removes the PID file while
// preserving the service configuration and logs. It uses the same termination logic as
// removeService but only cleans up the process state. This allows Services to be stopped
// temporarily without losing their configuration or historical data, enabling easy restart
// operations later.
func (pm *ProcessManager) stopService(ctx context.Context, identifier constants.ServiceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Infof("Stopping service: %s", identifier)

	// Check if the service exists in our services map
	if _, exists := pm.Services.Load(identifier); !exists {
		return fmt.Errorf("service %s not found", string(identifier))
	}

	servicePath := string(identifier) // Convert identifier back to servicePath
	pidFile := filepath.Join(pm.ServiceDirectory, "services", servicePath, constants.PidFileName)

	// Attempt to terminate the running process gracefully
	if err := pm.terminateServiceProcess(ctx, identifier, servicePath, fsService); err != nil {
		// For stop operations, we're more forgiving about termination errors
		// If the process is already dead, that's fine - we just want it stopped
		pm.Logger.Debugf("Process termination had issues during stop, but continuing: %v", err)
	}

	// Remove the PID file after termination attempt
	if err := fsService.Remove(ctx, pidFile); err != nil {
		// If the PID file doesn't exist, that's fine - the process might have cleaned up itself
		if !os.IsNotExist(err) {
			pm.Logger.Errorf("Error removing PID file: %v", err)
			return fmt.Errorf("error removing PID file: %w", err)
		}
	}

	pm.Logger.Infof("Service stopped successfully: %s", identifier)
	return nil
}

// terminateServiceProcess attempts to gracefully terminate a running service process.
// This function handles the complex task of finding and stopping a service process that
// may or may not be running. It first checks for the existence of a PID file, which
// indicates that a process was started. If a process is found, it attempts graceful
// termination. This approach ensures that running Services are stopped cleanly during
// service removal, preventing resource leaks and allowing processes to perform cleanup
// operations before shutting down.
func (pm *ProcessManager) terminateServiceProcess(ctx context.Context, identifier constants.ServiceIdentifier, servicePath string, fsService filesystem.Service) error {
	// Read the process PID from the pid file
	pid, err := pm.readProcessPid(ctx, servicePath, fsService)
	if err != nil {
		// No process to terminate
		return nil
	}

	// Find and terminate the process
	process, err := os.FindProcess(pid)
	if err != nil {
		pm.Logger.Infof("Process not running anymore, skipping termination: %d", pid)
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
	pidFile := filepath.Join(pm.ServiceDirectory, "services", servicePath, constants.PidFileName)
	pidBytes, err := fsService.ReadFile(ctx, pidFile)
	if err != nil {
		pm.Logger.Infof("Process not running anymore, skipping termination: %s", pidFile)
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
func (pm *ProcessManager) terminateProcess(ctx context.Context, identifier constants.ServiceIdentifier, process *os.Process, pid int) error {
	// First attempt: Send SIGTERM (graceful shutdown)
	// Try to terminate the process group first (negative PID), then fall back to individual process
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		pm.Logger.Debugf("Failed to send SIGTERM to process group (pid: %d), trying individual process: %v", pid, err)
		if err := process.Signal(syscall.SIGTERM); err != nil {
			pm.Logger.Errorf("Error sending SIGTERM (pid: %d), trying SIGKILL: %v", pid, err)
			return pm.forceKillProcess(identifier, process, pid)
		}
	} else {
		pm.Logger.Debugf("Sent SIGTERM to process group (pid: %d)", pid)
	}

	// Wait for graceful shutdown with timeout
	processState, err := pm.waitForProcessExit(ctx, process, pid)
	if err != nil {
		pm.Logger.Errorf("Process did not exit gracefully (pid: %d), forcing termination: %v", pid, err)
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

	// Don't clean up the LogLineWriter here - only when service is removed

	pm.Logger.Infof("Process terminated gracefully (pid: %d)", pid)
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
	waitCtx, cancel, err := GenerateContext(ctx, constants.CleanupTimeReserve)
	if err != nil {
		pm.Logger.Errorf("Error generating wait context: %v", err)
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
func (pm *ProcessManager) forceKillProcess(identifier constants.ServiceIdentifier, process *os.Process, pid int) error {
	// Try to kill the process group first (negative PID), then fall back to individual process
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		pm.Logger.Debugf("Failed to send SIGKILL to process group (pid: %d), trying individual process: %v", pid, err)
		if err := process.Signal(syscall.SIGKILL); err != nil {
			pm.Logger.Errorf("Error sending SIGKILL to process (pid: %d): %v", pid, err)
			return err
		}
	} else {
		pm.Logger.Debugf("Sent SIGKILL to process group (pid: %d)", pid)
	}

	// Record the exit event for force kill if we have a service identifier
	if identifier != "" {
		// For force kill, exit code is -1 and signal is SIGKILL
		pm.RecordExitEvent(identifier, -1, int(syscall.SIGKILL))
	}

	// Don't clean up the LogLineWriter here - only when service is removed

	pm.Logger.Infof("Process force-killed (pid: %d)", pid)
	return nil
}

// cleanupServiceDirectories removes both the logs and services directories for a service.
// This function handles the new directory structure where logs and services are in separate
// top-level directories under ServiceDirectory. It ensures complete cleanup of all service
// artifacts including logs, configuration files, and PID files.
func (pm *ProcessManager) cleanupServiceDirectories(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	// Clean up logs directory
	logDir := filepath.Join(pm.ServiceDirectory, "logs", servicePath)
	if err := fsService.RemoveAll(ctx, logDir); err != nil {
		pm.Logger.Errorf("Error removing logs directory %s: %v", logDir, err)
		// Continue with services directory cleanup even if logs cleanup fails
	}

	// Clean up services directory (contains config files and PID file)
	serviceDir := filepath.Join(pm.ServiceDirectory, "services", servicePath)
	if err := fsService.RemoveAll(ctx, serviceDir); err != nil {
		return fmt.Errorf("error removing services directory: %w", err)
	}

	pm.Logger.Debugf("Cleaned up service directories for %s (logs: %s, services: %s)", servicePath, logDir, serviceDir)

	return nil
}

// RecordExitEvent records an exit event in the service history
func (pm *ProcessManager) RecordExitEvent(identifier constants.ServiceIdentifier, exitCode int, signal int) {
	// Check if service exists in our registry
	serviceValue, exists := pm.Services.Load(identifier)
	if !exists {
		pm.Logger.Debugf("Service not found when recording exit event: %s", identifier)
		return
	}
	service := serviceValue.(IpmService)

	// Create exit event
	exitEvent := process_shared.ExitEvent{
		Timestamp: time.Now().UTC(),
		ExitCode:  exitCode,
		Signal:    signal,
	}

	// Add to exit history
	service.History.ExitHistory = append(service.History.ExitHistory, exitEvent)

	// Keep only the last N exit events to prevent unbounded memory growth
	if len(service.History.ExitHistory) > constants.MaxExitEvents {
		service.History.ExitHistory = service.History.ExitHistory[len(service.History.ExitHistory)-constants.MaxExitEvents:]
	}

	// Update service in the registry
	pm.Services.Store(identifier, service)

	pm.Logger.Debugf("Recorded exit event for %s: exit code %d, signal %d, timestamp %s", identifier, exitCode, signal, exitEvent.Timestamp.Format(time.RFC3339))
}
