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
// - Connections have multiple health indicators (running, reachable, flaky)
// - Flakiness detection requires multiple samples over time
// - The service translates network probe results into business-relevant health status
//
// # Thread Safety
//
// The ConnectionService is designed to be accessed by a single goroutine.
// All operations that modify the configuration (Add/Update/Remove) should be
// followed by a call to ReconcileManager to apply the changes.
package connection

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"go.uber.org/zap"
)

// ServiceInfo holds information about the connection's health status.
// It uses separate boolean flags for different health aspects instead of
// a single status value, making it easier to check specific conditions.
type ServiceInfo struct {
	// IsHealthy is true only when the connection‑monitoring probe
	// is in a steady, reliable state (Nmap FSM = "open").
	// If the FSM is stopped, starting, degraded, etc. it is false.
	IsHealthy bool

	// PortStateOpen indicates if the target port is open according to Nmap scan.
	// This is the raw network result from the most recent probe.
	PortStateOpen bool

	// IsReachable is a roll-up indicator of whether the target is reachable.
	// It combines IsRunning and PortStateOpen - both must be true for a
	// connection to be considered reachable.
	IsReachable bool

	// IsFlaky indicates intermittent connectivity based on recent scan history.
	// A connection is considered flaky if it alternates between open/closed states
	// within the recent history window (default: last 60 scans).
	IsFlaky bool

	// LastChange stores the tick when the status last changed.
	// This timestamp uses the FSM tick counter rather than wall clock time
	// to ensure consistency with the FSM reconciliation system.
	LastChange uint64
}

// IConnectionService abstracts "is the remote asset reachable?" functionality
// using Nmap for checking connectivity. It provides methods for managing the
// lifecycle of connections and monitoring their health.
type IConnectionService interface {
	// Status returns information about the connection health for the specified connection.
	// The connName corresponds to the name defined in the configuration.
	// The tick parameter is used for reconciliation timing and to timestamp the result.
	//
	// Returns a ServiceInfo struct containing multiple health indicators:
	// - IsRunning: if the monitoring service is operational
	// - PortStateOpen: if the most recent scan found the port accessible
	// - IsReachable: combined indicator (IsRunning && PortStateOpen)
	// - IsFlaky: if the connection has been unstable in recent history
	// - LastChange: tick when the status last changed
	//
	// Errors:
	// - If the connection doesn't exist
	// - If there's an underlying Nmap error
	// - If the context is canceled
	Status(ctx context.Context, fs filesystem.Service, connName string, tick uint64) (ServiceInfo, error)

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
	AddConnection(ctx context.Context, fs filesystem.Service, cfg *connectionserviceconfig.ConnectionServiceConfig, connName string) error

	// UpdateConnection modifies an existing connection configuration.
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
	UpdateConnection(ctx context.Context, fs filesystem.Service, cfg *connectionserviceconfig.ConnectionServiceConfig, connName string) error

	// RemoveConnection deletes a connection configuration.
	// This stops monitoring the connection and removes all configuration.
	//
	// Note: This method only removes the config from a local slice and does not
	// immediately apply the changes. Call ReconcileManager after this
	// to apply the changes.
	//
	// Returns an error if the connection doesn't exist or removal fails.
	RemoveConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// StartConnection begins the monitoring of a connection.
	// This initiates periodic scanning of the target using Nmap.
	//
	// Returns an error if the connection doesn't exist or start fails.
	StartConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// StopConnection stops the monitoring of a connection.
	// This halts scanning but retains the configuration for later restart.
	//
	// Returns an error if the connection doesn't exist or stop fails.
	StopConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// ServiceExists checks if a connection with the given name exists.
	// Used by the FSM to determine appropriate transitions.
	//
	// Returns true if the connection exists, false otherwise.
	ServiceExists(ctx context.Context, fs filesystem.Service, connName string) bool

	// ReconcileManager synchronizes all connections on each tick.
	// This is typically called in a loop by the FSM system.
	//
	// Returns an error and a boolean indicating if reconciliation occurred.
	// The boolean is false if reconciliation was skipped (e.g., due to an error).
	ReconcileManager(ctx context.Context, fs filesystem.Service, tick uint64) (error, bool)
}

