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

// Package connection provides a service for managing and monitoring network
// connectivity to remote assets using Nmap as the underlying probe mechanism.
//
// # Key Concepts
//
// - Connections has a health indicators (flaky)
// - Flakiness detection requires multiple samples over time
//
// # Thread Safety
//
// The ConnectionService is designed to be accessed by a single goroutine.
// All operations that modify the configuration (Add/Update/Remove) should be
// followed by a call to ReconcileManager to apply the changes.

package connection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
	"go.uber.org/zap"
)

// IConnectionService abstracts "is the remote asset reachable?" functionality
// using Nmap for checking connectivity. It provides methods for managing the
// lifecycle of connections and monitoring their health.
//
//nolint:interfacebloat // Core connection service interface requires comprehensive methods for connection lifecycle management
type IConnectionService interface {
	// GenerateNmapConfigForConnection generates a Nmap config for a given connection
	GenerateNmapConfigForConnection(connectionConfig *connectionserviceconfig.ConnectionServiceConfig, connectionName string) (nmapserviceconfig.NmapServiceConfig, error)

	// GetConfig returns the actual Connection config from the Nmap service
	GetConfig(ctx context.Context, filesystemService filesystem.Service, connectionName string) (connectionserviceconfig.ConnectionServiceConfig, error)

	// Status returns information about the connection health for the specified connection.
	// The connName corresponds to the name defined in the configuration.
	// The tick parameter is used for reconciliation timing and to timestamp the result.
	//
	// Returns a ServiceInfo struct containing multiple indicators:
	// - IsFlaky: if the connection has been unstable in recent history
	// - NmapObservedState: the observed state of nmap
	// - NmapFSMState: the current state of the nmap fsm
	//
	// Errors:
	// - If the connection doesn't exist
	// - If there's an underlying Nmap error
	// - If the context is canceled
	Status(ctx context.Context, filesystemService filesystem.Service, connName string, tick uint64) (ServiceInfo, error)

	// AddConnectionToNmapManager registers a new connection in the Nmap Manager.
	// The connName is a unique identifier used to reference this connection.
	// The cfg parameter specifies the target hostname, port, and protocol.
	//
	// This method initializes the monitoring but does not start it automatically.
	// Call StartConnection to begin active monitoring.
	//
	// Note: This method only stores the config in a local slice and does not
	// immediately activate the connection. Call ReconcileManager after this
	// to apply the changes.
	//
	// Returns an error if the connection already exists or if registration fails.
	AddConnectionToNmapManager(ctx context.Context, filesystemService filesystem.Service, cfg *connectionserviceconfig.ConnectionServiceConfig, connName string) error

	// UpdateConnectionInNmapManager modifies an existing connection configuration.
	// This can be used to change the target hostname, port, or protocol.
	//
	// The service must exist already. The update takes effect on the next
	// monitor cycle and does not restart the monitoring process.
	//
	// Note: This method only updates the config in a local slice and does not
	// immediately apply the changes. Call ReconcileManager after this
	// to apply the changes.
	//
	// Returns an error if the connection doesn't exist or if update fails.
	UpdateConnectionInNmapManager(ctx context.Context, filesystemService filesystem.Service, cfg *connectionserviceconfig.ConnectionServiceConfig, connName string) error

	// RemoveConnectionFromNmapManager deletes a connection configuration.
	// This stops monitoring the connection and removes all configuration.
	//
	// Note: This method only removes the config from a local slice and does not
	// immediately apply the changes. Call ReconcileManager after this
	// to apply the changes.
	//
	// Returns an error if the connection doesn't exist or removal fails.
	RemoveConnectionFromNmapManager(ctx context.Context, filesystemService filesystem.Service, connName string) error

	// StartConnection begins the monitoring of a connection.
	// This initiates periodic scanning of the target using Nmap.
	//
	// Returns an error if the connection doesn't exist or start fails.
	StartConnection(ctx context.Context, filesystemService filesystem.Service, connName string) error

	// StopConnection stops the monitoring of a connection.
	// This halts scanning but retains the configuration for later restart.
	//
	// Returns an error if the connection doesn't exist or stop fails.
	StopConnection(ctx context.Context, filesystemService filesystem.Service, connName string) error

	// ForceRemoveConnection removes a Connection instance
	ForceRemoveConnection(ctx context.Context, filesystemService filesystem.Service, connName string) error

	// ServiceExists checks if a connection with the given name exists.
	// Used by the FSM to determine appropriate transitions.
	//
	// Returns true if the connection exists, false otherwise.
	ServiceExists(ctx context.Context, filesystemService filesystem.Service, connName string) bool

	// ReconcileManager synchronizes all connections on each tick.
	// This is typically called in a loop by the FSM system.
	//
	// Returns an error and a boolean indicating if reconciliation occurred.
	// The boolean is false if reconciliation was skipped (e.g., due to an error).
	ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool)
}

