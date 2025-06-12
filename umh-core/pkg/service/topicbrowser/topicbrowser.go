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

package topicbrowser

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"go.uber.org/zap"
)

type ITopicBrowserService interface {
	// GenerateConfig generates a topic browser config
	GenerateConfig(tbConfig *topicbrowserserviceconfig.Config, tbName string) (s6serviceconfig.S6ServiceConfig, error)

	// GetConfig returns the actual topic browser config
	GetConfig(ctx context.Context, filesystemService filesystem.Service, tbName string) (topicbrowserserviceconfig.Config, error)

	// Status returns information about the topic browser health
	Status(ctx context.Context, filesystemService filesystem.Service, tbName string, tick uint64) (ServiceInfo, error)

	// AddToManager registers a new topic browser
	AddToManager(ctx context.Context, filesystemService filesystem.Service, cfg *topicbrowserserviceconfig.Config, tbName string) error

	// UpdateInManager modifies an existing topic browser configuration.
	UpdateInManager(ctx context.Context, filesystemService filesystem.Service, cfg *topicbrowserserviceconfig.Config, tbName string) error

	// RemoveFromManager deletes a topic browser configuration.
	RemoveFromManager(ctx context.Context, filesystemService filesystem.Service, tbName string) error

	// Start begins the monitoring of a topic browser.
	Start(ctx context.Context, filesystemService filesystem.Service, tbName string) error

	// Stop stops the monitoring of a topic browser.
	Stop(ctx context.Context, filesystemService filesystem.Service, tbName string) error

	// ForceRemove removes a topic browser instance
	ForceRemove(ctx context.Context, filesystemService filesystem.Service, tbName string) error

	// ServiceExists checks if a topic browser with the given name exists.
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, tbName string) bool

	// ReconcileManager synchronizes all connections on each tick.
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
}

// ServiceInfo holds information about the topic browsers health status.
type ServiceInfo struct {
	// needs to be filled
}

// Service implements IConnectionService
type Service struct {
	logger *zap.SugaredLogger
}

// ServiceOption is a function that configures a Service.
// This follows the functional options pattern for flexible configuration.
type ServiceOption func(*Service)

func WithService(nmapService nmap.INmapService) ServiceOption {
	return func(c *Service) {
		// to be filled
	}
}

func WithNmapManager(mgr *nmapfsm.NmapManager) ServiceOption {
	return func(c *Service) {
		// to be filled
	}
}

// NewDefaultService creates a new ConnectionService with default options.
// It initializes the logger and sets default values.
func NewDefaultService(tbName string, opts ...ServiceOption) *Service {
	managerName := fmt.Sprintf("%s%s", logger.ComponentTopicBrowserService, tbName)
	service := &Service{
		logger: logger.For(managerName),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getName converts a tbName to its underlying service name
func (svc *Service) getName(tbname string) string {
	return fmt.Sprintf("topicbrowser-%s", tbname)
}

// GenerateConfig generates a config for a given topic browser
func (svc *Service) GenerateConfig(tbConfig *topicbrowserserviceconfig.Config, tbName string) (s6serviceconfig.S6ServiceConfig, error) {
	return s6serviceconfig.S6ServiceConfig{}, nil
}

// GetConfig returns the actual Connection config from the underlying service
func (svc *Service) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	connectionName string,
) (topicbrowserserviceconfig.Config, error) {
	return topicbrowserserviceconfig.Config{}, nil
}

// Status returns information about the connection health for the specified topic browser.
func (svc *Service) Status(
	ctx context.Context,
	filesystemService filesystem.Service,
	connName string,
	tick uint64,
) (ServiceInfo, error) {
	return ServiceInfo{}, nil
}

// AddConnection registers a new connection in the Nmap service.
func (svc *Service) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *topicbrowserserviceconfig.Config,
	tbName string,
) error {
	return nil
}

// UpdateInManager modifies an existing topic browser configuration.
func (svc *Service) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *topicbrowserserviceconfig.Config,
	tbName string,
) error {
	return nil
}

// RemoveFromManager deletes a topic browser configuration.
// This stops monitoring the connection and removes all configuration.
func (svc *Service) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	return nil
}

// Start starts a topic browser
func (svc *Service) Start(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	return nil
}

// Stop stops a topic browser
func (svc *Service) Stop(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	return nil
}

// ReconcileManager synchronizes all topic browser on each tick.
func (svc *Service) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (error, bool) {
	return nil, false
}

// ServiceExists checks if a topic browser with the given name exists.
// Used by the FSM to determine appropriate transitions.
func (svc *Service) ServiceExists(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) bool {
	return true
}

// ForceRemove removes a Topic Browser from the manager
func (svc *Service) ForceRemove(
	ctx context.Context,
	filesystemService filesystem.Service,
	tbName string,
) error {
	return nil
}
