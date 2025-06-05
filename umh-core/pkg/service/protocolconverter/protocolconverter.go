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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
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

// IProtocolConverterService defines the public contract for a *protocol-converter*:
// a logical unit that combines **one Connection + one Data-Flow-Component (DFC)**
// and surfaces them as a single object to the rest of UMH-Core.
type IProtocolConverterService interface {

	// GetConfig pulls the **actual** runtime configuration that is currently
	// deployed for the given Protocol-Converter.
	//
	// It does so by:
	//  1. Deriving the names of the underlying resources (Connection, read-DFC,
	//     write-DFC).
	//  2. Asking the respective sub-services for their *Runtime* configs.
	//  3. Stitching those parts back together via
	//     `FromConnectionAndDFCServiceConfig`.
	//
	// The resulting **ProtocolConverterServiceConfigRuntime** reflects the state
	// the FSM is *really* running with and therefore forms the "live" side of the
	// reconcile equation:
	GetConfig(ctx context.Context, filesystemService filesystem.Service, protConvName string) (protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime, error)

	// Status aggregates health from Connection, DFC and Redpanda into a single
	// snapshot.  The returned structure is **read-only** – callers must not
	// mutate it.
	Status(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot, protConvName string) (ServiceInfo, error)

	// AddToManager adds a ProtocolConverter to the Connection & DFC manager
	AddToManager(ctx context.Context, filesystemService filesystem.Service, protConvCfg *protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime, protConvName string) error

	// UpdateInManager updates an existing ProtocolConverter in the Connection & DFC manager
	UpdateInManager(ctx context.Context, filesystemService filesystem.Service, protConvCfg *protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime, protConvName string) error

	// RemoveFromManager removes a ProtocolConverter from the Connection & DFC manager
	RemoveFromManager(ctx context.Context, filesystemService filesystem.Service, protConvName string) error

	// Start starts a ProtocolConverter
	StartProtocolConverter(ctx context.Context, filesystemService filesystem.Service, protConvName string) error

	// Stop stops a ProtocolConverter
	StopProtocolConverter(ctx context.Context, filesystemService filesystem.Service, protConvName string) error

	// ForceRemove removes a ProtocolConverter from the Connetion & DFC manager
	ForceRemoveProtocolConverter(ctx context.Context, filesystemService filesystem.Service, protConvName string) error

	// ServiceExists checks if a connection and a dataflowcomponent with the given name exist.
	// If only one of the services exists, it returns false.
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, protConvName string) bool

	// ReconcileManager is the heart-beat: on every tick it feeds the *desired*
	// slices into the child-managers, lets them run one state-machine cycle and
	// returns:
	//   err        – unrecoverable problem (configuration bug, ctx cancellation)
	//   reconciled – true when *any* child manager made progress
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
}

// ServiceInfo holds information about the ProtocolConverters underlying health states.
type ServiceInfo struct {
	// ConnectionObservedState is the last sample from the connection manager.
	ConnectionObservedState connectionfsm.ConnectionObservedState
	// ConnectionFSMState is the *current* FSM state string (e.g. "up", "stopped").
	ConnectionFSMState string

	// DataflowComponentReadObservedState mirrors the DFC manager.
	DataflowComponentReadObservedState dfcfsm.DataflowComponentObservedState
	DataflowComponentReadFSMState      string

	// DataflowComponentWriteObservedState mirrors the DFC manager.
	DataflowComponentWriteObservedState dfcfsm.DataflowComponentObservedState
	DataflowComponentWriteFSMState      string

	// RedpandaObservedState is included so a protocol-converter can degrade
	// itself when the message bus is down.
	RedpandaObservedState redpandafsm.RedpandaObservedState
	RedpandaFSMState      string

	// StatusReason is a short human string (log excerpt, metrics finding, …)
	// explaining *why* the converter is not "green".
	StatusReason string
}

// ProtocolConverterService implements IProtocolConverterService using it's
// underlying components
type ProtocolConverterService struct {
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

	// Redpanda is not part of a protocol converter service, we just monitor it here in the protocol converter for better error reporting itself in the protocol converter fsm
	// e.g., if redpanda is down, the protocol converter will likely also have issues
}