// ServiceInfo holds information about the connection's health status.
// It uses separate boolean flags for different health aspects instead of
// a single status value, making it easier to check specific conditions.
type ServiceInfo struct {
	// NmapFSMState contains the current state of the Nmap FSM
	NmapFSMState string
	// NmapObservedState contains information about the Nmap service
	NmapObservedState nmapfsm.NmapObservedState
	// LastChange stores the tick when the status last changed.
	// This timestamp uses the FSM tick counter rather than wall clock time
	// to ensure consistency with the FSM reconciliation system.
	LastChange uint64
	// IsFlaky indicates intermittent connectivity based on recent scan history.
	// A connection is considered flaky if it alternates between open/closed states
	// within the recent history window (default: last 60 scans).
	IsFlaky bool
}

// ConnectionService implements IConnectionService using Nmap as the underlying
// connectivity probe mechanism. It maintains a history of recent states to detect
// flaky connections and provides a higher-level abstraction over raw Nmap results.
type ConnectionService struct {
	nmapService      nmap.INmapService
	logger           *zap.SugaredLogger
	nmapManager      *nmapfsm.NmapManager
	recentNmapStates map[string][]string
	nmapConfigs      []config.NmapConfig
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
//
// Options can be passed to customize behavior:
// - WithNmapService: Set a custom service
// - WithNmapmanager: Set a custom manager
//
// Example:
//
//	service := NewDefaultConnectionService("my-connection-service")
//	// or with custom max recent scans
//	service := NewDefaultConnectionService("my-connection-service", WithMaxRecentScans(10))
func NewDefaultConnectionService(connectionName string, opts ...ConnectionServiceOption) *ConnectionService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentConnectionService, connectionName)
	service := &ConnectionService{
		logger:           logger.For(managerName),
		nmapManager:      nmapfsm.NewNmapManager(managerName),
		nmapService:      nmap.NewDefaultNmapService(connectionName),
		nmapConfigs:      []config.NmapConfig{},
		recentNmapStates: make(map[string][]string),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// getNmapName converts a connectionName to its Nmap service name.
func (c *ConnectionService) getNmapName(connectionName string) string {
	return "connection-" + connectionName
}

// GenerateNmapConfigConnection generates a nmap config for a given connection.
func (c *ConnectionService) GenerateNmapConfigForConnection(connectionConfig *connectionserviceconfig.ConnectionServiceConfig, connectionName string) (nmapserviceconfig.NmapServiceConfig, error) {
	if connectionConfig == nil {
		return nmapserviceconfig.NmapServiceConfig{}, errors.New("connection config is nil")
	}

	// Convert Connection config to Nmap service config
	return connectionConfig.GetNmapServiceConfig(), nil
}

// GetConfig returns the actual Connection config from the nmap service.
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
//
// The tick parameter is used both for timestamping the result and as part of
// the FSM reconciliation system's timing mechanism.
func (c *ConnectionService) Status(
	ctx context.Context,
	filesystemService filesystem.Service,
	connName string,
	tick uint64,
) (ServiceInfo, error) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(logger.ComponentConnectionService, connName+".Status", time.Since(start))
	}()

	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	if !c.ServiceExists(ctx, filesystemService, connName) {
		return ServiceInfo{}, ErrServiceNotExist
	}

	nmapName := c.getNmapName(connName)

	// Get status from Nmap service
	nmapStatus, err := c.nmapManager.GetLastObservedState(nmapName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get nmap observed state: %w", err)
	}

	nmapStatusObservedState, ok := nmapStatus.(nmapfsm.NmapObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("nmap status for connection %s is not a NmapObservedState", connName)
	}

	nmapFSMState, err := c.nmapManager.GetCurrentFSMState(nmapName)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get nmap FSM state: %w", err)
	}

	c.updateRecentScans(connName, nmapFSMState)

	return ServiceInfo{
		NmapObservedState: nmapStatusObservedState,
		NmapFSMState:      nmapFSMState,
		IsFlaky:           c.isConnectionFlaky(connName),
		LastChange:        tick,
	}, nil
}

