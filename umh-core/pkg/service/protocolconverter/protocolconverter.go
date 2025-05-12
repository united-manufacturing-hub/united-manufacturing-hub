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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
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

// IProtocolConverterService is the interface for managing DataFlowComponent services
type IProtocolConverterService interface {
	// GenerateConfig generates a connection & dfc config for a given protocolconverter
	GenerateConfig(protConvConfig *protocolconverterserviceconfig.ProtocolConverterServiceConfig, protConvName string) (connectionserviceconfig.ConnectionServiceConfig, dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error)

	// GetConfig returns the actual ProtocolConverter serviceconfig from the underlying services
	GetConfig(ctx context.Context, filesystemService filesystem.Service, protConvName string) (protocolconverterserviceconfig.ProtocolConverterServiceConfig, error)

	// Status checks the status of a ProtocolConverter service
	Status(ctx context.Context, services serviceregistry.Provider, connName string, tick uint64) (ServiceInfo, error)

	// AddToManager adds a ProtocolConverter to the Connection & DFC manager
	AddToManager(ctx context.Context, filesystemService filesystem.Service, protConvCfg *protocolconverterserviceconfig.ProtocolConverterServiceConfig, protConvName string) error

	// UpdateInManager updates an existing ProtocolConverter in the Connection & DFC manager
	UpdateInManager(ctx context.Context, filesystemService filesystem.Service, protConvCfg *protocolconverterserviceconfig.ProtocolConverterServiceConfig, protConvName string) error

	// RemoveFromManager removes a ProtocolConverter from the Connection & DFC manager
	RemoveFromManager(ctx context.Context, filesystemService filesystem.Service, protConvName string) error

	// Start starts a ProtocolConverter
	Start(ctx context.Context, filesystemService filesystem.Service, protConvName string) error

	// Stop stops a ProtocolConverter
	Stop(ctx context.Context, filesystemService filesystem.Service, protConvName string) error

	// ForceRemove removes a ProtocolConverter from the Connetion & DFC manager
	ForceRemove(ctx context.Context, filesystemService filesystem.Service, protConvName string) error

	// ServiceExists checks if a ProtocolConverter service exists
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, protConvName string) bool

	// ReconcileManager reconciles the ProtocolConverter manager with the actual state
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
}

// ServiceInfo holds information about the ProtocolConverters underlying health states.
type ServiceInfo struct {
	// Observed states from the underlying fsm-components
	ConnectionObservedState        connectionfsm.ConnectionObservedState
	DataflowComponentObservedState dfcfsm.DataflowComponentObservedState
	RedpandaObservedState          redpandafsm.RedpandaObservedState

	// Current states of the underlying fsm-components
	ConnectionFSMState        string
	DataflowComponentFSMState string
	RedpandaFSMState          string

	// LastChange stores the tick when the status last changed.
	LastChange uint64

	// StatusReason is the reason for the current state
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
	dataflowComponentManager *dfcfsm.DataflowComponentManager
	dataflowComponentService dfc.IDataFlowComponentService
	dataflowComponentConfig  []config.DataFlowComponentConfig

	// Redpanda
	redpandaManager *redpandafsm.RedpandaManager
}

// ProtocolConverterServiceOption is a function that configures a ConnectionService.
// This follows the functional options pattern for flexible configuration.
type ProtocolConverterServiceOption func(*ProtocolConverterService)

func WithUnderlyingServices(
	connService connection.IConnectionService,
	dfcService dfc.IDataFlowComponentService,
) ProtocolConverterServiceOption {
	return func(p *ProtocolConverterService) {
		p.connectionService = connService
		p.dataflowComponentService = dfcService
	}
}

func WithUnderlyingManagers(
	connMgr *connectionfsm.ConnectionManager,
	dfcMgr *dfcfsm.DataflowComponentManager,
) ProtocolConverterServiceOption {
	return func(c *ProtocolConverterService) {
		c.connectionManager = connMgr
		c.dataflowComponentManager = dfcMgr

	}
}

// NewDefaultProtocolConverterService creates a new ConnectionService with default options.
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

