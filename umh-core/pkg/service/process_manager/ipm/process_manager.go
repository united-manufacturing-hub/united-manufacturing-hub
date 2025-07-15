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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

// OperationType represents the type of operation to be performed on a service
type OperationType int

const (
	OperationCreate OperationType = iota
	OperationRemove
	OperationRestart
	OperationStart
	OperationStop
)

// String returns the string representation of the operation type
func (o OperationType) String() string {
	switch o {
	case OperationCreate:
		return "create"
	case OperationRemove:
		return "remove"
	case OperationRestart:
		return "restart"
	case OperationStart:
		return "start"
	case OperationStop:
		return "stop"
	default:
		return "unknown"
	}
}

// Task represents a pending operation on a service
type Task struct {
	Identifier serviceIdentifier
	Operation  OperationType
}

type ProcessManager struct {
	Logger *zap.SugaredLogger
	mu     sync.Mutex

	services map[serviceIdentifier]service

	// taskQueue is a list of pending operations to be processed
	taskQueue []Task

	// serviceDirectory is the root directory where service files are stored
	serviceDirectory string

	// logManager manages log files with rotation
	logManager *LogManager
}

type service struct {
	config  process_manager_serviceconfig.ProcessManagerServiceConfig
	history process_shared.ServiceInfo
}

const DefaultServiceDirectory = "/data/ipm"

// ProcessManagerOption is a functional option for configuring ProcessManager
type ProcessManagerOption func(*ProcessManager)

// WithServiceDirectory sets the service directory for the ProcessManager
func WithServiceDirectory(dir string) ProcessManagerOption {
	return func(pm *ProcessManager) {
		pm.serviceDirectory = dir
	}
}

// NewProcessManager creates a new ProcessManager with the given options
func NewProcessManager(logger *zap.SugaredLogger, options ...ProcessManagerOption) *ProcessManager {
	if logger == nil {
		panic("logger cannot be nil - ProcessManager requires a valid logger")
	}

	pm := &ProcessManager{
		Logger:           logger,
		services:         make(map[serviceIdentifier]service),
		taskQueue:        make([]Task, 0),
		serviceDirectory: DefaultServiceDirectory, // Default value
		logManager:       NewLogManager(logger),
	}

	// Apply options
	for _, option := range options {
		option(pm)
	}

	return pm
}

func (pm *ProcessManager) Create(ctx context.Context, servicePath string, config process_manager_serviceconfig.ProcessManagerServiceConfig, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Creating process manager service", zap.String("servicePath", servicePath), zap.Any("config", config))

	identifier := servicePathToIdentifier(servicePath)
	// Add to services map (return err if already exists)
	if _, ok := pm.services[identifier]; ok {
		return fmt.Errorf("service %s already exists", servicePath)
	}

	pm.services[identifier] = service{
		config: config,
		history: process_shared.ServiceInfo{
			Status:      process_shared.ServiceUnknown,
			ExitHistory: make([]process_shared.ExitEvent, 0),
		},
	}

	// Add to task queue
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationCreate,
	})

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Removing process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)
	// Remove from services map (return err if not exists)
	if _, ok := pm.services[identifier]; !ok {
		return fmt.Errorf("service %s does not exist", servicePath)
	}

	// Remove from services map (this is safe as for removal we only need the serviceIdentifier)
	delete(pm.services, identifier)

	// Add to task queue
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationRemove,
	})

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Start(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Starting process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)
	// Validate that service exists before queuing
	if _, ok := pm.services[identifier]; !ok {
		return fmt.Errorf("service %s not found", servicePath)
	}

	// Add to task queue
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationStart,
	})

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Stopping process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)
	// Validate that service exists before queuing
	if _, ok := pm.services[identifier]; !ok {
		return fmt.Errorf("service %s not found", servicePath)
	}

	// Add to task queue
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationStop,
	})

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Restarting process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)
	// Validate that service exists before queuing
	if _, ok := pm.services[identifier]; !ok {
		return fmt.Errorf("service %s not found", servicePath)
	}

	// Add to task queue
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationRestart,
	})

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Status(ctx context.Context, servicePath string, fsService filesystem.Service) (process_shared.ServiceInfo, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Debug("Getting status of process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)

	// Check if service exists in our registry
	service, exists := pm.services[identifier]
	if !exists {
		return process_shared.ServiceInfo{}, process_shared.ErrServiceNotExist
	}

	// Start with the current tracked status
	info := service.history

	// Update runtime status by checking actual process state
	serviceDir := filepath.Join(pm.serviceDirectory, string(identifier))
	pidFile := filepath.Join(serviceDir, pidFileName)

	// Check if PID file exists
	pidBytes, err := fsService.ReadFile(ctx, pidFile)
	if err != nil {
		// No PID file means service is down
		info.Status = process_shared.ServiceDown
		info.Pid = 0
		info.Pgid = 0
		info.Uptime = 0
		// Keep existing DownTime and exit information
		return info, nil
	}

	// Parse PID from file
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		pm.Logger.Error("Invalid PID file content", zap.String("pidFile", pidFile), zap.Error(err))
		info.Status = process_shared.ServiceDown
		info.Pid = 0
		info.Pgid = 0
		info.Uptime = 0
		return info, nil
	}

	// Check if process is actually running
	process, err := os.FindProcess(pid)
	if err != nil {
		// Process not found - service is down
		info.Status = process_shared.ServiceDown
		info.Pid = 0
		info.Pgid = 0
		info.Uptime = 0
		return info, nil
	}

	// Try to signal the process to verify it's alive
	// Signal 0 doesn't actually send a signal, just checks if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process is not responsive - service is down
		info.Status = process_shared.ServiceDown
		info.Pid = 0
		info.Pgid = 0
		info.Uptime = 0
		return info, nil
	}

	// Process is running - service is up
	info.Status = process_shared.ServiceUp
	info.Pid = pid

	// Get process group ID
	pgid, err := syscall.Getpgid(pid)
	if err == nil {
		info.Pgid = pgid
	}

	// Get process start time for uptime calculation
	if stat, err := fsService.Stat(ctx, pidFile); err == nil {
		pidFileModTime := stat.ModTime()
		uptime := time.Since(pidFileModTime)
		info.Uptime = int64(uptime.Seconds())
		info.LastChangedAt = pidFileModTime
	}

	// For IPM, we consider the service ready immediately when it's up
	info.IsReady = true
	info.ReadyTime = info.Uptime

	// Check if service wants to be up (no explicit down state in IPM)
	info.WantUp = true
	info.IsWantingUp = true

	// Update the tracked status in our registry
	service.history = info
	pm.services[identifier] = service

	return info, nil
}

