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

package nmap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
)

// NmapService is the implementation of the INmapService interface
type NmapService struct {
	logger           *zap.SugaredLogger
	s6Manager        *s6fsm.S6Manager
	s6Service        s6service.Service
	s6ServiceConfigs []config.S6FSMConfig
	lastScanResult   *NmapScanResult // Cache for scan results
}

// NmapServiceOption is a function that modifies a NmapService
type NmapServiceOption func(*NmapService)

// WithS6Service sets a custom S6 service for the NmapService
func WithS6Service(s6Service s6service.Service) NmapServiceOption {
	return func(s *NmapService) {
		s.s6Service = s6Service
	}
}

// NewDefaultNmapService creates a new default Nmap service manager
func NewDefaultNmapService(nmapName string, opts ...NmapServiceOption) *NmapService {
	managerName := fmt.Sprintf("%s%s", logger.ComponentNmapService, nmapName)
	service := &NmapService{
		logger:         logger.For(managerName),
		s6Manager:      s6fsm.NewS6Manager(managerName),
		s6Service:      s6service.NewDefaultService(),
		lastScanResult: nil,
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// generateNmapScript generates a shell script to run nmap periodically
func (s *NmapService) generateNmapScript(config *nmapserviceconfig.NmapServiceConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	// Build the nmap command - fixed format with -n -Pn -p PORT TARGET -v
	nmapCmd := fmt.Sprintf("nmap -n -Pn -p %d %s -v", config.Port, config.Target)

	// Create the script content with a loop that executes nmap every second
	// Log output in a structured format that we can parse later
	scriptContent := fmt.Sprintf(`#!/bin/sh
while true; do
  echo "NMAP_SCAN_START"
  echo "NMAP_TIMESTAMP: $(date -Iseconds)"
  SCAN_START=$(date +%%s.%%N)
  echo "NMAP_COMMAND: %s"
  %s
  EXIT_CODE=$?
  SCAN_END=$(date +%%s.%%N)
  SCAN_DURATION=$(echo "$SCAN_END - $SCAN_START" | bc)
  echo "NMAP_EXIT_CODE: $EXIT_CODE"
  echo "NMAP_DURATION: $SCAN_DURATION"
  echo "NMAP_SCAN_END"
  sleep %d
done
`, nmapCmd, nmapCmd, ScanIntervalSeconds)

	return scriptContent, nil
}

// getS6ServiceName converts a nmapName (e.g. "myscan") to its S6 service name (e.g. "nmap-myscan")
func (s *NmapService) getS6ServiceName(nmapName string) string {
	return fmt.Sprintf("nmap-%s", nmapName)
}

// GenerateS6ConfigForNmap creates a S6 config for a given nmap instance
func (s *NmapService) GenerateS6ConfigForNmap(nmapConfig *nmapserviceconfig.NmapServiceConfig, s6ServiceName string) (s6serviceconfig.S6ServiceConfig, error) {
	scriptContent, err := s.generateNmapScript(nmapConfig)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, err
	}

	s6Config := s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/bin/sh",
			fmt.Sprintf("%s/%s/config/run_nmap.sh", constants.S6BaseDir, s6ServiceName),
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			"run_nmap.sh": scriptContent,
		},
	}

	return s6Config, nil
}