// getUnderlyingName converts a protConvName to its underlying service name
func (p *ProtocolConverterService) getUnderlyingName(protConvName string) string {
	return fmt.Sprintf("protocolconverter-%s", protConvName)
}

func (p *ProtocolConverterService) generateProtocolConverterYaml(config *protocolconverterserviceconfig.ProtocolConverterServiceConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}
	return protocolconverterserviceconfig.RenderProtocolConverterYAML(config.ConnectionServiceConfig, config.DataflowComponentServiceConfig)
}

// GenerateConfig generates a dfc & connection config for a given protocolconverter
func (p *ProtocolConverterService) GenerateConfig(
	protConvConfig *protocolconverterserviceconfig.ProtocolConverterServiceConfig,
	protConvName string,
) (
	*connectionserviceconfig.ConnectionServiceConfig,
	*dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	error,
) {
	if protConvConfig == nil {
		return &connectionserviceconfig.ConnectionServiceConfig{},
			&dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
			fmt.Errorf("protocolConverter config is nil")
	}

	generatedConfig, err := p.generateProtocolConverterYaml(protConvConfig)
	if err != nil {
		return &connectionserviceconfig.ConnectionServiceConfig{},
			&dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
			fmt.Errorf("failed to generate ProtocolConverterServiceConfig: %w", err)
	}

	protConvConfig.DataflowComponentServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Output: map[string]any{"uns": generatedConfig},
		},
	}

	return protConvConfig.GetConnectionServiceConfig(), protConvConfig.GetDFCServiceConfig(), nil
}

// GetConfig returns the actual ProtocolConverter config from the protocolConverter service
func (p *ProtocolConverterService) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) (protocolconverterserviceconfig.ProtocolConverterServiceConfig, error) {
	if ctx.Err() != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfig{}, ctx.Err()
	}

	serviceName := p.getUnderlyingName(protConvName)

	// Get the Connection config
	connConfig, err := p.connectionService.GetConfig(ctx, filesystemService, serviceName)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfig{}, fmt.Errorf("failed to get connection config: %w", err)
	}

	dfcConfig, err := p.dataflowComponentService.GetConfig(ctx, filesystemService, serviceName)
	if err != nil {
		return protocolconverterserviceconfig.ProtocolConverterServiceConfig{}, fmt.Errorf("failed to get connection config: %w", err)
	}

	// Convert Connection & DFC config to ProtocolConverter config
	return protocolconverterserviceconfig.FromConnectionAndDFCServiceConfig(connConfig, dfcConfig), nil
}

// Status returns information about the connection health for the specified connection.
// It queries the underlying Connection service and then enhances the result with
// additional context like flakiness detection based on historical data.
func (p *ProtocolConverterService) Status(
	ctx context.Context,
	services serviceregistry.Provider,
	protConvName string,
	tick uint64,
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

	underlyingName := p.getUnderlyingName(protConvName)

	// get last observed states
	connectionStatus, err := p.connectionManager.GetLastObservedState(underlyingName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get connection observed state: %w", err)
	}

	dfcStatus, err := p.dataflowComponentManager.GetLastObservedState(underlyingName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get dataflowcomponent observed state: %w", err)
	}

	redpandaStatus, err := p.redpandaManager.GetLastObservedState(underlyingName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get redpanda observed state: %w", err)
	}

	// check observed state types
	connectionObservedState, ok := connectionStatus.(connectionfsm.ConnectionObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("connection status for connection %s is not a ConnectionObservedState", protConvName)
	}

	dfcObservedState, ok := dfcStatus.(dfcfsm.DataflowComponentObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("dataflowcomponent status for dataflowcomponent %s is not a DataflowComponentObservedState", protConvName)
	}

	redpandaObservedState, ok := redpandaStatus.(redpandafsm.RedpandaObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("redpanda status for redpanda %s is not a RedpandaObservedState", protConvName)
	}

	// get current fsm states
	connectionFSMState, err := p.connectionManager.GetCurrentFSMState(protConvName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get connection FSM state: %w", err)
	}

	dfcFSMState, err := p.dataflowComponentManager.GetCurrentFSMState(protConvName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get dataflowcomponent FSM state: %w", err)
	}

	redpandaFSMState, err := p.dataflowComponentManager.GetCurrentFSMState(protConvName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get redpanda FSM state: %w", err)
	}

	return ServiceInfo{
		ConnectionObservedState:        connectionObservedState,
		ConnectionFSMState:             connectionFSMState,
		DataflowComponentObservedState: dfcObservedState,
		DataflowComponentFSMState:      dfcFSMState,
		RedpandaObservedState:          redpandaObservedState,
		RedpandaFSMState:               redpandaFSMState,
		LastChange:                     tick,
	}, nil
}