// ConnectionService implements IConnectionService using Nmap as the underlying
// connectivity probe mechanism. It maintains a history of recent scans to detect
// flaky connections and provides a higher-level abstraction over raw Nmap results.
type ConnectionService struct {
	// Logger for the connection service
	logger *zap.SugaredLogger

	nmapManager        *nmapfsm.NmapManager
	nmapServiceConfigs []config.NmapConfig

	// Cache of recent scan results used for flicker detection
	// Map key is the connection name, value is a slice of recent scan results
	recentScans map[string][]nmap.ServiceInfo

	// Maximum number of recent scans to keep in history per connection
	// Used for flakiness detection
	maxRecentScans int

	// Previous connection state info per connection
	prevInfo map[string]ServiceInfo // keyed by connection name
}

// ConnectionServiceOption is a function that configures a ConnectionService.
// This follows the functional options pattern for flexible configuration.
type ConnectionServiceOption func(*ConnectionService)

// WithMaxRecentScans allows setting a custom recent scans limit for testing
func WithMaxRecentScans(maxScans int) ConnectionServiceOption {
	return func(c *ConnectionService) {
		if maxScans > 0 {
			c.maxRecentScans = maxScans
		}
	}
}

func WithNmapManager(mgr *nmapfsm.NmapManager) ConnectionServiceOption {
	return func(c *ConnectionService) { c.nmapManager = mgr }
}

// NewDefaultConnectionService creates a new ConnectionService with default options.
// It initializes the logger, recent scans cache, and sets default values.
//
// Options can be passed to customize behavior:
// - WithMaxRecentScans: Set a custom limit for recent scans history
//
// Example:
//
//	service := NewDefaultConnectionService("my-connection-service")
//	// or with custom max recent scans
//	service := NewDefaultConnectionService("my-connection-service", WithMaxRecentScans(10))
func NewDefaultConnectionService(connectionName string, opts ...ConnectionServiceOption) *ConnectionService {
	service := &ConnectionService{
		logger:         logger.For(connectionName),
		nmapManager:    nmapfsm.NewNmapManager(connectionName),
		recentScans:    make(map[string][]nmap.ServiceInfo),
		maxRecentScans: constants.MaxRecentScans, // Keep last scan results for flicker detection
		prevInfo:       make(map[string]ServiceInfo),
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// Status returns information about the connection health for the specified connection.
// It queries the underlying Nmap service and then enhances the result with
// additional context like flakiness detection based on historical data.
//
// The tick parameter is used both for timestamping the result and as part of
// the FSM reconciliation system's timing mechanism.
func (c *ConnectionService) Status(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
	tick uint64,
) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	// First, check if the service exists in the S6 manager
	// This is a crucial check that prevents "instance not found" errors
	// during reconciliation when a service is being created or removed
	instance, exists := c.nmapManager.GetInstance(connName)
	if !exists {
		c.logger.Debugf("Service %s not found in S6 manager", connName)
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

	nmapSvcInfo := nmapStatusObservedState.ServiceInfo
	// Update recent scans history for flicker detection
	c.updateRecentScans(connName, nmapSvcInfo)

	IsHealthy := false

	// if it is any other state other than running (such as stopped, starting, degraded, etc.)
	// it is not healthy
	if nmapfsm.IsRunningState(nmapInstanceState) && nmapInstanceState != nmapfsm.OperationalStateDegraded {
		IsHealthy = true
	}

	// Convert the Nmap status to ServiceInfo
	info := ServiceInfo{
		IsHealthy: IsHealthy,
	}

	// Check if we have scan results
	if nmapSvcInfo.NmapStatus.LastScan != nil {
		info.PortStateOpen = nmapSvcInfo.NmapStatus.LastScan.PortResult.State == "open"
	}

	// Fix for false positive reachability: If the FSM state is not "open",
	// force PortStateOpen to false regardless of the last scan result
	if nmapSvcInfo.S6FSMState != s6fsm.OperationalStateRunning {
		info.PortStateOpen = false
	}

	// Derive the IsReachable flag (up/degraded/down logic)
	info.IsReachable = info.IsHealthy && info.PortStateOpen

	// Check for flakiness in recent scans
	info.IsFlaky = c.isConnectionFlaky(connName)

	// ----- LastChange handling -----
	prev, exists := func() (ServiceInfo, bool) {
		prev, exists := c.prevInfo[connName]
		return prev, exists
	}()

	stateChanged := !exists || prev.IsReachable != info.IsReachable || prev.IsFlaky != info.IsFlaky

	// Only take write lock if we're about to modify the map
	if stateChanged {
		info.LastChange = tick
		c.prevInfo[connName] = info
	} else {
		info.LastChange = prev.LastChange
	}

	return info, nil
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
func (c *ConnectionService) AddConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionserviceconfig.ConnectionServiceConfig,
	connName string,
) error {
	c.logger.Infof("Adding connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the connection already exists in our configs
	for _, existingConfig := range c.nmapServiceConfigs {
		if existingConfig.Name == connName {
			return ErrServiceAlreadyExists
		}
	}

	// Convert our connection config to Nmap service config
	nmapServiceConfig := c.convertToNmapConfig(*cfg)

	// Create a config.NmapConfig that wraps the NmapServiceConfig
	nmapConfig := config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            connName,
			DesiredFSMState: nmapfsm.OperationalStateOpen,
		},
		NmapServiceConfig: nmapServiceConfig,
	}

	// Add the configuration to the list
	c.nmapServiceConfigs = append(c.nmapServiceConfigs, nmapConfig)

	return nil
}

// UpdateConnection modifies an existing connection configuration.
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
func (c *ConnectionService) UpdateConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionserviceconfig.ConnectionServiceConfig,
	connName string,
) error {
	c.logger.Infof("Updating connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the service exists in the manager
	if _, exists := c.nmapManager.GetInstance(connName); !exists {
		return ErrServiceNotExist
	}

	// Convert our connection config to Nmap service config
	nmapServiceConfig := c.convertToNmapConfig(*cfg)

	// Create a config.NmapConfig that wraps the NmapServiceConfig
	newNmapConfig := config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name: connName,
			// Default to open state if not found in existing configs
			DesiredFSMState: nmapfsm.OperationalStateOpen,
		},
		NmapServiceConfig: nmapServiceConfig,
	}

	// Find and update the config in nmapServiceConfigs
	found := false
	for i, existingConfig := range c.nmapServiceConfigs {
		if existingConfig.Name == connName {
			// Preserve the existing desired state
			newNmapConfig.DesiredFSMState = existingConfig.DesiredFSMState
			c.nmapServiceConfigs[i] = newNmapConfig
			found = true
			break
		}
	}

	if !found {
		// Add a new config if not found - this shouldn't happen normally
		// but provides better resilience
		c.nmapServiceConfigs = append(c.nmapServiceConfigs, newNmapConfig)
	}

	return nil
}