// GetConfig returns the actual nmap config from the S6 service
func (s *NmapService) GetConfig(ctx context.Context, filesystemService filesystem.Service, nmapName string) (nmapserviceconfig.NmapServiceConfig, error) {
	if ctx.Err() != nil {
		return nmapserviceconfig.NmapServiceConfig{}, ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(nmapName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	// Get the script file
	scriptData, err := s.s6Service.GetS6ConfigFile(ctx, s6ServicePath, "run_nmap.sh", filesystemService)
	if err != nil {
		return nmapserviceconfig.NmapServiceConfig{}, fmt.Errorf("failed to get nmap config file for service %s: %w", s6ServiceName, err)
	}

	// Parse the script to extract configuration
	result := nmapserviceconfig.NmapServiceConfig{}

	// Extract target
	targetRegex := regexp.MustCompile(`nmap -n -Pn -p \d+ ([^ ]+) -v`)
	if matches := targetRegex.FindStringSubmatch(string(scriptData)); len(matches) > 1 {
		result.Target = matches[1]
	}

	// Extract port
	portRegex := regexp.MustCompile(`-p (\d+)`)
	if matches := portRegex.FindStringSubmatch(string(scriptData)); len(matches) > 1 {
		port, err := strconv.Atoi(matches[1])
		if err == nil {
			result.Port = port
		}
	}

	return result, nil
}

// parseScanLogs parses the logs of an nmap service and extracts scan results
func (s *NmapService) parseScanLogs(logs []s6service.LogEntry, port int) *NmapScanResult {
	if len(logs) == 0 {
		return nil
	}

	// We'll reconstruct scan blocks from the logs
	var currentScan []string
	var scanBlocks [][]string

	inScanBlock := false

	// First, extract complete scan blocks from logs
	for _, log := range logs {
		if strings.Contains(log.Content, "NMAP_SCAN_START") {
			inScanBlock = true
			currentScan = []string{log.Content}
		} else if strings.Contains(log.Content, "NMAP_SCAN_END") {
			if inScanBlock {
				currentScan = append(currentScan, log.Content)
				scanBlocks = append(scanBlocks, currentScan)
				currentScan = []string{}
				inScanBlock = false
			}
		} else if inScanBlock {
			currentScan = append(currentScan, log.Content)
		}
	}

	// If no complete scans, return nil
	if len(scanBlocks) == 0 {
		return nil
	}

	// Parse the most recent complete scan
	latestScan := scanBlocks[len(scanBlocks)-1]
	scanOutput := strings.Join(latestScan, "\n")

	// Create the scan result
	result := &NmapScanResult{
		RawOutput: scanOutput,
		PortResult: PortResult{
			Port: port,
		},
		Metrics: ScanMetrics{},
	}

	// Extract timestamp
	timestampRegex := regexp.MustCompile(`NMAP_TIMESTAMP: (.+)`)
	if matches := timestampRegex.FindStringSubmatch(scanOutput); len(matches) > 1 {
		timestamp, err := time.Parse(time.RFC3339, matches[1])
		if err == nil {
			result.Timestamp = timestamp
		}
	}

	// Extract duration
	durationRegex := regexp.MustCompile(`NMAP_DURATION: ([0-9.]+)`)
	if matches := durationRegex.FindStringSubmatch(scanOutput); len(matches) > 1 {
		duration, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			result.Metrics.ScanDuration = duration
		}
	}

	// Extract port state
	portStateRegex := regexp.MustCompile(`(?i)` + strconv.Itoa(port) + `/tcp\s+(\w+)`)
	if matches := portStateRegex.FindStringSubmatch(scanOutput); len(matches) > 1 {
		result.PortResult.State = matches[1]
	} else {
		result.PortResult.State = "unknown"
	}

	// Extract latency
	latencyRegex := regexp.MustCompile(`rtt=([0-9.]+)([a-z]+)`)
	if matches := latencyRegex.FindStringSubmatch(scanOutput); len(matches) > 2 {
		latency, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			// Convert to milliseconds if needed
			unit := matches[2]
			switch unit {
			case "s":
				latency *= 1000
			case "us":
				latency /= 1000
			}
			result.PortResult.LatencyMs = latency
		}
	}

	// Extract errors if occured (case-insensitive)
	errorRegex := regexp.MustCompile(`(?im)^.*error.*$`)
	if matches := errorRegex.FindString(scanOutput); matches != "" {
		result.Error = matches
	}

	return result
}

// Status checks the status of a nmap service
func (s *NmapService) Status(ctx context.Context, filesystemService filesystem.Service, nmapName string, tick uint64) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(nmapName)

	// Check if service exists in the S6 manager
	if _, exists := s.s6Manager.GetInstance(s6ServiceName); !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// Get S6 state
	s6StateRaw, err := s.s6Manager.GetLastObservedState(s6ServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "instance "+s6ServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get last observed state: %w", err)
	}

	s6State, ok := s6StateRaw.(s6fsm.S6ObservedState)
	if !ok {
		return ServiceInfo{}, fmt.Errorf("observed state is not a S6ObservedState: %v", s6StateRaw)
	}

	// Get FSM state
	fsmState, err := s.s6Manager.GetCurrentFSMState(s6ServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "instance "+s6ServiceName+" not found") ||
			strings.Contains(err.Error(), "not found") {
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get current FSM state: %w", err)
	}

	// Get logs
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)
	logs, err := s.s6Service.GetLogs(ctx, s6ServicePath, filesystemService)
	if err != nil {
		if errors.Is(err, s6service.ErrServiceNotExist) {
			return ServiceInfo{}, ErrServiceNotExist
		} else if errors.Is(err, s6service.ErrLogFileNotFound) {
			return ServiceInfo{}, ErrServiceNotExist
		}
		return ServiceInfo{}, fmt.Errorf("failed to get logs: %w", err)
	}

	// Get scan results from logs
	var scanResult *NmapScanResult
	if fsmState == s6fsm.OperationalStateRunning && len(logs) > 0 {
		// Get the current config to know which port we're scanning
		config, err := s.GetConfig(ctx, filesystemService, nmapName)
		if err == nil {
			// Parse logs to extract scan results
			scanResult = s.parseScanLogs(logs, config.Port)
		} else {
			s.logger.Warnw("Failed to get config", "error", err)
		}
	}

	return ServiceInfo{
		S6ObservedState: s6State,
		S6FSMState:      fsmState,
		NmapStatus: NmapServiceInfo{
			LastScan:  scanResult,
			IsRunning: fsmState == s6fsm.OperationalStateRunning,
			Logs:      logs,
		},
	}, nil
}