func (pm *ProcessManager) ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]process_shared.ExitEvent, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Debug("Getting exit history of process manager service", zap.String("servicePath", superviseDir))

	// For IPM, superviseDir is actually the servicePath since IPM doesn't use separate supervise directories
	identifier := servicePathToIdentifier(superviseDir)

	// Check if service exists in our registry
	service, exists := pm.services[identifier]
	if !exists {
		return nil, process_shared.ErrServiceNotExist
	}

	// Return the exit history from the service info
	return service.history.ExitHistory, nil
}

func (pm *ProcessManager) ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Debug("Checking if process manager service exists", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)
	_, exists := pm.services[identifier]
	return exists, nil
}

func (pm *ProcessManager) GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (process_manager_serviceconfig.ProcessManagerServiceConfig, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Debug("Getting config of process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)

	// Check if service exists in our registry
	service, exists := pm.services[identifier]
	if !exists {
		return process_manager_serviceconfig.ProcessManagerServiceConfig{}, process_shared.ErrServiceNotExist
	}

	// For IPM, we return the stored configuration directly
	// This is simpler than S6 which needs to parse scripts
	config := service.config

	// Optionally validate that config files on disk match what we have stored
	// This ensures consistency between in-memory state and filesystem
	serviceDir := filepath.Join(pm.serviceDirectory, string(identifier))
	configDir := filepath.Join(serviceDir, configDirectoryName)

	// Check if config directory exists
	if exists, err := fsService.PathExists(ctx, configDir); err == nil && exists {
		// Verify stored config files match what's on disk
		for fileName := range config.ConfigFiles {
			configFile := filepath.Join(configDir, fileName)
			if fileExists, err := fsService.FileExists(ctx, configFile); err == nil && !fileExists {
				pm.Logger.Warn("Config file missing on disk",
					zap.String("servicePath", servicePath),
					zap.String("fileName", fileName))
				// Continue anyway - return what we have in memory
			}
		}
	}

	pm.Logger.Debug("Retrieved config for service",
		zap.String("servicePath", servicePath),
		zap.Int("configFiles", len(config.ConfigFiles)),
		zap.Int64("memoryLimit", config.MemoryLimit),
		zap.Int64("logFilesize", config.LogFilesize))

	return config, nil
}

func (pm *ProcessManager) GetConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Debug("Getting config file of process manager service",
		zap.String("servicePath", servicePath),
		zap.String("configFileName", configFileName))

	identifier := servicePathToIdentifier(servicePath)

	// Check if service exists in our registry
	service, exists := pm.services[identifier]
	if !exists {
		return nil, process_shared.ErrServiceNotExist
	}

	// Check if the config file exists in our stored configuration
	fileContent, exists := service.config.ConfigFiles[configFileName]
	if !exists {
		return nil, fmt.Errorf("config file %s does not exist in service %s", configFileName, servicePath)
	}

	// Optionally verify the file exists on disk and matches
	serviceDir := filepath.Join(pm.serviceDirectory, string(identifier))
	configFile := filepath.Join(serviceDir, configDirectoryName, configFileName)

	if fileExists, err := fsService.FileExists(ctx, configFile); err == nil && fileExists {
		// Read from disk to verify consistency
		diskContent, err := fsService.ReadFile(ctx, configFile)
		if err != nil {
			pm.Logger.Warn("Failed to read config file from disk for verification",
				zap.String("servicePath", servicePath),
				zap.String("configFileName", configFileName),
				zap.Error(err))
			// Return stored content anyway
		} else if string(diskContent) != fileContent {
			pm.Logger.Warn("Config file content mismatch between memory and disk",
				zap.String("servicePath", servicePath),
				zap.String("configFileName", configFileName))
			// Return stored content anyway - memory is authoritative for IPM
		}
	} else {
		pm.Logger.Warn("Config file missing on disk",
			zap.String("servicePath", servicePath),
			zap.String("configFileName", configFileName))
		// Return stored content anyway
	}

	pm.Logger.Debug("Retrieved config file",
		zap.String("servicePath", servicePath),
		zap.String("configFileName", configFileName),
		zap.Int("contentSize", len(fileContent)))

	return []byte(fileContent), nil
}

