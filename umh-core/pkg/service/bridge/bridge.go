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

package bridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/bridgeserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	connectionfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
	dfc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
	"go.uber.org/zap"
)

// IService defines the public contract for a bridge:
// a logical unit that combines one Connection + one Data-Flow-Component (DFC)
// and surfaces them as a single object to the rest of UMH-Core.
type IService interface {
	// GetConfig pulls the actual runtime configuration that is currently
	// deployed for the given Bridge.
	//
	// It does so by:
	//  1. Deriving the names of the underlying resources (Connection, read-DFC,
	//     write-DFC).
	//  2. Asking the respective sub-services for their Runtime configs.
	//  3. Stitching those parts back together via
	//     `FromConfigs`.
	//
	// The resulting ConfigRuntime reflects the state the FSM is really running
	// with and therefore forms the "live" side of the reconcile equation.
	GetConfig(ctx context.Context, filesystemService filesystem.Service, name string) (bridgeserviceconfig.ConfigRuntime, error)

	// Status aggregates health from Connection, DFC and Redpanda into a single
	// snapshot.  The returned structure is read-only – callers must not
	// mutate it.
	Status(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot, name string) (ServiceInfo, error)

	// AddToManager adds a Bridge to the Connection & DFC manager
	AddToManager(ctx context.Context, filesystemService filesystem.Service, cfg *bridgeserviceconfig.ConfigRuntime, name string) error

	// UpdateInManager updates an existing Bridge in the Connection & DFC manager
	UpdateInManager(ctx context.Context, filesystemService filesystem.Service, cfg *bridgeserviceconfig.ConfigRuntime, name string) error

	// RemoveFromManager removes a Bridge from the Connection & DFC manager
	RemoveFromManager(ctx context.Context, filesystemService filesystem.Service, name string) error

	// Start starts a Bridge
	Start(ctx context.Context, filesystemService filesystem.Service, name string) error

	// Stop stops a Bridge
	Stop(ctx context.Context, filesystemService filesystem.Service, name string) error

	// ForceRemove removes a Bridge from the Connection & DFC manager
	ForceRemove(ctx context.Context, filesystemService filesystem.Service, name string) error

	// ServiceExists checks if a connection and a dataflowcomponent with the given name exist.
	// If only one of the services exists, it returns false.
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, name string) bool

	// ReconcileManager is the heart-beat: on every tick it feeds the desired
	// slices into the child-managers, lets them run one state-machine cycle and
	// returns:
	//   err        – unrecoverable problem (configuration bug, ctx cancellation)
	//   reconciled – true when *any* child manager made progress
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)

	// EvaluateDFCDesiredStates determines the appropriate desired states for the underlying
	// DFCs based on the bridge's desired state and current DFC configurations.
	// This is used internally for both initial startup and config change re-evaluation.
	EvaluateDFCDesiredStates(name string, desiredState string) error
}

// ServiceInfo holds information about the Bridges underlying health states.
type ServiceInfo struct {
	// FSMStates are the current FSM state strings (e.g. "up", "stopped", "active").
	ConnectionFSMState string
	DFCReadFSMState    string
	DFCWriteFSMState   string
	RedpandaFSMState   string

	// StatusReason is a short human string (log excerpt, metrics finding, …)
	// explaining *why* the converter is not "green".
	StatusReason string

	// ObservedStates are holding the observed runtime state of their instances:
	// RedpandaObservedState is included so a bridge can degrade
	// itself when the message bus is down.
	RedpandaObservedState   redpandafsm.RedpandaObservedState
	ConnectionObservedState connectionfsm.ConnectionObservedState
	DFCReadObservedState    dfcfsm.DataflowComponentObservedState
	DFCWriteObservedState   dfcfsm.DataflowComponentObservedState
}

