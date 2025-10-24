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

package streamprocessor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dataflowfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
	runtime_config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor/runtime_config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the DataflowComponent's Benthos manager might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting error state and scheduling a retry/backoff.

// CreateInstance registers the protocol-converter with the subordinate
// managers **without** an initial runtime configuration.
//
// Rationale
// ----------
// The full runtime config depends on data that is only available in the
// control-loop's SystemSnapshot (agent location, global vars, node name).
// Rather than widening the BaseFSM callbacks to pass the snapshot, we
// start with an empty config here and perform the real rendering at the
// very beginning of the first Reconcile() tick.
//
// ⚠️  Do **not** assume the underlying Dataflow components are
// already configured when this function returns – they will be updated in
// the next reconciliation cycle.
func (i *Instance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Stream Processor service %s to DFC  manager ...", i.baseFSMInstance.GetID())

	// AddToManager intentionally receives an empty runtime config because template
	// rendering requires SystemSnapshot data not available at creation time.
	// The first UpdateObservedStateOfInstance() call will render and push the real config.
	// At creation time, i.dfcRuntimeConfig will be empty (zero value), which is what we want.
	err := i.service.AddToManager(ctx, filesystemService, &i.dfcRuntimeConfig, i.baseFSMInstance.GetID())
	if err != nil {
		if errors.Is(err, spsvc.ErrServiceAlreadyExists) {
			i.baseFSMInstance.GetLogger().Debugf("Stream Processor service %s already exists in DFC  manager", i.baseFSMInstance.GetID())

			return nil // do not throw an error, as each action is expected to be idempotent
		}

		return fmt.Errorf("failed to add Stream Processor service %s to DFC  manager: %w", i.baseFSMInstance.GetID(), err)
	}

	i.baseFSMInstance.GetLogger().Debugf("Stream Processor service %s added to DFC  manager", i.baseFSMInstance.GetID())

	return nil
}

// RemoveInstance attempts to remove the StreamProcessor from the Benthos and DFC manager.
// It requires the service to be stopped before removal.
func (i *Instance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Stream Processor service %s from DFC  manager ...", i.baseFSMInstance.GetID())

	// Remove the Stream Processor from the DFC manager
	err := i.service.RemoveFromManager(ctx, filesystemService, i.baseFSMInstance.GetID())
	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		i.baseFSMInstance.GetLogger().
			Debugf("StreamProcessor service %s removed from DFC manager",
				i.baseFSMInstance.GetID())

		return nil

	case errors.Is(err, spsvc.ErrServiceNotExist):
		i.baseFSMInstance.GetLogger().
			Debugf("StreamProcessor service %s already removed from DFC manager",
				i.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		i.baseFSMInstance.GetLogger().
			Debugf("StreamProcessor service %s removal still in progress",
				i.baseFSMInstance.GetID())
		// not an error from the FSM's perspective – just means "try again"
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		return fmt.Errorf("failed to remove service %s: %w",
			i.baseFSMInstance.GetID(), err)
	}
}

// StartInstance to start the Stream Processor by setting the desired state to running for the given instance.
func (i *Instance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Stream Processor service %s ...", i.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := i.service.Start(ctx, filesystemService, i.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Stream Processor service %s: %w", i.baseFSMInstance.GetID(), err)
	}

	i.baseFSMInstance.GetLogger().Debugf("Stream Processor service %s start command executed", i.baseFSMInstance.GetID())

	return nil
}

// StopInstance attempts to stop the DataflowComponent by setting the desired state to stopped for the given instance.
func (i *Instance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	i.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Stream Processor service %s ...", i.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := i.service.Stop(ctx, filesystemService, i.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Stream Processor service %s: %w", i.baseFSMInstance.GetID(), err)
	}

	i.baseFSMInstance.GetLogger().Debugf("Stream Processor service %s stop command executed", i.baseFSMInstance.GetID())

	return nil
}

