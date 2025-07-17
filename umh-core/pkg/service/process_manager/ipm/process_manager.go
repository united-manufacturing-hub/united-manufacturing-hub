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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/logging"
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
	Identifier constants.ServiceIdentifier
	Operation  OperationType
}

type ProcessManager struct {
	Logger *zap.SugaredLogger
	mu     sync.Mutex

	Services sync.Map // map[constants.ServiceIdentifier]IpmService

	// TaskQueue is a list of pending operations to be processed
	TaskQueue []Task

	// ServiceDirectory is the root directory where service files are stored
	ServiceDirectory string

	// logManager manages log files with rotation
	logManager *logging.LogManager

	// StartupCompleted is a flag that indicates if the startup process has completed
	StartupCompleted atomic.Bool
}

type IpmService struct {
	Config  process_manager_serviceconfig.ProcessManagerServiceConfig
	History process_shared.ServiceInfo
	// LogLineWriter handles logging for this service (nil if never started)
	LogLineWriter *logging.LogLineWriter
}

const DefaultServiceDirectory = "/data"

// ProcessManagerOption is a functional option for configuring ProcessManager
type ProcessManagerOption func(*ProcessManager)

// WithServiceDirectory sets the service directory for the ProcessManager
func WithServiceDirectory(dir string) ProcessManagerOption {
	return func(pm *ProcessManager) {
		pm.ServiceDirectory = dir
	}
}

var initOnce sync.Once
var instance *ProcessManager

func NewProcessManagerInstance(options ...ProcessManagerOption) *ProcessManager {
	initOnce.Do(func() {
		serviceLogger := logger.For(logger.ComponentS6Service)
		instance = NewProcessManager(serviceLogger, options...)
	})
	return instance
}

// NewProcessManager creates a new ProcessManager with the given options
func NewProcessManager(logger *zap.SugaredLogger, options ...ProcessManagerOption) *ProcessManager {
	if logger == nil {
		panic("logger cannot be nil - ProcessManager requires a valid logger")
	}

	pm := &ProcessManager{
		Logger:           logger,
		Services:         sync.Map{},
		TaskQueue:        make([]Task, 0),
		ServiceDirectory: DefaultServiceDirectory, // Default value
		logManager:       logging.NewLogManager(logger),
	}

	// Apply options
	for _, option := range options {
		option(pm)
	}

	return pm
}

