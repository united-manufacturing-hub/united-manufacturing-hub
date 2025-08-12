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

package redpanda_monitor

import (
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
)

// These are the redpanda-monitor operational states, in addition
// to the lifecycle states from internal_fsm.
const (
	// redpanda_monitoring_stopped is the operational state when monitoring is disabled.
	OperationalStateStopped = "redpanda_monitoring_stopped"

	// redpanda_monitoring_stopping is the operational state when monitoring is stopping.
	OperationalStateStopping = "redpanda_monitoring_stopping"

	// redpanda_monitoring_starting is the operational state when monitoring is starting.
	OperationalStateStarting = "redpanda_monitoring_starting"

	// degraded means monitoring is running, but metrics are not OK.
	OperationalStateDegraded = "degraded"

	// active means monitoring is running, and metrics are OK.
	OperationalStateActive = "active"
)

// IsOperationalState returns true if the given state is one of the
// redpanda monitor states. (Note that the instance might be in lifecycle states too.)
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

// IsStartingState returns true if the given state is a starting state.
func IsStartingState(state string) bool {
	switch state {
	case OperationalStateStarting:
		return true
	}

	return false
}

// IsRunningState returns true if the given state is a running state.
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

// RedpandaMonitorObservedState holds the last known redpanda metrics and health status.
type RedpandaMonitorObservedState struct {
	// We store the redpanda data from redpanda_monitor.GetStatus
	ServiceInfo *redpanda_monitor.ServiceInfo

	// Normally this would have also have an ObservedAgentConfig, but we don't need it here
}

// Ensure it implements the ObservedState interface.
func (b RedpandaMonitorObservedState) IsObservedState() {}

// RedpandaMonitorInstance implements fsm.FSMInstance
// If RedpandaMonitorInstance does not implement the FSMInstance interface, this will
// be detected at compile time.
var _ publicfsm.FSMInstance = (*RedpandaMonitorInstance)(nil)

// RedpandaMonitorInstance holds the FSM instance and references to the redpanda monitor service.
type RedpandaMonitorInstance struct {
	// This embeds the "BaseFSMInstance" which handles lifecycle states,
	// desired state, removal, etc.
	baseFSMInstance *internal_fsm.BaseFSMInstance

	// ObservedState: last known redpanda metrics, updated in reconcile
	ObservedState RedpandaMonitorObservedState

	// The redpanda monitor service used to gather metrics
	monitorService redpanda_monitor.IRedpandaMonitorService

	// Possibly store config needed for the redpanda monitor
	config config.RedpandaMonitorConfig
}

func (b *RedpandaMonitorInstance) GetLastObservedState() publicfsm.ObservedState {
	return b.ObservedState
}

// IsTransientStreakCounterMaxed returns true if the instance has been in a transient state for too long.
func (b *RedpandaMonitorInstance) IsTransientStreakCounterMaxed() bool {
	return b.baseFSMInstance.IsTransientStreakCounterMaxed()
}