func (pm *ProcessManager) GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]process_shared.LogEntry, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Debug("Getting logs of process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)

	// Check if service exists in our registry
	_, exists := pm.services[identifier]
	if !exists {
		return nil, process_shared.ErrServiceNotExist
	}

	// Construct the path to the current log file
	serviceDir := filepath.Join(pm.serviceDirectory, string(identifier))
	logDir := filepath.Join(serviceDir, logDirectoryName)
	currentLogFile := filepath.Join(logDir, "current")

	// Check if the log file exists
	exists, err := fsService.FileExists(ctx, currentLogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check if log file exists: %w", err)
	}
	if !exists {
		pm.Logger.Debug("Log file does not exist, returning empty logs", zap.String("logFile", currentLogFile))
		return []process_shared.LogEntry{}, nil
	}

	// Read the log file content
	logContent, err := fsService.ReadFile(ctx, currentLogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file %s: %w", currentLogFile, err)
	}

	// Parse the log content using the same parser as S6
	entries, err := process_shared.ParseLogsFromBytes(logContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log content: %w", err)
	}

	pm.Logger.Debug("Retrieved logs for service",
		zap.String("servicePath", servicePath),
		zap.String("logFile", currentLogFile),
		zap.Int("entryCount", len(entries)))

	return entries, nil
}
func (pm *ProcessManager) CleanServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Cleaning process manager service directory", zap.String("servicePath", path))
	// We don't use this here, so we return nil
	return nil
}

func (pm *ProcessManager) ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Force removing process manager service", zap.String("servicePath", servicePath))

	// We don't use this here, so we return nil
	return nil
}

func (pm *ProcessManager) EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Ensuring supervision of process manager service", zap.String("servicePath", servicePath))

	// We don't use this here, so we return true and nil (supervision is always ensured)
	return true, nil
}

// Reconcile processes all queued tasks in the task queue.
// This is the main entry point for processing service operations and should be called periodically.
// It acquires the ProcessManager mutex and processes tasks until the queue is empty or context times out.
func (pm *ProcessManager) Reconcile(ctx context.Context, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.Logger.Debug("Starting reconciliation", zap.Int("queueLength", len(pm.taskQueue)))

	// Process all queued tasks
	return pm.step(ctx, fsService)
}

// Close closes all log files and performs cleanup
func (pm *ProcessManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.Logger.Info("Closing ProcessManager and all log files")

	// Close all log files
	if pm.logManager != nil {
		if err := pm.logManager.CloseAll(); err != nil {
			pm.Logger.Error("Error closing log files", zap.Error(err))
			return err
		}
	} else {
		pm.Logger.Warn("LogManager is nil - skipping log file cleanup")
	}

	return nil
}
