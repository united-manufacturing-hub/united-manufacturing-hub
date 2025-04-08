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
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	dataflowcomponentconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
)

// DataflowComponentObservedState contains the observed runtime state of a DataflowComponent instance
type DataflowComponentObservedState struct {
	// ServiceInfo contains information about the S6 service
	ServiceInfo dataflowcomponentsvc.ServiceInfo

	// ObservedDataflowComponentConfig contains the observed DataflowComponent service config
	ObservedDataflowComponentConfig dataflowcomponentconfig.DataFlowComponentConfig
}

// IsObservedState implements the ObservedState interface
func (b DataflowComponentObservedState) IsObservedState() {}

// BenthosInstance implements the FSMInstance interface
// If BenthosInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*DataflowComponentInstance)(nil)

// BenthosInstance is a state-machine managed instance of a Benthos service
// DataflowComponentInstance is a state-machine managed instance of a DataflowComponent service.
type DataflowComponentInstance struct {
	baseFSMInstance *internalfsm.BaseFSMInstance

	// ObservedState represents the observed state of the service
	// ObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	ObservedState DataflowComponentObservedState

	// service is the DataflowComponent service implementation to use
	// It has a manager that manages the benthos service instances
	service dataflowcomponentsvc.IDataFlowComponentService

	// config contains all the configuration for this service
	config dataflowcomponentconfig.DataFlowComponentConfig
}

// GetLastObservedState returns the last known state of the instance
func (d *DataflowComponentInstance) GetLastObservedState() publicfsm.ObservedState {
	return d.ObservedState
}

// SetService sets the DataflowComponent service implementation to use
// This is a testing-only utility to access the private service field
func (d *DataflowComponentInstance) SetService(service dataflowcomponentsvc.IDataFlowComponentService) {
	d.service = service
}

// GetConfig returns the DataflowComponentServiceConfig for this service
// This is a testing-only utility to access the private service field
func (d *DataflowComponentInstance) GetConfig() dataflowcomponentconfig.DataFlowComponentConfig {
	return d.config
}