// AddConnection registers a new connection in the Nmap service.
// The connName is a unique identifier used to reference this connection.
// The cfg parameter specifies the target hostname, port, and protocol.
//
// This method initializes the monitoring but does not start it automatically.
// Call StartConnection to begin active monitoring.
//
// Note: This method only stores the config in a local slice and does not
// immediately activate the connection. Call ReconcileManager after this
// to apply the changes.
//
// Returns an error if the connection already exists or if registration fails.
func (c *ConnectionService) AddConnectionToNmapManager(
	ctx context.Context,
	filesystemService filesystem.Service,
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

	// Check if the connection already exists in our configs
	for _, existingConfig := range c.nmapConfigs {
		if existingConfig.Name == nmapName {
			return ErrServiceAlreadyExists
		}
	}

	// Convert our connection config to Nmap service config
	nmapServiceConfig, err := c.GenerateNmapConfigForConnection(cfg, connectionName)
	if err != nil {
		return fmt.Errorf("failed to generate nmap config: %w", err)
	}

	// Create a config.NmapConfig that wraps the NmapServiceConfig
	nmapConfig := config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            nmapName,
			DesiredFSMState: nmapfsm.OperationalStateStopped,
		},
		NmapServiceConfig: nmapServiceConfig,
	}

	// Add the configuration to the list
	c.nmapConfigs = append(c.nmapConfigs, nmapConfig)

	return nil
}

// UpdateConnectionInNmapManager modifies an existing connection configuration.
// This can be used to change the target hostname, port, or protocol.
//
// The service must exist already. The update takes effect on the next
// monitor cycle and does not restart the monitoring process.
//
// Note: This method only updates the config in a local slice and does not
// immediately apply the changes. Call ReconcileManager after this
// to apply the changes.
//
// Returns an error if the connection doesn't exist or if update fails.
func (c *ConnectionService) UpdateConnectionInNmapManager(
	ctx context.Context,
	filesystemService filesystem.Service,
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
			Name:            nmapName,
			DesiredFSMState: currentDesiredState,
		},
		NmapServiceConfig: nmapCfg,
	}

	return nil
}

// RemoveConnectionFromNmapManager deletes a connection configuration.
// This stops monitoring the connection and removes all configuration.
//
// Note: This method only removes the config from a local slice and does not
// immediately apply the changes. Call ReconcileManager after this
// to apply the changes.
//
// Returns an error if the connection doesn't exist or removal fails.
func (c *ConnectionService) RemoveConnectionFromNmapManager(
	ctx context.Context,
	filesystemService filesystem.Service,
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

	sliceRemoveByName := func(in []config.NmapConfig, name string) []config.NmapConfig {
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
	c.nmapConfigs = sliceRemoveByName(c.nmapConfigs, nmapName)

	//--------------------------------------------
	// 2) is the child FSM still alive?
	//--------------------------------------------
	if inst, ok := c.nmapManager.GetInstance(nmapName); ok {
		return fmt.Errorf("%w: Nmap instance state=%s",
			standarderrors.ErrRemovalPending, inst.GetCurrentFSMState())
	}

	return nil
}

// StartConnection starts a Connection
// Expects nmapName (e.g. "connection-myservice") as defined in the UMH config.
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
// Expects nmapName (e.g. "connection-myservice") as defined in the UMH config.
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
//
// This should be called periodically by a control loop.
func (c *ConnectionService) ReconcileManager(
	ctx context.Context,
	services serviceregistry.Provider,
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
	}, services)
}

