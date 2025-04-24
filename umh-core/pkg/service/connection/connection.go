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

package connection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"go.uber.org/zap"
)

// IConnectionService abstracts "is the remote asset reachable?" functionality
// using Nmap for checking connectivity. It provides methods for managing the
// lifecycle of connections and monitoring their health.
type IConnectionService interface {
	// GenerateNmapConfigForConnection generates a Nmap config for a given connection
	GenerateNmapConfigForConnection(connectionConfig *connectionserviceconfig.ConnectionServiceConfig, connectionName string) (nmapserviceconfig.NmapServiceConfig, error)

	// GetConfig returns the actual Connection config from the Nmap service
	GetConfig(ctx context.Context, filesystemService filesystem.Service, connectionName string) (connectionserviceconfig.ConnectionServiceConfig, error)

	// Status returns information about the connection health for the specified connection.
	Status(ctx context.Context, fs filesystem.Service, connName string, tick uint64) (ServiceInfo, error)

	// AddConnection registers a new connection in the Nmap service.
	AddConnectionToNmapManager(ctx context.Context, fs filesystem.Service, cfg *connectionserviceconfig.ConnectionServiceConfig, connName string) error

	// UpdateConnection modifies an existing connection configuration.
	UpdateConnectionInNmapManager(ctx context.Context, fs filesystem.Service, cfg *connectionserviceconfig.ConnectionServiceConfig, connName string) error

	// RemoveConnection deletes a connection configuration.
	RemoveConnectionFromNmapManager(ctx context.Context, fs filesystem.Service, connName string) error

	// StartConnection begins the monitoring of a connection.
	StartConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// StopConnection stops the monitoring of a connection.
	StopConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// ForceRemoveConnection removes a Connection instance
	ForceRemoveConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// ServiceExists checks if a connection with the given name exists.
	ServiceExists(ctx context.Context, fs filesystem.Service, connName string) bool

	// ReconcileManager synchronizes all connections on each tick.
	ReconcileManager(ctx context.Context, fs filesystem.Service, tick uint64) (error, bool)
}

// ServiceInfo contains information about a Connection service
type ServiceInfo struct {
	// NmapObservedState contains information about the Benthos service
	NmapObservedState nmapfsm.NmapObservedState
	// NmapFSMState contains the current state of the Benthos FSM
	NmapFSMState string
	// IsHealthy is true only when the connection‑monitoring probe
	// is in a steady, reliable state (Nmap FSM = "open").
	IsHealthy bool
}

// ConnectionService is the default implementation of the IConnectionService interface
type ConnectionService struct {
	logger      *zap.SugaredLogger
	nmapManager *nmapfsm.NmapManager
	nmapService nmap.INmapService
	nmapConfigs []config.NmapConfig
}

// ConnectionServiceOption is a function that configures a ConnectionService.
// This follows the functional options pattern for flexible configuration.
type ConnectionServiceOption func(*ConnectionService)

func WithNmapService(nmapService nmap.INmapService) ConnectionServiceOption {
	return func(c *ConnectionService) { c.nmapService = nmapService }
}

func WithNmapManager(mgr *nmapfsm.NmapManager) ConnectionServiceOption {
	return func(c *ConnectionService) { c.nmapManager = mgr }
}

