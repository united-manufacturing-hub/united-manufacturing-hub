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

package redpanda

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/monitor"
	redpanda_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"golang.org/x/sync/errgroup"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the Redpanda FSM might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting error state and scheduling a retry/backoff.

// CreateInstance attempts to add the Redpanda to the S6 manager.
func (r *RedpandaInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Redpanda service %s to S6 manager ...", r.baseFSMInstance.GetID())

	err := r.service.AddRedpandaToS6Manager(ctx, &r.config, filesystemService, r.baseFSMInstance.GetID())
	if err != nil {
		if errors.Is(err, redpanda_service.ErrServiceAlreadyExists) {
			r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s already exists in S6 manager", r.baseFSMInstance.GetID())

			return nil // do not throw an error, as each action is expected to be idempotent
		}

		return fmt.Errorf("failed to add Redpanda service %s to S6 manager: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s added to S6 manager", r.baseFSMInstance.GetID())

	return nil
}

// RemoveInstance attempts to remove the Redpanda from the S6 manager.
// It requires the service to be stopped before removal.
func (r *RedpandaInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Redpanda service %s from S6 manager ...", r.baseFSMInstance.GetID())

	// Remove the Redpanda from the S6 manager
	err := r.service.RemoveRedpandaFromS6Manager(ctx, r.baseFSMInstance.GetID())
	if err != nil {
		if errors.Is(err, redpanda_service.ErrServiceNotExist) {
			r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s not found in S6 manager", r.baseFSMInstance.GetID())

			return nil // do not throw an error, as each action is expected to be idempotent
		}

		return fmt.Errorf("failed to remove Redpanda service %s from S6 manager: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s removed from S6 manager", r.baseFSMInstance.GetID())

	return nil
}

// StartInstance attempts to start the redpanda by setting the desired state to running for the given instance.
func (r *RedpandaInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Redpanda service %s ...", r.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := r.service.StartRedpanda(ctx, r.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Redpanda service %s: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s start command executed", r.baseFSMInstance.GetID())

	return nil
}

// StopInstance attempts to stop the Redpanda by setting the desired state to stopped for the given instance.
func (r *RedpandaInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Redpanda service %s ...", r.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := r.service.StopRedpanda(ctx, r.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Redpanda service %s: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s stop command executed", r.baseFSMInstance.GetID())

	return nil
}

