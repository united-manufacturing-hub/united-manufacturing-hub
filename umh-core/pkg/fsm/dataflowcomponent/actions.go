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
	"fmt"
)

// initiateAddComponentToBenthosConfig adds the data flow component to the benthos config
func (d *DataFlowComponent) initiateAddComponentToBenthosConfig(ctx context.Context) error {
	d.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding DataFlowComponent %s to Benthos config...", d.Config.Name)

	err := d.BenthosConfigManager.AddComponentToBenthosConfig(d.Config)
	if err != nil {
		return fmt.Errorf("failed to add data flow component %s to benthos config: %w", d.Config.Name, err)
	}

	d.baseFSMInstance.GetLogger().Debugf("DataFlowComponent %s added to Benthos config", d.Config.Name)
	return nil
}

// initiateRemoveComponentFromBenthosConfig removes the data flow component from the benthos config
func (d *DataFlowComponent) initiateRemoveComponentFromBenthosConfig(ctx context.Context) error {
	d.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing DataFlowComponent %s from Benthos config...", d.Config.Name)

	err := d.BenthosConfigManager.RemoveComponentFromBenthosConfig(d.Config.Name)
	if err != nil {
		return fmt.Errorf("failed to remove data flow component %s from benthos config: %w", d.Config.Name, err)
	}

	d.baseFSMInstance.GetLogger().Debugf("DataFlowComponent %s removed from Benthos config", d.Config.Name)
	return nil
}

// initiateUpdateComponentInBenthosConfig updates the data flow component in the benthos config
func (d *DataFlowComponent) initiateUpdateComponentInBenthosConfig(ctx context.Context) error {
	d.baseFSMInstance.GetLogger().Debugf("Starting Action: Updating DataFlowComponent %s in Benthos config...", d.Config.Name)

	err := d.BenthosConfigManager.UpdateComponentInBenthosConfig(d.Config)
	if err != nil {
		return fmt.Errorf("failed to update data flow component %s in benthos config: %w", d.Config.Name, err)
	}

	d.baseFSMInstance.GetLogger().Debugf("DataFlowComponent %s updated in Benthos config", d.Config.Name)
	return nil
}

// checkComponentExistsInBenthosConfig checks if the data flow component exists in the benthos config
func (d *DataFlowComponent) checkComponentExistsInBenthosConfig(ctx context.Context) (bool, error) {
	d.baseFSMInstance.GetLogger().Debugf("Checking if DataFlowComponent %s exists in Benthos config...", d.Config.Name)

	exists, err := d.BenthosConfigManager.ComponentExistsInBenthosConfig(d.Config.Name)
	if err != nil {
		return false, fmt.Errorf("failed to check if data flow component %s exists in benthos config: %w", d.Config.Name, err)
	}

	return exists, nil
}

// updateObservedState updates the observed state of the component
func (d *DataFlowComponent) updateObservedState(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	exists, err := d.checkComponentExistsInBenthosConfig(ctx)
	if err != nil {
		d.ObservedState.LastError = err.Error()
		d.ObservedState.LastConfigUpdateSuccessful = false
		return err
	}

	d.ObservedState.ConfigExists = exists
	d.ObservedState.LastError = ""
	d.ObservedState.LastConfigUpdateSuccessful = true

	return nil
}
