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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	dfc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
	"go.uber.org/zap"
)

type IStreamProcessorService interface {
	// GetConfig pulls the **actual** runtime configuration that is currently
	// deployed for the given Stream Processor.
	//
	// It does so by:
	//  1. Deriving the names of the underlying resources (DFC).
	//  2. Asking the respective sub-services for their *Runtime* configs.
	//  3. Stitching those parts back together via
	//     `FromDFCServiceConfig`.
	//
	// The resulting **StreamProcessorServiceConfigRuntime** reflects the state
	// the FSM is *really* running with and therefore forms the "live" side of the
	// reconcile equation:
	GetConfig(ctx context.Context, filesystemService filesystem.Service, spName string) (streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime, error)

	// Status aggregates health  DFC and Redpanda into a single
	// snapshot.  The returned structure is **read-only** – callers must not
	// mutate it.
	Status(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot, spName string) (ServiceInfo, error)

	// AddToManager adds a Stream Processor to the  DFC manager
	AddToManager(ctx context.Context, filesystemService filesystem.Service, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, spName string) error

	// UpdateInManager updates an existing Stream Processor in the  DFC manager
	UpdateInManager(ctx context.Context, filesystemService filesystem.Service, cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig, spName string) error

	// RemoveFromManager removes a Stream Processor from the  DFC manager
	RemoveFromManager(ctx context.Context, filesystemService filesystem.Service, spName string) error

	// Start starts a Stream Processor
	Start(ctx context.Context, filesystemService filesystem.Service, spName string) error

	// Stop stops a Stream Processor
	Stop(ctx context.Context, filesystemService filesystem.Service, spName string) error

	// ForceRemove removes a Stream Processor from the  DFC manager
	ForceRemove(ctx context.Context, filesystemService filesystem.Service, spName string) error

	// ServiceExists checks if a connection and a dataflowcomponent with the given name exist.
	// If only one of the services exists, it returns false.
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, spName string) bool

	// ReconcileManager is the heart-beat: on every tick it feeds the *desired*
	// slices into the child-managers, lets them run one state-machine cycle and
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)

	// EvaluateDFCDesiredStates determines the appropriate desired states for the underlying
	// DFCs based on the stream processor's desired state and current DFC configurations.
	// This is used internally for both initial startup and config change re-evaluation.
	EvaluateDFCDesiredStates(spName string, spDesiredState string) error
}

// ServiceInfo holds information about the StreamProcessor underlying health states.
type ServiceInfo struct {
	// DFCObservedState mirrors the DFC manager.
	DFCObservedState dfcfsm.DataflowComponentObservedState
	DFCFSMState      string

	// RedpandaObservedState is included so a stream processor can degrade
	// itself when the message bus is down.
	RedpandaObservedState redpandafsm.RedpandaObservedState
	RedpandaFSMState      string

	// StatusReason is a short human string (log excerpt, metrics finding, …)
	// explaining *why* the converter is not "green".
	StatusReason string
}

// Service implements IStreamProcessorService using it's underlying components
type Service struct {
	logger *zap.SugaredLogger

	// DataflowComponent
	// It has the config for a reading DFC and a writing DFC
	dataflowComponentManager *dfcfsm.DataflowComponentManager
	dataflowComponentService dfc.IDataFlowComponentService
	dataflowComponentConfig  []config.DataFlowComponentConfig
}

// ServiceOption is a function that configures a ConnectionService.
// This follows the functional options pattern for flexible configuration.
type ServiceOption func(*Service)

// WithUnderlyingService sets the underlying services for the stream processor service
// Used for testing purposes
func WithUnderlyingService(
	dfcService dfc.IDataFlowComponentService,
) ServiceOption {
	return func(p *Service) {
		p.dataflowComponentService = dfcService
	}
}

// WithUnderlyingManager sets the underlying managers for the stream processor service
// Used for testing purposes
func WithUnderlyingManager(
	dfcMgr *dfcfsm.DataflowComponentManager,
) ServiceOption {
	return func(c *Service) {
		c.dataflowComponentManager = dfcMgr
	}
}