func (pm *ProcessManager) Create(ctx context.Context, servicePath string, config process_manager_serviceconfig.ProcessManagerServiceConfig, fsService filesystem.Service) error {
	pm.Logger.Infof("Creating process manager service: %s, config: %+v", servicePath, config)

	identifier := constants.ServicePathToIdentifier(servicePath)
	// Add to services map (return err if already exists)
	if _, ok := pm.Services.Load(identifier); ok {
		return fmt.Errorf("service %s already exists", servicePath)
	}

	pm.Services.Store(identifier, IpmService{
		Config: config,
		History: process_shared.ServiceInfo{
			Status:      process_shared.ServiceUnknown,
			ExitHistory: make([]process_shared.ExitEvent, 0),
		},
	})

	// Add to task queue - only lock around this operation
	pm.mu.Lock()
	pm.TaskQueue = append(pm.TaskQueue, Task{
		Identifier: identifier,
		Operation:  OperationCreate,
	})
	pm.mu.Unlock()

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.Logger.Infof("Removing process manager service: %s", servicePath)

	identifier := constants.ServicePathToIdentifier(servicePath)
	// Remove from services map (return err if not exists)
	if _, ok := pm.Services.Load(identifier); !ok {
		return fmt.Errorf("service %s does not exist", servicePath)
	}

	// Remove from services map (this is safe as for removal we only need the serviceIdentifier)
	pm.Services.Delete(identifier)

	// Add to task queue - only lock around this operation
	pm.mu.Lock()
	pm.TaskQueue = append(pm.TaskQueue, Task{
		Identifier: identifier,
		Operation:  OperationRemove,
	})
	pm.mu.Unlock()

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Start(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.Logger.Infof("Starting process manager service: %s", servicePath)

	identifier := constants.ServicePathToIdentifier(servicePath)
	// Validate that service exists before queuing
	if _, ok := pm.Services.Load(identifier); !ok {
		return fmt.Errorf("service %s not found", servicePath)
	}

	// Add to task queue - only lock around this operation
	pm.mu.Lock()
	pm.TaskQueue = append(pm.TaskQueue, Task{
		Identifier: identifier,
		Operation:  OperationStart,
	})
	pm.mu.Unlock()

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.Logger.Infof("Stopping process manager service: %s", servicePath)

	identifier := constants.ServicePathToIdentifier(servicePath)
	// Validate that service exists before queuing
	if _, ok := pm.Services.Load(identifier); !ok {
		return fmt.Errorf("service %s not found", servicePath)
	}

	// Add to task queue - only lock around this operation
	pm.mu.Lock()
	pm.TaskQueue = append(pm.TaskQueue, Task{
		Identifier: identifier,
		Operation:  OperationStop,
	})
	pm.mu.Unlock()

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.Logger.Infof("Restarting process manager service: %s", servicePath)

	identifier := constants.ServicePathToIdentifier(servicePath)
	// Validate that service exists before queuing
	if _, ok := pm.Services.Load(identifier); !ok {
		return fmt.Errorf("service %s not found", servicePath)
	}

	// Add to task queue - only lock around this operation
	pm.mu.Lock()
	pm.TaskQueue = append(pm.TaskQueue, Task{
		Identifier: identifier,
		Operation:  OperationRestart,
	})
	pm.mu.Unlock()

	// Tasks are queued and will be processed by Reconcile
	return nil
}

func (pm *ProcessManager) Status(ctx context.Context, servicePath string, fsService filesystem.Service) (process_shared.ServiceInfo, error) {
	pm.Logger.Debugf("Getting status of process manager service: %s", servicePath)

	identifier := constants.ServicePathToIdentifier(servicePath)

	// Check if service exists in our registry
	serviceValue, exists := pm.Services.Load(identifier)
	if !exists {
		pm.Logger.Errorf("[StatusGenerator] Service %s does not exist", servicePath)
		return process_shared.ServiceInfo{}, process_shared.ErrServiceNotExist
	}
	service := serviceValue.(IpmService)

	// Start with the current tracked status
	info := service.History

	// Remove the /run/services/ prefix from the servicePath
	servicePath = strings.ReplaceAll(servicePath, "/run/service/", "")

	// Update runtime status by checking actual process state
	pidFile := filepath.Join(pm.ServiceDirectory, "services", servicePath, constants.PidFileName)

	// Check if PID file exists
	pidBytes, err := fsService.ReadFile(ctx, pidFile)
	if err != nil {
		pm.Logger.Errorf("[StatusGenerator] No PID file found for service %s: %v", servicePath, err)
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
		pm.Logger.Errorf("[StatusGenerator] Invalid PID file content in %s: %v", pidFile, err)
		info.Status = process_shared.ServiceDown
		info.Pid = 0
		info.Pgid = 0
		info.Uptime = 0
		return info, nil
	}

	// Check if process is actually running
	process, err := os.FindProcess(pid)
	if err != nil {
		pm.Logger.Errorf("[StatusGenerator] Process not found for service %s: %v", servicePath, err)
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
		pm.Logger.Errorf("[StatusGenerator] Process not responsive for service %s: %v", servicePath, err)
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
		pm.Logger.Debugf("[StatusGenerator] Process group ID for service %s: %d", servicePath, pgid)
		info.Pgid = pgid
	}

	// Get process start time for uptime calculation
	if stat, err := fsService.Stat(ctx, pidFile); err == nil {
		pm.Logger.Debugf("[StatusGenerator] Process start time for service %s: %v", servicePath, stat.ModTime())
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
	pm.Services.Store(identifier, IpmService{
		Config:        service.Config,
		History:       info,
		LogLineWriter: service.LogLineWriter,
	})

	return info, nil
}

func (pm *ProcessManager) ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]process_shared.ExitEvent, error) {
	pm.Logger.Debugf("Getting exit history of process manager service: %s", superviseDir)

	// For IPM, superviseDir is actually the servicePath since IPM doesn't use separate supervise directories
	identifier := constants.ServicePathToIdentifier(superviseDir)

	// Check if service exists in our registry
	serviceValue, exists := pm.Services.Load(identifier)
	if !exists {
		return nil, process_shared.ErrServiceNotExist
	}
	service := serviceValue.(IpmService)

	// Return the exit history from the service info
	return service.History.ExitHistory, nil
}

