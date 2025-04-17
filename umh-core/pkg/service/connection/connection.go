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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
	"go.uber.org/zap"
)

// ServiceInfo holds information about the connection health
type ServiceInfo struct {
	// IsRunning indicates if the underlying Nmap service is running
	IsRunning bool
	// PortStateOpen indicates if the target port is open according to Nmap scan
	PortStateOpen bool
	// IsReachable is a roll-up indicator of whether the target is reachable (nmap must be running and port must be open)
	IsReachable bool
	// IsFlaky indicates intermittent connectivity based on recent scan history
	IsFlaky bool
	// LastChange stores the tick when the status last changed
	LastChange uint64
}

// IConnectionService abstracts "is the remote asset reachable?" functionality
// using Nmap for checking connectivity.
type IConnectionService interface {
	// Status returns information about the connection health for the specified connection.
	// The connName corresponds to the name defined in the configuration.
	// The tick parameter is used for reconciliation timing.
	Status(ctx context.Context, fs filesystem.Service, connName string, tick uint64) (ServiceInfo, error)

	// AddConnection registers a new connection in the Nmap service.
	AddConnection(ctx context.Context, fs filesystem.Service, cfg *connectionconfig.ConnectionServiceConfig, connName string) error

	// UpdateConnection modifies an existing connection configuration.
	UpdateConnection(ctx context.Context, fs filesystem.Service, cfg *connectionconfig.ConnectionServiceConfig, connName string) error

	// RemoveConnection deletes a connection configuration.
	RemoveConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// StartConnection begins the monitoring of a connection.
	StartConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// StopConnection stops the monitoring of a connection.
	StopConnection(ctx context.Context, fs filesystem.Service, connName string) error

	// ServiceExists checks if a connection with the given name exists.
	// Used by the FSM to determine appropriate transitions.
	ServiceExists(ctx context.Context, fs filesystem.Service, connName string) bool

	// ReconcileManager synchronizes all connections on each tick.
	// Returns an error and a boolean indicating if reconciliation occurred.
	ReconcileManager(ctx context.Context, fs filesystem.Service, tick uint64) (error, bool)
}

// ConnectionService implements IConnectionService using Nmap as the underlying
// connectivity probe mechanism.
type ConnectionService struct {
	logger         *zap.SugaredLogger
	nmapService    nmap.INmapService
	recentScans    map[string][]nmap.ServiceInfo // Cache of recent scan results used for flicker detection
	maxRecentScans int
}

// ConnectionServiceOption is a function that configures a ConnectionService
type ConnectionServiceOption func(*ConnectionService)

// WithNmapService allows injecting a custom Nmap service implementation
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

// NewDefaultConnectionService creates a new ConnectionService with default options
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

// Status returns information about the connection health for the specified connection
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

// AddConnection registers a new connection in the Nmap service
func (c *ConnectionService) AddConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionconfig.ConnectionServiceConfig,
	connName string,
) error {
	c.logger.Debugf("Adding connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Convert our connection config to Nmap config
	nmapConfig := c.convertToNmapConfig(cfg)

	// Delegate to Nmap service
	return c.nmapService.AddNmapToS6Manager(ctx, &nmapConfig, connName)
}

// UpdateConnection modifies an existing connection configuration
func (c *ConnectionService) UpdateConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionconfig.ConnectionServiceConfig,
	connName string,
) error {
	c.logger.Debugf("Updating connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Convert our connection config to Nmap config
	nmapConfig := c.convertToNmapConfig(cfg)

	// Delegate to Nmap service
	return c.nmapService.UpdateNmapInS6Manager(ctx, &nmapConfig, connName)
}

// RemoveConnection deletes a connection configuration
func (c *ConnectionService) RemoveConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	c.logger.Debugf("Removing connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Delegate to Nmap service
	return c.nmapService.RemoveNmapFromS6Manager(ctx, connName)
}

// StartConnection begins the monitoring of a connection
func (c *ConnectionService) StartConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	c.logger.Debugf("Starting connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Delegate to Nmap service
	return c.nmapService.StartNmap(ctx, connName)
}

// StopConnection stops the monitoring of a connection
func (c *ConnectionService) StopConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	c.logger.Debugf("Stopping connection %s", connName)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Delegate to Nmap service
	return c.nmapService.StopNmap(ctx, connName)
}

// ServiceExists checks if a connection with the given name exists
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

// ReconcileManager synchronizes all connections on each tick
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

// convertToNmapConfig converts a ConnectionServiceConfig to an NmapServiceConfig
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

// updateRecentScans adds a new scan result to the history for flakiness detection
func (c *ConnectionService) updateRecentScans(connName string, scan nmap.ServiceInfo) {
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

// isConnectionFlaky determines if a connection is flaky based on recent scan history
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