// CheckForCreation checks whether the creation was successful
// For DataflowComponent, this is a no-op as we don't need to check anything.
func (i *Instance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// getServiceStatus gets the status of the StreamProcessor service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running.
func (i *Instance) getServiceStatus(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) (spsvc.ServiceInfo, error) {
	info, err := i.service.Status(ctx, services, snapshot, i.baseFSMInstance.GetID())
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases
		if errors.Is(err, spsvc.ErrServiceNotExist) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if i.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				i.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return spsvc.ServiceInfo{}, spsvc.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			i.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")

			return spsvc.ServiceInfo{}, nil
		}

		// For other errors, log them and return
		i.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", i.baseFSMInstance.GetID(), err)

		infoWithFailedHealthChecks := info

		// Set health flags to false to indicate failure, following the pattern used by other FSMs
		// Only set health checks for components that have them (Benthos components and Redpanda)
		infoWithFailedHealthChecks.DFCObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive = false
		infoWithFailedHealthChecks.DFCObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady = false

		// Set the StatusReason to explain the error
		infoWithFailedHealthChecks.StatusReason = "service status error: " + err.Error()

		// return the info with healthchecks failed
		return infoWithFailedHealthChecks, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service.
func (i *Instance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()

	info, err := i.getServiceStatus(ctx, services, snapshot)
	if err != nil {
		return fmt.Errorf("error while getting service status: %w", err)
	}

	metrics.ObserveReconcileTime(logger.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	i.ObservedState.ServiceInfo = info

	currentState := i.baseFSMInstance.GetCurrentFSMState()
	desiredState := i.baseFSMInstance.GetDesiredFSMState()
	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	// Fetch the actual StreamProcessor config from the service
	start = time.Now()
	observedConfig, err := i.service.GetConfig(ctx, services.GetFileSystem(), i.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".getConfig", time.Since(start))

	if err == nil {
		// Only update if we successfully got the config
		i.ObservedState.ObservedRuntimeConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), spsvc.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			i.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)

			return nil
		} else {
			return fmt.Errorf("failed to get observed StreamProcessor config: %w", err)
		}
	}

	// Store spec config in observed state for reference
	// BuildRuntimeConfig will perform the authoritative location merge
	i.ObservedState.ObservedSpecConfig = i.specConfig

	// Now render the config
	// WARN: TODO
	start = time.Now()

	agentLocationStr := convertIntMapToStringMap(snapshot.CurrentConfig.Agent.Location)

	i.runtimeConfig, i.dfcRuntimeConfig, err = runtime_config.BuildRuntimeConfig(
		i.specConfig,
		agentLocationStr,
		nil,             // TODO: add global vars
		"unimplemented", // TODO: add node name
		i.baseFSMInstance.GetID(),
	)
	if err != nil {
		// Capture the configuration error in StatusReason for troubleshooting
		i.ObservedState.ServiceInfo.StatusReason = "config error: " + err.Error()

		return fmt.Errorf("failed to build runtime config: %w", err)
	}

	metrics.ObserveReconcileTime(logger.ComponentStreamProcessorInstance, i.baseFSMInstance.GetID()+".buildRuntimeConfig", time.Since(start))

	// Compare StreamProcessor configs for detecting changes
	if !streamprocessorserviceconfig.ConfigsEqualRuntime(i.runtimeConfig, i.ObservedState.ObservedRuntimeConfig) {
		// Check if the service exists before attempting to update
		if i.service.ServiceExists(ctx, services.GetFileSystem(), i.baseFSMInstance.GetID()) {
			i.baseFSMInstance.GetLogger().Debugf("Observed StreamProcessor config is different from desired config, updating StreamProcessor configuration")

			diffStr := streamprocessorserviceconfig.ConfigDiffRuntime(i.runtimeConfig, i.ObservedState.ObservedRuntimeConfig)
			i.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the DFC manager with the rendered DFC config
			err := i.service.UpdateInManager(ctx, services.GetFileSystem(), &i.dfcRuntimeConfig, i.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update Stream Processor service configuration: %w", err)
			}

			i.baseFSMInstance.GetLogger().Debugf("config updated")

			// UNIQUE BEHAVIOR: Re-evaluate DFC desired states after config changes
			// This is different from other FSMs which set desired states once and don't change them.
			// Stream processors must re-evaluate because:
			// 1. DFC configs may transition from empty -> populated as templates are rendered
			// 2. Empty DFCs should remain stopped, populated DFCs should be started
			// 3. This ensures we don't start broken Benthos instances with empty configs
			if i.baseFSMInstance.GetDesiredFSMState() == OperationalStateActive {
				i.baseFSMInstance.GetLogger().Debugf("re-evaluating DFC desired states and will be active")

				err := i.service.EvaluateDFCDesiredStates(i.baseFSMInstance.GetID(), "active")
				if err != nil {
					i.baseFSMInstance.GetLogger().Debugf("Failed to re-evaluate DFC states after config update: %v", err)
					// Don't fail the entire update - this is a best-effort re-evaluation
				} else {
					i.baseFSMInstance.GetLogger().Debugf("Re-evaluated DFC desired states after config update")
				}
			}
		} else {
			i.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// convertIntMapToStringMap converts a map[int]string to map[string]string.
func convertIntMapToStringMap(m map[int]string) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[strconv.Itoa(k)] = v
	}

	return result
}

// IsRedpandaHealthy checks whether the underlying redpanda is healthy
// so either idle or active
//
// It returns:
//
//	ok     – true when the redpanda is healthy, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (i *Instance) IsRedpandaHealthy() (bool, string) {
	if i.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle || i.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateActive {
		return true, ""
	}

	statusReason := "redpanda: " + i.ObservedState.ServiceInfo.RedpandaObservedState.ServiceInfo.StatusReason
	if statusReason == "" {
		statusReason = "Redpanda Health status unknown"
	}

	return false, statusReason
}

