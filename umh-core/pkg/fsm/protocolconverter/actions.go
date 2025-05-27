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

package protocolconverter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	connectionfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	dataflowfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
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
// control-loop’s SystemSnapshot (agent location, global vars, node name).
// Rather than widening the BaseFSM callbacks to pass the snapshot, we
// start with an empty config here and perform the real rendering at the
// very beginning of the first Reconcile() tick.
//
// ⚠️  Do **not** assume the underlying Connection / Dataflow components are
// already configured when this function returns – they will be updated in
// the next reconciliation cycle.
func (p *ProtocolConverterInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	p.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding ProtocolConverter service %s to DFC and Connection manager ...", p.baseFSMInstance.GetID())

	// AddToManager intentionally receives an empty runtime config; the
	// first Reconcile() call will render and push the real one.
	err := p.service.AddToManager(ctx, filesystemService, &p.renderedConfig, p.baseFSMInstance.GetID())
	if err != nil {
		if errors.Is(err, protocolconvertersvc.ErrServiceAlreadyExists) {
			p.baseFSMInstance.GetLogger().Debugf("ProtocolConverter service %s already exists in DFC and Connection manager", p.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add ProtocolConverter service %s to DFC and Connection manager: %w", p.baseFSMInstance.GetID(), err)
	}

	p.baseFSMInstance.GetLogger().Debugf("ProtocolConverter service %s added to DFC and Connection manager", p.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance attempts to remove the ProtocolConverter from the Benthos and connection manager.
// It requires the service to be stopped before removal.
func (b *ProtocolConverterInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing ProtocolConverter service %s from DFC and Connection manager ...", b.baseFSMInstance.GetID())

	// Remove the initiateDataflowComponent from the Benthos manager
	err := b.service.RemoveFromManager(ctx, filesystemService, b.baseFSMInstance.GetID())
	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		b.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s removed from S6 manager",
				b.baseFSMInstance.GetID())
		return nil

	case errors.Is(err, protocolconvertersvc.ErrServiceNotExist):
		b.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s already removed from S6 manager",
				b.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		b.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s removal still in progress",
				b.baseFSMInstance.GetID())
		// not an error from the FSM’s perspective – just means “try again”
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		return fmt.Errorf("failed to remove service %s: %w",
			b.baseFSMInstance.GetID(), err)
	}
}

// StartInstance to start the DataflowComponent by setting the desired state to running for the given instance
func (p *ProtocolConverterInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	p.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting ProtocolConverter service %s ...", p.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := p.service.StartProtocolConverter(ctx, filesystemService, p.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start ProtocolConverter service %s: %w", p.baseFSMInstance.GetID(), err)
	}

	p.baseFSMInstance.GetLogger().Debugf("ProtocolConverter service %s start command executed", p.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the DataflowComponent by setting the desired state to stopped for the given instance
func (p *ProtocolConverterInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	p.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping ProtocolConverter service %s ...", p.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := p.service.StopProtocolConverter(ctx, filesystemService, p.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop ProtocolConverter service %s: %w", p.baseFSMInstance.GetID(), err)
	}

	p.baseFSMInstance.GetLogger().Debugf("ProtocolConverter service %s stop command executed", p.baseFSMInstance.GetID())
	return nil
}

