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
// See the License for the specifileInfoc language governing permissions and
// limitations under the License.

package s6_orig

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// Global map to store last config change timestamps keyed by service path
// This ensures timestamps are shared across all S6 service instances operating on the same service
// TODO: This is to be replaced with the archive storage when it is implemented.
var (
	configChangeTimestamps = sync.Map{} // map[string]time.Time
)

// setLastDeployedTime sets the last config change timestamp for a service path.
func setLastDeployedTime(servicePath string, timestamp time.Time) {
	configChangeTimestamps.Store(servicePath, timestamp)
}

// getLastDeploymentTime gets the last config change timestamp for a service path.
func getLastDeploymentTime(servicePath string) time.Time {
	if timestamp, ok := configChangeTimestamps.Load(servicePath); ok {
		if timeVal, ok := timestamp.(time.Time); ok {
			return timeVal
		}
	}

	return time.Time{} // zero time if not found
}

// DefaultService is the default implementation of the S6 Service interface.
type DefaultService struct {
	logger     *zap.SugaredLogger
	artifacts  *s6_shared.ServiceArtifacts // cached artifacts for the service
	logCursors sync.Map                    // map[string]*s6_shared.LogState (key = abs log path)

	// Lifecycle management with concurrency protection
	mu sync.Mutex // serializes all state-changing calls
}

// NewDefaultService creates a new default S6 service.
func NewDefaultService() s6_shared.Service {
	serviceLogger := logger.For(logger.ComponentS6Service)

	return &DefaultService{
		logger: serviceLogger,
	}
}

// withLifecycleGuard serializes all state-changing operations to prevent race conditions
// This addresses concurrent Create/Remove/ForceRemove operations.
func (s *DefaultService) withLifecycleGuard(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return fn()
}

// Create creates the S6 service with specifileInfoc configuration using proven patterns:
// - UnifileInfoed lifecycle mutex prevents concurrent Create/Remove/ForceRemove operations
// - Uses lifecycle manager for atomic creation with EXDEV protection
// - SimplifileInfoed 3-path approach with health checks.
func (s *DefaultService) Create(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) error {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".create", time.Since(start))
	}()

	s.logger.Debugf("Creating S6 service %s", servicePath)

	return s.withLifecycleGuard(func() error {
		// No need to ensure artifacts here - they'll be created by CreateArtifacts

		// 1. Directory doesn't exist → create it fresh
		exists, err := fsService.PathExists(ctx, servicePath)
		if err != nil {
			return fmt.Errorf("failed to check if service path exists: %w", err)
		}

		if !exists {
			s.logger.Debugf("Service %s doesn't exist, creating fresh", servicePath)

			artifacts, err := s.CreateArtifacts(ctx, servicePath, config, fsService)
			if err != nil {
				return err
			}

			s.artifacts = artifacts

			return nil
		}

		// 2. Directory exists but we have no artifacts → orphaned directory from restart
		if s.artifacts == nil {
			s.logger.Debugf("Service %s exists but has no artifacts (orphaned from restart), removing and recreating", servicePath)
			// Remove the orphaned directory immediately - we can't track what files were created
			err = fsService.RemoveAll(ctx, servicePath)
			if err != nil {
				return fmt.Errorf("failed to remove orphaned service directory: %w", err)
			}
			// Proceed with fresh creation
			artifacts, err := s.CreateArtifacts(ctx, servicePath, config, fsService)
			if err != nil {
				return err
			}

			s.artifacts = artifacts

			return nil
		}

		// 3. Directory exists and we have artifacts → check health
		health, err := s.CheckArtifactsHealth(ctx, s.artifacts, fsService)
		if err != nil && health != s6_shared.HealthUnknown {
			return fmt.Errorf("failed to check service health: %w", err)
		}

		if health == s6_shared.HealthBad {
			s.logger.Debugf("Service %s is unhealthy, removing and recreating", servicePath)

			err = s.ForceCleanup(ctx, s.artifacts, fsService)
			if err != nil {
				return fmt.Errorf("failed to remove unhealthy service: %w", err)
			}

			artifacts, err := s.CreateArtifacts(ctx, servicePath, config, fsService)
			if err != nil {
				return err
			}

			s.artifacts = artifacts

			return nil
		}

		// 4. Directory exists and is healthy → check config
		currentConfileInfog, err := s.GetConfig(ctx, servicePath, fsService)
		if err != nil {
			return fmt.Errorf("failed to get current config: %w", err)
		}

		// 5. Same config → do nothing (success)
		if currentConfileInfog.Equal(config) {
			s.logger.Debugf("Service %s config unchanged, nothing to do", servicePath)

			return nil
		}

		// 6. Different config → remove and recreate
		s.logger.Debugf("Service %s config changed, removing and recreating", servicePath)

		err = s.RemoveArtifacts(ctx, s.artifacts, fsService)
		if err != nil {
			return fmt.Errorf("failed to remove service for recreation: %w", err)
		}

		artifacts, err := s.CreateArtifacts(ctx, servicePath, config, fsService)
		if err != nil {
			return err
		}

		s.artifacts = artifacts

		return nil
	})
}