// AddToManager registers a new protocolconverter in the Connection & DFC service.
func (p *ProtocolConverterService) AddToManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *protocolconverterserviceconfig.ProtocolConverterServiceConfig,
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

	underlyingName := p.getUnderlyingName(protConvName)

	// Check if the connection already exists in our configs
	for _, existingConfig := range p.connectionConfig {
		if existingConfig.Name == protConvName {
			return ErrServiceAlreadyExists
		}
	}

	// Generate Connection & DFC Serviceconfigs from ProtocolConverter config
	connServiceConfig, dfcServiceConfig, err := p.GenerateConfig(cfg, protConvName)
	if err != nil {
		return fmt.Errorf("failed to generate serviceconfigs: %v", err)
	}

	// Create a config.ConnectionConfig that wraps the NmapServiceConfig
	connectionConfig := config.ConnectionConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingName,
			DesiredFSMState: connectionfsm.OperationalStateUp,
		},
		ConnectionServiceConfig: *connServiceConfig,
	}

	dfcConfig := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            underlyingName,
			DesiredFSMState: dfcfsm.OperationalStateActive,
		},
		DataFlowComponentServiceConfig: *dfcServiceConfig,
	}

	// Add the configurations to the lists
	p.connectionConfig = append(p.connectionConfig, connectionConfig)
	p.dataflowComponentConfig = append(p.dataflowComponentConfig, dfcConfig)

	return nil
}