// ProtocolConverterServiceOption is a function that configures a ConnectionService.
// This follows the functional options pattern for flexible configuration.
type ProtocolConverterServiceOption func(*ProtocolConverterService)

// WithUnderlyingServices sets the underlying services for the protocol converter service
// Used for testing purposes
func WithUnderlyingServices(
	connService connection.IConnectionService,
	dfcService dfc.IDataFlowComponentService,
) ProtocolConverterServiceOption {
	return func(p *ProtocolConverterService) {
		p.connectionService = connService
		p.dataflowComponentService = dfcService
	}
}

// WithUnderlyingManagers sets the underlying managers for the protocol converter service
// Used for testing purposes
func WithUnderlyingManagers(
	connMgr *connectionfsm.ConnectionManager,
	dfcMgr *dfcfsm.DataflowComponentManager,
) ProtocolConverterServiceOption {
	return func(c *ProtocolConverterService) {
		c.connectionManager = connMgr
		c.dataflowComponentManager = dfcMgr

	}
}

// NewDefaultProtocolConverterService returns a fully initialised service with
// default child managers.  Call-site may inject mocks via functional options.
//
// Note: `protConvName` is the *logical* name – **without** the "protocolconverter-"
// prefix – exactly as it appears in the UMH YAML.
func NewDefaultProtocolConverterService(protConvName string, opts ...ProtocolConverterServiceOption) *ProtocolConverterService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentProtocolConverterService, protConvName)
	service := &ProtocolConverterService{
		logger:                   logger.For(managerName),
		connectionManager:        connectionfsm.NewConnectionManager(managerName),
		connectionService:        connection.NewDefaultConnectionService(protConvName),
		connectionConfig:         []config.ConnectionConfig{},
		dataflowComponentManager: dfcfsm.NewDataflowComponentManager(managerName),
		dataflowComponentService: dfc.NewDefaultDataFlowComponentService(protConvName),
		dataflowComponentConfig:  []config.DataFlowComponentConfig{},
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getUnderlyingName returns the *base* external name that all child services of
// a Protocol-Converter share. It simply prepends the fixed
// `"protocolconverter-"` prefix to the logical name the user supplied in the
// UMH YAML.
//
// Example:
//
//	protConvName = "mixing-station"
//	→ "protocolconverter-mixing-station"
func (p *ProtocolConverterService) getUnderlyingName(protConvName string) string {
	return fmt.Sprintf("protocolconverter-%s", protConvName)
}

// getUnderlyingConnectionName returns the external name handed to the **Connection
// manager**. For the Connection we do **not** add any read/write qualifier –
// the underlying base name is already unique across all Connection instances.
//
// Example:
//
//	protConvName = "mixing-station"
//	→ "protocolconverter-mixing-station"
func (p *ProtocolConverterService) getUnderlyingConnectionName(protConvName string) string {
	return p.getUnderlyingName(protConvName)
}

// getUnderlyingDFCReadName returns the external name handed to the
// **Data-flow-Component manager** for the *reading* DFC.
// The `"read-"` role prefix is added so the manager can distinguish the two
// sibling DFCs that belong to the same converter.
//
// Example:
//
//	protConvName = "mixing-station"
//	→ "read-protocolconverter-mixing-station"
func (p *ProtocolConverterService) getUnderlyingDFCReadName(protConvName string) string {
	return fmt.Sprintf("read-%s", p.getUnderlyingName(protConvName))
}

// getUnderlyingDFCWriteName returns the external name handed to the
// **Data-flow-Component manager** for the *writing* DFC.
// The `"write-"` role prefix is added so the manager can distinguish the two
// sibling DFCs that belong to the same converter.
//
// Example:
//
//	protConvName = "mixing-station"
//	→ "write-protocolconverter-mixing-station"
func (p *ProtocolConverterService) getUnderlyingDFCWriteName(protConvName string) string {
	return fmt.Sprintf("write-%s", p.getUnderlyingName(protConvName))
}

// GetConfig pulls the **actual** runtime configuration that is currently
// deployed for the given Protocol-Converter.
//
// It does so by:
//  1. Deriving the names of the underlying resources (Connection, read-DFC,
//     write-DFC).
//  2. Asking the respective sub-services for their *Runtime* configs.
//  3. Stitching those parts back together via
//     `FromConnectionAndDFCServiceConfig`.
//
// The resulting **ProtocolConverterServiceConfigRuntime** reflects the state
// the FSM is *really* running with and therefore forms the "live" side of the
// reconcile equation:
func (p *ProtocolConverterService) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) (protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime, error) {
	if ctx.Err() != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, ctx.Err()
	}

	underlyingConnectionName := p.getUnderlyingConnectionName(protConvName)
	underlyingDFCReadName := p.getUnderlyingDFCReadName(protConvName)
	underlyingDFCWriteName := p.getUnderlyingDFCWriteName(protConvName)

	// Get the Connection config
	connConfig, err := p.connectionService.GetConfig(ctx, filesystemService, underlyingConnectionName)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, fmt.Errorf("failed to get connection config: %w", err)
	}

	dfcReadConfig, err := p.dataflowComponentService.GetConfig(ctx, filesystemService, underlyingDFCReadName)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, fmt.Errorf("failed to get read dataflowcomponent config: %w", err)
	}

	dfcWriteConfig, err := p.dataflowComponentService.GetConfig(ctx, filesystemService, underlyingDFCWriteName)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{}, fmt.Errorf("failed to get write dataflowcomponent config: %w", err)
	}

	actualConfig := protocolconverterserviceconfig.FromConnectionAndDFCServiceConfig(connConfig, dfcReadConfig, dfcWriteConfig)

	return actualConfig, nil
}

