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

package topicbrowser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	tbsvccfg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	logger "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	tbsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// CreateInstance attempts to add the Topic Browser to the manager.
func (i *Instance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Topic Browser service %s to S6 manager ...", i.baseFSMInstance.GetID())

	err := i.service.AddToManager(ctx, filesystemService, &i.config, i.baseFSMInstance.GetID())
	if err != nil {
		if err == tbsvc.ErrServiceAlreadyExists {
			i.baseFSMInstance.GetLogger().Debugf("Topic Browser service %s already exists in S6 manager", i.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add Topic Browser service %s to S6 manager: %w", i.baseFSMInstance.GetID(), err)
	}

	i.baseFSMInstance.GetLogger().Debugf("Topic Browser service %s added to S6 manager", i.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance is executed while the Topic Browser FSM sits in the *removing* state.
func (i *Instance) RemoveInstance(
	ctx context.Context,
	filesystemService filesystem.Service,
) error {
	i.baseFSMInstance.GetLogger().
		Infof("Removing Topic Browser service %s from S6 manager …",
			i.baseFSMInstance.GetID())

	err := i.service.RemoveFromManager(ctx, filesystemService, i.baseFSMInstance.GetID())

	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		i.baseFSMInstance.GetLogger().
			Infof("Topic Browser service %s removed from S6 manager",
				i.baseFSMInstance.GetID())
		return nil

	case errors.Is(err, tbsvc.ErrServiceNotExist):
		i.baseFSMInstance.GetLogger().
			Infof("Topic Browser service %s already removed from S6 manager",
				i.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		i.baseFSMInstance.GetLogger().
			Infof("Topic Browser service %s removal still in progress",
				i.baseFSMInstance.GetID())
		// not an error from the FSM's perspective – just means "try again"
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		i.baseFSMInstance.GetLogger().
			Errorf("failed to remove service %s: %s",
				i.baseFSMInstance.GetID(), err)
		return fmt.Errorf("failed to remove service %s: %w",
			i.baseFSMInstance.GetID(), err)
	}
}

// StartInstance attempts to start the topic browser by setting the desired state to running for the given instance
func (i *Instance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Topic Browser service %s ...", i.baseFSMInstance.GetID())

	// Set the desired state to running for the given instance
	err := i.service.Start(ctx, filesystemService, i.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Topic Browser service %s: %w", i.baseFSMInstance.GetID(), err)
	}

	i.baseFSMInstance.GetLogger().Debugf("Topic Browser service %s start command executed", i.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the Topic Browser by setting the desired state to stopped for the given instance
func (i *Instance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Topic Browser service %s ...", i.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := i.service.Stop(ctx, filesystemService, i.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Topic Browser service %s: %w", i.baseFSMInstance.GetID(), err)
	}

	i.baseFSMInstance.GetLogger().Debugf("Topic Browser service %s stop command executed", i.baseFSMInstance.GetID())
	return nil
}

// CheckForCreation checks if the Topic Browser service should be created
// NOTE: check if we really need this or just set true locally
func (i *Instance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// getServiceStatus gets the status of the Topic Browser service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running
func (i *Instance) getServiceStatus(ctx context.Context, services serviceregistry.Provider, tick uint64, loopStartTime time.Time) (tbsvc.ServiceInfo, error) {
	info, err := i.service.Status(ctx, services.GetFileSystem(), i.baseFSMInstance.GetID(), tick)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases

		if errors.Is(err, tbsvc.ErrServiceNotExist) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if i.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				i.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return tbsvc.ServiceInfo{}, tbsvc.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			i.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")
			return tbsvc.ServiceInfo{}, nil
		}

		// For other errors, log them and return
		i.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", i.baseFSMInstance.GetID(), err)
		return tbsvc.ServiceInfo{}, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (i *Instance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	info, err := i.getServiceStatus(ctx, services, snapshot.Tick, snapshot.SnapshotTime)
	if err != nil {
		return err
	}
	metrics.ObserveReconcileTime(logger.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	i.ObservedState.ServiceInfo = info

	currentState := i.baseFSMInstance.GetCurrentFSMState()
	desiredState := i.baseFSMInstance.GetDesiredFSMState()
	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	// Fetch the actual Topic Browser config from the service
	start = time.Now()
	observedConfig, err := i.service.GetConfig(ctx, services.GetFileSystem(), i.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		i.ObservedState.ObservedServiceConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), tbsvc.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			i.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed Topic Browser config: %w", err)
		}
	}

	// Detect a config change - but let the S6 manager handle the actual reconciliation
	// Use new ConfigsEqual function that handles Benthos defaults properly
	if !tbsvccfg.ConfigsEqual(i.config, i.ObservedState.ObservedServiceConfig) {
		// Check if the service exists before attempting to update
		if i.service.ServiceExists(ctx, services.GetFileSystem(), i.baseFSMInstance.GetID()) {
			i.baseFSMInstance.GetLogger().Debugf("Observed Topic Browser config is different from desired config, updating S6 configuration")

			// Use the new ConfigDiff function for better debug output
			diffStr := tbsvccfg.ConfigDiff(i.config, i.ObservedState.ObservedServiceConfig)
			i.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the S6 manager
			err := i.service.UpdateInManager(ctx, services.GetFileSystem(), &i.config, i.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update Topic Browser service configuration: %w", err)
			}
		} else {
			i.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}
