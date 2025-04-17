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
package connection

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"go.uber.org/zap"
)

// ServiceInfo holds information about the connection's health status.
// It uses separate boolean flags for different health aspects instead of
// a single status value, making it easier to check specific conditions.
type ServiceInfo struct {
	// IsRunning indicates if the underlying Nmap service is running.
	// This refers to the monitoring service itself, not the target endpoint.
	IsRunning bool

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
	// Returns an error if the connection already exists or if registration fails.
	AddConnection(ctx context.Context, fs filesystem.Service, cfg *connectionconfig.ConnectionServiceConfig, connName string) error

	// UpdateConnection modifies an existing connection configuration.
	// This can be used to change the target hostname, port, or protocol.
	//
	// The service must exist already. The update takes effect on the next
	// monitor cycle and does not restart the monitoring process.
	//
	// Returns an error if the connection doesn't exist or if update fails.
	UpdateConnection(ctx context.Context, fs filesystem.Service, cfg *connectionconfig.ConnectionServiceConfig, connName string) error

	// RemoveConnection deletes a connection configuration.
	// This stops monitoring the connection and removes all configuration.
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

	// The underlying Nmap service used for network probing
	nmapService nmap.INmapService

	// Cache of recent scan results used for flicker detection
	// Map key is the connection name, value is a slice of recent scan results
	recentScans map[string][]nmap.ServiceInfo

	// Maximum number of recent scans to keep in history per connection
	// Used for flakiness detection
	maxRecentScans int
}

// ConnectionServiceOption is a function that configures a ConnectionService.
// This follows the functional options pattern for flexible configuration.
type ConnectionServiceOption func(*ConnectionService)

// WithNmapService allows injecting a custom Nmap service implementation.
// This is primarily used for testing but can also be used to customize
// the underlying network probing mechanism.
func WithNmapService(nmapService nmap.INmapService) ConnectionServiceOption {
	return func(c *ConnectionService) {
		c.nmapService = nmapService
	}
}

// WithMaxRecentScans allows setting a custom recent scans limit for testing
func WithMaxRecentScans(maxScans int) ConnectionServiceOption {
	return func(c *ConnectionService) {
		c.maxRecentScans = maxScans
	}
}

// NewDefaultConnectionService creates a new ConnectionService with default options.
// It initializes the logger, recent scans cache, and sets default values.
//
// Options can be passed to customize behavior:
// - WithNmapService: Use a custom Nmap service implementation
//
// Example:
//
//	service := NewDefaultConnectionService()
//	// or with custom Nmap service
//	mockNmap := nmap.NewMockNmapService()
//	service := NewDefaultConnectionService(WithNmapService(mockNmap))
func NewDefaultConnectionService(opts ...ConnectionServiceOption) *ConnectionService {
	service := &ConnectionService{
		logger:         logger.For(logger.ComponentConnectionService),
		recentScans:    make(map[string][]nmap.ServiceInfo),
		maxRecentScans: constants.MaxRecentScans, // Keep last scan results for flicker detection
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	// Initialize Nmap service if not provided
	if service.nmapService == nil {
		service.nmapService = nmap.NewDefaultNmapService("connection")
	}

	return service
}

// NewConnectionServiceForTesting creates a new ConnectionService specifically for testing
// with the ability to override settings like maxRecentScans
func NewConnectionServiceForTesting(opts ...ConnectionServiceOption) *ConnectionService {
	return NewDefaultConnectionService(opts...)
}

// GetRecentScansForTesting provides access to the internal recentScans state for testing purposes
func (c *ConnectionService) GetRecentScansForTesting(connName string) []nmap.ServiceInfo {
	if scans, exists := c.recentScans[connName]; exists {
		return scans
	}
	return nil
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
	c.logger.Debugf("Checking status for connection %s at tick %d", connName, tick)

	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	// Get status from Nmap service
	nmapStatus, err := c.nmapService.Status(ctx, fs, connName, tick)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to get nmap status for connection %s: %w", connName, err)
	}

	// Update recent scans history for flicker detection
	c.updateRecentScans(connName, nmapStatus)

	// Convert the Nmap status to ServiceInfo
	info := ServiceInfo{
		IsRunning:  nmapStatus.S6FSMState == "running" && nmapStatus.NmapStatus.IsRunning,
		LastChange: tick, // This should ideally come from Nmap status
	}

	// Check if we have scan results
	if nmapStatus.NmapStatus.LastScan != nil {
		info.PortStateOpen = nmapStatus.NmapStatus.LastScan.PortResult.State == "open"
	}

	// Derive the IsReachable flag (up/degraded/down logic)
	info.IsReachable = info.IsRunning && info.PortStateOpen

	// Check for flakiness in recent scans
	info.IsFlaky = c.isConnectionFlaky(connName)

	return info, nil
}