// NewDefaultService returns a fully initialised service with
// default child managers.  Call-site may inject mocks via functional options.
//
// Note: `spName` is the *logical* name – **without** the "streamprocessor-"
// prefix – exactly as it appears in the UMH YAML.
func NewDefaultService(spName string, opts ...ServiceOption) *Service {
	managerName := fmt.Sprintf("%s%s", logger.ComponentStreamProcessorService, spName)
	service := &Service{
		logger:                   logger.For(managerName),
		dataflowComponentManager: dfcfsm.NewDataflowComponentManager(managerName),
		dataflowComponentService: dfc.NewDefaultDataFlowComponentService(spName),
		dataflowComponentConfig:  []config.DataFlowComponentConfig{},
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getUnderlyingName returns the *base* external name that all child services of
// a Stream Processor share.
func (p *Service) getUnderlyingName(spName string) string {
	return fmt.Sprintf("streamprocessor-%s", spName)
}

// getUnderlyingDFCReadName returns the external name handed to the
// **Data-flow-Component manager** for the  DFC.
func (p *Service) getUnderlyingDFCReadName(spName string) string {
	return fmt.Sprintf("dfc-%s", p.getUnderlyingName(spName))
}

// GetConfig pulls the **actual** runtime configuration that is currently
// deployed for the given Stream Processor.
//
// It does so by:
//  1. Deriving the names of the underlying resources (Connection, read-DFC,
//     write-DFC).
//  2. Asking the respective sub-services for their *Runtime* configs.
//  3. Stitching those parts back together via
//     `FromConnectionAndDFCServiceConfig`.
//
// The resulting **StreamProcessorServiceConfigRuntime** reflects the state
// the FSM is *really* running with and therefore forms the "live" side of the
// reconcile equation:
func (p *Service) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) (streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime, error) {
	if ctx.Err() != nil {
		return streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{}, ctx.Err()
	}

	underlyingDFCName := p.getUnderlyingDFCReadName(spName)

	dfcConfig, err := p.dataflowComponentService.GetConfig(ctx, filesystemService, underlyingDFCName)
	if err != nil {
		return streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{}, fmt.Errorf("failed to get read dataflowcomponent config: %w", err)
	}

	// TODO: From Config
	// actualConfig := protocolconverterserviceconfig.FromConnectionAndDFCServiceConfig(connConfig, dfcConfig, dfcWriteConfig)

	var actualConfig streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime
	// placeholder
	dfcConfig.GetBenthosServiceConfig()
	return actualConfig, nil
}

// Status returns information about the connection health for the specified connection.
// It queries the underlying Connection service and then enhances the result with
// additional context like flakiness detection based on historical data.
func (p *Service) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	snapshot fsm.SystemSnapshot,
	spName string,
) (ServiceInfo, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentStreamProcessorService, spName+".Status", time.Since(start))
	}()

	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}
	if !p.ServiceExists(ctx, services.GetFileSystem(), spName) {
		return ServiceInfo{}, ErrServiceNotExist
	}

	underlyingDFCName := p.getUnderlyingDFCReadName(spName)

	// --- redpanda (only one instance) -------------------------------------------------------------
	rpInst, ok := fsm.FindInstance(snapshot, constants.RedpandaManagerName, constants.RedpandaInstanceName)
	if !ok || rpInst == nil {
		return ServiceInfo{}, fmt.Errorf("redpanda instance not found")
	}

	redpandaStatus := rpInst.LastObservedState

	// redpandaObservedStateSnapshot is slightly different from the others as it comes from the snapshot
	// and not from the fsm-instance
	redpandaObservedStateSnapshot, ok := redpandaStatus.(*redpandafsm.RedpandaObservedStateSnapshot)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("redpanda status for redpanda %s is not a RedpandaObservedStateSnapshot", spName)
	}

	// Now it is in the same format
	redpandaObservedState := redpandafsm.RedpandaObservedState{
		ServiceInfo:                   redpandaObservedStateSnapshot.ServiceInfoSnapshot,
		ObservedRedpandaServiceConfig: redpandaObservedStateSnapshot.Config,
	}

	redpandaFSMState := rpInst.CurrentState

	// -- DFC

	dfcStatus, err := p.dataflowComponentManager.GetLastObservedState(underlyingDFCName)
	dfcExists := err == nil
	var dfcObservedState dfcfsm.DataflowComponentObservedState
	var dfcFSMState string
	if dfcExists {
		dfcObservedState, ok = dfcStatus.(dfcfsm.DataflowComponentObservedState)
		if !ok {
			return ServiceInfo{}, fmt.Errorf("read dataflowcomponent status for dataflowcomponent %s is not a DataflowComponentObservedState", spName)
		}

		dfcFSMState, err = p.dataflowComponentManager.GetCurrentFSMState(underlyingDFCName)
		if err != nil {
			return ServiceInfo{}, fmt.Errorf("failed to get read dataflowcomponent FSM state: %w", err)
		}
	}

	// Error if both DFCs are missing
	if !dfcExists {
		return ServiceInfo{}, fmt.Errorf("dataflowcomponent does not exists for stream processor %s", spName)
	}

	return ServiceInfo{
		DFCObservedState:      dfcObservedState,
		DFCFSMState:           dfcFSMState,
		RedpandaObservedState: redpandaObservedState,
		RedpandaFSMState:      redpandaFSMState,
	}, nil
}