// Status returns information about the connection health for the specified connection.
// It queries the underlying Connection service and then enhances the result with
// additional context like flakiness detection based on historical data.
func (p *ProtocolConverterService) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	snapshot fsm.SystemSnapshot,
	protConvName string,
) (ServiceInfo, error) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentProtocolConverterService, protConvName+".Status", time.Since(start))
	}()

	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}
	if !p.ServiceExists(ctx, services.GetFileSystem(), protConvName) {
		return ServiceInfo{}, ErrServiceNotExist
	}

	underlyingConnectionName := p.getUnderlyingConnectionName(protConvName)
	underlyingDFCReadName := p.getUnderlyingDFCReadName(protConvName)
	underlyingDFCWriteName := p.getUnderlyingDFCWriteName(protConvName)

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
		return ServiceInfo{}, fmt.Errorf("redpanda status for redpanda %s is not a RedpandaObservedStateSnapshot", protConvName)
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
		return ServiceInfo{}, fmt.Errorf("connection status for connection %s is not a ConnectionObservedState", protConvName)
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
			return ServiceInfo{}, fmt.Errorf("read dataflowcomponent status for dataflowcomponent %s is not a DataflowComponentObservedState", protConvName)
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
			return ServiceInfo{}, fmt.Errorf("write dataflowcomponent status for dataflowcomponent %s is not a DataflowComponentObservedState", protConvName)
		}

		dfcWriteFSMState, err = p.dataflowComponentManager.GetCurrentFSMState(underlyingDFCWriteName)
		if err != nil {
			return ServiceInfo{}, fmt.Errorf("failed to get write dataflowcomponent FSM state: %w", err)
		}
	}

	// Error if both DFCs are missing
	if !dfcReadExists && !dfcWriteExists {
		return ServiceInfo{}, fmt.Errorf("neither read nor write dataflowcomponent exists for protocolconverter %s", protConvName)
	}

	return ServiceInfo{
		ConnectionObservedState:             connectionObservedState,
		ConnectionFSMState:                  connectionFSMState,
		DataflowComponentReadObservedState:  dfcReadObservedState,
		DataflowComponentReadFSMState:       dfcReadFSMState,
		DataflowComponentWriteObservedState: dfcWriteObservedState,
		DataflowComponentWriteFSMState:      dfcWriteFSMState,
		RedpandaObservedState:               redpandaObservedState,
		RedpandaFSMState:                    redpandaFSMState,
	}, nil
}