// AddConnection registers a new connection in the Nmap service.
// It converts the connection service configuration to the appropriate
// Nmap configuration format and delegates to the Nmap service.
func (c *ConnectionService) AddConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionconfig.ConnectionServiceConfig,
	connName string,
) error {
	c.logger.Infof("Adding connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Convert our connection config to Nmap config
	nmapConfig := c.convertToNmapConfig(cfg)

	// Delegate to Nmap service
	return c.nmapService.AddNmapToS6Manager(ctx, &nmapConfig, connName)
}

// UpdateConnection modifies an existing connection configuration.
// It converts the updated configuration to the Nmap format and
// delegates to the underlying Nmap service.
func (c *ConnectionService) UpdateConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionconfig.ConnectionServiceConfig,
	connName string,
) error {
	c.logger.Infof("Updating connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Convert our connection config to Nmap config
	nmapConfig := c.convertToNmapConfig(cfg)

	// Delegate to Nmap service
	return c.nmapService.UpdateNmapInS6Manager(ctx, &nmapConfig, connName)
}

// RemoveConnection deletes a connection configuration.
// It delegates to the Nmap service to remove the underlying
// monitoring configuration.
func (c *ConnectionService) RemoveConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	c.logger.Infof("Removing connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Delegate to Nmap service
	return c.nmapService.RemoveNmapFromS6Manager(ctx, connName)
}

// StartConnection begins the monitoring of a connection.
// It delegates to the Nmap service to start the underlying
// monitoring process.
func (c *ConnectionService) StartConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	c.logger.Infof("Starting connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Delegate to Nmap service
	return c.nmapService.StartNmap(ctx, connName)
}

// StopConnection stops the monitoring of a connection.
// It delegates to the Nmap service to stop the underlying
// monitoring process.
func (c *ConnectionService) StopConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	c.logger.Infof("Stopping connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Delegate to Nmap service
	return c.nmapService.StopNmap(ctx, connName)
}

// ServiceExists checks if a connection with the given name exists.
// It delegates to the Nmap service to check if the underlying
// configuration exists.
func (c *ConnectionService) ServiceExists(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) bool {
	exists := c.nmapService.ServiceExists(ctx, fs, connName)
	c.logger.Debugf("Connection %s exists: %v", connName, exists)

	if ctx.Err() != nil {
		return false
	}

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

	// Delegate to Nmap service
	return c.nmapService.ReconcileManager(ctx, fs, tick)
}

// convertToNmapConfig converts a ConnectionServiceConfig to an NmapServiceConfig.
// This is an adapter function that maps the connection service's configuration
// format to the format expected by the underlying Nmap service.
//
// The Connection service uses a simplified configuration that focuses on
// connectivity aspects, while the Nmap service has more detailed configuration
// options specific to network scanning.
func (c *ConnectionService) convertToNmapConfig(
	cfg *connectionconfig.ConnectionServiceConfig,
) nmapserviceconfig.NmapServiceConfig {
	return nmapserviceconfig.NmapServiceConfig{
		Target: cfg.Hostname,
		Port:   int(cfg.Port),
		// Protocol is not directly mapped in NmapServiceConfig
		// but could be used to influence scan type
	}
}

// updateRecentScans adds a new scan result to the history for flakiness detection.
// It maintains a circular buffer of recent scan results for each connection,
// discarding the oldest result when the buffer is full.
//
// This history is used by isConnectionFlaky to detect unstable connections
// that alternate between up and down states.
func (c *ConnectionService) updateRecentScans(connName string, scan nmap.ServiceInfo) {
	// Initialize if this is the first scan for this connection
	if _, exists := c.recentScans[connName]; !exists {
		c.recentScans[connName] = make([]nmap.ServiceInfo, 0, c.maxRecentScans)
	}

	// Add the new scan (keeping only the most recent ones)
	scans := c.recentScans[connName]
	if len(scans) >= c.maxRecentScans {
		// Remove oldest scan
		scans = scans[1:]
	}
	scans = append(scans, scan)
	c.recentScans[connName] = scans
}

// isConnectionFlaky determines if a connection is flaky based on recent scan history.
// A connection is considered flaky if it has shown different port states
// in the recent history window (default: last 5 scans).
//
// The algorithm looks for:
// - Mix of open and closed states
// - Mix of open and filtered states
// - Mix of closed and filtered states
//
// At least 3 samples are required to make this determination, to avoid
// false positives from normal temporary network conditions.
func (c *ConnectionService) isConnectionFlaky(connName string) bool {
	scans, exists := c.recentScans[connName]
	if !exists || len(scans) < 3 {
		// Need at least 3 samples to determine flakiness
		return false
	}

	// Check for changes in port state in last scans
	hasOpen := false
	hasClosed := false
	hasFiltered := false

	for _, scan := range scans {
		if scan.NmapStatus.LastScan == nil {
			continue
		}

		state := scan.NmapStatus.LastScan.PortResult.State
		switch state {
		case "open":
			hasOpen = true
		case "closed":
			hasClosed = true
		case "filtered", "open|filtered", "closed|filtered":
			hasFiltered = true
		}
	}

	// If we have mixed states, we consider it flaky
	return (hasOpen && hasClosed) ||
		(hasOpen && hasFiltered) ||
		(hasClosed && hasFiltered)
}