// Remove removes S6 service artifacts using a fast, idempotent approach:
// - Uses unifileInfoed lifecycle mutex to prevent concurrent operations
// - Implements rename-then-delete pattern for immediate S6 scanner visibility removal
// - Returns quickly (<1 second) to respect FSM context timeouts
// - Fully idempotent - safe to call when directories are partially removed
// - Returns nil only when nothing is left.
func (s *DefaultService) Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service,
			servicePath+".remove", time.Since(start))
	}()

	if ctx.Err() != nil {
		return ctx.Err() // context already cancelled / deadline exceeded
	}

	s.logger.Debugf("Removing S6 service %s", servicePath)

	return s.withLifecycleGuard(func() error {
		// If we have tracked artifacts, use them for proper removal
		if s.artifacts != nil && len(s.artifacts.CreatedFiles) > 0 {
			return s.RemoveArtifacts(ctx, s.artifacts, fsService)
		}

		// No tracked files - service is in an inconsistent state
		// Remove() requires tracked files to work properly
		s.logger.Debugf("No tracked files for service %s, cannot do tracked removal", servicePath)

		return fmt.Errorf("service %s has no tracked files - use ForceRemove() instead", servicePath)
	})
}

// Start starts the S6 service.
func (s *DefaultService) Start(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".start", time.Since(start))
	}()

	s.logger.Debugf("Starting S6 service %s", servicePath)

	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return fmt.Errorf("failed to check if service exists: %w", err)
	}

	if !exists {
		return s6_shared.ErrServiceNotExist
	}

	// Remove down files to indicate the service should stay up
	downFiles := []string{
		filepath.Join(servicePath, "down"),        // Main service down file
		filepath.Join(servicePath, "log", "down"), // Log service down file
	}

	for _, downFile := range downFiles {
		if exists, _ := fsService.FileExists(ctx, downFile); exists {
			err := fsService.Remove(ctx, downFile)
			if err != nil {
				s.logger.Warnf("Failed to remove down file %s: %v", downFile, err)
				// Continue anyway - s6-svc -u might still work
			} else {
				s.logger.Debugf("Removed down file %s", downFile)
				// Update artifacts to remove the down file from tracking
				if s.artifacts != nil {
					s.removeFileFromArtifacts(downFile)
				}
			}
		}
	}

	// First start the log service
	logServicePath := filepath.Join(servicePath, "log")

	_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-u", logServicePath)
	if err != nil {
		s.logger.Warnf("Failed to start log service: %v", err)

		return fmt.Errorf("failed to start log service: %w", err)
	}

	// Then start the main service to prevent a race condition
	_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-u", servicePath)
	if err != nil {
		s.logger.Warnf("Failed to start service: %v", err)

		return fmt.Errorf("failed to start service: %w", err)
	}

	s.logger.Debugf("Started S6 service %s", servicePath)

	return nil
}

// Stop stops the S6 service.
func (s *DefaultService) Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".stop", time.Since(start))
	}()

	s.logger.Debugf("Stopping S6 service %s", servicePath)

	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return fmt.Errorf("failed to check if service exists: %w", err)
	}

	if !exists {
		return s6_shared.ErrServiceNotExist
	}

	// Stop the service fileInforst
	_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-d", servicePath)
	if err != nil {
		s.logger.Warnf("Failed to stop service: %v", err)

		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Also stop the log service separately
	logServicePath := filepath.Join(servicePath, "log")

	_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-d", logServicePath)
	if err != nil {
		s.logger.Warnf("Failed to stop log service: %v", err)

		return fmt.Errorf("failed to stop log service: %w", err)
	}

	// Create down files to ensure the service stays down
	downFiles := []string{
		filepath.Join(servicePath, "down"),        // Main service down file
		filepath.Join(servicePath, "log", "down"), // Log service down file
	}

	for _, downFile := range downFiles {
		err := fsService.WriteFile(ctx, downFile, []byte{}, 0644)
		if err != nil {
			s.logger.Warnf("Failed to create down file %s: %v", downFile, err)
			// Continue anyway - service is already stopped
		} else {
			s.logger.Debugf("Created down file %s", downFile)
			// Update artifacts to track the down file
			if s.artifacts != nil {
				s.addFileToArtifacts(downFile)
			}
		}
	}

	s.logger.Debugf("Stopped S6 service %s", servicePath)

	return nil
}

// Restart restarts the S6 service.
func (s *DefaultService) Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".restart", time.Since(start))
	}()

	s.logger.Debugf("Restarting S6 service %s", servicePath)

	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return fmt.Errorf("failed to check if service exists: %w", err)
	}

	if !exists {
		return s6_shared.ErrServiceNotExist
	}

	_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-r", servicePath)
	if err != nil {
		s.logger.Warnf("Failed to restart service: %v", err)

		return fmt.Errorf("failed to restart service: %w", err)
	}

	s.logger.Debugf("Restarted S6 service %s", servicePath)

	return nil
}

