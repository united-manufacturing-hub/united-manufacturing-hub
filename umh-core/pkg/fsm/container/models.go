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

package container

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
)

// These are the container-monitor operational states, in addition
// to the lifecycle states from internal_fsm.
const (
	// monitoring_stopped is the operational state when monitoring is disabled
	MonitoringStateStopped = "monitoring_stopped"

	// degraded means monitoring is running, but metrics are not OK
	MonitoringStateDegraded = "degraded"

	// active means monitoring is running, and metrics are OK
	MonitoringStateActive = "active"
)

// Operational events
// (We also rely on the standard lifecycle events from internal_fsm.)
const (
	EventStart        = "start_monitoring"
	EventStop         = "stop_monitoring"
	EventMetricsAllOK = "metrics_all_ok"
	EventMetricsNotOK = "metrics_not_ok"
)

// IsOperationalState returns true if the given state is one of the three
// container monitor states. (Note that the instance might be in lifecycle states too.)
func IsOperationalState(state string) bool {
	switch state {
	case MonitoringStateStopped,
		MonitoringStateDegraded,
		MonitoringStateActive:
		return true
	}
	return false
}

// ContainerObservedState holds the last known container metrics and health status
type ContainerObservedState struct {
	// We store the container data from container_monitor.GetStatus
	ContainerStatus *container_monitor.ContainerStatus
}

// Ensure it implements the ObservedState interface
func (c ContainerObservedState) IsObservedState() {}

// ContainerMonitorInstance implements fsm.FSMInstance
// (See machine.go for the struct definition.)
type ContainerMonitorInstance interface {
	fsm.FSMInstance
	// Additional Container-monitor–specific methods here if needed
}

// GetLastObservedState returns the last known observed data
func (c *ContainerInstance) GetLastObservedState() publicfsm.ObservedState {
	return c.ObservedState
}