// Service implements IService using it's underlying components
type Service struct {
	logger *zap.SugaredLogger

	// Connection
	connectionManager *connectionfsm.ConnectionManager
	connectionService connection.IConnectionService
	connectionConfig  []config.ConnectionConfig

	// DataflowComponent
	// It has the config for a reading DFC and a writing DFC
	dataflowComponentManager *dfcfsm.DataflowComponentManager
	dataflowComponentService dfc.IDataFlowComponentService
	dataflowComponentConfig  []config.DataFlowComponentConfig

	// Redpanda is not part of the service, we just monitor it here for better error reporting itself in the fsm
	// e.g., if redpanda is down, the instance will likely also have issues
}

// ServiceOption is a function that configures a Service.
// This follows the functional options pattern for flexible configuration.
type ServiceOption func(*Service)

// WithUnderlyingServices sets the underlying services for the service.
// Used for testing purposes
func WithUnderlyingServices(
	connService connection.IConnectionService,
	dfcService dfc.IDataFlowComponentService,
) ServiceOption {
	return func(p *Service) {
		p.connectionService = connService
		p.dataflowComponentService = dfcService
	}
}

// WithUnderlyingManagers sets the underlying managers for the service.
// Used for testing purposes
func WithUnderlyingManagers(
	connMgr *connectionfsm.ConnectionManager,
	dfcMgr *dfcfsm.DataflowComponentManager,
) ServiceOption {
	return func(c *Service) {
		c.connectionManager = connMgr
		c.dataflowComponentManager = dfcMgr
	}
}