// CheckForCreation checks whether the creation was successful
// For Redpanda, this is a no-op as we don't need to check anything.
func (r *RedpandaInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// getServiceStatus gets the status of the Redpanda service
// its main purpose is to habdle the edge cases where the service is not yet created or not yet running.
func (r *RedpandaInstance) GetServiceStatus(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) (redpanda_service.ServiceInfo, error) {
	info, err := r.service.Status(ctx, filesystemService, r.baseFSMInstance.GetID(), tick, loopStartTime)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases
		switch {
		case errors.Is(err, redpanda_service.ErrServiceNotExist):
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if r.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				r.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return redpanda_service.ServiceInfo{}, redpanda_service.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			r.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")

			return redpanda_service.ServiceInfo{}, nil
		case errors.Is(err, redpanda_service.ErrServiceNoLogFile):
			// This is only an error, if redpanda is already running, otherwise there are simply no logs
			// This includes degraded states, as he can only go from active/idle to degraded, and therefore there should be logs
			if IsRunningState(r.baseFSMInstance.GetCurrentFSMState()) {
				r.baseFSMInstance.GetLogger().Debugf("Health check had no logs to process for service %s, returning ServiceInfo with failed health checks", r.baseFSMInstance.GetID())

				infoWithFailedHealthChecks := info
				infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsLive = false
				infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsReady = false
				// Return ServiceInfo with health checks failed but preserve S6FSMState if available
				return infoWithFailedHealthChecks, nil
			} else {
				// We have no running service, therefore we can simply return nil as error
				return info, nil
			}
		case errors.Is(err, monitor.ErrServiceConnectionRefused):
			// If we are in the starting phase or stopped, ..., we can ignore this error, as it is expected
			if !IsRunningState(r.baseFSMInstance.GetCurrentFSMState()) {
				infoWithFailedHealthChecks := info
				infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsLive = false
				infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsReady = false

				return infoWithFailedHealthChecks, nil
			}
		case errors.Is(err, redpanda_service.ErrRedpandaMonitorNotRunning):
			// If the metrics service is not running, we are unable to get the logs/metrics, therefore we must return an empty status
			if !IsRunningState(r.baseFSMInstance.GetCurrentFSMState()) {
				return info, nil
			}
		case errors.Is(err, redpanda_service.ErrLastObservedStateNil):
			// If the last observed state is nil, we can ignore this error
			infoWithFailedHealthChecks := info
			infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsLive = false
			infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsReady = false

			return infoWithFailedHealthChecks, nil
		}

		// For other errors, log them and return
		r.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s (current state: %s, desired state: %s)", r.baseFSMInstance.GetID(), err, r.baseFSMInstance.GetCurrentFSMState(), r.baseFSMInstance.GetDesiredFSMState())

		return redpanda_service.ServiceInfo{}, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service.
//

func (r *RedpandaInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	if ctx.Err() != nil {
		if r.baseFSMInstance.IsDeadlineExceededAndHandle(ctx.Err(), snapshot.Tick, "UpdateObservedStateOfInstance") {
			return nil
		}

		return ctx.Err()
	}

	currentState := r.baseFSMInstance.GetCurrentFSMState()
	desiredState := r.baseFSMInstance.GetDesiredFSMState()

	// Skip health checks if the desired state or current state indicates stopped/stopping
	if desiredState == OperationalStateStopped || currentState == OperationalStateStopped || currentState == OperationalStateStopping {
		// For stopped instances, just check if the S6 service exists but don't do health checks
		// This minimal information is sufficient for reconciliation
		exists := r.service.ServiceExists(ctx, services.GetFileSystem(), r.baseFSMInstance.GetID())
		if !exists {
			// If the service doesn't exist, nothing more to do
			r.PreviousObservedState = RedpandaObservedState{}

			return nil
		}
	}

	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	// Start an errgroup with the **same context** so if one sub-task
	// fails or the context is canceled, all sub-tasks are signaled to stop.
	g, gctx := errgroup.WithContext(ctx)
	observedStateMu := sync.Mutex{}

	g.Go(func() error {
		start := time.Now()
		info, err := r.GetServiceStatus(gctx, services.GetFileSystem(), snapshot.Tick, snapshot.SnapshotTime)
		metrics.ObserveReconcileTime(logger.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))

		switch {
		case err == nil:
			// Store the raw service info
			observedStateMu.Lock()

			r.PreviousObservedState.ServiceInfo = info

			observedStateMu.Unlock()
		case strings.Contains(err.Error(), redpanda_service.ErrServiceNotExist.Error()) ||
			strings.Contains(err.Error(), redpanda_service.ErrServiceNoLogFile.Error()) ||
			strings.Contains(err.Error(), redpanda_service.ErrRedpandaMonitorInstanceNotFound.Error()) ||
			strings.Contains(err.Error(), redpanda_service.ErrLastObservedStateNil.Error()):
			return nil
		case strings.Contains(err.Error(), redpanda_service.ErrRedpandaMonitorNotRunning.Error()):
			// This is expected during the startup phase of the redpanda service, when the redpanda monitor service is not yet running
			r.baseFSMInstance.GetLogger().Debugf("Redpanda monitor service not yet running: %v", err)

			return nil
		}

		return err
	})

	g.Go(func() error {
		start := time.Now()
		// This GetConfig requires the tick parameter, which will be used to calculate the metrics state
		observedConfig, err := r.service.GetConfig(gctx, services.GetFileSystem(), r.baseFSMInstance.GetID(), snapshot.Tick, snapshot.SnapshotTime)
		metrics.ObserveReconcileTime(logger.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".getConfig", time.Since(start))

		if err == nil {
			// Only update if we successfully got the config
			observedStateMu.Lock()

			r.PreviousObservedState.ObservedRedpandaServiceConfig = observedConfig

			observedStateMu.Unlock()

			return nil
		} else {
			if strings.Contains(err.Error(), redpanda_service.ErrServiceNotExist.Error()) ||
				strings.Contains(err.Error(), redpanda_service.ErrServiceNoLogFile.Error()) ||
				strings.Contains(err.Error(), redpanda_service.ErrRedpandaMonitorInstanceNotFound.Error()) ||
				strings.Contains(err.Error(), redpanda_service.ErrLastObservedStateNil.Error()) {
				// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
				// Note: as we use the logs of the underlying redpanda_monitor service, we need to ignore ErrServiceNoLogFile here.
				r.baseFSMInstance.GetLogger().Debugf("Monitor service not found, will be created during reconciliation: %v", err)

				return nil
			}

			if strings.Contains(err.Error(), monitor.ErrServiceConnectionRefused.Error()) {
				// This is expected during the startup phase of the redpanda service, when the service is not yet ready to receive connections
				r.baseFSMInstance.GetLogger().Debugf("Monitor service not yet ready: %v", err)

				return nil
			}

			if strings.Contains(err.Error(), redpanda_service.ErrRedpandaMonitorNotRunning.Error()) {
				// This is expected during the startup phase of the redpanda service, when the redpanda monitor service is not yet running
				r.baseFSMInstance.GetLogger().Debugf("Redpanda monitor service not yet running: %v", err)

				return nil
			}

			return fmt.Errorf("failed to get observed Redpanda config: %w", err)
		}
	})

	// Create a buffered channel to receive the result from g.Wait().
	// The channel is buffered so that the goroutine sending on it doesn't block.
	errc := make(chan error, 1)

	// Run g.Wait() in a separate goroutine.
	// This allows us to use a select statement to return early if the context is canceled.
	go func() {
		// g.Wait() blocks until all goroutines launched with g.Go() have returned.
		// It returns the first non-nil error, if any.
		errc <- g.Wait()
	}()

	// Use a select statement to wait for either the g.Wait() result or the context's cancellation.
	select {
	case err := <-errc:
		// g.Wait() has finished, so check if any goroutine returned an error.
		if err != nil {
			// If there was an error in any sub-call, return that error.
			return err
		}
		// If err is nil, all goroutines completed successfully.
	case <-ctx.Done():
		// The context was canceled or its deadline was exceeded before all goroutines finished.
		// Although some goroutines might still be running in the background,
		// they use a context (gctx) that should cause them to terminate promptly.
		return ctx.Err()
	}

	// If the config could not be fetched, we can't update the S6 configuration
	if r.PreviousObservedState.ObservedRedpandaServiceConfig.Resources.MaxCores == 0 {
		r.baseFSMInstance.GetLogger().Debugf("Observed Redpanda config is not available, skipping update")

		return nil
	}

	// Detect a config change - but let the S6 manager handle the actual reconciliation
	// Use new ConfigsEqual function that handles Redpanda defaults properly
	if !redpandaserviceconfig.ConfigsEqual(r.config, r.PreviousObservedState.ObservedRedpandaServiceConfig) {
		// Check if the service exists before attempting to update
		if r.service.ServiceExists(ctx, services.GetFileSystem(), r.baseFSMInstance.GetID()) {
			r.baseFSMInstance.GetLogger().Debugf("Observed Redpanda config is different from desired config, updating S6 configuration")

			// Use the new ConfigDiff function for better debug output
			diffStr := redpandaserviceconfig.ConfigDiff(r.config, r.PreviousObservedState.ObservedRedpandaServiceConfig)
			r.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the S6 manager
			err := r.service.UpdateRedpandaInS6Manager(ctx, &r.config, r.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update Redpanda service configuration: %w", err)
			}
		} else {
			r.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	// If the config differs and we are currently running,
	// we will apply the changes by doing an REST call to the Redpanda Admin API (https://docs.redpanda.com/api/admin-api/).
	// We also check that the desired state is a running state,
	// preventing issues when redpanda is currently stopping, but the current state is not yet updated
	//
	// Important: This *must* run after the rest of the update state, otherwise we will incorrectly look at an potential old state and infinity loop.
	currentState = r.baseFSMInstance.GetCurrentFSMState()

	desiredState = r.baseFSMInstance.GetDesiredFSMState()
	if IsRunningState(currentState) && IsRunningState(desiredState) {
		// Reconcile the cluster config via HTTP
		// 1. Check if we have changes in the config
		var changes = make(map[string]interface{})

		if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicRetentionBytes != r.config.Topic.DefaultTopicRetentionBytes {
			// https://docs.redpanda.com/current/reference/properties/cluster-properties/#retention_bytes
			if r.config.Topic.DefaultTopicRetentionBytes != 0 {
				changes["retention_bytes"] = r.config.Topic.DefaultTopicRetentionBytes
			} else if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicRetentionBytes != constants.DefaultRedpandaTopicDefaultTopicRetentionBytes {
				// If the config does not set a value, we update as needed
				changes["retention_bytes"] = constants.DefaultRedpandaTopicDefaultTopicRetentionBytes
			}
		}

		if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicRetentionMs != r.config.Topic.DefaultTopicRetentionMs {
			// https://docs.redpanda.com/current/reference/properties/cluster-properties/#log_retention_ms
			if r.config.Topic.DefaultTopicRetentionMs != 0 {
				changes["log_retention_ms"] = r.config.Topic.DefaultTopicRetentionMs
			} else if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicRetentionMs != constants.DefaultRedpandaTopicDefaultTopicRetentionMs {
				// If the config does not set a value, we update as needed
				changes["log_retention_ms"] = constants.DefaultRedpandaTopicDefaultTopicRetentionMs
			}
		}

		if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicCompressionAlgorithm != r.config.Topic.DefaultTopicCompressionAlgorithm {
			// https://docs.redpanda.com/current/reference/properties/cluster-properties/#log_compression_type
			if r.config.Topic.DefaultTopicCompressionAlgorithm != "" {
				changes["log_compression_type"] = r.config.Topic.DefaultTopicCompressionAlgorithm
			} else if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicCompressionAlgorithm != constants.DefaultRedpandaTopicDefaultTopicCompressionAlgorithm {
				// If the config does not set a value, we update as needed
				changes["log_compression_type"] = constants.DefaultRedpandaTopicDefaultTopicCompressionAlgorithm
			}
		}

		if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicCleanupPolicy != r.config.Topic.DefaultTopicCleanupPolicy {
			// https://docs.redpanda.com/current/reference/properties/cluster-properties/#log_cleanup_policy
			if r.config.Topic.DefaultTopicCleanupPolicy != "" {
				changes["log_cleanup_policy"] = r.config.Topic.DefaultTopicCleanupPolicy
			} else if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicCleanupPolicy != constants.DefaultRedpandaTopicDefaultTopicCleanupPolicy {
				// If the config does not set a value, we update as needed
				changes["log_cleanup_policy"] = constants.DefaultRedpandaTopicDefaultTopicCleanupPolicy
			}
		}

		if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicSegmentMs != r.config.Topic.DefaultTopicSegmentMs {
			// https://docs.redpanda.com/current/reference/properties/cluster-properties/#log_segment_ms
			if r.config.Topic.DefaultTopicSegmentMs != 0 {
				changes["log_segment_ms"] = r.config.Topic.DefaultTopicSegmentMs
			} else if r.PreviousObservedState.ObservedRedpandaServiceConfig.Topic.DefaultTopicSegmentMs != constants.DefaultRedpandaTopicDefaultTopicSegmentMs {
				// If the config does not set a value, we update as needed
				changes["log_segment_ms"] = constants.DefaultRedpandaTopicDefaultTopicSegmentMs
			}
		}

		// Only apply if there are changes.
		if len(changes) > 0 {
			err := r.service.UpdateRedpandaClusterConfig(ctx, r.baseFSMInstance.GetID(), changes)
			if err != nil {
				return fmt.Errorf("failed to update Redpanda cluster config: %w", err)
			}

			return nil
		}
	}

	// Reconcile schema registry when Redpanda is running
	// This ensures that JSON schemas derived from DataModels and DataContracts are synchronized with the registry
	if IsRunningState(currentState) && IsRunningState(desiredState) {
		// Get the configuration data from the RedpandaInstance
		// These are set by the RedpandaManager during configuration extraction
		dataModels := r.getDataModels()
		dataContracts := r.getDataContracts()
		payloadShapes := r.getPayloadShapes()

		// Only reconcile if we have data models or contracts to process
		if len(dataModels) > 0 || len(dataContracts) > 0 {
			r.baseFSMInstance.GetLogger().Debugf("Reconciling schema registry with %d data models and %d data contracts", len(dataModels), len(dataContracts))

			err := r.schemaRegistry.Reconcile(ctx, dataModels, dataContracts, payloadShapes)
			if err != nil {
				return fmt.Errorf("failed to reconcile schema registry: %w", err)
			}

			r.baseFSMInstance.GetLogger().Debugf("Schema registry reconciliation completed successfully")
		}
	}

	return nil
}

// IsRedpandaS6Running reports true when the FSM state is
// s6fsm.OperationalStateRunning.
//
// It returns:
//
//	ok     – true when running, false otherwise.
//	reason – empty when ok is true; otherwise a short operator-friendly
//	         explanation.
func (r *RedpandaInstance) IsRedpandaS6Running() (bool, string) {
	if r.PreviousObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateRunning {
		return true, ""
	}

	return false, "s6 is not running, current state: " + r.PreviousObservedState.ServiceInfo.S6FSMState
}

// IsRedpandaS6Stopped reports true when the FSM state is
// s6fsm.OperationalStateStopped.
//
// It returns:
//
//	ok     – true when stopped, false otherwise.
//	reason – empty when ok is true; otherwise a short operator-friendly
//	         explanation.
func (r *RedpandaInstance) IsRedpandaS6Stopped() (bool, string) {
	if r.PreviousObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateStopped {
		return true, ""
	}

	return false, "s6 is not stopped, current state: " + r.PreviousObservedState.ServiceInfo.S6FSMState
}

// IsRedpandaHealthchecksPassed reports true when both liveness and readiness
// probes pass.
//
// It returns:
//
//	ok     – true when probes pass, false otherwise.
//	reason – empty when ok is true; otherwise details of the failed probe(s).
func (r *RedpandaInstance) IsRedpandaHealthchecksPassed() (bool, string) {
	if r.PreviousObservedState.ServiceInfo.RedpandaStatus.HealthCheck.IsLive &&
		r.PreviousObservedState.ServiceInfo.RedpandaStatus.HealthCheck.IsReady {
		return true, ""
	}

	return false, fmt.Sprintf("healthchecks did not pass, live: %t, ready: %t", r.PreviousObservedState.ServiceInfo.RedpandaStatus.HealthCheck.IsLive, r.PreviousObservedState.ServiceInfo.RedpandaStatus.HealthCheck.IsReady)
}

// AnyRestartsSinceCreation reports true when the S6 exit history contains at
// least one entry, indicating Redpanda has restarted since creation.
//
// It returns:
//
//	restarted – true when restarts were observed, false otherwise.
//	reason    – empty when restarted is false; otherwise the restart count.
func (r *RedpandaInstance) AnyRestartsSinceCreation() (bool, string) {
	// We can analyse the S6 ExitHistory to determine if the service has restarted in the last seconds
	// We need to check if any of the exit codes are 0 (which means a restart)
	// and if the time of the restart is within the last seconds
	if len(r.PreviousObservedState.ServiceInfo.S6ObservedState.ServiceInfo.ExitHistory) == 0 {
		return false, ""
	}

	return true, fmt.Sprintf("restarted %d times", len(r.PreviousObservedState.ServiceInfo.S6ObservedState.ServiceInfo.ExitHistory))
}

// AnyUnexpectedRestartsInTheLastHour reports true when the S6 exit history contains at
// least one entry, indicating Redpanda has restarted in the last hour using an exit code other than 0
//
// It returns:
//
//	restarted – true when restarts were observed, false otherwise.
//	reason    – empty when restarted is false; otherwise the restart count.
func (r *RedpandaInstance) AnyUnexpectedRestartsInTheLastHour() (bool, string) {
	// We can analyse the S6 ExitHistory to determine if the service has restarted in the last hour
	// We need to check if any of the exit codes are 0 (which means a restart)
	// and if the time of the restart is within the last hour
	if len(r.PreviousObservedState.ServiceInfo.S6ObservedState.ServiceInfo.ExitHistory) == 0 {
		return false, ""
	}

	// Check if any of the exit codes are 0 (which means a restart)
	// and if the time of the restart is within the last hour
	var restarts []int

	for _, exitCode := range r.PreviousObservedState.ServiceInfo.S6ObservedState.ServiceInfo.ExitHistory {
		if exitCode.ExitCode != 0 && exitCode.Timestamp.After(time.Now().Add(-1*time.Hour)) {
			restarts = append(restarts, exitCode.ExitCode)
		}
	}

	if len(restarts) > 0 {
		return true, fmt.Sprintf("unexpected restarts within the last hour: %v", restarts)
	}

	return false, ""
}

// IsRedpandaRunningWithoutErrors reports true when Redpanda has
// been up for at least ten seconds, recent logs are clean, and metrics show no
// errors.
//
// It returns:
//
//	ok     – true when all conditions pass, false otherwise.
//	reason – empty when ok is true; otherwise the first detected failure.
func (r *RedpandaInstance) IsRedpandaRunningWithoutErrors(currentTime time.Time, logWindow time.Duration) (bool, string) {
	// Check if there are any issues in the Redpanda logs
	logsFine, reason := r.IsRedpandaLogsFine(currentTime, logWindow)
	if !logsFine {
		return false, reason
	}

	// Check if there are any errors in the Redpanda metrics
	metricsFine, reason := r.IsRedpandaMetricsErrorFree()
	if !metricsFine {
		return false, reason
	}

	return true, ""
}

// IsRedpandaLogsFine reports true when recent logs (within logWindow) have no
// critical errors or warnings.
//
// It returns:
//
//	ok     – true when logs look clean, false otherwise.
//	reason – empty when ok is true; otherwise the first offending log line.
func (r *RedpandaInstance) IsRedpandaLogsFine(currentTime time.Time, logWindow time.Duration) (bool, string) {
	logsFine, logEntry := r.service.IsLogsFine(r.PreviousObservedState.ServiceInfo.RedpandaStatus.Logs, currentTime, logWindow, r.transitionToRunningTime)
	if !logsFine {
		return false, fmt.Sprintf("log issue: [%s] %s", logEntry.Timestamp.Format(time.RFC3339), logEntry.Content)
	}

	return true, ""
}

// IsRedpandaMetricsErrorFree proxies service.IsMetricsErrorFree.
//
// It returns:
//
//	ok     – true when metrics are error‑free, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (r *RedpandaInstance) IsRedpandaMetricsErrorFree() (bool, string) {
	return r.service.IsMetricsErrorFree(r.PreviousObservedState.ServiceInfo.RedpandaStatus.RedpandaMetrics.Metrics)
}

// IsRedpandaDegraded reports true when a previously healthy instance has
// degraded (i.e. any of the startup predicates now fail).
//
// It returns:
//
//	degraded – true when degraded, false when still healthy.
//	reason   – empty when degraded is false; otherwise the first failure cause.
func (r *RedpandaInstance) IsRedpandaDegraded(currentTime time.Time, logWindow time.Duration) (bool, string) {
	s6Running, reasonS6Running := r.IsRedpandaS6Running()
	healthchecksPassed, reasonHealthchecksPassed := r.IsRedpandaHealthchecksPassed()
	runningForSomeTimeWithoutErrors, reasonRunningForSomeTimeWithoutErrors := r.IsRedpandaRunningWithoutErrors(currentTime, logWindow)
	unexpectedRestarts, reasonUnexpectedRestarts := r.AnyUnexpectedRestartsInTheLastHour()

	if !s6Running {
		return true, reasonS6Running
	}

	if !healthchecksPassed {
		return true, reasonHealthchecksPassed
	}

	if !runningForSomeTimeWithoutErrors {
		return true, reasonRunningForSomeTimeWithoutErrors
	}

	if unexpectedRestarts {
		return true, reasonUnexpectedRestarts
	}

	return false, ""
}

// IsRedpandaWithProcessingActivity reports true when metrics or other status
// signals indicate active data processing.
//
// It returns:
//
//	ok     – true when activity is detected, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (r *RedpandaInstance) IsRedpandaWithProcessingActivity() (bool, string) {
	if r.PreviousObservedState.ServiceInfo.RedpandaStatus.RedpandaMetrics.MetricsState == nil {
		return false, ""
	}

	hasProcessingActivity, reason := r.service.HasProcessingActivity(r.PreviousObservedState.ServiceInfo.RedpandaStatus)
	if !hasProcessingActivity {
		return false, reason
	}

	return true, ""
}

// IsRedpandaStarted reports true when the success message
// "Successfully started Redpanda!" is present in logs.
//
// It returns:
//
//	ok     – true when the message is found, false otherwise.
//	reason – empty when ok is true; otherwise a brief explanation.
func (r *RedpandaInstance) IsRedpandaStarted(ctx context.Context) (bool, string) {
	if r.PreviousObservedState.ServiceInfo.RedpandaStatus.Logs == nil {
		return false, "no logs found"
	}

	var hasSuccessMessage bool
	// Check if the success message is in the logs
	for _, log := range r.PreviousObservedState.ServiceInfo.RedpandaStatus.Logs {
		if strings.Contains(log.Content, "Successfully started Redpanda!") {
			hasSuccessMessage = true
		}
	}

	// Validate that we can reach the cluster's schema registry (curl localhost:8081/subjects)
	hasSchemaRegistryReachable, schemaRegistryReason := r.IsSchemaRegistryReachable(ctx)

	if !hasSuccessMessage {
		return false, "no success message found in logs"
	}

	if !hasSchemaRegistryReachable {
		return false, "schema registry not reachable: " + schemaRegistryReason
	}

	return true, ""
}

func (r *RedpandaInstance) GetBaseFSMInstanceForTest() *internalfsm.BaseFSMInstance {
	return r.baseFSMInstance
}

func (r *RedpandaInstance) IsSchemaRegistryReachable(ctx context.Context) (bool, string) {
	// Create a short ctx (20ms)
	ctx, cancel := context.WithTimeout(ctx, constants.SchemaRegistryTimeout)
	defer cancel()

	// Try to reach the schema registry (HEAD would be faster, but is buggy (it sends an 0x00 byte at the start of the response))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://localhost:%d/subjects", constants.SchemaRegistryPort), nil)
	if err != nil {
		return false, fmt.Sprintf("failed to create HEAD request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("failed to reach schema registry: %v", err)
	}

	if resp == nil {
		return false, "received nil response from schema registry"
	}

	if resp.Body != nil {
		err = resp.Body.Close()
		if err != nil {
			r.baseFSMInstance.GetLogger().Warnf("failed to close schema registry response body: %v", err)
		}
	}
	// It should return either a 2XX or a 4XX status code
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		r.baseFSMInstance.GetLogger().Debugf("Schema registry reachable")

		return true, ""
	}

	return false, fmt.Sprintf("schema registry returned status code %d", resp.StatusCode)
}
