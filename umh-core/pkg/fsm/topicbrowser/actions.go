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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
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

// IsTopicBrowserS6Running checks if the Topic Browser S6 service is running
func (i *Instance) IsTopicBrowserS6Running() (bool, string) {
	s6State := i.ObservedState.ServiceInfo.S6FSMState
	if s6State == "" {
		return false, "S6 FSM state is empty"
	}

	// Check if S6 service is in a running state
	isRunning := s6State == "running" || s6State == "active"
	if !isRunning {
		return false, fmt.Sprintf("S6 service is in state: %s", s6State)
	}

	return true, "S6 service is running"
}

// IsTopicBrowserS6Stopped checks if the Topic Browser S6 service is stopped
func (i *Instance) IsTopicBrowserS6Stopped() (bool, string) {
	s6State := i.ObservedState.ServiceInfo.S6FSMState
	if s6State == "" {
		return true, "S6 FSM state is empty (considered stopped)"
	}

	isStopped := s6State == "stopped" || s6State == "stopping"
	if !isStopped {
		return false, fmt.Sprintf("S6 service is in state: %s", s6State)
	}

	return true, "S6 service is stopped"
}

// IsTopicBrowserHealthchecksPassed checks if the Topic Browser health checks are passing
func (i *Instance) IsTopicBrowserHealthchecksPassed() (bool, string) {
	healthCheck := i.ObservedState.ServiceInfo.TopicBrowserStatus.HealthCheck

	if !healthCheck.IsLive {
		return false, "liveness check failed"
	}

	if !healthCheck.IsReady {
		return false, "readiness check failed"
	}

	return true, "health checks passed"
}

// AnyRestartsSinceCreation checks if there were any restarts since the service was created
func (i *Instance) AnyRestartsSinceCreation() (bool, string) {
	restartCount := i.ObservedState.ServiceInfo.TopicBrowserStatus.RestartCount

	if restartCount > 0 {
		return true, fmt.Sprintf("service has restarted %d times", restartCount)
	}

	return false, "no restarts detected"
}

// IsTopicBrowserRunningForSomeTimeWithoutErrors checks if the service has been running without errors for a specified time window
func (i *Instance) IsTopicBrowserRunningForSomeTimeWithoutErrors(currentTime time.Time, logWindow time.Duration) (bool, string) {
	// Check if service has been up long enough
	uptime := i.ObservedState.ServiceInfo.TopicBrowserStatus.Uptime
	if uptime < logWindow {
		return false, fmt.Sprintf("service uptime (%v) is less than required window (%v)", uptime, logWindow)
	}

	// Check for errors in logs within the time window
	isLogsFine, reason := i.IsTopicBrowserLogsFine(currentTime, logWindow)
	if !isLogsFine {
		return false, fmt.Sprintf("logs contain errors: %s", reason)
	}

	return true, "service running without errors"
}

// IsTopicBrowserLogsFine checks if the Topic Browser logs are free of errors within a time window
func (i *Instance) IsTopicBrowserLogsFine(currentTime time.Time, logWindow time.Duration) (bool, string) {
	logs := i.ObservedState.ServiceInfo.TopicBrowserStatus.Logs

	// Check for error patterns in recent logs
	hasErrors := i.service.HasErrorsInLogs(logs, currentTime, logWindow)
	if hasErrors {
		return false, "error patterns found in logs"
	}

	return true, "no errors in logs"
}

// IsTopicBrowserMetricsErrorFree checks if the Topic Browser metrics indicate no errors
func (i *Instance) IsTopicBrowserMetricsErrorFree() (bool, string) {
	metrics := i.ObservedState.ServiceInfo.TopicBrowserStatus.Metrics

	if metrics.ErrorCount > 0 {
		return false, fmt.Sprintf("metrics show %d errors", metrics.ErrorCount)
	}

	return true, "metrics show no errors"
}

// IsTopicBrowserDegraded determines if the Topic Browser is in a degraded state
func (i *Instance) IsTopicBrowserDegraded(currentTime time.Time, logWindow time.Duration) (bool, string) {
	// Check if underlying S6 service is not running
	isS6Running, s6Reason := i.IsTopicBrowserS6Running()
	if !isS6Running {
		return true, fmt.Sprintf("underlying S6 service not running: %s", s6Reason)
	}

	// Check health checks
	healthPassed, healthReason := i.IsTopicBrowserHealthchecksPassed()
	if !healthPassed {
		return true, fmt.Sprintf("health checks failed: %s", healthReason)
	}

	// Check for errors in logs
	logsFine, logsReason := i.IsTopicBrowserLogsFine(currentTime, logWindow)
	if !logsFine {
		return true, fmt.Sprintf("logs indicate issues: %s", logsReason)
	}

	// Check metrics for errors
	metricsOk, metricsReason := i.IsTopicBrowserMetricsErrorFree()
	if !metricsOk {
		return true, fmt.Sprintf("metrics indicate errors: %s", metricsReason)
	}

	return false, "service appears healthy"
}

