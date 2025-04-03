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
	logger := d.baseFSMInstance.GetLogger()
	logger.Infof("Starting Action: Adding DataFlowComponent %s to Benthos config...", d.Config.Name)
	logger.Debugf("DataFlowComponent details: Name=%s, State=%s, UUID=%s",
		d.Config.Name, d.Config.DesiredState, d.Config.VersionUUID)

	logger.Debugf("Calling BenthosConfigManager.AddComponentToBenthosConfig for %s", d.Config.Name)
	err := d.BenthosConfigManager.AddComponentToBenthosConfig(ctx, d.Config)
	if err != nil {
		logger.Errorf("Failed to add data flow component %s to benthos config: %v", d.Config.Name, err)
		return fmt.Errorf("failed to add data flow component %s to benthos config: %w", d.Config.Name, err)
	}

	logger.Infof("DataFlowComponent %s successfully added to Benthos config", d.Config.Name)
	// Update observed state to reflect the configuration exists
	d.ObservedState.ConfigExists = true
	d.ObservedState.LastConfigUpdateSuccessful = true
	d.ObservedState.LastError = ""

	return nil
}

// initiateRemoveComponentFromBenthosConfig removes the data flow component from the benthos config
func (d *DataFlowComponent) initiateRemoveComponentFromBenthosConfig(ctx context.Context) error {
	logger := d.baseFSMInstance.GetLogger()
	logger.Infof("Starting Action: Removing DataFlowComponent %s from Benthos config...", d.Config.Name)

	logger.Debugf("Calling BenthosConfigManager.RemoveComponentFromBenthosConfig for %s", d.Config.Name)
	err := d.BenthosConfigManager.RemoveComponentFromBenthosConfig(ctx, d.Config.Name)
	if err != nil {
		logger.Errorf("Failed to remove data flow component %s from benthos config: %v", d.Config.Name, err)
		return fmt.Errorf("failed to remove data flow component %s from benthos config: %w", d.Config.Name, err)
	}

	logger.Infof("DataFlowComponent %s successfully removed from Benthos config", d.Config.Name)
	// Update observed state to reflect the configuration no longer exists
	d.ObservedState.ConfigExists = false
	d.ObservedState.LastConfigUpdateSuccessful = true
	d.ObservedState.LastError = ""

	return nil
}

// initiateUpdateComponentInBenthosConfig updates the data flow component in the benthos config
func (d *DataFlowComponent) initiateUpdateComponentInBenthosConfig(ctx context.Context) error {
	logger := d.baseFSMInstance.GetLogger()
	logger.Infof("Starting Action: Updating DataFlowComponent %s in Benthos config...", d.Config.Name)
	logger.Debugf("DataFlowComponent details: Name=%s, State=%s, UUID=%s",
		d.Config.Name, d.Config.DesiredState, d.Config.VersionUUID)

	logger.Debugf("Calling BenthosConfigManager.UpdateComponentInBenthosConfig for %s", d.Config.Name)
	err := d.BenthosConfigManager.UpdateComponentInBenthosConfig(ctx, d.Config)
	if err != nil {
		logger.Errorf("Failed to update data flow component %s in benthos config: %v", d.Config.Name, err)
		return fmt.Errorf("failed to update data flow component %s in benthos config: %w", d.Config.Name, err)
	}

	logger.Infof("DataFlowComponent %s successfully updated in Benthos config", d.Config.Name)
	// Update observed state to reflect the configuration exists and was updated
	d.ObservedState.ConfigExists = true
	d.ObservedState.LastConfigUpdateSuccessful = true
	d.ObservedState.LastError = ""

	return nil
}

// checkComponentExistsInBenthosConfig checks if the data flow component exists in the benthos config
func (d *DataFlowComponent) checkComponentExistsInBenthosConfig(ctx context.Context) (bool, error) {
	d.baseFSMInstance.GetLogger().Debugf("Checking if DataFlowComponent %s exists in Benthos config...", d.Config.Name)

	exists, err := d.BenthosConfigManager.ComponentExistsInBenthosConfig(ctx, d.Config.Name)
	if err != nil {
		return false, fmt.Errorf("failed to check if data flow component %s exists in benthos config: %w", d.Config.Name, err)
	}

	return exists, nil
}

// updateObservedState updates the observed state of the component by checking if it exists in the benthos config
func (d *DataFlowComponent) updateObservedState(ctx context.Context) error {
	logger := d.baseFSMInstance.GetLogger()
	logger.Debugf("Updating observed state for DataFlowComponent %s", d.Config.Name)

	logger.Debugf("Checking if component %s exists in Benthos config", d.Config.Name)
	exists, err := d.BenthosConfigManager.ComponentExistsInBenthosConfig(ctx, d.Config.Name)
	if err != nil {
		logger.Errorf("Failed to check if component %s exists in benthos config: %v", d.Config.Name, err)
		d.ObservedState.LastError = err.Error()
		d.ObservedState.LastConfigUpdateSuccessful = false
		return fmt.Errorf("failed to check if data flow component %s exists in benthos config: %w", d.Config.Name, err)
	}

	previousExists := d.ObservedState.ConfigExists
	d.ObservedState.ConfigExists = exists
	if previousExists != exists {
		logger.Infof("Component %s exists state changed: %v -> %v", d.Config.Name, previousExists, exists)
	} else {
		logger.Debugf("Component %s exists state unchanged: %v", d.Config.Name, exists)
	}

	return nil
}

// PrintState prints the current state of the DFC
func (d *DataFlowComponent) PrintState() {
	logger := d.baseFSMInstance.GetLogger()
	logger.Infof("DataFlowComponent %s: CurrentState=%s, DesiredState=%s, ConfigExists=%v, LastUpdateSuccessful=%v",
		d.Config.Name,
		d.GetCurrentFSMState(),
		d.GetDesiredFSMState(),
		d.ObservedState.ConfigExists,
		d.ObservedState.LastConfigUpdateSuccessful)

	if d.ObservedState.LastError != "" {
		logger.Infof("DataFlowComponent %s: LastError=%s", d.Config.Name, d.ObservedState.LastError)
	}
}
