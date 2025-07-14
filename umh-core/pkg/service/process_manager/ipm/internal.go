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
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

type ProcessManager struct {
	Logger *zap.SugaredLogger
	mu     sync.Mutex

	services map[string]service
}

type service struct {
	config process_manager_serviceconfig.ProcessManagerServiceConfig
}

const IPM_SERVICE_DIRECTORY = "/var/lib/umh/ipm"

func (pm *ProcessManager) Create(ctx context.Context, servicePath string, config process_manager_serviceconfig.ProcessManagerServiceConfig, _ filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Creating process manager service", zap.String("servicePath", servicePath), zap.Any("config", config))

	return errors.New("not implemented")
}

func (pm *ProcessManager) Remove(ctx context.Context, servicePath string, _ filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Removing process manager service", zap.String("servicePath", servicePath))

	return errors.New("not implemented")
}

func (pm *ProcessManager) Start(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Starting process manager service", zap.String("servicePath", servicePath))

	return errors.New("not implemented")
}

func (pm *ProcessManager) Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Stopping process manager service", zap.String("servicePath", servicePath))

	return errors.New("not implemented")
}

func (pm *ProcessManager) Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Logger.Info("Restarting process manager service", zap.String("servicePath", servicePath))

	return errors.New("not implemented")
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
