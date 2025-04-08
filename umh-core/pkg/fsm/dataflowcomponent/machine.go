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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// NewDataflowComponentInstance creates a new DataflowComponentInstance with a given ID and service path
func NewDataflowComponentInstance() *DataflowComponentInstance {
	panic("not implemented")
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (d *DataflowComponentInstance) SetDesiredFSMState(state string) error {
	panic("not implemented")
}

// GetCurrentFSMState returns the current state of the FSM
func (b *DataflowComponentInstance) GetCurrentFSMState() string {
	return b.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (b *DataflowComponentInstance) GetDesiredFSMState() string {
	return b.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (b *DataflowComponentInstance) Remove(ctx context.Context) error {
	return b.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (b *DataflowComponentInstance) IsRemoved() bool {
	return b.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (b *DataflowComponentInstance) IsRemoving() bool {
	return b.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (b *DataflowComponentInstance) IsStopping() bool {
	// TODO: Implement logics based on OperationalStates
	panic("unimplemented")
}

// IsStopped returns true if the instance is in the stopped state
func (b *DataflowComponentInstance) IsStopped() bool {
	// TODO: Implement logics based on OperationalStates
	panic("unimplemented")
}

// PrintState prints the current state of the FSM for debugging
func (b *DataflowComponentInstance) PrintState() {
	b.baseFSMInstance.GetLogger().Debugf("Current state: %s", b.baseFSMInstance.GetCurrentFSMState())
	b.baseFSMInstance.GetLogger().Debugf("Desired state: %s", b.baseFSMInstance.GetDesiredFSMState())
	b.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", b.ObservedState)
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (b *DataflowComponentInstance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.BenthosExpectedMaxP95ExecutionTimePerInstance
}