// AddToManager registers a new protocolconverter in the Connection & DFC service.
func (p *ProtocolConverterService) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime,
	protConvName string,
) error {
	if p.connectionManager == nil {
		return errors.New("connection manager not initialized")
	}

	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	p.logger.Infof("Adding ProtocolConverter %s", protConvName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	underlyingConnectionName := p.getUnderlyingConnectionName(protConvName)
	underlyingDFCReadName := p.getUnderlyingDFCReadName(protConvName)
	underlyingDFCWriteName := p.getUnderlyingDFCWriteName(protConvName)

	// Check if the connection already exists in our configs
	for _, existingConfig := range p.connectionConfig {
		if existingConfig.Name == underlyingConnectionName {
			return ErrServiceAlreadyExists
		}
	}

	connServiceConfig := cfg.ConnectionServiceConfig
	dfcReadServiceConfig := cfg.DataflowComponentReadServiceConfig
	dfcWriteServiceConfig := cfg.DataflowComponentWriteServiceConfig

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

	return nil
}

// UpdateInManager modifies an existing protocolconverter configuration.
func (p *ProtocolConverterService) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime,
	protConvName string,
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

	p.logger.Infof("Updating protocolconverter %s", protConvName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the connectionconfig exists
	foundConn := false
	indexConn := -1
	for i, config := range p.connectionConfig {
		if config.Name == p.getUnderlyingConnectionName(protConvName) {
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
		if config.Name == p.getUnderlyingDFCReadName(protConvName) {
			foundReadDFC = true
			indexReadDFC = i
			break
		}
	}

	foundWriteDFC := false
	indexWriteDFC := -1
	for i, config := range p.dataflowComponentConfig {
		if config.Name == p.getUnderlyingDFCWriteName(protConvName) {
			foundWriteDFC = true
			indexWriteDFC = i
			break
		}
	}

	// either read or write dfc must exist
	if !foundReadDFC && !foundWriteDFC {
		return ErrServiceNotExist
	}

	connConfig := cfg.ConnectionServiceConfig
	dfcReadConfig := cfg.DataflowComponentReadServiceConfig
	dfcWriteConfig := cfg.DataflowComponentWriteServiceConfig

	// Create a config.ConnectionConfig that wraps the ConnectionServiceConfig
	connCurrentDesiredState := p.connectionConfig[indexConn].DesiredFSMState
	p.connectionConfig[indexConn] = config.ConnectionConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            p.getUnderlyingConnectionName(protConvName),
			DesiredFSMState: connCurrentDesiredState,
		},
		ConnectionServiceConfig: connConfig,
	}

	// Create a config.DataflowComponentConfig that wraps the DataflowComponentServiceConfig
	if foundReadDFC {
		dfcCurrentDesiredState := p.dataflowComponentConfig[indexReadDFC].DesiredFSMState
		p.dataflowComponentConfig[indexReadDFC] = config.DataFlowComponentConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            p.getUnderlyingDFCReadName(protConvName),
				DesiredFSMState: dfcCurrentDesiredState,
			},
			DataFlowComponentServiceConfig: dfcReadConfig,
		}
	}

	if foundWriteDFC {
		dfcCurrentDesiredState := p.dataflowComponentConfig[indexWriteDFC].DesiredFSMState
		p.dataflowComponentConfig[indexWriteDFC] = config.DataFlowComponentConfig{
			FSMInstanceConfig: config.FSMInstanceConfig{
				Name:            p.getUnderlyingDFCWriteName(protConvName),
				DesiredFSMState: dfcCurrentDesiredState,
			},
			DataFlowComponentServiceConfig: dfcWriteConfig,
		}
	}

	return nil
}