// NewDefaultService returns a fully initialised service with
// default child managers.  Call-site may inject mocks via functional options.
//
// Note: `name` is the logical name – without the "bridge-"
// prefix – exactly as it appears in the UMH YAML.
func NewDefaultService(name string, opts ...ServiceOption) *Service {
	managerName := fmt.Sprintf("%s%s", logger.ComponentBridgeService, name)
	service := &Service{
		logger:                   logger.For(managerName),
		connectionManager:        connectionfsm.NewConnectionManager(managerName),
		connectionService:        connection.NewDefaultConnectionService(name),
		connectionConfig:         []config.ConnectionConfig{},
		dataflowComponentManager: dfcfsm.NewDataflowComponentManager(managerName),
		dataflowComponentService: dfc.NewDefaultDataFlowComponentService(name),
		dataflowComponentConfig:  []config.DataFlowComponentConfig{},
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getUnderlyingName returns the base external name that all child services of
// a Bridge share. It simply prepends the fixed
// `"bridge-"` prefix to the logical name the user supplied in the
// UMH YAML.
//
// Example:
//
//	name = "mixing-station"
//	→ "bridge-mixing-station"
func (p *Service) getUnderlyingName(name string) string {
	return fmt.Sprintf("bridge-%s", name)
}

// getUnderlyingDFCReadName returns the external name handed to the
// Data-flow-Component manager for the reading DFC.
// The `"read-"` role prefix is added so the manager can distinguish the two
// sibling DFCs that belong to the same converter.
//
// Example:
//
//	name = "mixing-station"
//	→ "read-bridge-mixing-station"
func (p *Service) getUnderlyingDFCReadName(name string) string {
	return fmt.Sprintf("read-%s", p.getUnderlyingName(name))
}

// getUnderlyingDFCWriteName returns the external name handed to the
// Data-flow-Component manager for the writing DFC.
// The `"write-"` role prefix is added so the manager can distinguish the two
// sibling DFCs that belong to the same bridge.
//
// Example:
//
//	name = "mixing-station"
//	→ "write-bridge-mixing-station"
func (p *Service) getUnderlyingDFCWriteName(name string) string {
	return fmt.Sprintf("write-%s", p.getUnderlyingName(name))
}

// GetConfig pulls the actual runtime configuration that is currently
// deployed for the given Bridge.
//
// It does so by:
//  1. Deriving the names of the underlying resources (Connection, read-DFC,
//     write-DFC).
//  2. Asking the respective sub-services for their Runtime configs.
//  3. Stitching those parts back together via
//     `FromConfigs`.
//
// The resulting ConfigRuntime reflects the state
// the FSM is really running with and therefore forms the "live" side of the
// reconcile equation:
func (p *Service) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	name string,
) (bridgeserviceconfig.ConfigRuntime, error) {
	if ctx.Err() != nil {
		return bridgeserviceconfig.ConfigRuntime{}, ctx.Err()
	}

	underlyingConnectionName := p.getUnderlyingName(name)
	underlyingDFCReadName := p.getUnderlyingDFCReadName(name)
	underlyingDFCWriteName := p.getUnderlyingDFCWriteName(name)

	// Get the Connection config
	connConfig, err := p.connectionService.GetConfig(ctx, filesystemService, underlyingConnectionName)
	if err != nil {
		return bridgeserviceconfig.ConfigRuntime{}, fmt.Errorf("failed to get connection config: %w", err)
	}

	dfcReadConfig, err := p.dataflowComponentService.GetConfig(ctx, filesystemService, underlyingDFCReadName)
	if err != nil {
		return bridgeserviceconfig.ConfigRuntime{}, fmt.Errorf("failed to get read dataflowcomponent config: %w", err)
	}

	dfcWriteConfig, err := p.dataflowComponentService.GetConfig(ctx, filesystemService, underlyingDFCWriteName)
	if err != nil {
		return bridgeserviceconfig.ConfigRuntime{}, fmt.Errorf("failed to get write dataflowcomponent config: %w", err)
	}

	actualConfig := bridgeserviceconfig.FromConfigs(connConfig, dfcReadConfig, dfcWriteConfig)

	return actualConfig, nil
}

// Status returns information about the connection health for the specified connection.
// It queries the underlying Connection service and then enhances the result with
// additional context like flakiness detection based on historical data.
func (p *Service) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	snapshot fsm.SystemSnapshot,
	name string,
) (ServiceInfo, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentBridgeService, name+".Status", time.Since(start))
	}()

	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}
	if !p.ServiceExists(ctx, services.GetFileSystem(), name) {
		return ServiceInfo{}, ErrServiceNotExist
	}

	underlyingConnectionName := p.getUnderlyingName(name)
	underlyingDFCReadName := p.getUnderlyingDFCReadName(name)
	underlyingDFCWriteName := p.getUnderlyingDFCWriteName(name)

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
		return ServiceInfo{}, fmt.Errorf("redpanda status for redpanda %s is not a RedpandaObservedStateSnapshot", name)
	}

	// Now it is in the same format
	redpandaObservedState := redpandafsm.RedpandaObservedState{
		ServiceInfo:                   redpandaObservedStateSnapshot.ServiceInfoSnapshot,
		ObservedRedpandaServiceConfig: redpandaObservedStateSnapshot.Config,
	}

	redpandaFSMState := rpInst.CurrentState

	// -- connection --------------------------------------------------------------------------------

	// get last observed states
	connectionStatus, err := p.connectionManager.GetLastObservedState(underlyingConnectionName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get connection observed state: %w", err)
	}

	// check observed state types
	connectionObservedState, ok := connectionStatus.(connectionfsm.ConnectionObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("connection status for connection %s is not a ConnectionObservedState", name)
	}

	// get current fsm states
	connectionFSMState, err := p.connectionManager.GetCurrentFSMState(underlyingConnectionName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get connection FSM state: %w", err)
	}

	// -- DFC Read --------------------------------------------------------------------------------

	dfcReadStatus, err := p.dataflowComponentManager.GetLastObservedState(underlyingDFCReadName)
	dfcReadExists := err == nil
	var dfcReadObservedState dfcfsm.DataflowComponentObservedState
	var dfcReadFSMState string
	if dfcReadExists {
		dfcReadObservedState, ok = dfcReadStatus.(dfcfsm.DataflowComponentObservedState)
		if !ok {
			return ServiceInfo{}, fmt.Errorf("read dataflowcomponent status for dataflowcomponent %s is not a DataflowComponentObservedState", name)
		}

		dfcReadFSMState, err = p.dataflowComponentManager.GetCurrentFSMState(underlyingDFCReadName)
		if err != nil {
			return ServiceInfo{}, fmt.Errorf("failed to get read dataflowcomponent FSM state: %w", err)
		}
	}

	// -- DFC Write --------------------------------------------------------------------------------

	dfcWriteStatus, err := p.dataflowComponentManager.GetLastObservedState(underlyingDFCWriteName)
	dfcWriteExists := err == nil
	var dfcWriteObservedState dfcfsm.DataflowComponentObservedState
	var dfcWriteFSMState string
	if dfcWriteExists {
		dfcWriteObservedState, ok = dfcWriteStatus.(dfcfsm.DataflowComponentObservedState)
		if !ok {
			return ServiceInfo{}, fmt.Errorf("write dataflowcomponent status for dataflowcomponent %s is not a DataflowComponentObservedState", name)
		}

		dfcWriteFSMState, err = p.dataflowComponentManager.GetCurrentFSMState(underlyingDFCWriteName)
		if err != nil {
			return ServiceInfo{}, fmt.Errorf("failed to get write dataflowcomponent FSM state: %w", err)
		}
	}

	// Error if both DFCs are missing
	if !dfcReadExists && !dfcWriteExists {
		return ServiceInfo{}, fmt.Errorf("neither read nor write dataflowcomponent exists for bridge %s", name)
	}

	return ServiceInfo{
		ConnectionObservedState: connectionObservedState,
		ConnectionFSMState:      connectionFSMState,
		DFCReadObservedState:    dfcReadObservedState,
		DFCReadFSMState:         dfcReadFSMState,
		DFCWriteObservedState:   dfcWriteObservedState,
		DFCWriteFSMState:        dfcWriteFSMState,
		RedpandaObservedState:   redpandaObservedState,
		RedpandaFSMState:        redpandaFSMState,
	}, nil
}

