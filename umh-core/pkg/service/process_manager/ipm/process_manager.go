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
	"errors"
	"fmt"
	"sync"

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

const DefaultServiceDirectory = "/var/lib/umh/ipm"

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

	// Add to toBeCreated list
	// pm.toBeCreated = append(pm.toBeCreated, identifier) // This line is removed as per the new_code
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationCreate,
	})

	// Advance the service handling process
	return pm.step(ctx, fsService)
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

	// Add to toBeRemoved list
	// pm.toBeRemoved = append(pm.toBeRemoved, identifier) // This line is removed as per the new_code
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationRemove,
	})

	// Advance the service handling process
	return pm.step(ctx, fsService)
}

func (pm *ProcessManager) Start(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Starting process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)
	// Add to toBeStarted list
	// pm.toBeStarted = append(pm.toBeStarted, identifier) // This line is removed as per the new_code
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationStart,
	})

	// Advance the service handling process
	return pm.step(ctx, fsService)
}

func (pm *ProcessManager) Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Stopping process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)
	// Add to toBeStopped list
	// pm.toBeStopped = append(pm.toBeStopped, identifier) // This line is removed as per the new_code
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationStop,
	})

	// Advance the service handling process
	return pm.step(ctx, fsService)
}

func (pm *ProcessManager) Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Restarting process manager service", zap.String("servicePath", servicePath))

	identifier := servicePathToIdentifier(servicePath)
	// Add to toBeRestarted list
	// pm.toBeRestarted = append(pm.toBeRestarted, identifier) // This line is removed as per the new_code
	pm.taskQueue = append(pm.taskQueue, Task{
		Identifier: identifier,
		Operation:  OperationRestart,
	})

	// Advance the service handling process
	return pm.step(ctx, fsService)
}

func (pm *ProcessManager) Status(ctx context.Context, servicePath string, fsService filesystem.Service) (process_shared.ServiceInfo, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Getting status of process manager service", zap.String("servicePath", servicePath))

	return process_shared.ServiceInfo{}, errors.New("not implemented")
}

func (pm *ProcessManager) ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]process_shared.ExitEvent, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Getting exit history of process manager service", zap.String("servicePath", superviseDir))

	return []process_shared.ExitEvent{}, errors.New("not implemented")
}

func (pm *ProcessManager) ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Checking if process manager service exists", zap.String("servicePath", servicePath))

	return false, errors.New("not implemented")
}

func (pm *ProcessManager) GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (process_manager_serviceconfig.ProcessManagerServiceConfig, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Getting config of process manager service", zap.String("servicePath", servicePath))

	return process_manager_serviceconfig.ProcessManagerServiceConfig{}, errors.New("not implemented")
}

func (pm *ProcessManager) CleanServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Cleaning process manager service directory", zap.String("servicePath", path))

	return errors.New("not implemented")
}

func (pm *ProcessManager) GetConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Getting config file of process manager service", zap.String("servicePath", servicePath), zap.String("configFileName", configFileName))

	return []byte{}, errors.New("not implemented")
}

func (pm *ProcessManager) ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Force removing process manager service", zap.String("servicePath", servicePath))

	return errors.New("not implemented")
}

func (pm *ProcessManager) GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]process_shared.LogEntry, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Getting logs of process manager service", zap.String("servicePath", servicePath))

	return []process_shared.LogEntry{}, errors.New("not implemented")
}

func (pm *ProcessManager) EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Ensuring supervision of process manager service", zap.String("servicePath", servicePath))

	return false, errors.New("not implemented")
}

// Close closes all log files and performs cleanup
func (pm *ProcessManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.Logger.Info("Closing ProcessManager and all log files")

	// Close all log files
	if err := pm.logManager.CloseAll(); err != nil {
		pm.Logger.Error("Error closing log files", zap.Error(err))
		return err
	}

	return nil
}
