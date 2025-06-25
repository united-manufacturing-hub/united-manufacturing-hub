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
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
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

	// Generate the benthos config from the topic browser config
	benthosConfig, err := i.service.GenerateConfig(i.baseFSMInstance.GetID())
	if err != nil {
		return fmt.Errorf("failed to generate benthos config for Topic Browser service %s: %w", i.baseFSMInstance.GetID(), err)
	}

	err = i.service.AddToManager(ctx, filesystemService, &benthosConfig, constants.TopicBrowserServiceName)
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
func (i *Instance) getServiceStatus(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) (tbsvc.ServiceInfo, error) {
	info, err := i.service.Status(ctx, services, i.baseFSMInstance.GetID(), snapshot)
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
	info, err := i.getServiceStatus(ctx, services, snapshot)
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

	// Check if the service exists and if we need to update the configuration
	if i.service.ServiceExists(ctx, services, i.baseFSMInstance.GetID()) {
		// Generate the benthos config from the current topic browser config
		benthosConfig, err := i.service.GenerateConfig(i.baseFSMInstance.GetID())
		if err != nil {
			return fmt.Errorf("failed to generate benthos config for Topic Browser service %s: %w", i.baseFSMInstance.GetID(), err)
		}

		// Update the config in the S6 manager
		err = i.service.UpdateInManager(ctx, services.GetFileSystem(), &benthosConfig, constants.TopicBrowserServiceName)
		if err != nil {
			return fmt.Errorf("failed to update Topic Browser service configuration: %w", err)
		}

		i.baseFSMInstance.GetLogger().Debugf("Topic Browser service configuration updated")
	} else {
		i.baseFSMInstance.GetLogger().Debugf("Service does not exist yet, skipping configuration update")
	}

	return nil
}

// isTopicBrowserHealthy checks if the Topic Browser service is healthy based on ServiceInfo
// This leverages the existing service health analysis instead of reimplementing it
func (i *Instance) isTopicBrowserHealthy() (bool, string) {
	// Use the service's existing health analysis
	serviceInfo := i.ObservedState.ServiceInfo

	// If there's a specific status reason indicating issues, it's not healthy
	if serviceInfo.StatusReason != "" {
		return false, "unknown status reason"
	}

	// Check if the underlying Benthos FSM is in a healthy state
	if serviceInfo.BenthosFSMState != "active" {
		return false, "benthos fsm not active"
	}

	// Check if Redpanda FSM is in a healthy state
	if serviceInfo.RedpandaFSMState != "active" {
		return false, "redpanda fsm not active"
	}

	// Additional basic health checks from Benthos observed state
	benthosObservedState := serviceInfo.BenthosObservedState
	if benthosObservedState.ServiceInfo.S6FSMState != "running" && benthosObservedState.ServiceInfo.S6FSMState != "active" {
		return false, "benthos s6 fsm not running"
	}

	// Check Benthos health checks
	healthCheck := benthosObservedState.ServiceInfo.BenthosStatus.HealthCheck
	if !healthCheck.IsLive || !healthCheck.IsReady {
		return false, "benthos health check not live or ready"
	}

	return true, ""
}

// isTopicBrowserDegraded determines if the Topic Browser should be considered degraded
// This leverages the service's sophisticated cross-component analysis
func (i *Instance) isTopicBrowserDegraded() (bool, string) {
	serviceInfo := i.ObservedState.ServiceInfo

	// If the service has provided a specific status reason, use that
	if serviceInfo.StatusReason != "" {
		return true, serviceInfo.StatusReason
	}

	// If there's processing activity but health checks fail, it's degraded
	healthy, reason := i.isTopicBrowserHealthy()
	if serviceInfo.HasProcessingActivity && !healthy {
		return true, fmt.Sprintf("has processing activity but health checks failing: %s", reason)
	}

	// Check for restart events in S6 service
	s6ObservedState := serviceInfo.BenthosObservedState.ServiceInfo.S6ObservedState
	if len(s6ObservedState.ServiceInfo.ExitHistory) > 0 {
		return true, fmt.Sprintf("service has %d restart events", len(s6ObservedState.ServiceInfo.ExitHistory))
	}

	return false, "service appears healthy"
}

// isTopicBrowserStopped determines if the Topic Browser is stopped
func (i *Instance) isTopicBrowserStopped() (bool, string) {
	if i.ObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateStopped {
		return true, ""
	}

	return false, fmt.Sprintf("service is not in %s state", s6fsm.OperationalStateStopped)
}

// shouldRecoverFromDegraded determines if the Topic Browser should recover from degraded state
func (i *Instance) shouldRecoverFromDegraded() bool {
	// If the service is now healthy and there are no status reasons indicating issues
	healthy, _ := i.isTopicBrowserHealthy()
	return healthy
}