// AddToManager registers a new bridge in the Connection & DFC service.
func (p *Service) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *bridgeserviceconfig.ConfigRuntime,
	name string,
) error {
	if p.connectionManager == nil {
		return errors.New("connection manager not initialized")
	}

	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	p.logger.Infof("Adding Bridge %s", name)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	underlyingConnectionName := p.getUnderlyingName(name)
	underlyingDFCReadName := p.getUnderlyingDFCReadName(name)
	underlyingDFCWriteName := p.getUnderlyingDFCWriteName(name)

	// Check if the connection already exists in our configs
	for _, existingConfig := range p.connectionConfig {
		if existingConfig.Name == underlyingConnectionName {
			return ErrServiceAlreadyExists
		}
	}

	connServiceConfig := cfg.ConnectionConfig
	dfcReadServiceConfig := cfg.DFCReadConfig
	dfcWriteServiceConfig := cfg.DFCWriteConfig

	// Create a config.ConnectionConfig that wraps the NmapServiceConfig
	connectionConfig := config.ConnectionConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingConnectionName,
			DesiredFSMState: connectionfsm.OperationalStateStopped,
		},
		ConnectionServiceConfig: connServiceConfig,
	}

	dfcReadConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingDFCReadName,
			DesiredFSMState: dfcfsm.OperationalStateStopped,
		},
		DataFlowComponentServiceConfig: dfcReadServiceConfig,
	}

	dfcWriteConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingDFCWriteName,
			DesiredFSMState: dfcfsm.OperationalStateStopped,
		},
		DataFlowComponentServiceConfig: dfcWriteServiceConfig,
	}

	// Add the configurations to the lists
	p.connectionConfig = append(p.connectionConfig, connectionConfig)
	p.dataflowComponentConfig = append(p.dataflowComponentConfig, dfcReadConfig, dfcWriteConfig)

	p.logger.Infof("Bridge %s added to manager", name)
	p.logger.Infof("Connection config: %+v", p.connectionConfig)
	p.logger.Infof("Dataflow component config: %+v", p.dataflowComponentConfig)

	return nil
}