func (s *DefaultService) Status(ctx context.Context, servicePath string, fsService filesystem.Service) (s6_shared.ServiceInfo, error) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".status", time.Since(start))
	}()

	// First, check that the service exists.
	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return s6_shared.ServiceInfo{}, fmt.Errorf("failed to check if service exists: %w", err)
	}

	if !exists {
		return s6_shared.ServiceInfo{}, s6_shared.ErrServiceNotExist
	}

	// Default info.
	info := s6_shared.ServiceInfo{
		Status: s6_shared.ServiceUnknown,
	}

	// Build supervise directory path.
	superviseDir := filepath.Join(servicePath, "supervise")

	exists, err = fsService.FileExists(ctx, superviseDir)
	if err != nil {
		return info, fmt.Errorf("failed to check if supervise directory exists: %w", err)
	}

	if !exists {
		return info, s6_shared.ErrServiceNotExist // This is a temporary thing that can happen in bufered filesystems, when s6 did not yet have time to create the directory
	}

	// Parse the status file using centralized parser from status.go
	// This eliminates code duplication and uses the same parsing logic everywhere
	statusFile := filepath.Join(superviseDir, s6_shared.S6SuperviseStatusFile)

	exists, err = fsService.FileExists(ctx, statusFile)
	if err != nil {
		return info, fmt.Errorf("failed to check if status file exists: %w", err)
	}

	if !exists {
		return info, s6_shared.ErrServiceNotExist // This is a temporary thing that can happen in bufered filesystems, when s6 did not yet have time to create the file
	}

	// Use centralized S6 status parser
	statusData, err := s6_shared.ParseS6StatusFile(ctx, statusFile, fsService)
	if err != nil {
		return info, fmt.Errorf("failed to parse status file: %w", err)
	}

	// Build full ServiceInfo using the parsed data
	return s.buildFullServiceInfo(ctx, servicePath, statusData, fsService)
}

// ServiceExists checks if the service directory exists.
func (s *DefaultService) ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".serviceExists", time.Since(start))
	}()

	exists, err := fsService.PathExists(ctx, servicePath)
	if err != nil {
		return false, fmt.Errorf("failed to check if S6 service exists: %w", err)
	}

	if !exists {
		if s.logger != nil {
			s.logger.Debugf("S6 service %s does not exist", servicePath)
		}

		return false, nil
	}

	return true, nil
}

// GetConfig gets the actual service config from s6.
func (s *DefaultService) GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (s6serviceconfig.S6ServiceConfig, error) {
	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to check if service exists: %w", err)
	}

	if !exists {
		return s6serviceconfig.S6ServiceConfig{}, s6_shared.ErrServiceNotExist
	}

	observedS6ServiceConfileInfog := s6serviceconfig.S6ServiceConfig{
		ConfigFiles: make(map[string]string),
		Env:         make(map[string]string),
		MemoryLimit: 0,
		LogFilesize: 0,
	}

	// Fetch run script
	runScript := filepath.Join(servicePath, "run")

	exists, err = fsService.FileExists(ctx, runScript)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to check if run script exists: %w", err)
	}

	if !exists {
		return s6serviceconfig.S6ServiceConfig{}, errors.New("run script not found")
	}

	content, err := fsService.ReadFile(ctx, runScript)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to read run script: %w", err)
	}

	// Parse the run script content
	scriptContent := string(content)

	// Extract environment variables using regex
	envMatches := envVarParser.FindAllStringSubmatch(scriptContent, -1)
	for _, match := range envMatches {
		if len(match) == 3 {
			key := match[1]
			value := strings.TrimSpace(match[2])
			// Remove any quotes
			value = strings.Trim(value, "\"'")
			observedS6ServiceConfileInfog.Env[key] = value
		}
	}

	// Extract command using regex
	cmdMatch := runScriptParser.FindStringSubmatch(scriptContent)

	if len(cmdMatch) >= 2 && cmdMatch[1] != "" {
		// If we captured the command on the same line as fdmove
		cmdLine := strings.TrimSpace(cmdMatch[1])

		observedS6ServiceConfileInfog.Command, err = parseCommandLine(cmdLine)
		if err != nil {
			return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse command: %w", err)
		}
	} else {
		// If the command is on the line after fdmove, or regex didn't match properly
		lines := strings.Split(scriptContent, "\n")

		var commandLine string

		// Find the fdmove line
		for index, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "fdmove") {
				// Check if command is on the same line after fdmove
				if strings.Contains(line, " ") && len(line) > strings.LastIndex(line, " ")+1 {
					parts := strings.SplitN(line, "fdmove -c 2 1", 2)
					if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
						commandLine = strings.TrimSpace(parts[1])

						break
					}
				}

				// Otherwise, look for fileInforst non-empty line after fdmove
				for j := index + 1; j < len(lines); j++ {
					nextLine := strings.TrimSpace(lines[j])
					if nextLine != "" {
						commandLine = nextLine

						break
					}
				}

				if commandLine != "" {
					break
				}
			}
		}

		if commandLine != "" {
			observedS6ServiceConfileInfog.Command, err = parseCommandLine(commandLine)
			if err != nil {
				return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse command: %w", err)
			}
		} else {
			// Absolute fallback - try to look for the command we know should be there
			sentry.ReportIssuef(sentry.IssueTypeWarning, s.logger, "[s6.GetConfig] Could not find command in run script for %s, searching for known paths", servicePath)

			cmdRegex := regexp.MustCompile(`(/[^\s]+)`)
			cmdMatches := cmdRegex.FindAllString(scriptContent, -1)

			if len(cmdMatches) > 0 {
				// Use the fileInforst matching path-like string we find as the command
				cmd := cmdMatches[0]
				args := []string{}

				// Look for arguments after the command
				argIndex := strings.Index(scriptContent, cmd) + len(cmd)
				if argIndex < len(scriptContent) {
					argPart := strings.TrimSpace(scriptContent[argIndex:])
					if argPart != "" {
						args, err = parseCommandLine(argPart)
						if err != nil {
							return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse command: %w", err)
						}
					}
				}

				observedS6ServiceConfileInfog.Command = append([]string{cmd}, args...)
			} else {
				return s6serviceconfig.S6ServiceConfig{}, errors.New("failed to parse run script: no valid command found")
			}
		}
	}

	// Extract memory limit using regex
	memoryLimitMatches := memoryLimitParser.FindStringSubmatch(scriptContent)
	if len(memoryLimitMatches) >= 2 && memoryLimitMatches[1] != "" {
		observedS6ServiceConfileInfog.MemoryLimit, err = strconv.ParseInt(memoryLimitMatches[1], 10, 64)
		if err != nil {
			return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse memory limit: %w", err)
		}
	}

	// Fetch config files from servicePath
	configPath := filepath.Join(servicePath, "config")

	exists, err = fsService.FileExists(ctx, configPath)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to check if config directory exists: %w", err)
	}

	if !exists {
		return observedS6ServiceConfileInfog, nil
	}

	entries, err := fsService.ReadDir(ctx, configPath)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to read config directory: %w", err)
	}

	// Extract config files
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(configPath, entry.Name())

		content, err := fsService.ReadFile(ctx, filePath)
		if err != nil {
			return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to read config file %s: %w", entry.Name(), err)
		}

		observedS6ServiceConfileInfog.ConfigFiles[entry.Name()] = string(content)
	}

	// Extract LogFilesize using regex
	// Fetch run script
	logServicePath := filepath.Join(servicePath, "log")
	logScript := filepath.Join(logServicePath, "run")

	exists, err = fsService.FileExists(ctx, logScript)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to check if log run§ script exists: %w", err)
	}

	if !exists {
		return s6serviceconfig.S6ServiceConfig{}, errors.New("log run script not found")
	}

	logScriptContentRaw, err := fsService.ReadFile(ctx, logScript)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to read log run script: %w", err)
	}

	// Parse the run script content
	logScriptContent := string(logScriptContentRaw)

	// Extract log filesize using the dedicated log filesize parser
	logSizeMatches := logFilesizeParser.FindStringSubmatch(logScriptContent)
	if len(logSizeMatches) >= 2 {
		observedS6ServiceConfileInfog.LogFilesize, err = strconv.ParseInt(logSizeMatches[1], 10, 64)
		if err != nil {
			return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse log filesize: %w", err)
		}
	} else {
		// If no match found, default to 0
		observedS6ServiceConfileInfog.LogFilesize = 0
	}

	return observedS6ServiceConfileInfog, nil
}

