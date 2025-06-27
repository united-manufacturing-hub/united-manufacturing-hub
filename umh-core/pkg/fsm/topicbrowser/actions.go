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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	rpfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	logger "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	tbsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// CreateInstance attempts to add the Topic Browser to the manager.
func (i *TopicBrowserInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Topic Browser service %s to S6 manager ...", i.baseFSMInstance.GetID())

	// Generate the benthos config from the topic browser config
	// Since it is static, we can just generate it here
	benthosConfig, err := i.service.GenerateConfig(i.baseFSMInstance.GetID())
	if err != nil {
		return fmt.Errorf("failed to generate benthos config for Topic Browser service %s: %w", i.baseFSMInstance.GetID(), err)
	}

	err = i.service.AddToManager(ctx, filesystemService, &benthosConfig, i.baseFSMInstance.GetID())
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

// RemoveInstance attempts to remove the DataflowComponent from the Benthos manager.
// It requires the service to be stopped before removal.
func (i *TopicBrowserInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Topic Browser service %s from Benthos manager ...", i.baseFSMInstance.GetID())

	// Remove the initiateDataflowComponent from the Benthos manager
	err := i.service.RemoveFromManager(ctx, filesystemService, i.baseFSMInstance.GetID())
	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		i.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s removed from S6 manager",
				i.baseFSMInstance.GetID())
		return nil

	case errors.Is(err, tbsvc.ErrServiceNotExist):
		i.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s already removed from S6 manager",
				i.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		i.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s removal still in progress",
				i.baseFSMInstance.GetID())
		// not an error from the FSM’s perspective – just means “try again”
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		return fmt.Errorf("failed to remove service %s: %w",
			i.baseFSMInstance.GetID(), err)
	}
}