// RemoveConnection deletes a connection configuration.
// This stops monitoring the connection and removes all configuration.
//
// Note: This method only removes the config from a local slice and does not
// immediately apply the changes. Call ReconcileManager after this
// to apply the changes.
//
// Returns an error if the connection doesn't exist or removal fails.
func (c *ConnectionService) RemoveConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	c.logger.Infof("Removing connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the service exists in the manager
	if _, exists := c.nmapManager.GetInstance(connName); !exists {
		return ErrServiceNotExist
	}

	// Simply filter the config list to exclude the one for this connection
	newConfigs := make([]config.NmapConfig, 0, len(c.nmapServiceConfigs))
	for _, config := range c.nmapServiceConfigs {
		// Skip the config with matching name
		if config.Name != connName {
			newConfigs = append(newConfigs, config)
		}
	}
	c.nmapServiceConfigs = newConfigs

	// Clean up in-memory state
	delete(c.recentScans, connName)
	delete(c.prevInfo, connName)

	return nil
}

// StartConnection begins the monitoring of a connection.
// This initiates periodic scanning of the target using Nmap.
//
// Returns an error if the connection doesn't exist or start fails.
func (c *ConnectionService) StartConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the service exists
	instance, exists := c.nmapManager.GetInstance(connName)
	if !exists {
		return ErrServiceNotExist
	}

	// Get the instance to check current desired state
	nmapInstance, ok := instance.(*nmapfsm.NmapInstance)
	if !ok {
		return fmt.Errorf("instance is not a NmapInstance")
	}

	// Fast path: if already in desired state, do nothing
	currentDesiredState := nmapInstance.GetDesiredFSMState()
	if currentDesiredState == nmapfsm.OperationalStateOpen {
		// Already in desired state, no need to log or update
		return nil
	}

	c.logger.Infof("Starting connection %s", connName)
	return nmapInstance.SetDesiredFSMState(nmapfsm.OperationalStateOpen)
}

// StopConnection stops the monitoring of a connection.
// This halts scanning but retains the configuration for later restart.
//
// Returns an error if the connection doesn't exist or stop fails.
func (c *ConnectionService) StopConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the service exists
	instance, exists := c.nmapManager.GetInstance(connName)
	if !exists {
		return ErrServiceNotExist
	}

	// Get the instance to check current desired state
	nmapInstance, ok := instance.(*nmapfsm.NmapInstance)
	if !ok {
		return fmt.Errorf("instance is not a NmapInstance")
	}

	// Fast path: if already in desired state, do nothing
	currentDesiredState := nmapInstance.GetDesiredFSMState()
	if currentDesiredState == nmapfsm.OperationalStateStopped {
		// Already in desired state, no need to log or update
		return nil
	}

	c.logger.Infof("Stopping connection %s", connName)
	return nmapInstance.SetDesiredFSMState(nmapfsm.OperationalStateStopped)
}