// ServiceExists checks if a connection with the given name exists.
// Used by the FSM to determine appropriate transitions.
//
// Returns true if the connection exists, false otherwise.
func (c *ConnectionService) ServiceExists(
	ctx context.Context,
	filesystemService filesystem.Service,
	connectionName string,
) bool {
	if ctx.Err() != nil {
		return false
	}

	nmapName := c.getNmapName(connectionName)

	// Check if the actual service exists
	return c.nmapService.ServiceExists(ctx, filesystemService, nmapName)
}

// ForceRemoveConnection removes a Connection from the Nmap manager
// Expects nmapName (e.g. "connection-myservice") as defined in the UMH config.
func (c *ConnectionService) ForceRemoveConnection(
	ctx context.Context,
	filesystemService filesystem.Service,
	connectionName string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// force remove from Nmap manager
	return c.nmapService.ForceRemoveNmap(ctx, filesystemService, c.getNmapName(connectionName))
}

// updateRecentScans adds a new scan result to the history for flakiness detection.
// If len(scans) == maxRecentScans (not >=) we drop the oldest element before appending.
//
// This history is used by isConnectionFlaky to detect unstable connections
// that alternate between different port states.
func (c *ConnectionService) updateRecentScans(connName string, state string) {
	// TODO(perf): replace slice with constant-size ring-buffer for O(1) append
	// Consider using container/ring or a simple array with head index

	// Initialize if this is the first scan for this connection
	if _, exists := c.recentNmapStates[connName]; !exists {
		c.recentNmapStates[connName] = make([]string, 0, constants.MaxRecentStates)
	}

	// Add the new scan (keeping only the most recent ones)
	states := c.recentNmapStates[connName]
	if len(states) >= constants.MaxRecentStates {
		// Remove oldest scan
		states = states[1:]
	}

	if state != "" {
		states = append(states, state)
	}

	c.recentNmapStates[connName] = states
}

// isConnectionFlaky determines if a connection is flaky based on recent scan history.
// A connection is considered flaky if it has shown different port states
// in the recent history window (default: last 5 scans).
//
// At least 3 samples are required to make this determination, to avoid
// false positives from normal temporary network conditions.
//
// Note: if the FSM is not running we store the synthetic "unknown" state;
// it anchors the flakiness window without marking the connection flaky.
func (c *ConnectionService) isConnectionFlaky(connName string) bool {
	scans, exists := c.recentNmapStates[connName]

	if !exists || len(scans) < 3 {
		// Need at least 3 samples to determine flakiness
		return false
	}

	firstState := scans[0]
	secondState := scans[1]

	// if those states are equal everything is right
	if firstState == secondState {
		return false
	}

	// if not we check if theres a second difference, which we then consider flaky
	for _, state := range scans[2:] {
		if state != secondState {
			return true
		}
	}

	return false
}

// GetRecentStatesCount returns the number of recent scan results for a connection.
func (s *ConnectionService) GetRecentStatesCount(name string) int {
	return len(s.recentNmapStates[name])
}

// GetRecentStatesAtIndex returns the scan at the specified index for a connection.
func (s *ConnectionService) GetRecentStatesAtIndex(name string, index int) (*string, bool) {
	if states, ok := s.recentNmapStates[name]; ok && len(states) > index {
		return &states[index], true
	}

	return nil, false
}