// AddToManager registers a new stream processor in the  DFC service.
func (p *Service) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	spName string,
) error {
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	p.logger.Infof("Adding StreamProcessor %s", spName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	underlyingDFCName := p.getUnderlyingDFCReadName(spName)

	dfcConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingDFCName,
			DesiredFSMState: dfcfsm.OperationalStateStopped,
		},
		DataFlowComponentServiceConfig: *cfg,
	}

	// Add the configurations to the lists
	p.dataflowComponentConfig = append(p.dataflowComponentConfig, dfcConfig)

	p.logger.Infof("Stream Processor %s added to manager", spName)
	p.logger.Infof("Dataflow component config: %+v", p.dataflowComponentConfig)

	return nil
}

// UpdateInManager modifies an existing streamprocessor configuration.
func (p *Service) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	spName string,
) error {
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	if cfg == nil {
		return errors.New("config is nil")
	}

	p.logger.Infof("Updating streamprocessor %s", spName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the dfcconfig exists
	foundDFC := false
	indexDFC := -1
	for i, config := range p.dataflowComponentConfig {
		if config.Name == p.getUnderlyingDFCReadName(spName) {
			foundDFC = true
			indexDFC = i
			break
		}
	}

	// either read or write dfc must exist
	if !foundDFC {
		return ErrServiceNotExist
	}

	// Create a config.DataflowComponentConfig that wraps the DataflowComponentServiceConfig
	if foundDFC {
		dfcCurrentDesiredState := p.dataflowComponentConfig[indexDFC].DesiredFSMState
		p.dataflowComponentConfig[indexDFC] = config.DataFlowComponentConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            p.getUnderlyingDFCReadName(spName),
				DesiredFSMState: dfcCurrentDesiredState,
			},
			DataFlowComponentServiceConfig: *cfg,
		}
	}

	p.logger.Info("Updated streamprocessor config in manager")
	p.logger.Infof("Dataflow component config: %+v", p.dataflowComponentConfig)

	return nil
}

