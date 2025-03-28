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
	"errors"
	"time"
)

var (
	// ErrAgentNotExist is returned when an agent does not exist
	ErrAgentNotExist = errors.New("agent does not exist")
	// ErrAgentInvalidState is returned when an agent is in an invalid state
	ErrAgentInvalidState = errors.New("agent is in an invalid state")
)

// AgentStatus represents the status of an agent
type AgentStatus string

const (
	// AgentActive indicates the agent is active and running
	AgentActive AgentStatus = "active"
	// AgentStopped indicates the agent is stopped
	AgentStopped AgentStatus = "stopped"
	// AgentDegraded indicates the agent is running but in a degraded state
	AgentDegraded AgentStatus = "degraded"
	// AgentUnknown indicates the agent status is unknown
	AgentUnknown AgentStatus = "unknown"
)

// AgentInfo contains information about an agent
type AgentInfo struct {
	// Status is the current status of the agent
	Status AgentStatus
	// LastHeartbeat is the timestamp of the last heartbeat from the agent
	LastHeartbeat int64
	// IsConnected indicates if the agent is connected
	IsConnected bool
	// ErrorCount is the number of errors encountered by the agent
	ErrorCount int
	// LastErrorTime is the timestamp of the last error
	LastErrorTime int64
}

// Agent defines the interface for interacting with agents
type Agent interface {
	// Start starts monitoring an agent
	Start(ctx context.Context, agentID string) error

	// Stop stops monitoring an agent
	Stop(ctx context.Context, agentID string) error

	// Status retrieves the current status of an agent
	Status(ctx context.Context, agentID string) (AgentInfo, error)

	// Remove forcefully removes an agent
	Remove(ctx context.Context, agentID string) error
}

// DefaultAgent is the default implementation of the Agent interface
type DefaultAgent struct{}

// NewDefaultAgent creates a new DefaultAgent
func NewDefaultAgent() *DefaultAgent {
	return &DefaultAgent{}
}

// Start starts monitoring an agent
func (d *DefaultAgent) Start(ctx context.Context, agentID string) error {
	// In a real implementation, this would start monitoring the agent
	// For now, this is just a placeholder
	return nil
}

// Stop stops monitoring an agent
func (d *DefaultAgent) Stop(ctx context.Context, agentID string) error {
	// In a real implementation, this would stop monitoring the agent
	// For now, this is just a placeholder
	return nil
}

// Status retrieves the current status of an agent
func (d *DefaultAgent) Status(ctx context.Context, agentID string) (AgentInfo, error) {
	// In a real implementation, this would fetch the actual agent status
	// For now, return a placeholder status
	return AgentInfo{
		Status:        AgentActive,
		LastHeartbeat: time.Now().Unix(),
		IsConnected:   true,
		ErrorCount:    0,
		LastErrorTime: 0,
	}, nil
}

// Remove forcefully removes an agent
func (d *DefaultAgent) Remove(ctx context.Context, agentID string) error {
	// In a real implementation, this would remove the agent from monitoring
	// For now, this is just a placeholder
	return nil
}

// MockAgent is a mock implementation of the Agent interface for testing
type MockAgent struct {
	StartFunc  func(ctx context.Context, agentID string) error
	StopFunc   func(ctx context.Context, agentID string) error
	StatusFunc func(ctx context.Context, agentID string) (AgentInfo, error)
	RemoveFunc func(ctx context.Context, agentID string) error
}

// Start calls the mock StartFunc
func (m *MockAgent) Start(ctx context.Context, agentID string) error {
	if m.StartFunc != nil {
		return m.StartFunc(ctx, agentID)
	}
	return nil
}

// Stop calls the mock StopFunc
func (m *MockAgent) Stop(ctx context.Context, agentID string) error {
	if m.StopFunc != nil {
		return m.StopFunc(ctx, agentID)
	}
	return nil
}

// Status calls the mock StatusFunc
func (m *MockAgent) Status(ctx context.Context, agentID string) (AgentInfo, error) {
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx, agentID)
	}
	return AgentInfo{
		Status:        AgentActive,
		LastHeartbeat: time.Now().Unix(),
		IsConnected:   true,
		ErrorCount:    0,
		LastErrorTime: 0,
	}, nil
}

// Remove calls the mock RemoveFunc
func (m *MockAgent) Remove(ctx context.Context, agentID string) error {
	if m.RemoveFunc != nil {
		return m.RemoveFunc(ctx, agentID)
	}
	return nil
}
