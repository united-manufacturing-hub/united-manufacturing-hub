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

package agent_monitor

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/version"
)

// IAgentMonitorService defines the interface for the agent monitor service
type IAgentMonitorService interface {
	// GetStatus returns the status of the agent, this is the main feature of this service
	GetStatus(ctx context.Context, cfg config.FullConfig) (*ServiceInfo, error)
	// GetAgentLogs retrieves the logs for the umh-core service from the log file
	GetAgentLogs(ctx context.Context) ([]s6.LogEntry, error)
	// GetFilesystemService returns the filesystem service - used for testing only
	GetFilesystemService() filesystem.Service
}

// ServiceInfo contains both raw metrics and health assessments
type ServiceInfo struct {
	// General: Location, Latency, Agent Logs, Agent Metrics
	Location     map[int]string         `json:"location"`
	Latency      *models.Latency        `json:"latency"`
	AgentLogs    []s6.LogEntry          `json:"agentLogs"`
	AgentMetrics map[string]interface{} `json:"agentMetrics,omitempty"`
	// Release: Channel, Version, Supported Feature
	Release *models.Release `json:"release"`
}

// Service defines the interface for agent monitoring
type Service interface {
	// GetStatus returns the status of the agent itself
	// It requires the full config to get the location and release info
	GetStatus(ctx context.Context, cfg config.FullConfig) (*ServiceInfo, error)
}

// AgentMonitorService implements the Service interface
type AgentMonitorService struct {
	fs              filesystem.Service
	s6Service       s6.Service
	logger          *zap.SugaredLogger
	instanceName    string
	lastCollectedAt time.Time
}

// NewAgentMonitorService creates a new agent monitor service instance
func NewAgentMonitorService(fs filesystem.Service) *AgentMonitorService {
	log := logger.For(logger.ComponentAgentMonitorService)

	return &AgentMonitorService{
		fs:           fs,
		s6Service:    s6.NewDefaultService(),
		logger:       log,
		instanceName: "Core", // Single container instance name
	}
}

// NewAgentMonitorWithS6Service creates a new agent monitor service with a provided S6 service
// This is useful for testing with mocked services
func NewAgentMonitorWithS6Service(fs filesystem.Service, s6Service s6.Service) *AgentMonitorService {
	log := logger.For(logger.ComponentAgentMonitorService)

	return &AgentMonitorService{
		fs:           fs,
		s6Service:    s6Service,
		logger:       log,
		instanceName: "Core", // Single container instance name
	}
}

// GetFilesystemService returns the filesystem service - used for testing only
func (c *AgentMonitorService) GetFilesystemService() filesystem.Service {
	return c.fs
}

// GetStatus collects and returns the current agent status
func (c *AgentMonitorService) GetStatus(ctx context.Context, cfg config.FullConfig) (*ServiceInfo, error) {
	// Create a new status with default health (Active)
	status := &ServiceInfo{
		Location:     map[int]string{},
		Latency:      &models.Latency{},
		AgentLogs:    []s6.LogEntry{},
		AgentMetrics: map[string]interface{}{},
		Release:      &models.Release{},
	}

	// Get the location from the config
	location := cfg.Agent.Location
	if location != nil {
		status.Location = location
	}

	// Get the Latency
	// TODO: get latency from the communication module

	// Get the Agent Logs
	logs, err := c.GetAgentLogs(ctx)
	if err != nil {
		c.logger.Warnf("Failed to get agent logs: %v", err)
		return nil, err
	} else {
		status.AgentLogs = logs
	}

	// Get the Agent Metrics
	// TODO: get its own metrics either by calling its own metrics endpoint of accessing the struct from the metrics package directly

	// Get the Release Info
	release, err := c.getReleaseInfo(cfg)
	if err != nil {
		return nil, err
	}
	status.Release = release

	return status, nil
}

func (c *AgentMonitorService) getReleaseInfo(cfg config.FullConfig) (*models.Release, error) {
	release := &models.Release{}

	release.Channel = string(cfg.Agent.ReleaseChannel)

	// Get the core version from the version package
	release.Version = version.GetAppVersion()

	// Add component versions
	release.Versions = version.GetComponentVersions()

	return release, nil
}

// GetAgentLogs retrieves the logs for the umh-core service from the log file
func (c *AgentMonitorService) GetAgentLogs(ctx context.Context) ([]s6.LogEntry, error) {
	// Path to the umh-core service
	servicePath := filepath.Join(constants.S6BaseDir, "umh-core")

	// Use the S6 service to get logs
	entries, err := c.s6Service.GetLogs(ctx, servicePath, c.fs)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs from S6 service: %w", err)
	}

	return entries, nil
}