// ServiceExists checks if a connection with the given name exists.
// Used by the FSM to determine appropriate transitions.
//
// Returns true if the connection exists, false otherwise.
func (c *ConnectionService) ServiceExists(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) bool {
	if ctx.Err() != nil {
		return false
	}

	// Check if the instance exists in the manager
	_, exists := c.nmapManager.GetInstance(connName)
	return exists
}

// ReconcileManager synchronizes all connections on each tick.
// It delegates to the Nmap service's reconciliation function,
// which ensures that the actual state matches the desired state
// for all services.
//
// This should be called periodically by a control loop.
func (c *ConnectionService) ReconcileManager(
	ctx context.Context,
	fs filesystem.Service,
	tick uint64,
) (error, bool) {
	c.logger.Debugf("Reconciling connection manager at tick %d", tick)

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Create a system snapshot for the nmapManager to use
	snapshot := fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Nmap: c.nmapServiceConfigs}},
		Tick:          tick,
	}

	// Use the nmapManager's Reconcile method
	return c.nmapManager.Reconcile(ctx, snapshot, fs)
}

// convertToNmapConfig converts a ConnectionServiceConfig to an NmapServiceConfig.
// This is an adapter function that maps the connection service's configuration
// format to the format expected by the underlying Nmap service.
//
// The Connection service uses a simplified configuration that focuses on
// connectivity aspects, while the Nmap service has more detailed configuration
// options specific to network scanning.
func (c *ConnectionService) convertToNmapConfig(
	cfg connectionserviceconfig.ConnectionServiceConfig,
) nmapserviceconfig.NmapServiceConfig {
	return cfg.NmapServiceConfig
}

// updateRecentScans adds a new scan result to the history for flakiness detection.
// If len(scans) == maxRecentScans (not >=) we drop the oldest element before appending.
//
// This history is used by isConnectionFlaky to detect unstable connections
// that alternate between different port states.
func (c *ConnectionService) updateRecentScans(connName string, scan nmap.ServiceInfo) {
	// TODO(perf): replace slice with constant-size ring-buffer for O(1) append
	// Consider using container/ring or a simple array with head index

	// Initialize if this is the first scan for this connection
	if _, exists := c.recentScans[connName]; !exists {
		c.recentScans[connName] = make([]nmap.ServiceInfo, 0, c.maxRecentScans)
	}

	// Create a deep copy of the scan info to prevent shared pointers
	scanCopy := nmap.ServiceInfo{
		S6FSMState: scan.S6FSMState,
	}

	// Deep copy the NmapStatus
	if scan.NmapStatus.LastScan != nil {
		scanCopy.NmapStatus.IsRunning = scan.NmapStatus.IsRunning
		scanCopy.NmapStatus.LastScan = &nmap.NmapScanResult{
			Timestamp: scan.NmapStatus.LastScan.Timestamp,
			Error:     scan.NmapStatus.LastScan.Error,
			PortResult: nmap.PortResult{
				Port:      scan.NmapStatus.LastScan.PortResult.Port,
				State:     scan.NmapStatus.LastScan.PortResult.State,
				LatencyMs: scan.NmapStatus.LastScan.PortResult.LatencyMs,
			},
			Metrics: scan.NmapStatus.LastScan.Metrics,
		}
	}

	// Add the new scan (keeping only the most recent ones)
	scans := c.recentScans[connName]
	if len(scans) >= c.maxRecentScans {
		// Remove oldest scan
		scans = scans[1:]
	}
	scans = append(scans, scanCopy)
	c.recentScans[connName] = scans
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
	scans, exists := c.recentScans[connName]

	if !exists || len(scans) < 3 {
		// Need at least 3 samples to determine flakiness
		return false
	}

	firstState := portState(scans[0])
	for _, s := range scans[1:] {
		if portState(s) != firstState {
			// At least one different state – connection is flaky
			return true
		}
	}
	return false
}

// portState extracts the port state from a scan result for flakiness detection
func portState(s nmap.ServiceInfo) string {
	if s.NmapStatus.LastScan == nil {
		// Return "unknown" for scans with no data, which preserves flakiness window
		// but won't trigger flaky state by itself
		return "unknown"
	}
	return s.NmapStatus.LastScan.PortResult.State
}