// RemoveFromManager deletes a protocolconverter configuration.
func (p *ProtocolConverterService) RemoveFromManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) error {
	if p.connectionManager == nil {
		return errors.New("connection manager not initialized")
	}

	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	p.logger.Infof("Removing dataflowcomponent %s", protConvName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	connectionName := p.getUnderlyingConnectionName(protConvName)
	dfcReadName := p.getUnderlyingDFCReadName(protConvName)
	dfcWriteName := p.getUnderlyingDFCWriteName(protConvName)

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

// Start starts a ProtocolConverter
// Expects protConvName (e.g. "protocolconverter-myservice") as defined in the UMH config
// Starts DFCs only when they are not empty
func (p *ProtocolConverterService) StartProtocolConverter(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
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

	connectionName := p.getUnderlyingConnectionName(protConvName)
	dfcReadName := p.getUnderlyingDFCReadName(protConvName)
	dfcWriteName := p.getUnderlyingDFCWriteName(protConvName)

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

	// Find and update our cached config
	dfcReadFound := false
	dfcWriteFound := false
	for i, config := range p.dataflowComponentConfig {
		if config.Name == dfcReadName {
			// Only start the DFC, if it has been configured
			// We check benthos.input > 0 while setting dfcReadFound regardless because we distinguish
			// between two cases: (1) service config missing entirely (return error), vs (2) service
			// config exists but is unconfigured/empty (set to stopped state). This defensive programming
			// handles the edge case of calling start before AddToManager and gracefully manages
			// misconfigured services, consistent with other FSM patterns across the codebase.
			if len(p.dataflowComponentConfig[i].DataFlowComponentServiceConfig.BenthosConfig.Input) > 0 {
				p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateActive
			} else {
				p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			}
			dfcReadFound = true
			break
		}
	}

	for i, config := range p.dataflowComponentConfig {
		if config.Name == dfcWriteName {
			// Only start the DFC, if it has been configured
			if len(p.dataflowComponentConfig[i].DataFlowComponentServiceConfig.BenthosConfig.Input) > 0 {
				p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateActive
			} else {
				p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
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

// Stop stops a ProtocolConverter
// Expects protConvName (e.g. "protocolconverter-myservice") as defined in the UMH config
func (p *ProtocolConverterService) StopProtocolConverter(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
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

	connectionName := p.getUnderlyingConnectionName(protConvName)
	dfcReadName := p.getUnderlyingDFCReadName(protConvName)
	dfcWriteName := p.getUnderlyingDFCWriteName(protConvName)

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

	// Find and update our cached config
	dfcReadFound := false
	dfcWriteFound := false
	for i, config := range p.dataflowComponentConfig {
		if config.Name == dfcReadName {
			p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			dfcReadFound = true
			break
		}
	}

	for i, config := range p.dataflowComponentConfig {
		if config.Name == dfcWriteName {
			p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			dfcWriteFound = true
			break
		}
	}

	if !dfcReadFound && !dfcWriteFound {
		return ErrServiceNotExist
	}

	return nil
}

// ReconcileManager synchronizes all protocolconverters on each tick.
// It delegates to the ProtocolConverter service's reconciliation function,
// which ensures that the actual state matches the desired state
// for all services.
//
// This should be called periodically by a control loop.
func (p *ProtocolConverterService) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
	tick uint64,
) (error, bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentProtocolConverterService, "ReconcileManager", time.Since(start))
	}()
	p.logger.Debugf("Reconciling protocolconverter manager at tick %d", tick)

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
			}},
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
func (p *ProtocolConverterService) ServiceExists(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) bool {
	if ctx.Err() != nil {
		return false
	}

	connectionName := p.getUnderlyingConnectionName(protConvName)
	dfcReadName := p.getUnderlyingDFCReadName(protConvName)
	dfcWriteName := p.getUnderlyingDFCWriteName(protConvName)

	// Check if the actual service exists
	connExists := p.connectionService.ServiceExists(ctx, filesystemService, connectionName)
	dfcReadExists := p.dataflowComponentService.ServiceExists(ctx, filesystemService, dfcReadName)
	dfcWriteExists := p.dataflowComponentService.ServiceExists(ctx, filesystemService, dfcWriteName)

	// if one of the services doesn't exist we should return that
	return connExists && (dfcReadExists || dfcWriteExists)
}

// ForceRemove removes a ProtocolConverter from the Connection & DFC manager
// Expects protConvName (e.g. "protocolconverter-myservice") as defined in the UMH config
func (c *ProtocolConverterService) ForceRemoveProtocolConverter(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	connectionName := c.getUnderlyingConnectionName(protConvName)
	dfcReadName := c.getUnderlyingDFCReadName(protConvName)
	dfcWriteName := c.getUnderlyingDFCWriteName(protConvName)

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
