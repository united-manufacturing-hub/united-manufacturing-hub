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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
)

// AgentMonitorWorker implements the Worker interface for agent monitoring.
// It wraps the existing agent_monitor.Service to collect agent health metrics.
type AgentMonitorWorker struct {
	identity       fsmv2.Identity
	monitorService agent_monitor.IAgentMonitorService
}

// NewAgentMonitorWorker creates a new agent monitor worker.
func NewAgentMonitorWorker(id string, name string, service agent_monitor.IAgentMonitorService) *AgentMonitorWorker {
	return &AgentMonitorWorker{
		identity: fsmv2.Identity{
			ID:         id,
			Name:       name,
			WorkerType: "agent_monitor",
		},
		monitorService: service,
	}
}

// CollectObservedState monitors the actual system state.
// Called in a separate goroutine, can block for metric collection.
//
// This replaces the whole `_monitor` logic from FSM v1.
func (w *AgentMonitorWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	serviceInfo, err := w.monitorService.Status(ctx, fsm.SystemSnapshot{})
	if err != nil {
		return nil, err
	}

	observed := &AgentMonitorObservedState{
		ServiceInfo: serviceInfo,
		CollectedAt: time.Now(),
	}

	return observed, nil
}

// DeriveDesiredState transforms user configuration into desired state.
// Pure function - no side effects. Called on each tick.
//
// The spec parameter comes from user configuration.
func (w *AgentMonitorWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &AgentMonitorDesiredState{
		shutdownRequested: false,
	}, nil
}

// GetInitialState returns the starting state for this worker.
// Called once during worker creation.
func (w *AgentMonitorWorker) GetInitialState() fsmv2.State {
	return &StoppedState{}
}
