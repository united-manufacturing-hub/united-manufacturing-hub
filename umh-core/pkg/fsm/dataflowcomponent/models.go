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

package dataflowcomponent

import (
	"context"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
)

// Operational state constants (using internal_fsm compatible naming)
const (
	// OperationalStateStopped is the initial state and also the state when the service is stopped
	OperationalStateStopped = "stopped"

	// OperationalStateStarting is the state when the component is starting
	OperationalStateStarting = "starting"

	// OperationalStateActive is the state when the component is active and running
	OperationalStateActive = "active"

	// OperationalStateDegraded is the state when the component is running but has encountered issues
	OperationalStateDegraded = "degraded"

	// OperationalStateStopping is the state when the component is in the process of stopping
	OperationalStateStopping = "stopping"
)

// Operational event constants
const (
	// Basic lifecycle events
	EventStart     = "start"
	EventStartDone = "start_done"
	EventStop      = "stop"
	EventStopDone  = "stop_done"

	// Status events
	EventDegraded  = "degraded"
	EventRecovered = "recovered"
)

// IsOperationalState returns whether the given state is a valid operational state
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateStarting,
		OperationalStateActive,
		OperationalStateDegraded,
		OperationalStateStopping:
		return true
	}
	return false
}

// DataFlowComponentConfig represents the configuration for a data flow component
type DataFlowComponentConfig struct {
	// Basic component configuration
	Name         string `yaml:"name"`
	DesiredState string `yaml:"desiredState"`

	// Service configuration similar to BenthosServiceConfig
	ServiceConfig benthosserviceconfig.BenthosServiceConfig `yaml:"serviceConfig"`
}

// DFCObservedState contains the observed state of a DataFlowComponent
type DFCObservedState struct {
	// Component configuration in the benthos config
	ConfigExists bool

	// Whether the last config update was successful
	LastConfigUpdateSuccessful bool

	// Any error message from the last config update
	LastError string
}

// IsObservedState implements the ObservedState interface
func (d DFCObservedState) IsObservedState() {}

// DataFlowComponent implements the FSMInstance interface
var _ publicfsm.FSMInstance = (*DataFlowComponent)(nil)

// DataFlowComponent is a state-machine managed instance of a data flow component
type DataFlowComponent struct {
	baseFSMInstance *internalfsm.BaseFSMInstance

	// ObservedState represents the observed state of the component
	ObservedState DFCObservedState

	// Config contains the configuration for this component
	Config DataFlowComponentConfig

	// BenthosConfigManager is responsible for updating the benthos configuration
	BenthosConfigManager BenthosConfigManager
}

// BenthosConfigManager is the interface for managing benthos configuration
type BenthosConfigManager interface {
	// AddComponentToBenthosConfig adds a component to the benthos config
	AddComponentToBenthosConfig(ctx context.Context, component DataFlowComponentConfig) error

	// RemoveComponentFromBenthosConfig removes a component from the benthos config
	RemoveComponentFromBenthosConfig(ctx context.Context, componentName string) error

	// UpdateComponentInBenthosConfig updates a component in the benthos config
	UpdateComponentInBenthosConfig(ctx context.Context, component DataFlowComponentConfig) error

	// ComponentExistsInBenthosConfig checks if a component exists in the benthos config
	ComponentExistsInBenthosConfig(ctx context.Context, componentName string) (bool, error)
}

// GetLastObservedState returns the last known state of the component
func (d *DataFlowComponent) GetLastObservedState() publicfsm.ObservedState {
	return d.ObservedState
}

// GetCurrentFSMState returns the current state of the FSM
func (d *DataFlowComponent) GetCurrentFSMState() string {
	return d.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (d *DataFlowComponent) GetDesiredFSMState() string {
	return d.baseFSMInstance.GetDesiredFSMState()
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (d *DataFlowComponent) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.BenthosExpectedMaxP95ExecutionTimePerInstance
}