// StartInstance attempts to start the topic browser by setting the desired state to running for the given instance
func (i *TopicBrowserInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
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
func (i *TopicBrowserInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
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
func (i *TopicBrowserInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// getServiceStatus gets the status of the Topic Browser service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running
func (i *TopicBrowserInstance) getServiceStatus(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) (tbsvc.ServiceInfo, error) {
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
		infoWithFailedHealthChecks := info
		infoWithFailedHealthChecks.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive = false
		infoWithFailedHealthChecks.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady = false
		// return the info with healthchecks failed
		return infoWithFailedHealthChecks, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (i *TopicBrowserInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	info, err := i.getServiceStatus(ctx, services, snapshot)
	if err != nil {
		return fmt.Errorf("error while getting service status: %w", err)
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
	// Fetch the actual Benthos config from the service
	start = time.Now()
	observedConfig, err := i.service.GetConfig(ctx, services.GetFileSystem(), i.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentTopicBrowserInstance, i.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		i.ObservedState.ObservedServiceConfig.BenthosConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), tbsvc.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			i.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed TopicBrowser config: %w", err)
		}
	}

	if !topicbrowserserviceconfig.ConfigsEqual(i.config, i.ObservedState.ObservedServiceConfig) {
		// Check if the service exists before attempting to update
		if i.service.ServiceExists(ctx, services, i.baseFSMInstance.GetID()) {
			i.baseFSMInstance.GetLogger().Debugf("Observed TopicBrowser config is different from desired config, updating Benthos configuration")

			diffStr := topicbrowserserviceconfig.ConfigDiff(i.config, i.ObservedState.ObservedServiceConfig)
			i.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the Benthos manager
			err := i.service.UpdateInManager(ctx, services.GetFileSystem(), &i.ObservedState.ObservedServiceConfig.BenthosConfig, i.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update TopicBrowser service configuration: %w", err)
			}
		} else {
			i.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// isTopicBrowserHealthy checks if the Topic Browser service is healthy based on ServiceInfo
// This leverages the existing service health analysis instead of reimplementing it
func (i *TopicBrowserInstance) isTopicBrowserHealthy() (bool, string) {
	// Use the service's existing health analysis
	serviceInfo := i.ObservedState.ServiceInfo

	// Check if the underlying Benthos FSM is in a healthy state
	if serviceInfo.BenthosFSMState != benthosfsm.OperationalStateActive && serviceInfo.BenthosFSMState != benthosfsm.OperationalStateIdle {
		return false, "benthos fsm not active or idle"
	}

	// Check if Redpanda FSM is in a healthy state
	if serviceInfo.RedpandaFSMState != rpfsm.OperationalStateActive && serviceInfo.RedpandaFSMState != rpfsm.OperationalStateIdle {
		return false, "redpanda fsm not active or idle"
	}

	return true, ""
}

// IsTopicBrowserBenthosStopped checks if the Topic Browser service is stopped
func (i *TopicBrowserInstance) IsTopicBrowserBenthosStopped() bool {
	return i.ObservedState.ServiceInfo.BenthosFSMState == benthosfsm.OperationalStateStopped
}

// IsTopicBrowserDegraded determines if the Topic Browser should be considered degraded
// This leverages the service's sophisticated cross-component analysis
func (i *TopicBrowserInstance) IsTopicBrowserDegraded() (isDegraded bool, reason string) {
	defer func() {
		i.baseFSMInstance.GetLogger().Debugf("isTopicBrowserDegraded: %t, reason: %s", isDegraded, reason)
	}()

	serviceInfo := i.ObservedState.ServiceInfo

	// If the service has provided a specific status reason, use that
	if serviceInfo.StatusReason != "" {
		return true, serviceInfo.StatusReason
	}

	// If there's processing activity but health checks fail, it's degraded
	healthy, reason := i.isTopicBrowserHealthy()
	if serviceInfo.InvalidMetrics || !healthy {
		return true, fmt.Sprintf("has processing activity but health checks failing: %s", reason)
	}

	// Check for restart events in S6 service
	s6ObservedState := serviceInfo.BenthosObservedState.ServiceInfo.S6ObservedState
	if len(s6ObservedState.ServiceInfo.ExitHistory) > 0 {
		// Check if those where recent (within the last hour)
		var hasRecentRestart bool
		var lastRestartTime time.Time
		for _, exitEvent := range s6ObservedState.ServiceInfo.ExitHistory {
			if exitEvent.Timestamp.After(time.Now().Add(-1 * time.Hour)) {
				hasRecentRestart = true
				lastRestartTime = exitEvent.Timestamp
				break
			}
		}
		if hasRecentRestart {
			return true, fmt.Sprintf("service has %d restart events in the last hour, last restart at %s", len(s6ObservedState.ServiceInfo.ExitHistory), lastRestartTime)
		}
	}

	return false, "service appears healthy"
}

// isBenthosRunning checks if the Benthos service is running
func (i *TopicBrowserInstance) isBenthosRunning() bool {
	switch i.ObservedState.ServiceInfo.BenthosFSMState {
	case benthosfsm.OperationalStateActive, benthosfsm.OperationalStateIdle:
		return true
	}
	return false
}

// isRedpandaRunning checks if the Redpanda service is running
func (i *TopicBrowserInstance) isRedpandaRunning() bool {
	switch i.ObservedState.ServiceInfo.RedpandaFSMState {
	case rpfsm.OperationalStateActive, rpfsm.OperationalStateIdle:
		return true
	}
	return false
}

// getRedpandaStatusReason returns the status reason for Redpanda
func (i *TopicBrowserInstance) getRedpandaStatusReason() string {
	// Since RedpandaStatus doesn't have a StatusReason field, we can use the FSM state
	return string(i.ObservedState.ServiceInfo.RedpandaFSMState)
}

// shouldTransitionToIdle determines if the service should transition to idle state
func (i *TopicBrowserInstance) shouldTransitionToIdle() bool {
	return !i.hasDataActivity()
}

// hasDataActivity checks if there's ongoing data activity
func (i *TopicBrowserInstance) hasDataActivity() bool {
	return i.ObservedState.ServiceInfo.BenthosProcessing && i.ObservedState.ServiceInfo.RedpandaProcessing
}
