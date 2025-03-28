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

package agent

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"go.uber.org/zap"
)

// AgentStatus represents the status of the agent
type AgentStatus string

const (
	// AgentStatusUnknown indicates the agent status cannot be determined
	AgentStatusUnknown AgentStatus = "unknown"
	// AgentStatusRunning indicates the agent is running
	AgentStatusRunning AgentStatus = "running"
	// AgentStatusStopped indicates the agent is stopped
	AgentStatusStopped AgentStatus = "stopped"
	// AgentStatusError indicates the agent has encountered an error
	AgentStatusError AgentStatus = "error"
)

// AgentInfo contains information about the agent
type AgentInfo struct {
	Status         AgentStatus           // Current status of the agent
	Location       map[int]string        // Hierarchical location information
	ReleaseChannel config.ReleaseChannel // Release channel information
	LastChangedAt  time.Time             // Timestamp when the agent status was last changed
}

// Service defines the interface for interacting with the Agent service
type Service interface {
	// Initialize initializes the agent with the given configuration
	Initialize(ctx context.Context, config config.AgentConfig) error
	// GetStatus gets the current status of the agent
	GetStatus(ctx context.Context) (AgentInfo, error)
	// UpdateLocation updates the agent's location information
	UpdateLocation(ctx context.Context, location map[int]string) error
	// UpdateReleaseChannel updates the agent's release channel
	UpdateReleaseChannel(ctx context.Context, releaseChannel config.ReleaseChannel) error
}

// DefaultService is the default implementation of the Agent Service interface
type DefaultService struct {
	logger      *zap.SugaredLogger
	agentInfo   AgentInfo
	initialized bool
}

// NewDefaultService creates a new default Agent service
func NewDefaultService() Service {
	return &DefaultService{
		logger: logger.For(logger.ComponentAgentService),
		agentInfo: AgentInfo{
			Status:         AgentStatusUnknown,
			Location:       make(map[int]string),
			ReleaseChannel: "",
			LastChangedAt:  time.Time{},
		},
		initialized: false,
	}
}

// Initialize initializes the agent with the given configuration
func (s *DefaultService) Initialize(ctx context.Context, config config.AgentConfig) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentService, "initialize", time.Since(start))
	}()

	s.logger.Infof("Initializing agent service with config: %+v", config)

	// Update location information
	if len(config.Location) > 0 {
		s.agentInfo.Location = config.Location
	}

	// Update release channel
	if config.ReleaseChannel != "" {
		s.agentInfo.ReleaseChannel = config.ReleaseChannel
	}

	// Update status information
	s.agentInfo.Status = AgentStatusRunning
	s.agentInfo.LastChangedAt = time.Now()
	s.initialized = true

	s.logger.Info("Agent service initialized successfully")
	return nil
}

// GetStatus gets the current status of the agent
func (s *DefaultService) GetStatus(ctx context.Context) (AgentInfo, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentService, "getStatus", time.Since(start))
	}()

	if !s.initialized {
		return AgentInfo{}, ErrAgentNotInitialized
	}

	return s.agentInfo, nil
}

// UpdateLocation updates the agent's location information
func (s *DefaultService) UpdateLocation(ctx context.Context, location map[int]string) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentService, "updateLocation", time.Since(start))
	}()

	if !s.initialized {
		return ErrAgentNotInitialized
	}

	if location == nil {
		return ErrInvalidLocation
	}

	s.logger.Infof("Updating agent location: %+v", location)
	s.agentInfo.Location = location
	s.agentInfo.LastChangedAt = time.Now()
	return nil
}

// UpdateReleaseChannel updates the agent's release channel
func (s *DefaultService) UpdateReleaseChannel(ctx context.Context, releaseChannel config.ReleaseChannel) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentAgentService, "updateReleaseChannel", time.Since(start))
	}()

	if !s.initialized {
		return ErrAgentNotInitialized
	}

	if releaseChannel == "" {
		return ErrInvalidReleaseChannel
	}

	s.logger.Infof("Updating agent release channel: %s", releaseChannel)
	s.agentInfo.ReleaseChannel = releaseChannel
	s.agentInfo.LastChangedAt = time.Now()
	return nil
}