// NewDefaultConnectionService creates a new ConnectionService with default options.
// It initializes the logger, recent scans cache, and sets default values.
func NewDefaultConnectionService(connectionName string, opts ...ConnectionServiceOption) *ConnectionService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentConnectionService, connectionName)
	service := &ConnectionService{
		logger:      logger.For(managerName),
		nmapManager: nmapfsm.NewNmapManager(managerName),
		nmapService: nmap.NewDefaultNmapService(connectionName),
		nmapConfigs: []config.NmapConfig{},
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getBenthosName converts a connectionName to its Nmap service name
func (c *ConnectionService) getNmapName(connectionName string) string {
	return fmt.Sprintf("connection-%s", connectionName)
}

// GenerateNmapConfigConnection generates a nmap config for a given connection
func (c *ConnectionService) GenerateNmapConfigForConnection(connectionConfig *connectionserviceconfig.ConnectionServiceConfig, connectionName string) (nmapserviceconfig.NmapServiceConfig, error) {
	if connectionConfig == nil {
		return nmapserviceconfig.NmapServiceConfig{}, fmt.Errorf("connection config is nil")
	}

	// Convert Connection config to Nmap service config
	return connectionConfig.GetNmapServiceConfig(), nil
}

// GetConfig returns the actual Connection config from the nmap service
func (c *ConnectionService) GetConfig(
	ctx context.Context,
	filesystemService filesystem.Service,
	connectionName string,
) (connectionserviceconfig.ConnectionServiceConfig, error) {
	if ctx.Err() != nil {
		return connectionserviceconfig.ConnectionServiceConfig{}, ctx.Err()
	}

	nmapName := c.getNmapName(connectionName)

	// Get the Nmap config
	nmapCfg, err := c.nmapService.GetConfig(ctx, filesystemService, nmapName)
	if err != nil {
		return connectionserviceconfig.ConnectionServiceConfig{}, fmt.Errorf("failed to get nmap config: %w", err)
	}

	// Convert Nmap config to Connection config
	return connectionserviceconfig.FromNmapServiceConfig(nmapCfg), nil
}

// Status returns information about the connection health for the specified connection.
// It queries the underlying Nmap service and then enhances the result with
// additional context like flakiness detection based on historical data.
func (c *ConnectionService) Status(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
	tick uint64,
) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	// First, check if the service exists in the nmap manager
	instance, exists := c.nmapManager.GetInstance(connName)
	if !exists {
		c.logger.Debugf("Service %s not found in Nmap manager", connName)
		return ServiceInfo{}, ErrServiceNotExist
	}
	nmapInstanceState := instance.GetCurrentFSMState()

	// Get status from Nmap service
	nmapStatus, err := c.nmapManager.GetLastObservedState(connName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get nmap status for connection %s: %w", connName, err)
	}

	nmapStatusObservedState, ok := nmapStatus.(nmapfsm.NmapObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("nmap status for connection %s is not a NmapObservedState", connName)
	}

	nmapFSMState, err := c.nmapManager.GetCurrentFSMState(connName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get nmap FSM state: %w", err)
	}

	IsHealthy := false
	// if it is any other state other than running (such as stopped, starting, degraded, etc.)
	// it is not healthy
	if nmapfsm.IsRunningState(nmapInstanceState) && nmapInstanceState != nmapfsm.OperationalStateDegraded {
		IsHealthy = true
	}

	return ServiceInfo{
		NmapObservedState: nmapStatusObservedState,
		NmapFSMState:      nmapFSMState,
		IsHealthy:         IsHealthy,
	}, nil
}

// AddConnectionToNmapManager registers a new connection in the Nmap service.
// The connName is a unique identifier used to reference this connection.
// The cfg parameter specifies the target hostname, port, and protocol.
func (c *ConnectionService) AddConnectionToNmapManager(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionserviceconfig.ConnectionServiceConfig,
	connectionName string,
) error {
	if c.nmapManager == nil {
		return errors.New("nmap manager not initialized")
	}
	c.logger.Infof("Adding connection %s", connectionName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	nmapName := c.getNmapName(connectionName)
	fmt.Println(nmapName)

	// Check if the connection already exists in our configs
	for _, existingConfig := range c.nmapConfigs {
		if existingConfig.Name == nmapName {
			return ErrServiceAlreadyExists
		}
	}

	// Convert our connection config to Nmap service config
	nmapServiceConfig, err := c.GenerateNmapConfigForConnection(cfg, connectionName)
	if err != nil {
		return fmt.Errorf("failed to generate nmap config: %v", err)
	}

	// Create a config.NmapConfig that wraps the NmapServiceConfig
	nmapConfig := config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            nmapName,
			DesiredFSMState: nmapfsm.OperationalStateOpen,
		},
		NmapServiceConfig: nmapServiceConfig,
	}

	// Add the configuration to the list
	c.nmapConfigs = append(c.nmapConfigs, nmapConfig)

	return nil
}