// RemoveFromManager deletes a streamprocessor configuration.
func (p *Service) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) error {
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	p.logger.Infof("Removing dataflowcomponent %s", spName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	dfcName := p.getUnderlyingDFCReadName(spName)

	dfcSliceRemoveByName := func(in []config.DataFlowComponentConfig, name string) []config.DataFlowComponentConfig {
		for i, v := range in {
			if v.Name == name {
				return append(in[:i], in[i+1:]...)
			}
		}
		return in // already gone
	}

	//--------------------------------------------
	// 1) trim desired-state slices (idempotent)
	//--------------------------------------------
	p.dataflowComponentConfig = dfcSliceRemoveByName(p.dataflowComponentConfig, dfcName)

	//--------------------------------------------
	// 2) is the child FSM still alive?
	//--------------------------------------------

	if inst, ok := p.dataflowComponentManager.GetInstance(dfcName); ok {
		return fmt.Errorf("%w: DataflowComponent instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	return nil
}

// EvaluateDFCDesiredStates determines the appropriate desired states for the underlying
// DFCs based on the stream processors's desired state and current DFC configurations.
//
// UNIQUE BEHAVIOR: Unlike other FSMs that make start/stop decisions once during initial
// startup, stream processors must re-evaluate DFC states whenever configs change because:
//
//  1. Stream processors start with template-based configs containing variables like
//     {{ .internal.bridged_by }} and {{ .location.0 }}
//  2. These templates are rendered during reconciliation using runtime data (agent location,
//     node name, etc.) that's not available at creation time
//  3. DFC configs may transition from empty -> populated or populated -> empty as templates
//     are processed, requiring state re-evaluation
//
// Logic:
// - If stream processors desired state is stopped: all DFCs -> stopped
// - If stream processors desired state is active: DFCs -> active only if they have non-empty input configs
//
// Called from:
// 1. Start() - during initial activation
// 2. Stop() - during shutdown
// 3. UpdateObservedStateOfInstance() - when config changes are detected during reconciliation
func (p *Service) EvaluateDFCDesiredStates(
	spName string,
	spDesiredState string,
) error {
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	dfcName := p.getUnderlyingDFCReadName(spName)

	// Find and update our cached config for read DFC
	dfcFound := false
	for i, config := range p.dataflowComponentConfig {
		if config.Name == dfcName {
			if spDesiredState == "stopped" {
				p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			} else {
				// Only start the DFC, if it has been configured
				if len(p.dataflowComponentConfig[i].DataFlowComponentServiceConfig.BenthosConfig.Input) > 0 {
					p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateActive
				} else {
					p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
				}
			}
			dfcFound = true
			break
		}
	}

	if !dfcFound {
		return ErrServiceNotExist
	}

	return nil
}

// Start starts a StreamProcessor by setting connection to "up" and
// evaluating which DFCs should be active based on their current configurations.
//
// UNIQUE BEHAVIOR: Unlike other FSM start methods that simply set desired states to active,
// stream processors must check DFC config content because:
// - DFCs start with template configs that may be empty initially
// - Only DFCs with non-empty input configs should be started
// - Empty DFCs remain stopped to avoid creating broken Benthos instances
//
// This conditional starting is handled by EvaluateDFCDesiredStates.
func (p *Service) Start(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) error {
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Evaluate and set DFC states based on current configs
	// NOTE: This is different from other FSMs - we don't just set all DFCs to active,
	// we check if they have valid configs first (see EvaluateDFCDesiredStates docstring)
	return p.EvaluateDFCDesiredStates(spName, "active")
}

// Stop stops a Stream Processor
// Expects spName (e.g. "streamprocessor-myservice") as defined in the UMH config
func (p *Service) Stop(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) error {
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Set all DFCs to stopped
	return p.EvaluateDFCDesiredStates(spName, "stopped")
}

// ReconcileManager synchronizes all stream processors on each tick.
// It delegates to the Stream Processors service's reconciliation function,
// which ensures that the actual state matches the desired state
// for all services.
//
// This should be called periodically by a control loop.
func (p *Service) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (error, bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentStreamProcessorService, "ReconcileManager", time.Since(start))
	}()
	p.logger.Debugf("Reconciling streamprocessor manager at tick %d", tick)

	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initilized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Use the dfcManager's Reconcile method
	err, dfcReconciled := p.dataflowComponentManager.Reconcile(ctx, fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{
			DataFlow: p.dataflowComponentConfig,
		},
		Tick: tick,
	}, services)

	if err != nil {
		return err, dfcReconciled
	}

	return nil, dfcReconciled
}

// ServiceExists checks if a  dataflowcomponent with the given name exist.
// If only one of the services exists, it returns false.
func (p *Service) ServiceExists(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) bool {
	if ctx.Err() != nil {
		return false
	}

	dfcName := p.getUnderlyingDFCReadName(spName)

	// Check if the actual service exists
	dfcExists := p.dataflowComponentService.ServiceExists(ctx, filesystemService, dfcName)

	// if one of the services doesn't exist we should return that
	return dfcExists
}

// ForceRemove removes a StreamProcessor from the  DFC manager
// Expects spName (e.g. "streamrpcoessor-myservice") as defined in the UMH config
func (c *Service) ForceRemove(
	ctx context.Context,
	filesystemService filesystem.Service,
	spName string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	dfcName := c.getUnderlyingDFCReadName(spName)

	// force remove from dataflowcomponent manager
	// We attempt to remove both read and write DFCs regardless of individual errors.
	// If a component doesn't exist (ErrServiceNotExists), we log the error but continue
	// with removing the other component. Only return an error if it's a different failure
	// (like permission issues or system errors).
	err := c.dataflowComponentService.ForceRemoveDataFlowComponent(ctx, filesystemService, dfcName)
	if err != nil && !errors.Is(err, dfc.ErrServiceNotExists) {
		return err
	}

	return nil
}