// IsDFCHealthy checks whether the underlying DFC is healthy
// so either idle or active
//
// It returns:
//
//	ok     – true when the DFC is healthy, false otherwise.
//	reason – empty when ok is true; otherwise a explanation
func (i *Instance) IsDFCHealthy() (bool, string) {
	if i.ObservedState.ServiceInfo.DFCFSMState == dataflowfsm.OperationalStateIdle || i.ObservedState.ServiceInfo.DFCFSMState == dataflowfsm.OperationalStateActive {
		return true, ""
	}

	statusReason := "DFC: " + i.ObservedState.ServiceInfo.DFCObservedState.ServiceInfo.StatusReason
	if statusReason == "" {
		statusReason = "DFC Health status unknown"
	}

	return false, statusReason
}

// safeBenthosMetrics safely extracts Benthos metrics from the observed state,
// returning a zero-value metrics struct if any part of the chain is nil.
// This prevents panics during startup or error conditions when the full
// observedState structure may not be populated yet.
func (i *Instance) safeBenthosMetrics() (input, output struct{ ConnectionUp, ConnectionLost int64 }) {
	// Return zero values if the MetricsState pointer is nil (this is the only field that can actually be nil)
	if i.ObservedState.ServiceInfo.DFCObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState == nil {
		return
	}

	metrics := i.ObservedState.ServiceInfo.DFCObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics

	return struct{ ConnectionUp, ConnectionLost int64 }{
			ConnectionUp:   metrics.Input.ConnectionUp,
			ConnectionLost: metrics.Input.ConnectionLost,
		}, struct{ ConnectionUp, ConnectionLost int64 }{
			ConnectionUp:   metrics.Output.ConnectionUp,
			ConnectionLost: metrics.Output.ConnectionLost,
		}
}

// IsOtherDegraded checks for certain states that should never happen
// and moves the instance into a degraded state if they happen anyway
// Case 1: DFC and redpanda should either be both idle or both active, if they differ (for more than a tick) something must have gone wrong (except that redpanda can be active because of a different DFC)
// Case 2: if redpanda is idle or active, but the DFC has no output active, something must have gone wrong (either redpanda is actually down and not detected, or the DFC is not connecting to Kafka)
//
// It returns:
//
//	ok     – true when there is an issue, false otherwise.
//	reason – empty when ok is true; otherwise a explanation
func (i *Instance) IsOtherDegraded() (bool, string) {
	// TODO: check the write DFC as well

	// Check for case 1.1
	if i.ObservedState.ServiceInfo.DFCFSMState == dataflowfsm.OperationalStateActive &&
		i.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle {
		return true, "DFC is active, but redpanda is idle"
	}

	// Safely extract Benthos metrics to avoid nil pointer panics
	_, outputMetrics := i.safeBenthosMetrics()

	// Check for case 2
	isBenthosOutputActive := outputMetrics.ConnectionUp-outputMetrics.ConnectionLost > 0 // if the amount of connection losts is bigger than the amount of connection ups, the output is not active
	if (i.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle ||
		i.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateActive) &&
		!isBenthosOutputActive {
		return true, fmt.Sprintf("Redpanda is %s, but the DFC has no output active (connection up: %d, connection lost: %d)", i.ObservedState.ServiceInfo.RedpandaFSMState, outputMetrics.ConnectionUp, outputMetrics.ConnectionLost)
	}

	return false, ""
}

// IsDataflowComponentWithProcessingActivity checks whether the DFC has any processing activity
// so whether it is active
//
// It returns:
//
//	ok     – true when the DFC is active, false otherwise.
//	reason – empty when ok is true; otherwise a explanation
func (i *Instance) IsDataflowComponentWithProcessingActivity() (bool, string) {
	// TODO: check the write DFC as well
	if i.ObservedState.ServiceInfo.DFCFSMState == dataflowfsm.OperationalStateActive {
		return true, ""
	}

	dfcState := i.ObservedState.ServiceInfo.DFCFSMState
	if dfcState == "" {
		dfcState = "not existing"
	}

	return false, "DFC is " + dfcState
}

// IsStreamProcessorStopped checks whether the StreamProcessor is stopped
// which means that DFC is stopped
//
// It returns:
//
//	ok     – true when the StreamProcessor is stopped, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation
func (i *Instance) IsStreamProcessorStopped() (bool, string) {
	// TODO: check the write DFC as well
	if i.ObservedState.ServiceInfo.DFCFSMState == dataflowfsm.OperationalStateStopped {
		return true, ""
	}

	dfcState := i.ObservedState.ServiceInfo.DFCFSMState
	if dfcState == "" {
		dfcState = "not existing"
	}

	return false, "DFC is " + dfcState
}

// IsDFCExisting checks whether either the read or write DFC is existing
//
// It returns:
//
//	ok     – true when atleast one DFC is existing, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (i *Instance) IsDFCExisting() (bool, string) {
	if len(i.ObservedState.ServiceInfo.DFCObservedState.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig.Input) > 0 {
		return true, ""
	}

	return false, "no DFCs configured"
}
