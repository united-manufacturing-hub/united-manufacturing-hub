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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
)

// AgentMonitorIdentity represents the immutable identity of an agent monitor instance.
// These fields never change after creation.
type AgentMonitorIdentity struct {
	ID   string // Unique agent monitor identifier
	Name string // Human-readable name
}

// AgentMonitorDesiredState represents what we want the agent monitoring to be.
// This is derived from user configuration.
type AgentMonitorDesiredState struct {
	shutdownRequested bool
}

// ShutdownRequested returns true if graceful shutdown was requested.
func (d *AgentMonitorDesiredState) ShutdownRequested() bool {
	return d.shutdownRequested
}

// SetShutdownRequested updates the shutdown flag.
func (d *AgentMonitorDesiredState) SetShutdownRequested(requested bool) {
	d.shutdownRequested = requested
}

// AgentMonitorObservedState represents the actual state of agent monitoring.
// Collected from the agent_monitor service.
type AgentMonitorObservedState struct {
	ServiceInfo *agent_monitor.ServiceInfo // Health metrics and assessments from agent_monitor service
	CollectedAt time.Time                   // When metrics were collected
}

// GetTimestamp returns when the observed state was collected.
func (o *AgentMonitorObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the desired state that was observed in the system.
// For agent monitoring, there is no deployed configuration to observe,
// so this always returns an empty desired state.
func (o *AgentMonitorObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &AgentMonitorDesiredState{
		shutdownRequested: false,
	}
}