// UpdateInManager modifies an existing bridge configuration.
func (p *Service) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *bridgeserviceconfig.ConfigRuntime,
	name string,
) error {
	if p.connectionManager == nil {
		return errors.New("connection manager not initialized")
	}
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	if cfg == nil {
		return errors.New("config is nil")
	}

	p.logger.Infof("Updating bridge %s", name)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the connectionconfig exists
	foundConn := false
	indexConn := -1
	for i, config := range p.connectionConfig {
		if config.Name == p.getUnderlyingName(name) {
			foundConn = true
			indexConn = i
			break
		}
	}

	if !foundConn {
		return ErrServiceNotExist
	}

	// Check if the dfcconfig exists
	foundReadDFC := false
	indexReadDFC := -1
	for i, config := range p.dataflowComponentConfig {
		if config.Name == p.getUnderlyingDFCReadName(name) {
			foundReadDFC = true
			indexReadDFC = i
			break
		}
	}

	foundWriteDFC := false
	indexWriteDFC := -1
	for i, config := range p.dataflowComponentConfig {
		if config.Name == p.getUnderlyingDFCWriteName(name) {
			foundWriteDFC = true
			indexWriteDFC = i
			break
		}
	}

	// either read or write dfc must exist
	if !foundReadDFC && !foundWriteDFC {
		return ErrServiceNotExist
	}

	connConfig := cfg.ConnectionConfig
	dfcReadConfig := cfg.DFCReadConfig
	dfcWriteConfig := cfg.DFCWriteConfig

	// Create a config.ConnectionConfig that wraps the ConnectionServiceConfig
	connCurrentDesiredState := p.connectionConfig[indexConn].DesiredFSMState
	p.connectionConfig[indexConn] = config.ConnectionConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            p.getUnderlyingName(name),
			DesiredFSMState: connCurrentDesiredState,
		},
		ConnectionServiceConfig: connConfig,
	}

	// Create a config.DataflowComponentConfig that wraps the DataflowComponentServiceConfig
	if foundReadDFC {
		dfcCurrentDesiredState := p.dataflowComponentConfig[indexReadDFC].DesiredFSMState
		p.dataflowComponentConfig[indexReadDFC] = config.DataFlowComponentConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            p.getUnderlyingDFCReadName(name),
				DesiredFSMState: dfcCurrentDesiredState,
			},
			DataFlowComponentServiceConfig: dfcReadConfig,
		}
	}

	if foundWriteDFC {
		dfcCurrentDesiredState := p.dataflowComponentConfig[indexWriteDFC].DesiredFSMState
		p.dataflowComponentConfig[indexWriteDFC] = config.DataFlowComponentConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            p.getUnderlyingDFCWriteName(name),
				DesiredFSMState: dfcCurrentDesiredState,
			},
			DataFlowComponentServiceConfig: dfcWriteConfig,
		}
	}

	p.logger.Info("Updated bridge config in manager")
	p.logger.Infof("Connection config: %+v", p.connectionConfig)
	p.logger.Infof("Dataflow component config: %+v", p.dataflowComponentConfig)

	return nil
}