// parseCommandLine splits a command line into command and arguments, respecting quotes.
func parseCommandLine(cmdLine string) ([]string, error) {
	var (
		cmdParts    []string
		currentPart strings.Builder
	)

	inQuote := false
	quoteChar := byte(0)
	escaped := false

	for index := range len(cmdLine) {
		// Handle escape character
		if cmdLine[index] == '\\' && !escaped {
			escaped = true

			continue
		}

		switch {
		case (cmdLine[index] == '"' || cmdLine[index] == '\'') && !escaped:
			switch {
			case inQuote && cmdLine[index] == quoteChar:
				inQuote = false
				quoteChar = 0
			case !inQuote:
				inQuote = true
				quoteChar = cmdLine[index]
			default:
				// This is a different quote character inside a quote
				currentPart.WriteByte(cmdLine[index])
			}
		case escaped:
			// Handle the escaped character
			currentPart.WriteByte(cmdLine[index])

			escaped = false
		default:
			if cmdLine[index] == ' ' && !inQuote {
				if currentPart.Len() > 0 {
					cmdParts = append(cmdParts, currentPart.String())
					currentPart.Reset()
				}
			} else {
				currentPart.WriteByte(cmdLine[index])
			}
		}
	}

	// Check for unclosed quotes
	if inQuote {
		return nil, errors.New("unclosed quote")
	}

	if currentPart.Len() > 0 {
		cmdParts = append(cmdParts, currentPart.String())
	}

	return cmdParts, nil
}

// CleanS6ServiceDirectory cleans the S6 service directory except for the known services.
func (s *DefaultService) CleanS6ServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error {
	if ctx == nil {
		return errors.New("context is nil")
	}

	if ctx.Err() != nil {
		return fmt.Errorf("context is done: %w", ctx.Err())
	}

	// Get the list of entries in the S6 service directory
	entries, err := fsService.ReadDir(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to read S6 service directory: %w", err)
	}

	// Use safe logging that handles nil loggers
	if s.logger != nil {
		s.logger.Infof("Cleaning S6 service directory: %s, found %d entries", path, len(entries))
	}

	// Iterate over all directory entries
	for _, entry := range entries {
		// Skip files, only process directories
		if !entry.IsDir() {
			if s.logger != nil {
				s.logger.Debugf("Skipping non-directory: %s", entry.Name())
			}

			continue
		}

		// Check if the directory is a known service that should be preserved
		dirName := entry.Name()
		if !s.IsKnownService(dirName) {
			dirPath := filepath.Join(path, dirName)
			if s.logger != nil {
				s.logger.Infof("Removing unknown directory: %s", dirPath)
			}

			// Simply remove the directory (and its contents)
			err := fsService.RemoveAll(ctx, dirPath)
			if err != nil {
				if s.logger != nil {
					sentry.ReportIssuef(sentry.IssueTypeWarning, s.logger, "Failed to remove directory %s: %v", dirPath, err)
				}
			} else if s.logger != nil {
				s.logger.Infof("Successfully removed directory: %s", dirPath)
			}
		} else if s.logger != nil {
			s.logger.Debugf("Keeping known directory: %s", dirName)
		}
	}

	if s.logger != nil {
		s.logger.Infof("Finished cleaning S6 service directory: %s", path)
	}

	return nil
}