// AddNmapToS6Manager adds a nmap instance to the S6 manager
func (s *NmapService) AddNmapToS6Manager(ctx context.Context, cfg *nmapserviceconfig.NmapServiceConfig, nmapName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(nmapName)

	// Check whether s6ServiceConfigs already contains an entry for this instance
	for _, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			return ErrServiceAlreadyExists
		}
	}

	// Generate the S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForNmap(cfg, s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Nmap service %s: %w", s6ServiceName, err)
	}

	// Create the S6 FSM config for this instance
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6fsm.OperationalStateRunning,
		},
		S6ServiceConfig: s6Config,
	}

	// Add the S6 FSM config to the list of S6 FSM configs
	s.s6ServiceConfigs = append(s.s6ServiceConfigs, s6FSMConfig)

	return nil
}

// UpdateNmapInS6Manager updates an existing nmap instance in the S6 manager
func (s *NmapService) UpdateNmapInS6Manager(ctx context.Context, cfg *nmapserviceconfig.NmapServiceConfig, nmapName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(nmapName)

	// Check if the service exists
	found := false
	index := -1
	for i, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			found = true
			index = i
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Generate the new S6 config for this instance
	s6Config, err := s.GenerateS6ConfigForNmap(cfg, s6ServiceName)
	if err != nil {
		return fmt.Errorf("failed to generate S6 config for Nmap service %s: %w", s6ServiceName, err)
	}

	// Update the S6 service config while preserving the desired state
	currentDesiredState := s.s6ServiceConfigs[index].DesiredFSMState
	s.s6ServiceConfigs[index] = config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: currentDesiredState,
		},
		S6ServiceConfig: s6Config,
	}

	return nil
}

// RemoveNmapFromS6Manager removes a nmap instance from the S6 manager
func (s *NmapService) RemoveNmapFromS6Manager(ctx context.Context, nmapName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(nmapName)

	found := false

	// Remove the S6 FSM config from the list
	for i, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			s.s6ServiceConfigs = append(s.s6ServiceConfigs[:i], s.s6ServiceConfigs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Clean up the cached scan results
	s.lastScanResult = nil

	return nil
}

// StartNmap starts a nmap instance
func (s *NmapService) StartNmap(ctx context.Context, nmapName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(nmapName)

	found := false

	// Set the desired state to running for the given instance
	for i, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			s.s6ServiceConfigs[i].DesiredFSMState = s6fsm.OperationalStateRunning
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// StopNmap stops a nmap instance
func (s *NmapService) StopNmap(ctx context.Context, nmapName string) error {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s6ServiceName := s.getS6ServiceName(nmapName)

	found := false

	// Set the desired state to stopped for the given instance
	for i, s6Config := range s.s6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			s.s6ServiceConfigs[i].DesiredFSMState = s6fsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return nil
}

// ReconcileManager reconciles the Nmap manager
func (s *NmapService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (err error, reconciled bool) {
	if s.s6Manager == nil {
		return errors.New("s6 manager not initialized"), false
	}

	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Create a snapshot from the full config
	snapshot := fsm.SystemSnapshot{
		CurrentConfig: config.FullConfig{Internal: config.InternalConfig{Services: s.s6ServiceConfigs}},
		Tick:          tick,
	}

	return s.s6Manager.Reconcile(ctx, snapshot, filesystemService)
}

// ServiceExists checks if a nmap service exists
func (s *NmapService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, nmapName string) bool {
	s6ServiceName := s.getS6ServiceName(nmapName)
	s6ServicePath := filepath.Join(constants.S6BaseDir, s6ServiceName)

	exists, err := s.s6Service.ServiceExists(ctx, s6ServicePath, filesystemService)
	if err != nil {
		return false
	}

	return exists
}

// ForceRemoveNmap removes a Nmap instance from the S6 manager
// This should only be called if the Nmap instance is in a permanent failure state
// and the instance itself cannot be stopped or removed
// Expects nmapName (e.g. "myservice") as defined in the UMH config
func (s *NmapService) ForceRemoveNmap(
	ctx context.Context,
	filesystemService filesystem.Service,
	nmapName string,
) error {

	return s.s6Service.ForceRemove(ctx, s.getS6ServiceName(nmapName), filesystemService)
}