func (pm *ProcessManager) ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	pm.Logger.Debugf("Checking if process manager service exists: %s", servicePath)

	identifier := constants.ServicePathToIdentifier(servicePath)
	_, exists := pm.Services.Load(identifier)
	return exists, nil
}

func (pm *ProcessManager) GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (process_manager_serviceconfig.ProcessManagerServiceConfig, error) {
	pm.Logger.Debugf("Getting config of process manager service: %s", servicePath)

	identifier := constants.ServicePathToIdentifier(servicePath)

	// Check if service exists in our registry
	serviceValue, exists := pm.Services.Load(identifier)
	if !exists {
		return process_manager_serviceconfig.ProcessManagerServiceConfig{}, process_shared.ErrServiceNotExist
	}
	service := serviceValue.(IpmService)

	// For IPM, we return the stored configuration directly
	// This is simpler than S6 which needs to parse scripts
	config := service.Config

	pm.Logger.Debugf("Retrieved config for service %s: %d config files, memory limit: %d, log filesize: %d", servicePath, len(config.ConfigFiles), config.MemoryLimit, config.LogFilesize)

	return config, nil
}

func (pm *ProcessManager) GetConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error) {
	pm.Logger.Debugf("Getting config file of process manager service: %s, file: %s", servicePath, configFileName)

	identifier := constants.ServicePathToIdentifier(servicePath)

	// Check if service exists in our registry
	serviceValue, exists := pm.Services.Load(identifier)
	if !exists {
		return nil, process_shared.ErrServiceNotExist
	}
	service := serviceValue.(IpmService)

	// Check if the config file exists in our stored configuration
	fileContent, exists := service.Config.ConfigFiles[configFileName]
	if !exists {
		return nil, fmt.Errorf("config file %s does not exist in service %s", configFileName, servicePath)
	}

	pm.Logger.Debugf("Retrieved config file %s for service %s: %d bytes", configFileName, servicePath, len(fileContent))

	return []byte(fileContent), nil
}

func (pm *ProcessManager) GetLogs(_ context.Context, servicePath string, _ filesystem.Service) ([]process_shared.LogEntry, error) {
	pm.Logger.Debugf("Getting logs of process manager service: %s", servicePath)

	identifier := constants.ServicePathToIdentifier(servicePath)

	// Check if service exists in our registry and get its LogLineWriter
	serviceValue, exists := pm.Services.Load(identifier)
	if !exists {
		return nil, process_shared.ErrServiceNotExist
	}
	service := serviceValue.(IpmService)

	logWriter := service.LogLineWriter
	if logWriter == nil {
		// Service has no active LogLineWriter (never started), return empty logs
		pm.Logger.Debugf("No active LogLineWriter found, returning empty logs for service: %s", servicePath)
		return []process_shared.LogEntry{}, nil
	}

	// Get logs from the memory buffer
	entries := logWriter.MemoryBuffer.GetEntries()
	pm.Logger.Debugf("Retrieved logs from memory buffer for service %s: %d entries", servicePath, len(entries))
	return entries, nil
}
func (pm *ProcessManager) CleanServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error {
	pm.Logger.Infof("Cleaning process manager service directory: %s", path)
	// We don't use this here, so we return nil
	return nil
}

