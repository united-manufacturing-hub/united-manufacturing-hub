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
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/version"
)

// IAgentMonitorService defines the interface for the agent monitor service.
type IAgentMonitorService interface {
	// GetStatus returns the status of the agent, this is the main feature of this service
	Status(ctx context.Context, systemSnapshot fsm.SystemSnapshot) (*ServiceInfo, error)
	// GetFilesystemService returns the filesystem service - used for testing only
	GetFilesystemService() filesystem.Service
}

// ServiceInfo contains both raw metrics and health assessments.
type ServiceInfo struct {
	// General: Location, Latency, Agent Logs, Agent Metrics
	Location     map[int]string         `json:"location"`
	Latency      *models.Latency        `json:"latency"`
	AgentMetrics map[string]interface{} `json:"agentMetrics,omitempty"`
	// Release: Channel, Version, Supported Feature
	Release *models.Release `json:"release"`

	// AgentLogs contains the structured s6 log entries emitted by the
	// agent service.
	//
	// **Performance consideration**
	//
	//   • Logs can grow quickly, and profiling shows that naïvely deep-copying
	//     this slice dominates CPU time (see https://flamegraph.com/share/592a6a59-25d1-11f0-86bc-aa320ab09ef2).
	//
	//   • The FSM needs read-only access to the logs for historical snapshots;
	//     it never mutates them.
	//
	//   • The s6 layer *always* allocates a brand-new slice when it returns
	//     logs (see DefaultService.GetLogs), so sharing the slice's backing
	//     array across snapshots cannot introduce data races.
	//
	// Therefore we override the default behaviour and copy only the 3-word
	// slice header (24 B on amd64) — see CopyAgentLogs below.
	AgentLogs []s6.LogEntry `json:"agentLogs"`

	// Health: Overall, Latency, Release
	OverallHealth models.HealthCategory `json:"overallHealth"`
	LatencyHealth models.HealthCategory `json:"latencyHealth"`
	ReleaseHealth models.HealthCategory `json:"releaseHealth"`
}

// CopyAgentLogs is a go-deepcopy override for the AgentLogs field.
//
// go-deepcopy looks for a method with the signature
//
//	func (dst *T) Copy<FieldName>(src <FieldType>) error
//
// and, if present, calls it instead of performing its generic deep-copy logic.
// By assigning the slice directly we make a **shallow copy**: the header is
// duplicated but the underlying backing array is shared.
//
// Why this is safe:
//
//  1. The s6 service returns a fresh []LogEntry on every call, never reusing
//     or mutating a previously returned slice.
//  2. AgentLogs is treated as immutable after the snapshot is taken.
//
// If either assumption changes, delete this method to fall back to the default
// deep-copy (O(n) but safe for mutable slices).
//
// See also: https://github.com/tiendc/go-deepcopy?tab=readme-ov-file#copy-struct-fields-via-struct-methods
func (si *ServiceInfo) CopyAgentLogs(src []s6.LogEntry) error {
	si.AgentLogs = src

	return nil
}

// AgentMonitorService implements the Service interface.
type AgentMonitorService struct {
	lastCollectedAt time.Time
	fs              filesystem.Service
	s6Service       s6.Service
	logger          *zap.SugaredLogger
	instanceName    string
}

// NewAgentMonitorService creates a new agent monitor service instance.
func NewAgentMonitorService(opts ...AgentMonitorServiceOption) *AgentMonitorService {
	log := logger.For(logger.ComponentAgentMonitorService)

	service := &AgentMonitorService{
		logger:       log,
		instanceName: "Core", // Single container instance name
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

type AgentMonitorServiceOption func(*AgentMonitorService)

// WithS6Service sets a custom S6 service for the AgentMonitorService.
func WithS6Service(s6Service s6.Service) AgentMonitorServiceOption {
	return func(s *AgentMonitorService) {
		s.s6Service = s6Service
	}
}

func WithFilesystemService(fs filesystem.Service) AgentMonitorServiceOption {
	return func(s *AgentMonitorService) {
		s.fs = fs
	}
}

// GetFilesystemService returns the filesystem service - used for testing only.
func (c *AgentMonitorService) GetFilesystemService() filesystem.Service {
	return c.fs
}

// Status collects and returns the current agent status.
func (c *AgentMonitorService) Status(ctx context.Context, systemSnapshot fsm.SystemSnapshot) (*ServiceInfo, error) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentMonitor, c.instanceName+".status", time.Since(start))
	}()

	// Create a new status with default health (Active)
	status := &ServiceInfo{
		Location:     map[int]string{},
		Latency:      &models.Latency{},
		AgentLogs:    []s6.LogEntry{},
		AgentMetrics: map[string]interface{}{},
		Release:      &models.Release{},
	}

	status.OverallHealth = models.Active

	// Get the location from the config
	location := systemSnapshot.CurrentConfig.Agent.Location
	if location != nil {
		status.Location = location
	} else {
		location = map[int]string{}
		location[0] = "Unknown location" // fallback
		status.Location = location
		status.OverallHealth = models.Degraded
	}

	// Update last collected timestamp
	c.lastCollectedAt = time.Now()

	// Get the Latency
	// TODO: get latency from the communication module

	// Get the Agent Logs
	logs, err := c.getAgentLogs(ctx)
	if err != nil {
		c.logger.Warnf("Failed to get agent logs: %v", err)

		status.OverallHealth = models.Degraded

		return nil, err
	} else {
		status.AgentLogs = logs
	}

	// Get the Agent Metrics
	// TODO: get its own metrics either by calling its own metrics endpoint of accessing the struct from the metrics package directly

	// Get the Release Info
	release, err := c.getReleaseInfo(systemSnapshot.CurrentConfig)
	if err != nil {
		c.logger.Warnf("Failed to get release info: %v", err)

		status.OverallHealth = models.Degraded

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

// getAgentLogs retrieves the logs for the umh-core service from the log file.
func (c *AgentMonitorService) getAgentLogs(ctx context.Context) ([]s6.LogEntry, error) {
	// Path to the umh-core service
	servicePath := filepath.Join(constants.S6BaseDir, "umh-core")

	// Check if s6Service is initialized
	if c.s6Service == nil {
		return nil, errors.New("s6 service not initialized")
	}
	// Use the S6 service to get logs
	entries, err := c.s6Service.GetLogs(ctx, servicePath, c.fs)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs from S6 service: %w", err)
	}

	return entries, nil
}
