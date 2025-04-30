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
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
)

// These are the agent-monitor operational states, in addition
// to the lifecycle states from internal_fsm.
const (
	// agent_monitoring_stopped is the operational state when monitoring is disabled
	OperationalStateStopped = "agent_monitoring_stopped"

	// agent_monitoring_stopping is the operational state when monitoring is stopping
	OperationalStateStopping = "agent_monitoring_stopping"

	// agent_monitoring_starting is the operational state when monitoring is starting
	OperationalStateStarting = "agent_monitoring_starting"

	// degraded means monitoring is running, but metrics are not OK
	OperationalStateDegraded = "degraded"

	// active means monitoring is running, and metrics are OK
	OperationalStateActive = "active"
)

// IsOperationalState returns true if the given state is one of the
// agent monitor states. (Note that the instance might be in lifecycle states too.)
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateStopping,
		OperationalStateStarting,
		OperationalStateDegraded,
		OperationalStateActive:
		return true
	}
	return false
}

// IsStartingState returns true if the given state is a starting state
func IsStartingState(state string) bool {
	switch state {
	case OperationalStateStarting:
		return true
	}
	return false
}

// IsRunningState returns true if the given state is a running state
func IsRunningState(state string) bool {
	switch state {
	case OperationalStateActive,
		OperationalStateDegraded:
		return true
	}
	return false
}

// Operational events
// (We also rely on the standard lifecycle events from internal_fsm.)
const (
	EventStart        = "start_monitoring"
	EventStartDone    = "start_monitoring_done"
	EventStop         = "stop_monitoring"
	EventStopDone     = "stop_monitoring_done"
	EventMetricsAllOK = "metrics_all_ok"
	EventMetricsNotOK = "metrics_not_ok"
)

// AgentObservedState holds the last known agent metrics and health status
type AgentObservedState struct {
	// We store the agent data from agent_monitor.GetStatus
	ServiceInfo *agent_monitor.ServiceInfo

	// Normally this would have also have an ObservedAgentConfig, but we don't need it here
}

// Ensure it implements the ObservedState interface
func (a AgentObservedState) IsObservedState() {}

// AgentMonitorInstance implements fsm.FSMInstance
// If AgentInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*AgentInstance)(nil)

// AgentInstance holds the FSM instance and references to the agent monitor service.
type AgentInstance struct {
	// This embeds the "BaseFSMInstance" which handles lifecycle states,
	// desired state, removal, etc.
	baseFSMInstance *internal_fsm.BaseFSMInstance

	// ObservedState: last known agent metrics, updated in reconcile
	ObservedState AgentObservedState

	// The agent monitor service used to gather metrics
	monitorService agent_monitor.IAgentMonitorService

	// Possibly store config needed for the agent monitor
	config config.AgentMonitorConfig
}

// GetLastObservedState returns the last known observed data
func (a *AgentInstance) GetLastObservedState() publicfsm.ObservedState {
	return a.ObservedState
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed
func (a *AgentInstance) IsTransientStreakCounterMaxed() bool {
	return a.baseFSMInstance.IsTransientStreakCounterMaxed()
}
