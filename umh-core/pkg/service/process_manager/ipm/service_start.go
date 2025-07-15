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
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
)

// startService orchestrates the starting of a service process in the process manager.
// This function handles the complex task of checking for existing processes, terminating them if necessary,
// and starting a new process atomically. The function first checks for an existing PID file to detect
// running processes. If a process is already running, it terminates the existing process, removes the
// PID file, and returns an error to allow retry. If no process is running, it starts a new subprocess
// and saves its PID atomically to prevent race conditions during process startup.
func (pm *ProcessManager) startService(ctx context.Context, identifier serviceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Info("Starting service", zap.String("identifier", string(identifier)))

	servicePath := filepath.Join(pm.serviceDirectory, string(identifier))
	pidFile := filepath.Join(servicePath, pidFileName)

	// Check if there's already a PID file indicating a running process
	if _, err := fsService.Stat(ctx, pidFile); err == nil {
		pm.Logger.Info("Found existing PID file, terminating old process", zap.String("identifier", string(identifier)))

		// Terminate the existing process
		if err := pm.terminateServiceProcess(ctx, identifier, servicePath, fsService); err != nil {
			pm.Logger.Error("Error terminating existing process", zap.Error(err))
			return fmt.Errorf("error terminating existing process: %w", err)
		}

		// Remove the PID file
		if err := fsService.Remove(ctx, pidFile); err != nil {
			pm.Logger.Error("Error removing PID file", zap.Error(err))
			return fmt.Errorf("error removing PID file: %w", err)
		}

		pm.Logger.Info("terminated existing process, continuing with start")
		// Continue with start process after cleanup
	}

	// Get service configuration to determine what to execute
	service, err := pm.getServiceConfig(identifier)
	if err != nil {
		return err
	}

	// Start the new process atomically
	if err := pm.startProcessAtomically(ctx, identifier, service.config, fsService); err != nil {
		return err
	}

	pm.Logger.Info("Service started successfully", zap.String("identifier", string(identifier)))
	return nil
}

// startProcessAtomically starts a new service process and saves its PID in an atomic operation.
// This function creates a subprocess based on the service configuration and immediately writes
// its PID to the filesystem. If the PID writing fails, the process is terminated to maintain
// consistency. This atomic approach ensures that either both the process starts and its PID
// is recorded, or neither happens, preventing orphaned processes or missing PID files that
// could cause management issues later.
func (pm *ProcessManager) startProcessAtomically(ctx context.Context, identifier serviceIdentifier, config process_manager_serviceconfig.ProcessManagerServiceConfig, fsService filesystem.Service) error {
	servicePath := filepath.Join(pm.serviceDirectory, string(identifier))
	configPath := filepath.Join(servicePath, configDirectoryName)
	logPath := filepath.Join(servicePath, logDirectoryName)
	pidFile := filepath.Join(servicePath, pidFileName)

	// Ensure log directory exists
	if err := fsService.EnsureDirectory(ctx, logPath); err != nil {
		return fmt.Errorf("error ensuring log directory: %w", err)
	}

	// Register service with log manager for rotation monitoring
	pm.logManager.RegisterService(identifier, logPath, config.LogFilesize)

	// Determine the command to execute - for now, assume there's a "run.sh" script
	// TODO: This should be configurable in the service config
	commandPath := filepath.Join(configPath, "run.sh")
	currentLogFile := filepath.Join(logPath, "current")

	// Create the command with shell redirection
	// Format: /bin/bash /path/to/run.sh >> /path/to/logs/current 2>&1
	shellCommand := fmt.Sprintf("/bin/bash %s >> %s 2>&1", commandPath, currentLogFile)

	// Create the command using shell to handle the redirection
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", shellCommand)

	// Set memory limits if configured
	if config.MemoryLimit > 0 {
		memoryLimitMB := config.MemoryLimit / (1024 * 1024) // Convert bytes to MB
		if memoryLimitMB == 0 {
			memoryLimitMB = 1 // Minimum 1MB
		}

		pm.Logger.Info("Applying memory limit to process",
			zap.String("identifier", string(identifier)),
			zap.Int64("memoryLimitBytes", config.MemoryLimit),
			zap.Int64("memoryLimitMB", memoryLimitMB))

		// Modify the shell command to include ulimit
		shellCommand = fmt.Sprintf("ulimit -v %d && %s", memoryLimitMB*1024, shellCommand) // ulimit -v is in KB
		cmd = exec.CommandContext(ctx, "/bin/bash", "-c", shellCommand)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true, // Create new process group for better process management
		}
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting process: %w", err)
	}

	// Get the process PID
	pid := cmd.Process.Pid

	// Atomically write the PID to the file
	pidData := []byte(strconv.Itoa(pid))
	if err := fsService.WriteFile(ctx, pidFile, pidData, configFilePermission); err != nil {
		// If writing PID fails, terminate the process to maintain consistency
		pm.Logger.Error("Failed to write PID file, terminating process", zap.Int("pid", pid), zap.Error(err))
		if killErr := cmd.Process.Kill(); killErr != nil {
			pm.Logger.Error("Failed to kill process after PID write failure", zap.Int("pid", pid), zap.Error(killErr))
		}
		return fmt.Errorf("error writing PID file: %w", err)
	}

	pm.Logger.Info("Process started and PID saved", zap.String("identifier", string(identifier)), zap.Int("pid", pid))
	return nil
}