// UpdateInManager modifies an existing protocolconverter configuration.
func (p *ProtocolConverterService) UpdateInManager(
	ctx context.Context,
	filesystemService filesystem.Service,
	cfg *protocolconverterserviceconfig.ProtocolConverterServiceConfig,
	protConvName string,
) error {
	if p.connectionManager == nil {
		return errors.New("connection manager not initialized")
	}
	if p.dataflowComponentManager == nil {
		return errors.New("dataflowcomponent manager not initialized")
	}

	p.logger.Infof("Updating protocolconverter %s", protConvName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	underlyingName := p.getUnderlyingName(protConvName)

	// Check if the connectionconfig exists
	foundConn := false
	indexConn := -1
	for i, config := range p.connectionConfig {
		if config.Name == underlyingName {
			foundConn = true
			indexConn = i
			break
		}
	}
	// Check if the dfcconfig exists
	foundDFC := false
	indexDFC := -1
	for i, config := range p.dataflowComponentConfig {
		if config.Name == underlyingName {
			foundDFC = true
			indexDFC = i
			break
		}
	}

	if !foundConn || !foundDFC {
		return ErrServiceNotExist
	}

	// Convert our connection config to Connection service config
	connConfig, dfcConfig, err := p.GenerateConfig(cfg, protConvName)
	if err != nil {
		return fmt.Errorf("failed to generate configs: %w", err)
	}

	// Create a config.ConnectionConfig that wraps the ConnectionServiceConfig
	connCurrentDesiredState := p.connectionConfig[indexConn].DesiredFSMState
	p.connectionConfig[indexConn] = config.ConnectionConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            protConvName,
			DesiredFSMState: connCurrentDesiredState,
		},
		ConnectionServiceConfig: *connConfig,
	}

	// Create a config.DataflowComponentConfig that wraps the DataflowComponentServiceConfig
	dfcCurrentDesiredState := p.connectionConfig[indexConn].DesiredFSMState
	p.dataflowComponentConfig[indexDFC] = config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            protConvName,
			DesiredFSMState: dfcCurrentDesiredState,
		},
		DataFlowComponentServiceConfig: *dfcConfig,
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

	underlyingName := p.getUnderlyingName(protConvName)

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
	p.connectionConfig = connSliceRemoveByName(p.connectionConfig, underlyingName)
	p.dataflowComponentConfig = dfcSliceRemoveByName(p.dataflowComponentConfig, underlyingName)

	//--------------------------------------------
	// 2) is the child FSM still alive?
	//--------------------------------------------
	if inst, ok := p.connectionManager.GetInstance(underlyingName); ok {
		return fmt.Errorf("%w: Connection instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	if inst, ok := p.dataflowComponentManager.GetInstance(underlyingName); ok {
		return fmt.Errorf("%w: DataflowComponent instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	return nil
}

// Start starts a ProtocolConverter
// Expects protConvName (e.g. "protocolconverter-myservice") as defined in the UMH config
func (p *ProtocolConverterService) Start(
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

	underlyingName := p.getUnderlyingName(protConvName)

	// Find and update our cached config
	connFound := false
	for i, config := range p.connectionConfig {
		if config.Name == underlyingName {
			p.connectionConfig[i].DesiredFSMState = connectionfsm.OperationalStateUp
			connFound = true
			break
		}
	}

	// Find and update our cached config
	dfcFound := false
	for i, config := range p.connectionConfig {
		if config.Name == underlyingName {
			p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateActive
			dfcFound = true
			break
		}
	}

	if !connFound || !dfcFound {
		return ErrServiceNotExist
	}

	return nil
}

// Stop stops a ProtocolConverter
// Expects protConvName (e.g. "protocolconverter-myservice") as defined in the UMH config
func (p *ProtocolConverterService) Stop(
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

	underlyingName := p.getUnderlyingName(protConvName)

	// Find and update our cached config
	connFound := false
	for i, config := range p.connectionConfig {
		if config.Name == underlyingName {
			p.connectionConfig[i].DesiredFSMState = connectionfsm.OperationalStateStopped
			connFound = true
			break
		}
	}

	// Find and update our cached config
	dfcFound := false
	for i, config := range p.connectionConfig {
		if config.Name == underlyingName {
			p.dataflowComponentConfig[i].DesiredFSMState = dfcfsm.OperationalStateStopped
			dfcFound = true
			break
		}
	}

	if !connFound || !dfcFound {
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
			Internal: config.InternalConfig{
				Connection: p.connectionConfig,
			}},
		Tick: tick,
	}, services)

	if err != nil {
		return err, dfcReconciled
	}

	return nil, connReconciled || dfcReconciled
}

// ServiceExists checks if a connection with the given name exists.
// Used by the FSM to determine appropriate transitions.
//
// Returns true if the connection exists, false otherwise.
func (p *ProtocolConverterService) ServiceExists(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) bool {
	if ctx.Err() != nil {
		return false
	}
	underlyingName := p.getUnderlyingName(protConvName)

	// Check if the actual service exists
	connExists := p.connectionService.ServiceExists(ctx, filesystemService, underlyingName)
	dfcExists := p.dataflowComponentService.ServiceExists(ctx, filesystemService, underlyingName)

	// if one of the services doesn't exist we should return that
	return connExists && dfcExists
}

// ForceRemove removes a ProtocolConverter from the Connection & DFC manager
// Expects protConvName (e.g. "protocolconverter-myservice") as defined in the UMH config
func (c *ProtocolConverterService) ForceRemove(
	ctx context.Context,
	filesystemService filesystem.Service,
	protConvName string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	underlyingName := c.getUnderlyingName(protConvName)

	// force remove from Connection manager
	err := c.connectionService.ForceRemoveConnection(ctx, filesystemService, underlyingName)
	if err != nil {
		return err
	}

	// force remove from dataflowcomponent manager
	err = c.dataflowComponentService.ForceRemoveDataFlowComponent(ctx, filesystemService, underlyingName)
	if err != nil {
		return err
	}

	return nil
}