// CheckForCreation checks whether the creation was successful
// For DataflowComponent, this is a no-op as we don't need to check anything
func (p *ProtocolConverterInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// getServiceStatus gets the status of the ProtocolConverter service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running
func (p *ProtocolConverterInstance) getServiceStatus(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot, tick uint64) (protocolconvertersvc.ServiceInfo, error) {
	info, err := p.service.Status(ctx, services, snapshot, p.baseFSMInstance.GetID(), tick)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases

		if errors.Is(err, protocolconvertersvc.ErrServiceNotExist) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if p.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				p.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return protocolconvertersvc.ServiceInfo{}, protocolconvertersvc.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			p.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")
			return protocolconvertersvc.ServiceInfo{}, nil
		}

		// For other errors, log them and return
		p.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", p.baseFSMInstance.GetID(), err)
		infoWithFailedHealthChecks := info

		// TODO: set the healthchecks to false

		// return the info with healthchecks failed
		return infoWithFailedHealthChecks, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (p *ProtocolConverterInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot, tick uint64, loopStartTime time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	info, err := p.getServiceStatus(ctx, services, snapshot, tick)
	if err != nil {
		return fmt.Errorf("error while getting service status: %w", err)
	}
	metrics.ObserveReconcileTime(logger.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	p.ObservedState.ServiceInfo = info

	currentState := p.baseFSMInstance.GetCurrentFSMState()
	desiredState := p.baseFSMInstance.GetDesiredFSMState()
	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	// Fetch the actual Benthos config from the service
	start = time.Now()
	observedConfig, err := p.service.GetConfig(ctx, services.GetFileSystem(), p.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		p.ObservedState.ObservedProtocolConverterConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), protocolconvertersvc.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			p.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed ProtocolConverter config: %w", err)
		}
	}

	// Now render the config
	start = time.Now()
	p.renderedConfig, err = runtime_config.BuildRuntimeConfig(
		p.config,
		convertIntMapToStringMap(snapshot.CurrentConfig.Agent.Location),
		nil,       // TODO: add global vars
		"unknown", // TODO: add node name
		p.baseFSMInstance.GetID(),
	)
	if err != nil {
		return fmt.Errorf("failed to build runtime config: %w", err)
	}
	metrics.ObserveReconcileTime(logger.ComponentProtocolConverterInstance, p.baseFSMInstance.GetID()+".buildRuntimeConfig", time.Since(start))

	if !protocolconverterserviceconfig.ConfigsEqualRuntime(p.renderedConfig, p.ObservedState.ObservedProtocolConverterConfig) {
		// Check if the service exists before attempting to update
		if p.service.ServiceExists(ctx, services.GetFileSystem(), p.baseFSMInstance.GetID()) {
			p.baseFSMInstance.GetLogger().Debugf("Observed ProtocolConverter config is different from desired config, updating ProtocolConverter configuration")

			diffStr := protocolconverterserviceconfig.ConfigDiffRuntime(p.renderedConfig, p.ObservedState.ObservedProtocolConverterConfig)
			p.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the Benthos manager
			err := p.service.UpdateInManager(ctx, services.GetFileSystem(), &p.renderedConfig, p.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update ProtocolConverter service configuration: %w", err)
			}
		} else {
			p.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// convertIntMapToStringMap converts a map[int]string to map[string]string
func convertIntMapToStringMap(m map[int]string) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[strconv.Itoa(k)] = v
	}
	return result
}

// IsConnectionUp checks whether the underlying connection is up and running
// and not down or degraded (e.g., because of flakiness)
//
// It returns:
//
//	ok     – true when the connection is up and running, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (p *ProtocolConverterInstance) IsConnectionUp() (bool, string) {
	if p.ObservedState.ServiceInfo.ConnectionFSMState == connectionfsm.OperationalStateUp {
		return true, ""
	}
	return false, fmt.Sprintf("connection is %s", p.ObservedState.ServiceInfo.ConnectionFSMState) // TODO: add flaky status and latency, or alternaitvely status reason
}

// IsRedpandaHealthy checks whether the underlying redpanda is healthy
// so either idle or active
//
// It returns:
//
//	ok     – true when the redpanda is healthy, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (p *ProtocolConverterInstance) IsRedpandaHealthy() (bool, string) {
	if p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle || p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateActive {
		return true, ""
	}
	return false, p.ObservedState.ServiceInfo.RedpandaObservedState.ServiceInfo.StatusReason
}

// IsDFCHealthy checks whether the underlying DFC is healthy
// so either idle or active
//
// It returns:
//
//	ok     – true when the DFC is healthy, false otherwise.
//	reason – empty when ok is true; otherwise a explanation
func (p *ProtocolConverterInstance) IsDFCHealthy() (bool, string) {
	if p.ObservedState.ServiceInfo.DataflowComponentReadFSMState == dataflowfsm.OperationalStateIdle || p.ObservedState.ServiceInfo.DataflowComponentReadFSMState == dataflowfsm.OperationalStateActive {
		return true, ""
	}
	// TODO: check the write DFC as well
	return false, p.ObservedState.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.StatusReason
}