// IsKnownService checks if a service is known.
func (s *DefaultService) IsKnownService(name string) bool {
	// Core system services that should never be removed
	knownServices := []string{
		// S6 core services
		"s6-linux-init-shutdownd",
		"s6rc-fdholder",
		"s6rc-oneshot-runner",
		"syslogd",
		"syslogd-log",
		"umh-core",
		// S6 internal directories
		".s6-svscan",       // Special directory for s6-svscan control
		"user",             // Special user bundle directory
		"s6-rc",            // S6 runtime compiled database
		"log-user-service", // User service logs
	}

	for _, known := range knownServices {
		if name == known {
			return true
		}
	}

	// Check for standard S6 naming patterns
	if strings.HasSuffix(name, "-log") || // Logger services
		strings.HasSuffix(name, "-prepare") || // Preparation oneshots
		strings.HasSuffix(name, "-log-prepare") { // Preparation oneshots for loggers
		return true
	}

	return false
}

// GetS6ConfigFile retrieves the specified config file for a service
// servicePath should be the full path including S6BaseDir.
func (s *DefaultService) GetS6ConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check if the service exists with the full path
	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return nil, fmt.Errorf("failed to check if service exists: %w", err)
	}

	if !exists {
		return nil, s6_shared.ErrServiceNotExist
	}

	// Form the path to the config file using the constant
	configPath := filepath.Join(servicePath, constants.S6ConfigDirName, configFileName)

	// Check if the file exists
	exists, err = fsService.FileExists(ctx, configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check if config file exists: %w", err)
	}

	if !exists {
		return nil, fmt.Errorf("config file %s does not exist in service directory %s", configFileName, servicePath)
	}

	// Read the file
	content, err := fsService.ReadFile(ctx, configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %s in service directory %s: %w", configFileName, servicePath, err)
	}

	return content, nil
}

// ForceRemove performs aggressive cleanup for stuck S6 services using comprehensive patterns:
// - Uses unifileInfoed lifecycle mutex to prevent concurrent operations
// - Implements comprehensive process termination and supervisor killing
// - Performs timeout-aware recursive deletion
// - Called by FSM escalation - can take longer than Remove but must be thorough
// - Returns nil only when nothing remains.
func (s *DefaultService) ForceRemove(
	ctx context.Context,
	servicePath string,
	fsService filesystem.Service,
) error {
	start := time.Now()

	defer func() {
		metrics.IncErrorCount(metrics.ComponentS6Service, servicePath+".forceRemove") // a force remove is an error
		metrics.ObserveReconcileTime(
			metrics.ComponentS6Service,
			servicePath+".forceRemove",
			time.Since(start))
	}()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.logger.Warnf("Force removing S6 service %s", servicePath)

	return s.withLifecycleGuard(func() error {
		// ForceRemove doesn't need tracked files - it does aggressive cleanup
		// Create minimal artifacts for cleanup operations
		serviceName := filepath.Base(servicePath)
		artifacts := &s6_shared.ServiceArtifacts{
			ServiceDir: servicePath,
			LogDir:     filepath.Join(constants.S6LogBaseDir, serviceName),
			// CreatedFiles intentionally empty - we don't care about tracked files for force cleanup
		}

		// Use lifecycle manager for aggressive cleanup
		return s.ForceCleanup(ctx, artifacts, fsService)
	})
}

// appendToRingBuffer appends entries to the ring buffer, extracted from existing GetLogs logic.
func (s *DefaultService) appendToRingBuffer(entries []s6_shared.LogEntry, logState *s6_shared.LogState) {
	const maxLines = constants.S6MaxLines

	// Preallocate backing storage to full size once - it's recycled at runtime and never dropped
	if logState.Logs == nil {
		logState.Logs = make([]s6_shared.LogEntry, maxLines) // len == maxLines, cap == maxLines
		logState.Head = 0
		logState.Full = false
	}

	for _, e := range entries {
		logState.Logs[logState.Head] = e

		logState.Head = (logState.Head + 1) % maxLines
		if !logState.Full && logState.Head == 0 {
			logState.Full = true // wrapped around for the fileInforst time
		}
	}
}