// UpdateConnectionInNmapManager modifies an existing connection configuration.
// This can be used to change the target hostname, port, or protocol.
func (c *ConnectionService) UpdateConnectionInNmapManager(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionserviceconfig.ConnectionServiceConfig,
	connectionName string,
) error {
	if c.nmapManager == nil {
		return errors.New("nmap manager not initialized")
	}
	c.logger.Infof("Updating connection %s", connectionName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	nmapName := c.getNmapName(connectionName)

	// Check if the config exists
	found := false
	index := -1
	for i, config := range c.nmapConfigs {
		if config.Name == nmapName {
			found = true
			index = i
			break
		}
	}
	if !found {
		return ErrServiceNotExist
	}

	// Convert our connection config to Nmap service config
	nmapCfg, err := c.GenerateNmapConfigForConnection(cfg, connectionName)
	if err != nil {
		return fmt.Errorf("failed to generate nmap config: %w", err)
	}

	// Create a config.NmapConfig that wraps the NmapServiceConfig
	currentDesiredState := c.nmapConfigs[index].DesiredFSMState
	c.nmapConfigs[index] = config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            connectionName,
			DesiredFSMState: currentDesiredState,
		},
		NmapServiceConfig: nmapCfg,
	}

	return nil
}

// RemoveConnectionFromNmapManager deletes a connection configuration.
// This stops monitoring the connection and removes all configuration.
func (c *ConnectionService) RemoveConnectionFromNmapManager(
	ctx context.Context,
	fs filesystem.Service,
	connectionName string,
) error {
	if c.nmapManager == nil {
		return errors.New("nmap manager not initialized")
	}

	c.logger.Infof("Removing connection %s", connectionName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	nmapName := c.getNmapName(connectionName)

	// Remove the nmap config from the list of nmap configs
	// so that the NmapManager will stop the service
	// The NmapManager itself will handle a graceful shutdown of the underlying Nmap service
	found := false
	for i, config := range c.nmapConfigs {
		if config.Name == nmapName {
			c.nmapConfigs = append(c.nmapConfigs[:i], c.nmapConfigs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// StartConnection starts a Connection
// Expects nmapName (e.g. "connection-myservice") as defined in the UMH config
func (c *ConnectionService) StartConnection(
	ctx context.Context,
	filesystemService filesystem.Service,
	connectionName string,
) error {
	if c.nmapManager == nil {
		return errors.New("nmap manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	nmapName := c.getNmapName(connectionName)

	// Find and update our cached config
	found := false
	for i, config := range c.nmapConfigs {
		if config.Name == nmapName {
			c.nmapConfigs[i].DesiredFSMState = nmapfsm.OperationalStateOpen
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// StopConnection stops a Connection
// Expects nmapName (e.g. "connection-myservice") as defined in the UMH config
func (c *ConnectionService) StopConnection(
	ctx context.Context,
	filesystemService filesystem.Service,
	connectionName string,
) error {
	if c.nmapManager == nil {
		return errors.New("nmap manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	nmapName := c.getNmapName(connectionName)

	// Find and update our cached config
	found := false
	for i, config := range c.nmapConfigs {
		if config.Name == nmapName {
			c.nmapConfigs[i].DesiredFSMState = nmapfsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// ReconcileManager synchronizes all connections on each tick.
// It delegates to the Nmap service's reconciliation function,
// which ensures that the actual state matches the desired state
// for all services.
func (c *ConnectionService) ReconcileManager(
	ctx context.Context,
	fs filesystem.Service,
	tick uint64,
) (error, bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentConnectionService, "ReconcileManager", time.Since(start))
	}()
	c.logger.Debugf("Reconciling connection manager at tick %d", tick)

	if c.nmapManager == nil {
		return errors.New("nmap manager not initilized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Use the nmapManager's Reconcile method
	return c.nmapManager.Reconcile(ctx, fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{
			Internal: config.InternalConfig{
				Nmap: c.nmapConfigs,
			}},
		Tick: tick,
	}, fs)
}

// ServiceExists checks if a connection service with the given name exists.
func (c *ConnectionService) ServiceExists(
	ctx context.Context,
	fs filesystem.Service,
	connectionName string,
) bool {
	if ctx.Err() != nil {
		return false
	}

	nmapName := c.getNmapName(connectionName)

	// check for already existing configs
	for _, config := range c.nmapConfigs {
		if config.Name == nmapName {
			return true
		}
	}

	// Check if the actual service exists
	return c.nmapService.ServiceExists(ctx, fs, nmapName)
}

// ForceRemoveConnection removes a Connection from the Nmap manager
// Expects nmapName (e.g. "connection-myservice") as defined in the UMH config
func (c *ConnectionService) ForceRemoveDataFlowComponent(ctx context.Context, filesystemService filesystem.Service, connectionName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// force remove from Nmap manager
	return c.nmapService.ForceRemoveNmap(ctx, filesystemService, c.getNmapName(connectionName))
}