// RemoveFromManager deletes a bridge configuration.
func (p *Service) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	name string,
) error {
	if p.connectionManager == nil {
		return errors.New("connection manager not initialized")
	}

	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	p.logger.Infof("Removing dataflowcomponent %s", name)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	connectionName := p.getUnderlyingName(name)
	dfcReadName := p.getUnderlyingDFCReadName(name)
	dfcWriteName := p.getUnderlyingDFCWriteName(name)

	connSliceRemoveByName := func(in []config.ConnectionConfig, name string) []config.ConnectionConfig {
		for i, v := range in {
			if v.Name == name {
				return append(in[:i], in[i+1:]...)
			}
		}
		return in // already gone
	}

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
	p.connectionConfig = connSliceRemoveByName(p.connectionConfig, connectionName)
	p.dataflowComponentConfig = dfcSliceRemoveByName(p.dataflowComponentConfig, dfcReadName)
	p.dataflowComponentConfig = dfcSliceRemoveByName(p.dataflowComponentConfig, dfcWriteName)

	//--------------------------------------------
	// 2) is the child FSM still alive?
	//--------------------------------------------
	if inst, ok := p.connectionManager.GetInstance(connectionName); ok {
		return fmt.Errorf("%w: Connection instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	if inst, ok := p.dataflowComponentManager.GetInstance(dfcReadName); ok {
		return fmt.Errorf("%w: DataflowComponent instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	if inst, ok := p.dataflowComponentManager.GetInstance(dfcWriteName); ok {
		return fmt.Errorf("%w: DataflowComponent instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	return nil
}

// EvaluateDFCDesiredStates determines the appropriate desired states for the underlying
// DFCs based on the bridge's desired state and current DFC configurations.
//
// UNIQUE BEHAVIOR: Unlike other FSMs that make start/stop decisions once during initial
// startup, bridges must re-evaluate DFC states whenever configs change because:
//
//  1. Bridges start with template-based configs containing variables like
//     {{ .internal.bridged_by }} and {{ .location.0 }}
//  2. These templates are rendered during reconciliation using runtime data (agent location,
//     node name, etc.) that's not available at creation time
//  3. DFC configs may transition from empty -> populated or populated -> empty as templates
//     are processed, requiring state re-evaluation
//
// Logic:
// - If bridge desired state is stopped: all DFCs -> stopped
// - If bridge desired state is active: DFCs -> active only if they have non-empty input configs
//
// Called from:
// 1. Start() - during initial activation
// 2. Stop() - during shutdown
// 3. UpdateObservedStateOfInstance() - when config changes are detected during reconciliation
func (p *Service) EvaluateDFCDesiredStates(name string, desiredState string) error {
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	dfcReadName := p.getUnderlyingDFCReadName(name)
	dfcWriteName := p.getUnderlyingDFCWriteName(name)

	// Find and update our cached config for read DFC
	dfcReadFound := false
	dfcWriteFound := false
	for i, config := range p.dataflowComponentConfig {
		if config.Name == dfcReadName {
			if desiredState == "stopped" {
				p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			} else {
				// Only start the DFC, if it has been configured
				if len(p.dataflowComponentConfig[i].DataFlowComponentServiceConfig.BenthosConfig.Input) > 0 {
					p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateActive
				} else {
					p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
				}
			}
			dfcReadFound = true
			break
		}
	}

	// Find and update our cached config for write DFC
	for i, config := range p.dataflowComponentConfig {
		if config.Name == dfcWriteName {
			if desiredState == "stopped" {
				p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			} else {
				// Only start the DFC, if it has been configured
				if len(p.dataflowComponentConfig[i].DataFlowComponentServiceConfig.BenthosConfig.Input) > 0 {
					p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateActive
				} else {
					p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
				}
			}
			dfcWriteFound = true
			break
		}
	}

	if !dfcReadFound && !dfcWriteFound {
		return ErrServiceNotExist
	}

	return nil
}

// Start starts a Bridge by setting connection to "up" and
// evaluating which DFCs should be active based on their current configurations.
//
// UNIQUE BEHAVIOR: Unlike other FSM start methods that simply set desired states to active,
// bridge must check DFC config content because:
// - DFCs start with template configs that may be empty initially
// - Only DFCs with non-empty input configs should be started
// - Empty DFCs remain stopped to avoid creating broken Benthos instances
//
// This conditional starting is handled by EvaluateDFCDesiredStates.
func (p *Service) Start(
	ctx context.Context,
	filesystemService filesystem.Service,
	name string,
) error {
	if p.connectionManager == nil {
		return errors.New("connection manager not initialized")
	}

	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	connectionName := p.getUnderlyingName(name)

	// Find and update our cached config
	connFound := false
	for i, config := range p.connectionConfig {
		if config.Name == connectionName {
			p.connectionConfig[i].DesiredFSMState = connectionfsm.OperationalStateUp
			connFound = true
			break
		}
	}

	if !connFound {
		return ErrServiceNotExist
	}

	// Evaluate and set DFC states based on current configs
	// NOTE: This is different from other FSMs - we don't just set all DFCs to active,
	// we check if they have valid configs first (see EvaluateDFCDesiredStates docstring)
	return p.EvaluateDFCDesiredStates(name, "active")
}

// Stop stops a Bridge
// Expects name (e.g. "bridge-myservice") as defined in the UMH config
func (p *Service) Stop(
	ctx context.Context,
	filesystemService filesystem.Service,
	name string,
) error {
	if p.connectionManager == nil {
		return errors.New("connection manager not initialized")
	}

	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	connectionName := p.getUnderlyingName(name)

	// Find and update our cached config
	connFound := false
	for i, config := range p.connectionConfig {
		if config.Name == connectionName {
			p.connectionConfig[i].DesiredFSMState = connectionfsm.OperationalStateStopped
			connFound = true
			break
		}
	}

	if !connFound {
		return ErrServiceNotExist
	}

	// Set all DFCs to stopped
	return p.EvaluateDFCDesiredStates(name, "stopped")
}

// ReconcileManager synchronizes all bridges on each tick.
// It delegates to the Bridge service's reconciliation function,
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
		metrics.ObserveReconcileTime(logger.ComponentBridgeService, "ReconcileManager", time.Since(start))
	}()
	p.logger.Debugf("Reconciling bridge manager at tick %d", tick)

	if p.connectionManager == nil {
		return errors.New("connection manager not initilized"), false
	}

	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initilized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Use the connectionManager's Reconcile method
	err, connReconciled := p.connectionManager.Reconcile(ctx, fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{
			Internal: config.InternalConfig{
				Connection: p.connectionConfig,
			},
		},
		Tick: tick,
	}, services)

	if err != nil {
		return err, connReconciled
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

	return nil, connReconciled || dfcReconciled
}

// ServiceExists checks if a connection and a dataflowcomponent with the given name exist.
// If only one of the services exists, it returns false.
func (p *Service) ServiceExists(
	ctx context.Context,
	filesystemService filesystem.Service,
	name string,
) bool {
	if ctx.Err() != nil {
		return false
	}

	connectionName := p.getUnderlyingName(name)
	dfcReadName := p.getUnderlyingDFCReadName(name)
	dfcWriteName := p.getUnderlyingDFCWriteName(name)

	// Check if the actual service exists
	connExists := p.connectionService.ServiceExists(ctx, filesystemService, connectionName)
	dfcReadExists := p.dataflowComponentService.ServiceExists(ctx, filesystemService, dfcReadName)
	dfcWriteExists := p.dataflowComponentService.ServiceExists(ctx, filesystemService, dfcWriteName)

	// if one of the services doesn't exist we should return that
	return connExists && (dfcReadExists || dfcWriteExists)
}

// ForceRemove removes a Bridge from the Connection & DFC manager
// Expects name (e.g. "bridge-myservice") as defined in the UMH config
func (c *Service) ForceRemove(
	ctx context.Context,
	filesystemService filesystem.Service,
	name string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	connectionName := c.getUnderlyingName(name)
	dfcReadName := c.getUnderlyingDFCReadName(name)
	dfcWriteName := c.getUnderlyingDFCWriteName(name)

	// force remove from Connection manager
	err := c.connectionService.ForceRemoveConnection(ctx, filesystemService, connectionName)
	if err != nil {
		return err
	}

	// force remove from dataflowcomponent manager
	// We attempt to remove both read and write DFCs regardless of individual errors.
	// If a component doesn't exist (ErrServiceNotExists), we log the error but continue
	// with removing the other component. Only return an error if it's a different failure
	// (like permission issues or system errors).
	err = c.dataflowComponentService.ForceRemoveDataFlowComponent(ctx, filesystemService, dfcReadName)
	if err != nil && !errors.Is(err, dfc.ErrServiceNotExists) {
		return err
	}

	err = c.dataflowComponentService.ForceRemoveDataFlowComponent(ctx, filesystemService, dfcWriteName)
	if err != nil && !errors.Is(err, dfc.ErrServiceNotExists) {
		return err
	}

	return nil
}