// GetLogs reads "just the new bytes" of the log file located at
//
//	/data/logs/<service-name>/current
//
// and returns — *always in a brand-new backing array* — the last
// `constants.S6MaxLines` log lines parsed as `LogEntry` objects.
//
// The function keeps one in-memory cursor (`s6_shared.LogState`) *per log file*:
//
//	logCursors : map[absLogPath]*s6_shared.LogState
//
// That cursor stores where we stopped reading the file last time
// (`offset`) and a fileInfoxed-size **ring buffer** with the most recent
// lines. Because of the ring buffer, we never re-allocate or shift
// memory when trimming to the last *N* lines.
//
// High-level flow
// ───────────────
//
//  1. **Fast checks & bookkeeping**
//     • verify the service and the log file exist
//     • fetch/create the cursor in `s.logCursors`
//
//  2. **Detect file rotation/truncation (or fileInforst-time initialization)**
//     If the inode changed or the file shrank, we:
//     - Reset the ring buffer to start fresh (preserving backing array)
//     - Find the most recent rotated file by TAI64N timestamp
//     - Read any remaining data from that rotated file
//     - Reset inode and offset for the new current file
//
//     **FIRST CALL scenario**: When called on a newly created current file,
//     logState.inode == 0, so we simply initialize it to the current file's inode.
//
//  3. **Read content from current file**
//     `ReadFileRange(ctx, path, logState.offset)` returns the bytes that
//     appeared since the previous call.  The cursor's `offset` is
//     advanced whether or not anything changed.
//
//     **FIRST CALL**: Reads entire file from beginning (logState.offset == 0).
//
//  4. **Process rotated and current content separately**
//     Both rotated file content (if any) and current file content are
//     parsed and appended to the ring buffer in chronological order:
//     rotated content fileInforst (older), then current content (newer).
//
//  5. **Ring-append the entries**
//     Parsed log lines are appended to the ring.  Once the buffer
//     is full (`len == cap`), we start overwriting from `head`,
//     giving us an O(1) "keep the last N lines" strategy.
//
//     **FIRST CALL**: Ring buffer is allocated and populated with all entries.
//
//  6. **Copy-out**
//     We linearise the ring into chronological order and copy it
//     into a fresh slice so callers can never mutate our cache.
//
//     **FIRST CALL**: Simple copy since ring buffer hasn't wrapped yet.
//
// Performance guarantees
// ──────────────────────
//   - **At most one allocation per process** for the ring buffer.
//   - Zero‐copy maintenance while the program runs (except on rotation).
//   - At most one additional allocation per rotation (for parsing entries).
//   - One `memmove` operation per call for fileInfonal copy-out.
//   - Thread-safety via the `s6_shared.LogState.mu` mutex.
//
// Errors are returned early and unwrapped where they occur so callers
// see the root cause (e.g. "file disappeared", "permission denied").
func (s *DefaultService) GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]s6_shared.LogEntry, error) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".getLogs", time.Since(start))
	}()

	serviceName := filepath.Base(servicePath)

	// Check if the service exists fileInforst
	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return nil, fmt.Errorf("failed to check if service exists: %w", err)
	}

	if !exists {
		s.logger.Debugf("Service with path %s does not exist, returning empty logs", servicePath)

		return nil, s6_shared.ErrServiceNotExist
	}

	// Get the log file from /data/logs/<service-name>/current
	logFile := filepath.Join(constants.S6LogBaseDir, serviceName, "current")

	exists, err = fsService.PathExists(ctx, logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check if log file exists: %w", err)
	}

	if !exists {
		// s.logger.Debugf("Log file %s does not exist, returning ErrLogFileNotFound", logFile)
		return nil, fmt.Errorf("path: %s err :%w", logFile, s6_shared.ErrLogFileNotFound)
	}

	// ── 1. grab / create state ──────────────────────────────────────
	stAny, _ := s.logCursors.LoadOrStore(logFile, &s6_shared.LogState{})

	logState, stateValid := stAny.(*s6_shared.LogState)
	if !stateValid {
		return nil, errors.New("failed to get log state from cursor")
	}

	logState.Mu.Lock()
	defer logState.Mu.Unlock()

	// ── 2. check inode & size (rotation / truncation?) ──────────────
	fileInfo, err := fsService.Stat(ctx, logFile)
	if err != nil {
		return nil, err
	}

	if fileInfo == nil {
		return nil, fmt.Errorf("stat returned nil for log file: %s", logFile)
	}

	sys, ok := fileInfo.Sys().(*syscall.Stat_t) // on Linux / Alpine
	if !ok {
		return nil, fmt.Errorf("failed to get file system info for %s", logFile)
	}

	size, ino := fileInfo.Size(), sys.Ino

	// Check for rotation or truncation
	var rotatedContent []byte

	if logState.Inode != 0 && (logState.Inode != ino || logState.Offset > size) {
		s.logger.Debugf("Detected rotation for log file %s (inode: %d->%d, offset: %d, size: %d)",
			logFile, logState.Inode, ino, logState.Offset, size)

		// Ensure ring buffer is initialized but preserve existing entries
		if logState.Logs == nil {
			logState.Logs = make([]s6_shared.LogEntry, constants.S6MaxLines)
		}
		// NOTE: We do NOT reset logState.Head or logState.Full here to preserve existing entries
		// The ring buffer will naturally handle new entries being appended

		// Find the most recent rotated file
		logDir := filepath.Dir(logFile)
		pattern := filepath.Join(logDir, "@*.s")

		entries, err := fsService.Glob(ctx, pattern)
		if err != nil {
			s.logger.Debugf("Failed to read log directory %s: %v", logDir, err)

			return nil, err
		}

		rotatedFile := s.findLatestRotatedFile(entries)
		if rotatedFile != "" {
			var err error

			rotatedContent, _, err = fsService.ReadFileRange(ctx, rotatedFile, logState.Offset)
			if err != nil {
				s.logger.Warnf("Failed to read rotated file %s from offset %d: %v", rotatedFile, logState.Offset, err)
			} else if len(rotatedContent) > 0 {
				s.logger.Debugf("Read %d bytes from rotated file %s", len(rotatedContent), rotatedFile)
			}
		}

		// Reset only the file tracking for new current file
		logState.Inode, logState.Offset = ino, 0
	} else if logState.Inode == 0 {
		// First call - initialize inode
		logState.Inode = ino
	}

	// ── 3. read new bytes from current file ─────────────────────────
	currentContent, newSize, err := fsService.ReadFileRange(ctx, logFile, logState.Offset)
	if err != nil {
		return nil, err
	}

	logState.Offset = newSize // advance cursor even if currentContent == nil

	// ── 4. combine rotated and current content into the ring buffer ──────────────────────
	// The order of the content is important: rotated content is older, current content is newer.
	// We want to keep the newest lines at the end of the ring buffer.

	if len(rotatedContent) > 0 {
		entries, err := ParseLogsFromBytes(rotatedContent)
		if err != nil {
			return nil, err
		}

		s.appendToRingBuffer(entries, logState)
	}

	if len(currentContent) > 0 {
		entries, err := ParseLogsFromBytes(currentContent)
		if err != nil {
			return nil, err
		}

		s.appendToRingBuffer(entries, logState)
	}

	// ── 5. return *copy* so caller can't mutate our cache ───────────
	var length int
	if logState.Full {
		length = constants.S6MaxLines
	} else {
		length = logState.Head // number of valid entries written so far
	}

	out := make([]s6_shared.LogEntry, length)

	// `head` always points **to** the slot for the *next* write, so the
	// oldest entry is there, and the newest entry is just before it.
	// We need to lay the data out linearly in time order:
	//
	//	[head … max-1]  followed by  [0 … head-1]
	if logState.Full {
		// Ring buffer has wrapped - linearize it
		n := copy(out, logState.Logs[logState.Head:])
		copy(out[n:], logState.Logs[:logState.Head])
	} else {
		// Ring buffer hasn't wrapped yet - simple copy from beginning
		copy(out, logState.Logs[:logState.Head])
	}

	// fileInfolter the logs to only include logs since last deployment time
	// We linearly search the array for the fileInforst entry after the last deployment time
	// This is O(n) but the benchmark shows that the performance impact is negligible
	// compared to the cost of reading the log file (11μs per call for 10.000 lines)
	// this is why we decided against using a cached index that comes with a high complexity
	if !getLastDeploymentTime(servicePath).IsZero() {
		lastDeployed := getLastDeploymentTime(servicePath)
		for i, entry := range out {
			if entry.Timestamp.After(lastDeployed) {
				// Found fileInforst entry after deployment - return this and all subsequent entries
				// since they're chronologically sorted
				return out[i:], nil
			}
		}
		// No entries found after deployment time
		return nil, nil
	}

	return out, nil
}

