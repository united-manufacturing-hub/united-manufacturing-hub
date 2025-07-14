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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/process_manager_serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

type ProcessManager struct {
	Logger *zap.SugaredLogger
}

const IPM_SERVICE_DIRECTORY = "/var/lib/umh/ipm"

func (ProcessManager) Create(ctx context.Context, servicePath string, config process_manager_serviceconfig.ProcessManagerServiceConfig, fsService filesystem.Service) error {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) Start(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) Status(ctx context.Context, servicePath string, fsService filesystem.Service) (process_shared.ServiceInfo, error) {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]process_shared.ExitEvent, error) {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (process_manager_serviceconfig.ProcessManagerServiceConfig, error) {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) CleanServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) GetConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]process_shared.LogEntry, error) {
	//TODO implement me
	panic("implement me")
}

func (ProcessManager) EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	//TODO implement me
	panic("implement me")
}