// IsTopicBrowserWithProcessingActivity checks if the Topic Browser is actively processing data
func (i *Instance) IsTopicBrowserWithProcessingActivity() (bool, string) {
	metrics := i.ObservedState.ServiceInfo.TopicBrowserStatus.Metrics

	// Check if there's recent message processing activity
	if metrics.MessagesProcessed > 0 {
		return true, fmt.Sprintf("processed %d messages", metrics.MessagesProcessed)
	}

	// Check if there are active connections/requests
	if metrics.ActiveConnections > 0 {
		return true, fmt.Sprintf("%d active connections", metrics.ActiveConnections)
	}

	return false, "no processing activity detected"
}

// HasRedpandaProcessingActivity checks if the associated Redpanda instance has processing activity
// This uses the Redpanda manager from the system snapshot
func (i *Instance) HasRedpandaProcessingActivity(snapshot fsm.SystemSnapshot) (bool, string) {
	// Find the Redpanda instance using the helper function
	redpandaInstance, found := fsm.FindInstance(snapshot, "redpanda", constants.RedpandaInstanceName)
	if !found {
		return false, fmt.Sprintf("Redpanda instance '%s' not found in snapshot", constants.RedpandaInstanceName)
	}

	// Check the observed state for processing activity indicators
	if redpandaInstance.LastObservedState == nil {
		return false, "Redpanda instance has no observed state"
	}

	// For now, we can check basic indicators from the snapshot
	// The actual processing activity check would need to be implemented
	// when the Redpanda observed state structure is defined
	// This is a placeholder that assumes some activity if the instance is in active state
	if redpandaInstance.CurrentState == "active" {
		return true, "Redpanda instance is in active state"
	}

	return false, "no Redpanda processing activity detected"
}

// ShouldTransitionToDegraded determines if the Topic Browser should transition to degraded state
// This implements the degraded trigger conditions specified by the user
func (i *Instance) ShouldTransitionToDegraded(currentTime time.Time, logWindow time.Duration, snapshot fsm.SystemSnapshot) (bool, string) {
	// Check if underlying S6 service is not running
	isS6Running, s6Reason := i.IsTopicBrowserS6Running()
	if !isS6Running {
		return true, fmt.Sprintf("underlying S6 service not running: %s", s6Reason)
	}

	// Check for Redpanda processing activity mismatch
	hasRedpandaActivity, redpandaReason := i.HasRedpandaProcessingActivity(snapshot)
	hasTopicBrowserActivity, tbReason := i.IsTopicBrowserWithProcessingActivity()

	if hasRedpandaActivity && !hasTopicBrowserActivity {
		return true, fmt.Sprintf("Redpanda has activity (%s) but Topic Browser doesn't (%s)", redpandaReason, tbReason)
	}

	return false, "degraded conditions not met"
}

// ShouldRecoverFromDegraded determines if the Topic Browser should recover from degraded state
func (i *Instance) ShouldRecoverFromDegraded(currentTime time.Time, logWindow time.Duration, snapshot fsm.SystemSnapshot) (bool, string) {
	// Check if underlying S6 service is now running
	isS6Running, s6Reason := i.IsTopicBrowserS6Running()
	if !isS6Running {
		return false, fmt.Sprintf("S6 service still not running: %s", s6Reason)
	}

	// Check if health checks are passing
	healthPassed, healthReason := i.IsTopicBrowserHealthchecksPassed()
	if !healthPassed {
		return false, fmt.Sprintf("health checks still failing: %s", healthReason)
	}

	// Check if service has been running without errors for some time
	runningOk, runningReason := i.IsTopicBrowserRunningForSomeTimeWithoutErrors(currentTime, logWindow)
	if !runningOk {
		return false, fmt.Sprintf("service not stable yet: %s", runningReason)
	}

	// Check if the Redpanda processing activity mismatch is resolved
	hasRedpandaActivity, _ := i.HasRedpandaProcessingActivity(snapshot)
	hasTopicBrowserActivity, _ := i.IsTopicBrowserWithProcessingActivity()

	// If Redpanda has activity, we should also have activity now
	if hasRedpandaActivity && !hasTopicBrowserActivity {
		return false, "Redpanda activity mismatch still exists"
	}

	return true, "all recovery conditions met"
}
