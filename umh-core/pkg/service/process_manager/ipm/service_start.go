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
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/logging"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
)

// startService orchestrates the starting of a service process in the process manager.
// This function handles the complex task of checking for existing processes, terminating them if necessary,
// and starting a new process atomically. The function first checks for an existing PID file to detect
// running processes. If a process is already running, it terminates the existing process, removes the
// PID file, and returns an error to allow retry. If no process is running, it starts a new subprocess
// and saves its PID atomically to prevent race conditions during process startup.
func (pm *ProcessManager) startService(ctx context.Context, identifier constants.ServiceIdentifier, fsService filesystem.Service) error {
	pm.Logger.Infof("Starting service: %s", identifier)

	// Check for existing PID file and handle it
	servicePath := string(identifier) // Convert identifier back to servicePath
	pidFile := filepath.Join(pm.ServiceDirectory, "services", servicePath, constants.PidFileName)

	// Check if a PID file already exists
	if _, err := fsService.Stat(ctx, pidFile); err == nil {
		pm.Logger.Infof("Found existing PID file, terminating old process: %s", identifier)

		// Terminate the existing process
		if err := pm.terminateServiceProcess(ctx, identifier, servicePath, fsService); err != nil {
			pm.Logger.Errorf("Error terminating existing process: %v", err)
			return fmt.Errorf("error terminating existing process: %w", err)
		}

		// Remove the PID file after termination
		if err := fsService.Remove(ctx, pidFile); err != nil {
			pm.Logger.Errorf("Error removing PID file: %v", err)
		}
		pm.Logger.Info("terminated existing process, continuing with start")
	}

	// Get service configuration
	service, err := pm.getServiceConfig(identifier)
	if err != nil {
		return err
	}

	// Start the actual process
	if err := pm.startProcessAtomically(ctx, identifier, service.Config, fsService); err != nil {
		return err
	}

	pm.Logger.Infof("Service started successfully: %s", identifier)
	return nil
}

// startProcessAtomically starts a new service process and saves its PID in an atomic operation.
// This function creates a subprocess based on the service configuration and immediately writes
// its PID to the filesystem. If the PID writing fails, the process is terminated to maintain
// consistency. This atomic approach ensures that either both the process starts and its PID
// is recorded, or neither happens, preventing orphaned processes or missing PID files that
// could cause management issues later.
func (pm *ProcessManager) startProcessAtomically(ctx context.Context, identifier constants.ServiceIdentifier, config process_manager_serviceconfig.ProcessManagerServiceConfig, fsService filesystem.Service) error {
	servicePath := string(identifier) // Convert identifier back to servicePath
	configPath := filepath.Join(pm.ServiceDirectory, "services", servicePath)
	logPath := filepath.Join(pm.ServiceDirectory, "logs", servicePath)
	pidFile := filepath.Join(pm.ServiceDirectory, "services", servicePath, constants.PidFileName)

	// Ensure log directory exists
	if err := fsService.EnsureDirectory(ctx, logPath); err != nil {
		return fmt.Errorf("error ensuring log directory: %w", err)
	}

	// Create memory buffer and log line writer for this service
	memoryBuffer := logging.NewMemoryLogBuffer(constants.DefaultLogBufferSize)
	logLineWriter, err := logging.NewLogLineWriter(identifier, logPath, pm.logManager, memoryBuffer, fsService)
	if err != nil {
		return fmt.Errorf("error creating log line writer: %w", err)
	}

	// Determine the command to execute - for now, assume there's a "run.sh" script
	// TODO: This should be configurable in the service config
	commandPath := filepath.Join(configPath, constants.RunScriptFileName)

	var cmd *exec.Cmd

	// Set memory limits if configured
	if config.MemoryLimit > 0 {
		memoryLimitBytes := config.MemoryLimit
		if memoryLimitBytes == 0 {
			memoryLimitBytes = 1024 * 1024 // Minimum 1MB
		}

		memoryLimitMB := memoryLimitBytes / (1024 * 1024)
		if memoryLimitMB == 0 {
			memoryLimitMB = 1
		}

		pm.Logger.Infof("Applying memory limit to process: %d bytes", memoryLimitBytes)

		// Use exec to replace the shell process instead of creating a subprocess
		// This eliminates one bash layer while still applying memory limits
		shellCommand := fmt.Sprintf("ulimit -v %d && exec %s", memoryLimitMB*1024, commandPath)
		cmd = exec.Command("/bin/bash", "-c", shellCommand)
	} else {
		// Direct execution without memory limits
		cmd = exec.Command(commandPath)
	}

	// Set up process group for better process management
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group for better process management
	}

	// Set up pipes for stdout and stderr to capture logs
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logLineWriter.Close()
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logLineWriter.Close()
		return fmt.Errorf("error creating stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		logLineWriter.Close()
		return fmt.Errorf("error starting process: %w", err)
	}

	// Get the process PID
	pid := cmd.Process.Pid

	// Atomically write the PID to the file
	pidData := []byte(strconv.Itoa(pid))
	if err := fsService.WriteFile(ctx, pidFile, pidData, constants.ConfigFilePermission); err != nil {
		// If writing PID fails, terminate the process to maintain consistency
		pm.Logger.Errorf("Failed to write PID file, terminating process (pid: %d): %v", pid, err)
		if killErr := cmd.Process.Kill(); killErr != nil {
			pm.Logger.Errorf("Failed to kill process after PID write failure (pid: %d): %v", pid, killErr)
		}
		logLineWriter.Close()
		return fmt.Errorf("error writing PID file: %w", err)
	}

	// Start goroutines to stream logs from stdout and stderr to the LogLineWriter
	go pm.streamLogs(stdoutPipe, logLineWriter, "stdout", identifier)
	go pm.streamLogs(stderrPipe, logLineWriter, "stderr", identifier)

	// Store the LogLineWriter in the service instead of the separate map
	serviceValue, exists := pm.Services.Load(identifier)
	if !exists {
		logLineWriter.Close()
		return fmt.Errorf("service %s not found in registry", identifier)
	}
	service := serviceValue.(IpmService)
	service.LogLineWriter = logLineWriter
	pm.Services.Store(identifier, service)

	pm.Logger.Infof("Process started and PID saved: %s (pid: %d)", identifier, pid)
	return nil
}

// streamLogs reads from the provided pipe and writes each line to the LogLineWriter.
// This function runs in a goroutine and handles real-time log streaming from the process.
func (pm *ProcessManager) streamLogs(pipe io.ReadCloser, logLineWriter *logging.LogLineWriter, streamType string, identifier constants.ServiceIdentifier) {
	defer pipe.Close()

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()

		// Create log entry with current timestamp
		entry := process_shared.LogEntry{
			Timestamp: time.Now(),
			Content:   line,
		}

		// Write to the LogLineWriter (both memory and file)
		if err := logLineWriter.WriteLine(entry); err != nil {
			pm.Logger.Errorf("Error writing log line: %v", err)
			// Continue processing other lines even if one fails
		}
	}

	if err := scanner.Err(); err != nil {
		pm.Logger.Errorf("Error scanning log stream: %v", err)
	}

	pm.Logger.Debugf("Log stream ended: %s (%s)", identifier, streamType)
}