func (pm *ProcessManager) ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.Logger.Infof("Force removing process manager service: %s", servicePath)

	// We don't use this here, so we return nil
	return nil
}

func (pm *ProcessManager) EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	pm.Logger.Infof("Ensuring supervision of process manager service: %s", servicePath)

	// We don't use this here, so we return true and nil (supervision is always ensured)
	return true, nil
}

// Reconcile processes all queued tasks in the task queue.
// This is the main entry point for processing service operations and should be called periodically.
// It acquires the ProcessManager mutex and processes tasks until the queue is empty or context times out.
func (pm *ProcessManager) Reconcile(ctx context.Context, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.Logger.Debugf("Starting reconciliation, queue length: %d", len(pm.TaskQueue))

	// Startup the process manager
	if err := pm.startup(ctx, fsService); err != nil {
		pm.Logger.Errorf("Failed to startup process manager: %v", err)
		return err
	}

	// Process all queued tasks
	return pm.step(ctx, fsService)
}

func (pm *ProcessManager) startup(ctx context.Context, fsService filesystem.Service) error {
	shouldStartup := pm.StartupCompleted.CompareAndSwap(false, true)
	if !shouldStartup {
		return nil
	}

	// Wipe the service directory
	if err := pm.wipeServiceDirectory(ctx, fsService); err != nil {
		pm.Logger.Errorf("Failed to wipe service directory: %v", err)
		pm.StartupCompleted.Store(false)
		return nil
	}
	// Ensure services directory README.md exists on first run
	if err := pm.ensureServicesReadme(ctx, fsService); err != nil {
		pm.Logger.Errorf("Failed to create services README.md: %v", err)
		pm.StartupCompleted.Store(false)
		// Continue with reconciliation even if README creation fails
		return nil
	}
	pm.Logger.Info("Process manager startup completed")
	pm.StartupCompleted.Store(true)
	return nil
}

// wipeServiceDirectory wipes the service directory
// It assumes we already hold the lock and will be called only once on startup
func (pm *ProcessManager) wipeServiceDirectory(ctx context.Context, fsService filesystem.Service) error {
	serviceDir := filepath.Join(pm.ServiceDirectory, constants.ServiceDirectoryName)
	pm.Logger.Infof("Wiping service directory: %s", serviceDir)

	// Check if the service directory exists
	exists, err := fsService.PathExists(ctx, serviceDir)
	if err != nil {
		return fmt.Errorf("failed to check if service directory exists: %w", err)
	}

	// If the service directory exists, remove it
	if exists {
		if err := fsService.RemoveAll(ctx, serviceDir); err != nil {
			return fmt.Errorf("failed to remove service directory: %w", err)
		}
	}

	// Create the services directory
	if err := fsService.EnsureDirectory(ctx, serviceDir); err != nil {
		return fmt.Errorf("failed to ensure services directory exists: %w", err)
	}
	return nil
}

// ensureServicesReadme creates a README.md file in the services directory if it doesn't exist.
// This file serves as documentation for users indicating that the contents are auto-generated.
func (pm *ProcessManager) ensureServicesReadme(ctx context.Context, fsService filesystem.Service) error {

	servicesDir := filepath.Join(pm.ServiceDirectory, "services")
	readmePath := filepath.Join(servicesDir, "README.md")

	// Check if README.md already exists
	exists, err := fsService.FileExists(ctx, readmePath)
	if err != nil {
		return fmt.Errorf("failed to check if README.md exists: %w", err)
	}

	if exists {
		// README.md already exists, nothing to do
		return nil
	}

	// Ensure the services directory exists first
	if err := fsService.EnsureDirectory(ctx, servicesDir); err != nil {
		return fmt.Errorf("failed to ensure services directory exists: %w", err)
	}

	// Create the README content
	readmeContent := `# Services Directory

This directory contains auto-generated service configurations and files managed by the UMH Process Manager.

**IMPORTANT**: All files and folders in this directory are automatically generated and maintained by the system. 

- Any manual changes made to files in this directory will be ignored or overwritten.
- Service configurations should be managed through the appropriate APIs or configuration tools.
- Do not manually modify, add, or remove files in this directory.

For more information about managing services, please refer to the UMH documentation.
`

	// Write the README.md file
	if err := fsService.WriteFile(ctx, readmePath, []byte(readmeContent), constants.ConfigFilePermission); err != nil {

		return fmt.Errorf("failed to write README.md: %w", err)
	}

	pm.Logger.Infof("Created services directory README.md: %s", readmePath)

	return nil
}

