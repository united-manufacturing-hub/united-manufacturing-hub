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
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	nmap_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the Nmap FSM might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting error state and scheduling a retry/backoff.

// CreateInstance is called when the FSM transitions from to_be_created -> creating.
func (n *NmapInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".initiateS6Create", time.Since(start))
	}()

	n.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Nmap service %s to S6 manager ...", n.baseFSMInstance.GetID())

	err := n.monitorService.AddNmapToS6Manager(ctx, &n.config.NmapServiceConfig, n.baseFSMInstance.GetID())
	// Create service with custom configuration
	if err != nil {
		if err == nmap_service.ErrServiceAlreadyExists {
			n.baseFSMInstance.GetLogger().Debugf("Nmap service %s already exists in S6 manager", n.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to create service with config for %s: %w", n.baseFSMInstance.GetID(), err)

	}

	n.baseFSMInstance.GetLogger().Debugf("Nmap service %s added to S6 manager", n.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance attempts to remove the Nmap service.
// It requires the service to be stopped before removal.
func (n *NmapInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentNmapInstance, n.baseFSMInstance.GetID()+".initiateS6Remove", time.Since(start))
	}()

	n.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Nmap service %s from S6 manager ...", n.baseFSMInstance.GetID())

	// Initiate the removal cycle
	err := n.monitorService.RemoveNmapFromS6Manager(ctx, n.baseFSMInstance.GetID())
	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		n.baseFSMInstance.GetLogger().
			Debugf("Nmap service %s removed from S6 manager",
				n.baseFSMInstance.GetID())
		return nil

	case errors.Is(err, nmap_service.ErrServiceNotExist):
		n.baseFSMInstance.GetLogger().
			Debugf("Nmap service %s already removed from S6 manager",
				n.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		n.baseFSMInstance.GetLogger().
			Debugf("Nmap service %s removal still in progress",
				n.baseFSMInstance.GetID())
		// not an error from the FSM’s perspective – just means “try again”
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		return fmt.Errorf("failed to remove service %s: %w",
			n.baseFSMInstance.GetID(), err)
	}
}

// StartInstance attempts to start the Nmap by setting the desired state to running for the given instance
func (n *NmapInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	n.baseFSMInstance.GetLogger().Infof("Starting Action: Starting Nmap service %s ...", n.baseFSMInstance.GetID())

	err := n.monitorService.StartNmap(ctx, n.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Nmap service %s: %w", n.baseFSMInstance.GetID(), err)
	}

	n.baseFSMInstance.GetLogger().Debugf("Nmap service %s start command executed", n.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the Nmap by setting the desired state to stopped for the given instance
func (n *NmapInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	n.baseFSMInstance.GetLogger().Infof("Starting Action: Stopping Nmap service %s ...", n.baseFSMInstance.GetID())
	// Set the desired state to stopped for the given instance
	err := n.monitorService.StopNmap(ctx, n.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Nmap service %s: %w", n.baseFSMInstance.GetID(), err)
	}

	n.baseFSMInstance.GetLogger().Debugf("Nmap service %s stop command executed", n.baseFSMInstance.GetID())
	return nil
}

// CheckForCreation checks whether the creation was successful
// For Nmap, this is a no-op as we don't need to check anything
func (n *NmapInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (n *NmapInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// Context deadline exceeded should be retried with backoff, not ignored
			n.baseFSMInstance.SetError(ctx.Err(), snapshot.Tick)
			n.baseFSMInstance.GetLogger().Warnf("Context deadline exceeded in UpdateObservedStateOfInstance, will retry with backoff")
			return nil
		}
		return ctx.Err()
	}

	currentState := n.baseFSMInstance.GetCurrentFSMState()
	desiredState := n.baseFSMInstance.GetDesiredFSMState()
	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	start := time.Now()
	svcInfo, err := n.monitorService.Status(ctx, services.GetFileSystem(), n.config.Name, snapshot.Tick)
	if err != nil {
		if strings.Contains(err.Error(), nmap_service.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when nmap doesn't exist yet
			n.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get nmap metrics: %w", err)
		}
	}
	metrics.ObserveReconcileTime(logger.ComponentNmapInstance, n.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))

	// Stores the raw service info
	n.ObservedState.ServiceInfo = svcInfo

	if n.ObservedState.LastStateChange == 0 {
		n.ObservedState.LastStateChange = time.Now().Unix()
	}

	// Fetch the actual Nmap config from the service
	start = time.Now()
	observedConfig, err := n.monitorService.GetConfig(ctx, services.GetFileSystem(), n.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentNmapInstance, n.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		n.ObservedState.ObservedNmapServiceConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), nmap_service.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			n.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed Nmap config: %w", err)
		}
	}

	// Detect a config change - but let the S6 manager handle the actual reconciliation
	// Use new ConfigsEqual function that handles Nmap defaults properly
	if !n.config.NmapServiceConfig.Equal(n.ObservedState.ObservedNmapServiceConfig) {
		// Check if the service exists before attempting to update
		if n.monitorService.ServiceExists(ctx, services.GetFileSystem(), n.baseFSMInstance.GetID()) {
			n.baseFSMInstance.GetLogger().Debugf("Observed Nmap config is different from desired config, updating S6 configuration")

			// Update the config in the S6 manager
			err := n.monitorService.UpdateNmapInS6Manager(ctx, &n.config.NmapServiceConfig, n.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update Nmap service configuration: %w", err)
			}
		} else {
			n.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// IsNmapRunning is called to check on the state of Nmap
func (n *NmapInstance) IsNmapRunning() bool {
	return n.ObservedState.ServiceInfo.NmapStatus.IsRunning
}
