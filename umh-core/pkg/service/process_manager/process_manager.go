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

package process_manager

import (
	"context"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/s6"
)

// Service defines the interface for interacting with S6 services
type Service interface {
	// Create creates the service with specific configuration
	Create(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) error
	// Remove removes the service directory structure
	Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Start starts the service
	Start(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Stop stops the service
	Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Restart restarts the service
	Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Status gets the current status of the service
	Status(ctx context.Context, servicePath string, fsService filesystem.Service) (process_shared.ServiceInfo, error)
	// ExitHistory gets the exit history of the service
	ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]process_shared.ExitEvent, error)
	// ServiceExists checks if the service directory exists
	ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error)
	// GetConfig gets the actual service config from s6
	GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (s6serviceconfig.S6ServiceConfig, error)
	// CleanS6ServiceDirectory cleans the S6 service directory, removing non-standard services
	CleanS6ServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error
	// GetS6ConfigFile retrieves a config file for a service
	GetS6ConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error)
	// ForceRemove removes a service from the S6 manager
	ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// GetLogs gets the logs of the service
	GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]process_shared.LogEntry, error)
	// EnsureSupervision checks if the supervise directory exists for a service and notifies
	// s6-svscan if it doesn't, to trigger supervision setup.
	// Returns true if supervise directory exists (ready for supervision), false otherwise.
	EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error)
}

// NewDefaultService creates a new default S6 service
func NewDefaultService() Service {
	serviceLogger := logger.For(logger.ComponentS6Service)
	return &s6.DefaultService{
		Logger: serviceLogger,
	}
}