// IsOtherDegraded checks for certain states that should never happen
// and moves the instance into a degraded state if they happen anyway
// Case 1: DFC and redpanda should either be both idle or both active, if they differ (for more than a tick) something must have gone wrong (exept that redpanda can be active because of a different DFC)
// Case 2: if redpanda is idle or active, but the DFC has no output active, something must have gone wrong (either redpanda is actually down and not detected, or the DFC is not connecting to Kafka)
// Case 3: if the connection is down, but the DFC input is active, something must have gone wrong (either the connection is actually down and not detected, or the DFC is not handling it well)
//
// It returns:
//
//	ok     – true when there is an issue, false otherwise.
//	reason – empty when ok is true; otherwise a explanation
func (p *ProtocolConverterInstance) IsOtherDegraded() (bool, string) {

	// TODO: check the write DFC as well

	// Check for case 1.1
	if p.ObservedState.ServiceInfo.DataflowComponentReadFSMState == dataflowfsm.OperationalStateActive &&
		p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle {
		return true, "DFC is active, but redpanda is idle"
	}

	// Check for case 2
	benthosMetrics := p.ObservedState.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.Metrics
	isBenthosOutputActive := benthosMetrics.Output.ConnectionUp-(benthosMetrics.Output.ConnectionLost) > 0 // if the amount of connection losts is bigger than the amount of connection ups, the output is not active
	if (p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateIdle ||
		p.ObservedState.ServiceInfo.RedpandaFSMState == redpandafsm.OperationalStateActive) &&
		!isBenthosOutputActive {
		return true, fmt.Sprintf("Redpanda is %s, but the DFC has no output active (connection up: %d, connection lost: %d)", p.ObservedState.ServiceInfo.RedpandaFSMState, benthosMetrics.Output.ConnectionUp, benthosMetrics.Output.ConnectionLost)
	}

	// Check for case 3
	isBenthosInputActive := benthosMetrics.Input.ConnectionUp-(benthosMetrics.Input.ConnectionLost) > 0 // if the amount of connection losts is bigger than the amount of connection ups, the input is not active
	if p.ObservedState.ServiceInfo.ConnectionFSMState != connectionfsm.OperationalStateUp &&
		isBenthosInputActive {
		return true, fmt.Sprintf("Connection is %s, but the DFC has input active (connection up: %d, connection lost: %d)", p.ObservedState.ServiceInfo.ConnectionFSMState, benthosMetrics.Input.ConnectionUp, benthosMetrics.Input.ConnectionLost)
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
func (p *ProtocolConverterInstance) IsDataflowComponentWithProcessingActivity() (bool, string) {
	// TODO: check the write DFC as well
	if p.ObservedState.ServiceInfo.DataflowComponentReadFSMState == dataflowfsm.OperationalStateActive {
		return true, ""
	}
	return false, fmt.Sprintf("DFC is %s", p.ObservedState.ServiceInfo.DataflowComponentReadFSMState)
}

// IsProtocolConverterStopped checks whether the ProtocolConverter is stopped
// which means that connection and DFC are both stopped
//
// It returns:
//
//	ok     – true when the ProtocolConverter is stopped, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation
func (p *ProtocolConverterInstance) IsProtocolConverterStopped() (bool, string) {
	// TODO: check the write DFC as well
	if p.ObservedState.ServiceInfo.ConnectionFSMState == connectionfsm.OperationalStateStopped &&
		p.ObservedState.ServiceInfo.DataflowComponentReadFSMState == dataflowfsm.OperationalStateStopped {
		return true, ""
	}
	return false, fmt.Sprintf("connection is %s, DFC is %s", p.ObservedState.ServiceInfo.ConnectionFSMState, p.ObservedState.ServiceInfo.DataflowComponentReadFSMState)
}

// IsDFCExisting checks whether either the read or write DFC is existing
//
// It returns:
//
//	ok     – true when atleast one DFC is existing, false otherwise.
//	reason – empty when ok is true; otherwise a service‑provided explanation.
func (p *ProtocolConverterInstance) IsDFCExisting() (bool, string) {
	if len(p.config.Template.DataflowComponentReadServiceConfig.BenthosConfig.Input) > 0 ||
		len(p.config.Template.DataflowComponentWriteServiceConfig.BenthosConfig.Output) > 0 {
		return true, ""
	}
	return false, "no DFCs configured"
}