// ParseLogsFromBytes is a zero-allocation* parser for an s6 "current"
// file.  It scans the buffer **once**, pre-allocates the result slice
// and never calls strings.Split/Index, so the costly
// runtime.growslice/strings.* nodes vanish from the profile.
//
//	*apart from the unavoidable string↔[]byte conversions needed for the
//	LogEntry struct – those are julogState.Header copies, no heap memcopy.
func ParseLogsFromBytes(buf []byte) ([]s6_shared.LogEntry, error) {
	// Trim one trailing newline that is always present in rotated logs.
	buf = bytes.TrimSuffix(buf, []byte{'\n'})

	// 1) -------- pre-allocation --------------------------------------
	nLines := bytes.Count(buf, []byte{'\n'}) + 1
	entries := make([]s6_shared.LogEntry, 0, nLines) // avoids  runtime.growslice

	// 2) -------- single pass over the buffer -------------------------
	for start := 0; start < len(buf); {
		// find next '\n'
		newlineIndex := bytes.IndexByte(buf[start:], '\n')

		var line []byte
		if newlineIndex == -1 {
			line = buf[start:]
			start = len(buf)
		} else {
			line = buf[start : start+newlineIndex]
			start += newlineIndex + 1
		}

		if len(line) == 0 { // empty line – rotate artefact
			continue
		}

		// 3) -------- parse one line ----------------------------------
		// format: 2025-04-20 13:01:02.123456789␠␠payload
		sep := bytes.Index(line, []byte("  "))
		if sep == -1 || sep < 29 { // malformed – keep raw
			entries = append(entries, s6_shared.LogEntry{Content: string(line)})

			continue
		}

		timestamp, err := s6_shared.ParseNano(string(line[:sep])) // ParseNano is already fast
		if err != nil {
			entries = append(entries, s6_shared.LogEntry{Content: string(line)})

			continue
		}

		entries = append(entries, s6_shared.LogEntry{
			Timestamp: timestamp,
			Content:   string(line[sep+2:]),
		})
	}

	return entries, nil
}

