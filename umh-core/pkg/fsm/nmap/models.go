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

package nmap

import (
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// These are the nmap-monitor operational states, in addition
// to the lifecycle states from internal_fsm.
const (
	// stopped is the initial state when the service is stopped
	OperationalStateStopped = "stopped"

	// Starting phase states
	// starting is the operational state when nmap is starting
	OperationalStateStarting = "starting"
	// degraded means nmap is running, but execution is not OK
	OperationalStateDegraded = "degraded"

	// Running phase states
	// See nmap-states here: https://nmap.org/book/man-port-scanning-basics.html
	// active means nmap is running and it shows port filtered
	OperationalStateFiltered = "filtered"
	// active means nmap is running and it shows port down
	OperationalStateClosed = "closed"
	// open means nmap is running and it shows port open
	OperationalStateOpen = "open"
	// unfiltered means nmap is running and it shows port unfiltered
	OperationalStateUnfiltered = "unfiltered"
	// unfiltered means nmap is running and it shows port open|filtered
	OperationalStateOpenFiltered = "open_filtered"
	// unfiltered means nmap is running and it shows port closed|filtered
	OperationalStateClosedFiltered = "closed_filtered"

	// stopping is the operational state when nmap is stopping
	OperationalStateStopping = "stopping"
)

// IsOperationalState returns true if the given state is one of the
// nmap monitor states. (Note that the instance might be in lifecycle states too.)
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateStopping,
		OperationalStateStarting,
		OperationalStateDegraded,
		OperationalStateClosed,
		OperationalStateFiltered,
		OperationalStateUnfiltered,
		OperationalStateOpenFiltered,
		OperationalStateClosedFiltered,
		OperationalStateOpen:
		return true
	}
	return false
}

// Operational events
// (We also rely on the standard lifecycle events from internal_fsm.)
const (
	EventStart     = "start_monitoring"
	EventStartDone = "start_monitoring_done"

	EventStop     = "stop_monitoring"
	EventStopDone = "stop_monitoring_done"

	EventNmapExecutionFailed = "nmap_not_ok"

	EventPortOpen           = "port_open"
	EventPortClosed         = "port_closed"
	EventPortFiltered       = "port_filtered"
	EventPortUnfiltered     = "port_unfiltered"
	EventPortClosedFiltered = "port_closed_filtered"
	EventPortOpenFiltered   = "port_open_filtered"
)

// IsStartingState returns whether the given state is a starting state
func IsStartingState(state string) bool {
	switch state {
	case OperationalStateStarting:
		return true

	}
	return false
}

// IsRunningState returns whether the given state is a running state
func IsRunningState(state string) bool {
	switch state {
	case OperationalStateOpen,
		OperationalStateFiltered,
		OperationalStateClosed,
		OperationalStateUnfiltered,
		OperationalStateClosedFiltered,
		OperationalStateOpenFiltered,
		OperationalStateDegraded:
		return true

	}
	return false
}

// NmapObservedState holds the last known nmap metrics and health status
type NmapObservedState struct {
	// ObservedNmapServiceConfig contains the observed Nmap service config
	ObservedNmapServiceConfig nmapserviceconfig.NmapServiceConfig
	// We store the nmap data from nmap_monitor.GetStatus
	ServiceInfo nmap.ServiceInfo
	// LastStateChange is the timestamp of the last observed state change
	LastStateChange int64
}

// Ensure it implements the ObservedState interface
func (n NmapObservedState) IsObservedState() {}

// Verify at compile time that we implement fsm.FSMInstance
var _ publicfsm.FSMInstance = (*NmapInstance)(nil)

// NmapInstance holds the FSM instance and references to the container monitor service.
type NmapInstance struct {

	// The nmap service used to gather metrics
	monitorService nmap.INmapService

	// This embeds the "BaseFSMInstance" which handles lifecycle states,
	// desired state, removal, etc.
	baseFSMInstance *internal_fsm.BaseFSMInstance

	// Possibly store config needed for nmap monitoring
	config config.NmapConfig

	// ObservedState: last known nmap metrics, updated in reconcile
	ObservedState NmapObservedState
}

type PortState string

const (
	PortStateOpen           PortState = "open"
	PortStateFiltered       PortState = "filtered"
	PortStateClosed         PortState = "closed"
	PortStateUnfiltered     PortState = "unfiltered"
	PortStateOpenFiltered   PortState = "open|filtered"
	PortStateClosedFiltered PortState = "closed|filtered"
)

// GetLastObservedState returns the last known observed data
func (n *NmapInstance) GetLastObservedState() publicfsm.ObservedState {
	return n.ObservedState
}

// SetService sets the Nmap service implementation
// This is a testing-only utility to access the private field
func (n *NmapInstance) SetService(service nmap.INmapService) {
	n.monitorService = service
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed
func (n *NmapInstance) IsTransientStreakCounterMaxed() bool {
	return n.baseFSMInstance.IsTransientStreakCounterMaxed()
}