// Close closes all log files and performs cleanup
func (pm *ProcessManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.Logger.Info("Closing ProcessManager and all log files")

	// Close all LogLineWriter instances
	pm.Services.Range(func(key, value interface{}) bool {
		service := value.(IpmService)
		if service.LogLineWriter != nil {
			if err := service.LogLineWriter.Close(); err != nil {
				pm.Logger.Errorf("Error closing LogLineWriter for %s: %v", key, err)
			}
		}
		return true
	})

	// Close all log files
	if pm.logManager != nil {
		if err := pm.logManager.CloseAll(); err != nil {
			pm.Logger.Errorf("Error closing log files: %v", err)
			return err
		}
	} else {
		pm.Logger.Warn("LogManager is nil - skipping log file cleanup")
	}

	return nil
}

// CreateUMHCoreLoggingService creates a dummy service for umh-core logging and processes logs from the provided reader
func CreateUMHCoreLoggingService(reader io.Reader, readyChan chan struct{}) error {
	// Create a ProcessManager instance to manage the umh-core service
	pm := NewProcessManagerInstance()
	fsService := filesystem.NewDefaultService()

	// Create service configuration for umh-core dummy service
	serviceConfig := process_manager_serviceconfig.ProcessManagerServiceConfig{
		Command:     []string{"sleep", "infinity"},
		Env:         map[string]string{},
		ConfigFiles: map[string]string{},
		LogFilesize: constants.DefaultLogFileSize,
		MemoryLimit: 0, // No memory limit for dummy service
	}

	// Create the service in the process manager
	servicePath := "/run/service/umh-core"
	if err := pm.Create(context.Background(), servicePath, serviceConfig, fsService); err != nil {
		return fmt.Errorf("failed to create umh-core service: %w", err)
	}

	// Get the service from the registry to access its LogLineWriter
	serviceIdentifier := constants.ServicePathToIdentifier(servicePath)
	serviceValue, exists := pm.Services.Load(serviceIdentifier)
	if !exists {
		return fmt.Errorf("umh-core service not found in registry")
	}
	service := serviceValue.(IpmService)

	// If LogLineWriter doesn't exist yet, we need to create it
	if service.LogLineWriter == nil {
		// Create log directory
		logPath := filepath.Join(pm.ServiceDirectory, "logs", "umh-core")
		if err := fsService.EnsureDirectory(context.Background(), logPath); err != nil {
			return fmt.Errorf("failed to create umh-core log directory: %w", err)
		}

		// Create LogLineWriter
		memoryBuffer := logging.NewMemoryLogBuffer(constants.DefaultLogBufferSize)
		logWriter, err := logging.NewLogLineWriter(
			serviceIdentifier,
			logPath,
			pm.logManager,
			memoryBuffer,
			fsService,
		)
		if err != nil {
			return fmt.Errorf("failed to create log writer: %w", err)
		}

		// Update service with LogLineWriter
		service.LogLineWriter = logWriter
		pm.Services.Store(serviceIdentifier, service)
	}

	// Signal that the log handler is ready
	close(readyChan)

	// Process log lines from the pipe using the service's LogLineWriter
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   line,
			}
			if err := service.LogLineWriter.WriteLine(entry); err != nil {
				pm.Logger.Errorf("Failed to write log entry: %v", err)
			}
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading from log pipe: %w", err)
	}

	return nil
}
