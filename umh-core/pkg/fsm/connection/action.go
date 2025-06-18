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

package connection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the Connection's Nmap manager might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting error state and scheduling a retry/backoff.

// CreateInstance attempts to add the Connection to the Nmap manager.
func (c *ConnectionInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	c.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Connection service %s to Nmap manager ...", c.baseFSMInstance.GetID())

	err := c.service.AddConnectionToNmapManager(ctx, filesystemService, &c.config, c.baseFSMInstance.GetID())
	if err != nil {
		if errors.Is(err, connection.ErrServiceAlreadyExists) {
			c.baseFSMInstance.GetLogger().Debugf("Connection service %s already exists in Nmap manager", c.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add Connection service %s to Nmap manager: %w", c.baseFSMInstance.GetID(), err)
	}

	c.baseFSMInstance.GetLogger().Debugf("Connection service %s added to Nmap manager", c.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance attempts to remove the Connection from the Nmap manager.
// It requires the service to be stopped before removal.
func (c *ConnectionInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	c.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Connection service %s from Nmap manager ...", c.baseFSMInstance.GetID())

	// Remove the initiateConnection from the Nmap manager
	err := c.service.RemoveConnectionFromNmapManager(ctx, filesystemService, c.baseFSMInstance.GetID())
	switch {
	case err == nil: // fully removed
		c.baseFSMInstance.GetLogger().Debugf("Connection service %s removed from nmap manager", c.baseFSMInstance.GetID())
		return nil

	case errors.Is(err, connection.ErrServiceNotExist):
		c.baseFSMInstance.GetLogger().Debugf("Connection service %s already removed from nmap manager", c.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	case errors.Is(err, standarderrors.ErrRemovalPending):
		c.baseFSMInstance.GetLogger().Debugf("Connection service %s removal still in progress", c.baseFSMInstance.GetID())
		// not an error from the FSM's perspective - just means "try again"
		return err
	default:
		return fmt.Errorf("failed to remove the service %s: %w", c.baseFSMInstance.GetID(), err)
	}
}

// StartInstance to start the Connection by setting the desired state to running for the given instance
func (c *ConnectionInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	c.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Connection service %s ...", c.baseFSMInstance.GetID())

	// Set the desired state to running for the given instance
	err := c.service.StartConnection(ctx, filesystemService, c.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Connection service %s: %w", c.baseFSMInstance.GetID(), err)
	}

	c.baseFSMInstance.GetLogger().Debugf("Connection service %s start command executed", c.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the Connection by setting the desired state to stopped for the given instance
func (c *ConnectionInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	c.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Connection service %s ...", c.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := c.service.StopConnection(ctx, filesystemService, c.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Connection service %s: %w", c.baseFSMInstance.GetID(), err)
	}

	c.baseFSMInstance.GetLogger().Debugf("Connection service %s stop command executed", c.baseFSMInstance.GetID())
	return nil
}

// CheckForCreation checks whether the creation was successful
// For Connection, this is a no-op as we don't need to check anything
func (c *ConnectionInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// getServiceStatus gets the status of the Connection service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running
func (c *ConnectionInstance) getServiceStatus(ctx context.Context, filesystemService filesystem.Service, tick uint64) (connection.ServiceInfo, error) {
	info, err := c.service.Status(ctx, filesystemService, c.baseFSMInstance.GetID(), tick)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases

		if errors.Is(err, connection.ErrServiceNotExist) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if c.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				c.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return connection.ServiceInfo{}, connection.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			c.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")
			return connection.ServiceInfo{}, nil
		}

		// For other errors, log them and return
		c.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", c.baseFSMInstance.GetID(), err)
		infoWithFailedHealthChecks := info
		infoWithFailedHealthChecks.NmapObservedState.ServiceInfo.NmapStatus.IsRunning = false
		// return the info with healthchecks failed
		return infoWithFailedHealthChecks, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (c *ConnectionInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	info, err := c.getServiceStatus(ctx, services.GetFileSystem(), snapshot.Tick)
	if err != nil {
		return fmt.Errorf("error while getting service status: %w", err)
	}
	metrics.ObserveReconcileTime(logger.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	c.ObservedState.ServiceInfo = info

	currentState := c.baseFSMInstance.GetCurrentFSMState()
	desiredState := c.baseFSMInstance.GetDesiredFSMState()
	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	// Fetch the actual Nmap config from the service
	start = time.Now()
	observedConfig, err := c.service.GetConfig(ctx, services.GetFileSystem(), c.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentConnectionInstance, c.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		c.ObservedState.ObservedConnectionConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), connection.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			c.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed Connection config: %w", err)
		}
	}

	if !connectionserviceconfig.ConfigsEqual(c.config, c.ObservedState.ObservedConnectionConfig) {
		// Check if the service exists before attempting to update
		if c.service.ServiceExists(ctx, services.GetFileSystem(), c.baseFSMInstance.GetID()) {
			c.baseFSMInstance.GetLogger().Debugf("Observed Connection config is different from desired config, updating Nmap configuration")

			diffStr := connectionserviceconfig.ConfigDiff(c.config, c.ObservedState.ObservedConnectionConfig)
			c.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the Nmap manager
			err := c.service.UpdateConnectionInNmapManager(ctx, services.GetFileSystem(), &c.config, c.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update Connection service configuration: %w", err)
			}
		} else {
			c.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// IsConnectionNmapRunning determines if the Connection's Nmap FSM is in running state.
// Architecture Decision: We intentionally rely only on the FSM state, not the underlying
// service implementation details. This maintains a clean separation of concerns where:
// 1. The FSM is the source of truth for service state
// 2. We trust the FSM's state management completely
// 3. Implementation details of how Nmap determines running state are encapsulated away
//
// Note: This function requires the NmapFSMState to be updated in the ObservedState.
func (c *ConnectionInstance) IsConnectionNmapRunning() bool {
	return nmap.IsRunningState(c.ObservedState.ServiceInfo.NmapFSMState)
}

// IsConnectionNmapStopped determines if the Connection's Nmap FSM is in the stopped state.
// Note: This function requires the NmapFSMState to be updated in the ObservedState.
func (c *ConnectionInstance) IsConnectionNmapStopped() bool {
	return c.ObservedState.ServiceInfo.NmapFSMState == nmap.OperationalStateStopped
}

// IsConnectionDegraded determines if the Connection service is degraded.
// These check everything that is checked during the starting phase
// But it means that it once worked, and then degraded
func (c *ConnectionInstance) IsConnectionNmapDegraded() bool {
	return c.ObservedState.ServiceInfo.NmapFSMState == nmap.OperationalStateDegraded
}

// IsConnectionUp returns the value of IsConnectionFlaky and the OperationalStateOpen
// of the Nmap FSM, which then is considered as a healthy connection.
func (c *ConnectionInstance) IsConnectionNmapUp() bool {
	return !c.IsConnectionFlaky() &&
		c.ObservedState.ServiceInfo.NmapFSMState == nmap.OperationalStateOpen
}

// IsConnectionDown determines based on the state of the Nmap FSM if the connection
// can be considered as down due to the port scan
func (c *ConnectionInstance) IsConnectionNmapDown() bool {
	switch c.ObservedState.ServiceInfo.NmapFSMState {
	case nmap.OperationalStateOpenFiltered,
		nmap.OperationalStateClosedFiltered,
		nmap.OperationalStateClosed,
		nmap.OperationalStateFiltered,
		nmap.OperationalStateUnfiltered:
		return true
	default:
		return false
	}
}

// IsConnectionFlaky returns the value of IsFlaky so whether the Nmap-FSM can
// be considered as flaky if it has 3 different states in a short amount of time.
func (c *ConnectionInstance) IsConnectionFlaky() bool {
	return c.ObservedState.ServiceInfo.IsFlaky
}

// IsConnectionDegraded just wraps IsConnectionFlaky and IsConnectionNmapDegraded
// for better readability in reconcile-transitions.
func (c *ConnectionInstance) IsConnectionDegraded() bool {
	if c.IsConnectionFlaky() {
		return true
	}
	if c.IsConnectionNmapDegraded() {
		return true
	}

	return false
}