// ParseLogsFromBytes_Unoptimized is the more readable not optimized version of ParseLogsFromBytes.
func ParseLogsFromBytes_Unoptimized(content []byte) ([]s6_shared.LogEntry, error) {
	// Split logs by newline
	logs := strings.Split(strings.TrimSpace(string(content)), "\n")

	// Parse each log line into structured entries
	var entries []s6_shared.LogEntry

	for _, line := range logs {
		if line == "" {
			continue
		}

		entry := parseLogLine(line)
		if !entry.Timestamp.IsZero() {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// parseLogLine parses a log line from S6 format and returns a LogEntry.
func parseLogLine(line string) s6_shared.LogEntry {
	// Quick check for empty strings or too short lines
	if len(line) < 28 { // Minimum length for "YYYY-MM-DD HH:MM:SS.<9 digit nanoseconds>  content"
		return s6_shared.LogEntry{Content: line}
	}

	// Check if we have the double space separator
	sepIdx := strings.Index(line, "  ")
	if sepIdx == -1 || sepIdx > 29 {
		return s6_shared.LogEntry{Content: line}
	}

	// Extract timestamp part
	timestampStr := line[:sepIdx]

	// Extract content part (after the double space)
	content := ""
	if sepIdx+2 < len(line) {
		content = line[sepIdx+2:]
	}

	// Try to parse the timestamp
	// We are using ParseNano over time.Parse because it is faster for our specifileInfoc time format
	timestamp, err := s6_shared.ParseNano(timestampStr)
	if err != nil {
		return s6_shared.LogEntry{Content: line}
	}

	return s6_shared.LogEntry{
		Timestamp: timestamp,
		Content:   content,
	}
}

// EnsureSupervision checks if the supervise directory exists for a service and notifileInfoes
// s6-svscan if it doesn't, to trigger supervision setup.
// Returns true if supervise directory exists (ready for supervision), false otherwise.
func (s *DefaultService) EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	start := time.Now()

	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".ensureSupervision", time.Since(start))
	}()

	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return false, fmt.Errorf("failed to check if service exists: %w", err)
	}

	if !exists {
		return false, s6_shared.ErrServiceNotExist
	}

	superviseDir := filepath.Join(servicePath, "supervise")

	exists, err = fsService.FileExists(ctx, superviseDir)
	if err != nil {
		return false, fmt.Errorf("failed to check if supervise directory exists: %w", err)
	}

	if !exists {
		s.logger.Debugf("Supervise directory not found for %s, notifying s6-svscan", servicePath)

		_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svscanctl", "-a", constants.S6BaseDir)
		if err != nil {
			return false, fmt.Errorf("failed to notify s6-svscan: %w", err)
		}

		s.logger.Debugf("NotifileInfoed s6-svscan, waiting for supervise directory to be created on next reconcile")

		return false, nil
	}

	s.logger.Debugf("Supervise directory exists for %s", servicePath)

	return true, nil
}

// ExecuteS6Command executes an S6 command and handles its specifileInfoc exit codes.
// This function simplifileInfoes error handling by translating exit codes into appropriate errors:
//   - Exit code 0: Success
//   - Exit code 100: Permanent error (like command misuse), returns nil for idempotent operations
//   - Exit code 111: Temporary error, returns ErrS6TemporaryError
//   - Exit code 126: Permission error or similar, returns ErrS6ProgramNotExecutable
//   - Exit code 127: Command not found, returns ErrS6ProgramNotFound
//   - Other exit codes: Returns a descriptive error with the exit code and output
func (s *DefaultService) ExecuteS6Command(ctx context.Context, servicePath string, fsService filesystem.Service, name string, args ...string) (string, error) {
	output, err := fsService.ExecuteCommand(ctx, name, args...)
	if err != nil {
		// Check exit codes using errors.As to handle wrapped errors
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitCode() {
			case 111:
				s.logger.Debugf("S6 command encountered a temporary error (exit code 111) for service %s", servicePath)

				return "", s6_shared.ErrS6TemporaryError
			case 100:
				s.logger.Debugf("S6 service %s is already being monitored (exit code 100), continuing", servicePath)

				return "", nil
			case 127:
				s.logger.Debugf("S6 command could not find program (exit code 127) for service %s", servicePath)

				return "", s6_shared.ErrS6ProgramNotFound
			case 126:
				s.logger.Debugf("S6 command could not execute program (exit code 126) for service %s", servicePath)

				return "", s6_shared.ErrS6ProgramNotExecutable
			default:
				return "", fmt.Errorf("unknown S6 error (exit code %d) for service %s: %w, output: %s",
					exitErr.ExitCode(), servicePath, err, string(output))
			}
		}

		return "", fmt.Errorf("failed to execute s6 command (name: %s, args: %v): %w, output: %s", name, args, err, string(output))
	}

	return string(output), nil
}

// findLatestRotatedFile finds the most recently rotated file using slices.MaxFunc.
//
// S6 creates rotated files with TAI64N timestamps in their names (e.g., @400000006501234567890abc.s).
// TAI64N timestamps are designed to be lexicographically sortable, so we can use string comparison
// to find the chronologically latest file effileInfociently.
//
// This approach uses slices.MaxFunc which provides optimal performance:
//   - O(n) time complexity with single pass through entries
//   - Zero memory allocations
//   - No intermediate sorting required
//
// Returns an empty string if no valid rotated files are found.
func (s *DefaultService) findLatestRotatedFile(entries []string) string {
	if len(entries) == 0 {
		return ""
	}

	// Use slices.MaxFunc to find the latest file
	latestFile := slices.MaxFunc(entries, cmp.Compare[string])

	return latestFile
}

// CheckHealth performs tri-state health check with lifecycle manager:
// - Uses cached artifacts to avoid repeated path calculations
// - Implements proper separation of observation from action
// - Optional s6-svok integration for runtime health verifileInfocation
// Returns:
//   - HealthUnknown: probe failed due to I/O errors, timeouts, etc. - retry next tick
//   - HealthOK: service directory is healthy and complete
//   - HealthBad: service directory is defileInfonitely broken - triggers FSM transition
func (s *DefaultService) CheckHealth(ctx context.Context, servicePath string, fsService filesystem.Service) (s6_shared.HealthStatus, error) {
	// Context already cancelled → Unknown, never Bad
	if ctx.Err() != nil {
		return s6_shared.HealthUnknown, ctx.Err()
	}

	// If we don't have tracked artifacts, the service is in an inconsistent state
	artifacts := s.artifacts
	if artifacts == nil || len(artifacts.CreatedFiles) == 0 {
		s.logger.Debugf("No tracked files for service %s, returning HealthBad", servicePath)

		return s6_shared.HealthBad, nil
	}

	// Use lifecycle manager for comprehensive health check
	return s.CheckArtifactsHealth(ctx, artifacts, fsService)
}
