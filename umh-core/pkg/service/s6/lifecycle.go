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

package s6

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// ServiceArtifacts represents the essential paths for an S6 service
// Tracks only essential root paths to minimize I/O operations and improve performance
type ServiceArtifacts struct {
	// ServiceDir is the main service directory (e.g., /data/services/foo)
	ServiceDir string
	// LogDir is the external log directory (e.g., /data/logs/foo)
	LogDir string
	// TempDir is populated only during Create() for atomic operations
	TempDir string
	// CreatedFiles tracks all files created during service creation for health checks
	CreatedFiles []string
	// RemovalProgress tracks what has been completed during removal for idempotent incremental removal
	RemovalProgress *RemovalProgress
}

// RemovalProgress tracks the state of removal operations for incremental idempotent removal
// Each field represents a step that has been completed and verified
type RemovalProgress struct {
	// ProcessesStopped indicates that S6 processes have been terminated
	ProcessesStopped bool
	// SupervisorsStopped indicates that S6 supervisor processes have been killed
	SupervisorsStopped bool
	// SupervisorsCleanupConfirmed indicates that S6 supervisor cleanup is complete
	SupervisorsCleanupConfirmed bool
	// ServiceDirRemoved indicates that the service directory has been successfully removed
	ServiceDirRemoved bool
	// LogDirRemoved indicates that the log directory has been successfully removed
	LogDirRemoved bool
}

// InitRemovalProgress initializes removal progress tracking if not already present
func (artifacts *ServiceArtifacts) InitRemovalProgress() {
	if artifacts.RemovalProgress == nil {
		artifacts.RemovalProgress = &RemovalProgress{}
	}
}

// IsFullyRemoved checks if all removal steps have been completed
func (artifacts *ServiceArtifacts) IsFullyRemoved() bool {
	if artifacts.RemovalProgress == nil {
		return false
	}
	p := artifacts.RemovalProgress
	return p.ProcessesStopped && p.SupervisorsStopped && p.SupervisorsCleanupConfirmed && p.ServiceDirRemoved && p.LogDirRemoved
}

// CreateArtifacts creates a complete S6 service atomically
// Uses proven atomic creation patterns:
// - EXDEV-safe temp directory (sibling of target) to avoid cross-device link errors
// - Atomic rename operation to prevent partially created services
// - .complete sentinel file to detect creation completion
// - S6 scanner notification to trigger supervision setup
func (s *DefaultService) CreateArtifacts(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) (*ServiceArtifacts, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check if target already exists and remove it if needed
	if exists, _ := fsService.PathExists(ctx, servicePath); exists {
		if err := fsService.RemoveAll(ctx, servicePath); err != nil {
			return nil, fmt.Errorf("failed to remove existing service directory: %w", err)
		}
	}

	// Create artifacts structure
	serviceName := filepath.Base(servicePath)
	artifacts := &ServiceArtifacts{
		ServiceDir: servicePath,
		LogDir:     filepath.Join(constants.S6LogBaseDir, serviceName),
		TempDir:    "", // Only populated during creation
	}

	// Create temp directory within S6BaseDir with dot prefix
	// S6 scanner ignores directories starting with dots, ensuring no interference
	tempID := s.generateUniqueID()
	artifacts.TempDir = filepath.Join(constants.S6BaseDir, ".new-"+tempID)

	// Setup cleanup function for failure cases
	cleanupTemp := func() {
		if artifacts.TempDir != "" {
			if tempExists, _ := fsService.PathExists(ctx, artifacts.TempDir); tempExists {
				if cleanupErr := fsService.RemoveAll(ctx, artifacts.TempDir); cleanupErr != nil {
					s.logger.Warnf("Failed to clean up temp directory %s: %v", artifacts.TempDir, cleanupErr)
				}
			}
		}
	}

	// Create all files in temp directory first
	createdFiles, err := s.createS6FilesInTemp(ctx, artifacts.TempDir, servicePath, config, fsService)
	if err != nil {
		cleanupTemp()
		return nil, fmt.Errorf("failed to create service files: %w", err)
	}

	// Add .complete sentinel file for atomic completion detection
	sentinelPath := filepath.Join(artifacts.TempDir, ".complete")
	if err := fsService.WriteFile(ctx, sentinelPath, []byte("ok"), 0644); err != nil {
		cleanupTemp()
		return nil, fmt.Errorf("failed to create sentinel file: %w", err)
	}
	createdFiles = append(createdFiles, ".complete")

	// Atomically rename temp directory to final location
	if err := fsService.Rename(ctx, artifacts.TempDir, servicePath); err != nil {
		cleanupTemp()
		return nil, fmt.Errorf("failed to atomically create service: %w", err)
	}

	// Store the created files in artifacts (now in final location)
	artifacts.CreatedFiles = make([]string, len(createdFiles))
	for i, file := range createdFiles {
		artifacts.CreatedFiles[i] = filepath.Join(servicePath, file)
	}

	// Clear temp directory since rename succeeded
	artifacts.TempDir = ""

	// Notify S6 scanner of new service
	if _, err := s.EnsureSupervision(ctx, servicePath, fsService); err != nil {
		s.logger.Warnf("Failed to notify S6 scanner: %v", err)
	}

	s.logger.Debugf("Successfully created service artifacts: %+v", artifacts)
	return artifacts, nil
}

// RemoveArtifacts removes service artifacts using an incremental, idempotent approach:
// - Uses unified lifecycle mutex to prevent concurrent operations
// - Tracks removal progress in artifacts to continue from where it left off
// - Each call is fast (<100ms) to respect FSM context timeouts
// - Fully idempotent - safe to call repeatedly during the removal process
// - Returns nil only when nothing is left
func (s *DefaultService) RemoveArtifacts(ctx context.Context, artifacts *ServiceArtifacts, fsService filesystem.Service) error {
	if s == nil {
		return fmt.Errorf("lifecycle manager is nil")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if artifacts == nil {
		return fmt.Errorf("artifacts is nil")
	}

	// Initialize removal progress tracking
	artifacts.InitRemovalProgress()

	// Fast path: Check if already fully removed
	if artifacts.IsFullyRemoved() {
		s.logger.Debugf("Service artifacts already fully removed: %+v", artifacts)
		return nil
	}

	progress := artifacts.RemovalProgress

	// Step 1: Stop S6 processes (idempotent)
	if !progress.ProcessesStopped {
		serviceExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
		if serviceExists {
			if err := s.terminateProcesses(ctx, artifacts, fsService); err != nil {
				s.logger.Debugf("Failed to terminate processes during removal: %v", err)
				return fmt.Errorf("failed to terminate processes: %w", err)
			}
		}
		progress.ProcessesStopped = true
		s.logger.Debugf("Processes stopped for service: %s", artifacts.ServiceDir)
	}

	// Step 2: Stop supervisor processes (idempotent)
	if !progress.SupervisorsStopped {
		serviceExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
		if serviceExists {
			if err := s.killSupervisors(ctx, artifacts, fsService); err != nil {
				s.logger.Debugf("Failed to kill supervisors during removal: %v", err)
				return fmt.Errorf("failed to kill supervisors: %w", err)
			}
		}
		progress.SupervisorsStopped = true
		s.logger.Debugf("Supervisors stopped for service: %s", artifacts.ServiceDir)
	}

	// Step 2.5: Confirm supervisor cleanup is complete (idempotent)
	//
	// RACE CONDITION FIX:
	// This step solves a 4.5ms race condition discovered in integration tests.
	// Timeline analysis showed "directory not empty" errors when we attempted removal
	// immediately after killing supervisors, before S6 completed internal cleanup.
	//
	// THE PROBLEM:
	// 12:43:22.297 - killSupervisors() terminates S6 processes
	// 12:43:22.297 - Directory removal fails: "directory not empty"
	// 12:43:22.393 - Retry succeeds 96ms later
	//
	// ROOT CAUSE:
	// S6 supervisors need time after termination to:
	// 1. Run finish scripts (./finish with 5sec timeout)
	// 2. Clean up internal state files
	// 3. Close file handles in supervise/ directory
	// 4. Update status file with flagfinishing=0
	//
	// THE SOLUTION:
	// Instead of arbitrary delays, we use S6's own completion signal (flagfinishing flag).
	// From S6 source code (uplastup_z() and set_down_and_ready() functions):
	// - flagfinishing=1: Cleanup in progress (S6 still has files open)
	// - flagfinishing=0: Cleanup complete (safe to remove directories)
	//
	// This respects S6's documented behavior: "When killed or asked to exit,
	// it waits for the service to go down one last time, then exits."
	//
	// FSM COMPATIBILITY:
	// - No blocking: Returns immediately if cleanup not ready
	// - Incremental progress: Each FSM tick checks if ready to proceed
	// - Fast: Typically completes in 5-10ms (1-2 FSM ticks)
	if !progress.SupervisorsCleanupConfirmed {
		serviceExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
		if serviceExists {
			if confirmed, err := s.isSupervisorCleanupComplete(ctx, artifacts, fsService); err != nil {
				s.logger.Debugf("Failed to check supervisor cleanup: %v", err)
				return fmt.Errorf("failed to check supervisor cleanup: %w", err)
			} else if !confirmed {
				s.logger.Debugf("Supervisor cleanup not yet complete for service: %s", artifacts.ServiceDir)
				return nil // Return and wait for next FSM tick
			}
		}
		progress.SupervisorsCleanupConfirmed = true
		s.logger.Debugf("Supervisor cleanup confirmed for service: %s", artifacts.ServiceDir)
	}

	// Step 3: Remove service directory (idempotent)
	if !progress.ServiceDirRemoved {
		serviceExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
		if serviceExists {
			if err := fsService.RemoveAll(ctx, artifacts.ServiceDir); err != nil {
				s.logger.Debugf("Failed to remove service directory: %v", err)
				return fmt.Errorf("failed to remove service directory: %w", err)
			}
		}
		progress.ServiceDirRemoved = true
		s.logger.Debugf("Service directory removed: %s", artifacts.ServiceDir)
	}

	// Step 4: Remove log directory (idempotent)
	if !progress.LogDirRemoved {
		logExists, _ := fsService.PathExists(ctx, artifacts.LogDir)
		if logExists {
			if err := fsService.RemoveAll(ctx, artifacts.LogDir); err != nil {
				s.logger.Debugf("Failed to remove log directory: %v", err)
				return fmt.Errorf("failed to remove log directory: %w", err)
			}
		}
		progress.LogDirRemoved = true
		s.logger.Debugf("Log directory removed: %s", artifacts.LogDir)
	}

	s.logger.Debugf("Successfully completed all removal steps for service artifacts: %+v", artifacts)
	return nil
}

// CheckArtifactsHealth performs tri-state health check on service artifacts
// Returns:
// - HealthUnknown: I/O errors, timeouts, etc. (retry next tick)
// - HealthOK: Service directory is healthy and complete
// - HealthBad: Service directory is broken (triggers FSM transition)
func (s *DefaultService) CheckArtifactsHealth(ctx context.Context, artifacts *ServiceArtifacts, fsService filesystem.Service) (HealthStatus, error) {
	if s == nil {
		return HealthUnknown, fmt.Errorf("lifecycle manager is nil")
	}

	if ctx.Err() != nil {
		return HealthUnknown, ctx.Err()
	}

	if artifacts == nil {
		return HealthBad, fmt.Errorf("artifacts is nil")
	}

	// Always use tracked files for health check
	if len(artifacts.CreatedFiles) == 0 {
		// No tracked files indicates service was not properly created or is from old version
		s.logger.Debugf("Health check: no tracked files available, service needs recreation")
		return HealthBad, nil
	}

	// Check all tracked files exist
	for _, file := range artifacts.CreatedFiles {
		exists, err := fsService.FileExists(ctx, file)
		if err != nil {
			// I/O error - return Unknown so we retry next tick
			s.logger.Debugf("Health check: I/O error checking tracked file %s: %v", file, err)
			return HealthUnknown, err
		}
		if !exists {
			// Missing required file - definitely broken
			s.logger.Debugf("Health check: missing tracked file %s", file)
			return HealthBad, nil
		}
	}

	// Check supervise directory consistency
	superviseMain := filepath.Join(artifacts.ServiceDir, "supervise")
	superviseLog := filepath.Join(artifacts.ServiceDir, "log", "supervise")

	mainExists, mainErr := fsService.PathExists(ctx, superviseMain)
	logExists, logErr := fsService.PathExists(ctx, superviseLog)

	// If either check failed due to I/O error, return Unknown
	if mainErr != nil || logErr != nil {
		s.logger.Debugf("Health check: I/O error checking supervise directories: main=%v, log=%v", mainErr, logErr)
		return HealthUnknown, fmt.Errorf("supervise directory check failed: main=%v, log=%v", mainErr, logErr)
	}

	// If supervise directories exist, both must exist (prevents race condition)
	if mainExists != logExists {
		s.logger.Debugf("Health check: supervise directory mismatch - main=%v, log=%v", mainExists, logExists)
		return HealthBad, nil
	}

	// All checks passed
	return HealthOK, nil
}

// generateUniqueID creates a unique identifier for temp directories
func (s *DefaultService) generateUniqueID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// createS6FilesInTemp creates the service files in the temp directory
func (s *DefaultService) createS6FilesInTemp(ctx context.Context, tempDir string, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) ([]string, error) {
	var createdFiles []string
	// Create service directory structure
	if err := fsService.EnsureDirectory(ctx, tempDir); err != nil {
		return nil, fmt.Errorf("failed to create service directory: %w", err)
	}

	// Create down file to prevent automatic startup
	downFilePath := filepath.Join(tempDir, "down")
	if err := fsService.WriteFile(ctx, downFilePath, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("failed to create down file: %w", err)
	}
	createdFiles = append(createdFiles, "down")

	// Create type file (required for s6-rc)
	typeFile := filepath.Join(tempDir, "type")
	if err := fsService.WriteFile(ctx, typeFile, []byte("longrun"), 0644); err != nil {
		return nil, fmt.Errorf("failed to create type file: %w", err)
	}
	createdFiles = append(createdFiles, "type")

	// Create log service
	serviceName := filepath.Base(servicePath)
	logDir := filepath.Join(constants.S6LogBaseDir, serviceName)
	logServicePath := filepath.Join(tempDir, "log")

	if err := fsService.EnsureDirectory(ctx, logServicePath); err != nil {
		return nil, fmt.Errorf("failed to create log service directory: %w", err)
	}

	// Create log service type file (required for S6 to recognize it as a service)
	logTypeFile := filepath.Join(logServicePath, "type")
	if err := fsService.WriteFile(ctx, logTypeFile, []byte("longrun"), 0644); err != nil {
		return nil, fmt.Errorf("failed to create log service type file: %w", err)
	}
	createdFiles = append(createdFiles, "log/type")

	// Create log service down file to prevent automatic startup during creation
	logDownFile := filepath.Join(logServicePath, "down")
	if err := fsService.WriteFile(ctx, logDownFile, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("failed to create log service down file: %w", err)
	}
	createdFiles = append(createdFiles, "log/down")

	// Create log run script immediately after other log service files to avoid race conditions
	logRunContent, err := getLogRunScript(config, logDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get log run script: %w", err)
	}

	logRunPath := filepath.Join(logServicePath, "run")
	if err := fsService.WriteFile(ctx, logRunPath, []byte(logRunContent), 0755); err != nil {
		return nil, fmt.Errorf("failed to write log run script: %w", err)
	}
	createdFiles = append(createdFiles, "log/run")

	// Create main service run script using proven template system with the correct final service path
	if len(config.Command) > 0 {
		if err := s.createS6RunScript(ctx, tempDir, fsService, config.Command, config.Env, config.MemoryLimit, servicePath); err != nil {
			return nil, fmt.Errorf("failed to create S6 run script: %w", err)
		}
		createdFiles = append(createdFiles, "run")
	} else {
		return nil, fmt.Errorf("no command specified for service")
	}

	// Create config files using proven function
	configFiles, err := s.createS6ConfigFiles(ctx, tempDir, fsService, config.ConfigFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to create S6 config files: %w", err)
	}
	createdFiles = append(createdFiles, configFiles...)

	// Create dependencies
	dependenciesDPath := filepath.Join(tempDir, "dependencies.d")
	if err := fsService.EnsureDirectory(ctx, dependenciesDPath); err != nil {
		return nil, fmt.Errorf("failed to create dependencies.d directory: %w", err)
	}

	baseDepFile := filepath.Join(dependenciesDPath, "base")
	if err := fsService.WriteFile(ctx, baseDepFile, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("failed to create base dependency file: %w", err)
	}
	createdFiles = append(createdFiles, "dependencies.d/base")

	return createdFiles, nil
}

// createS6RunScript creates a run script for the service using the proven template system
func (s *DefaultService) createS6RunScript(ctx context.Context, servicePath string, fsService filesystem.Service, command []string, env map[string]string, memoryLimit int64, finalServicePath string) error {
	runScript := filepath.Join(servicePath, "run")

	// Create template data - include ServicePath for the template
	data := struct {
		Command     []string
		Env         map[string]string
		MemoryLimit int64
		ServicePath string
	}{
		Command:     command,
		Env:         env,
		MemoryLimit: memoryLimit,
		ServicePath: finalServicePath,
	}

	// Parse and execute the template
	tmpl, err := template.New("runscript").Parse(runScriptTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse run script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute run script template: %w", err)
	}

	// Write the templated content directly to the file with executable permissions
	if err := fsService.WriteFile(ctx, runScript, buf.Bytes(), 0755); err != nil {
		return fmt.Errorf("failed to write run script: %w", err)
	}

	return nil
}

// createS6ConfigFiles creates config files needed by the service using the proven method
func (s *DefaultService) createS6ConfigFiles(ctx context.Context, servicePath string, fsService filesystem.Service, configFiles map[string]string) ([]string, error) {
	if len(configFiles) == 0 {
		return nil, nil
	}

	configPath := filepath.Join(servicePath, "config")
	var createdFiles []string

	for path, content := range configFiles {

		// Validate config file path for security and correctness
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("config filename cannot be empty")
		}

		// Prevent path traversal attacks
		if strings.Contains(path, "..") {
			return nil, fmt.Errorf("invalid config filename contains path traversal: %s", path)
		}

		// For relative paths, ensure they don't contain directory separators (except for subdirectories)
		if !filepath.IsAbs(path) {
			// Clean the path to normalize it
			cleanPath := filepath.Clean(path)

			// Check if the cleaned path tries to escape the config directory
			if strings.HasPrefix(cleanPath, "../") || cleanPath == ".." {
				return nil, fmt.Errorf("invalid config filename attempts to escape config directory: %s", path)
			}

			// Reject absolute-like paths in relative context
			if strings.HasPrefix(path, "/") {
				return nil, fmt.Errorf("config filename cannot start with '/': %s", path)
			}
		}

		// Check for reserved/dangerous filenames
		baseName := filepath.Base(path)
		reservedNames := []string{".", "..", "CON", "PRN", "AUX", "NUL"}
		for _, reserved := range reservedNames {
			if strings.EqualFold(baseName, reserved) {
				return nil, fmt.Errorf("config filename cannot be reserved name: %s", path)
			}
		}

		// Validate filename doesn't contain dangerous characters
		if strings.ContainsAny(baseName, "\x00\r\n") {
			return nil, fmt.Errorf("config filename contains invalid characters: %s", path)
		}

		// Store original path for tracking
		originalPath := path

		// If path is relative, make it relative to service directory
		if !filepath.IsAbs(path) {
			path = filepath.Join(configPath, path)
		}

		// Create directory if it doesn't exist
		dir := filepath.Dir(path)
		if err := fsService.EnsureDirectory(ctx, dir); err != nil {
			return nil, fmt.Errorf("failed to create directory for config file: %w", err)
		}

		// Create and write the file
		if err := fsService.WriteFile(ctx, path, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("failed to write to config file %s: %w", path, err)
		}

		// Track the created file using relative path from service directory
		createdFiles = append(createdFiles, filepath.Join("config", originalPath))
	}

	return createdFiles, nil
}

// createDownFiles creates down files to prevent service startup
func (s *DefaultService) createDownFiles(ctx context.Context, artifacts *ServiceArtifacts, fsService filesystem.Service) error {
	downFiles := []string{
		filepath.Join(artifacts.ServiceDir, "down"),
		filepath.Join(artifacts.ServiceDir, "log", "down"),
	}

	for _, downFile := range downFiles {
		if err := fsService.WriteFile(ctx, downFile, []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to create down file %s: %w", downFile, err)
		}
	}

	return nil
}

// terminateProcesses attempts immediate termination of services and their supervisors
// This is called during force removal scenarios where graceful termination has already failed.
// Uses s6-svc -xd which brings down the service AND exits the supervisor immediately - no grace period.
func (s *DefaultService) terminateProcesses(ctx context.Context, artifacts *ServiceArtifacts, fsService filesystem.Service) error {
	servicePaths := []string{
		artifacts.ServiceDir,
		filepath.Join(artifacts.ServiceDir, "log"),
	}

	var lastErr error

	for _, servicePath := range servicePaths {
		// Use -xd flag: brings down service and exits supervisor immediately
		// This ensures supervisor processes don't remain after service termination
		if _, err := s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-xd", servicePath); err != nil {
			s.logger.Debugf("Failed to terminate service and supervisor for %s: %v", servicePath, err)
			lastErr = err

			// If s6-svc fails, fall back to direct supervisor process killing
			// This handles cases where S6 commands are unresponsive
			if killErr := s.killSupervisorProcess(ctx, servicePath, fsService); killErr != nil {
				s.logger.Debugf("Failed to kill supervisor process directly for %s: %v", servicePath, killErr)
				// Keep the s6-svc error as the primary error since it's more specific
			}
		}
	}

	return lastErr
}

// Note: parseStatusFile logic has been moved to status.go as parseS6StatusFile
// This centralizes all S6 status parsing logic in one place

// isSupervisorCleanupComplete checks if S6 supervisor cleanup is complete
//
// CRITICAL DESIGN: This function uses S6's own internal state tracking to detect
// when supervisors have completed their cleanup sequence. This eliminates the
// 4.5ms race condition that caused "directory not empty" errors in integration tests.
//
// S6 CLEANUP LIFECYCLE (from source code analysis):
// 1. Service dies → uplastup_z() called
// 2. S6 sets flagfinishing=1 (cleanup begins)
// 3. S6 spawns ./finish script (5sec timeout by default)
// 4. S6 cleans internal state, closes file handles
// 5. S6 calls set_down_and_ready() → flagfinishing=0 (cleanup complete)
//
// DETECTION METHODS:
// Method 1 (Primary): Check S6 status file flagfinishing flag
// - Uses parseS6StatusFile() to read 43-byte binary status file directly
// - flagfinishing=1: S6 still cleaning up (return false, wait for next FSM tick)
// - flagfinishing=0: S6 cleanup complete (safe to proceed with removal)
// - pid=0: Ensures service process has fully exited
//
// Method 2 (Fallback): Check supervisor PID file
// - If status file unavailable, check if supervisor process still exists
// - Uses syscall.Kill(pid, 0) to test process existence without sending signal
//
// AVOIDS CIRCULAR DEPENDENCIES:
// We cannot call Status() method during removal because:
// - FSM blocks Status() calls during removal operations
// - Status() reads config files that we're about to delete
// - This would create circular dependency: removal → Status() → config read → failure
//
// Instead we use parseS6StatusFile() directly:
// - Same binary parser as Status() method (centralized in status.go)
// - No business logic, just raw S6 state data
// - No config file dependencies
// - Lightweight, fast operation (<1ms)
//
// Uses parseStatusFile() to avoid circular dependency with Status() method
func (s *DefaultService) isSupervisorCleanupComplete(ctx context.Context, artifacts *ServiceArtifacts, fsService filesystem.Service) (bool, error) {
	servicePaths := []string{
		artifacts.ServiceDir,
		filepath.Join(artifacts.ServiceDir, "log"),
	}

	for _, servicePath := range servicePaths {
		superviseDir := filepath.Join(servicePath, "supervise")

		// Check if supervise directory exists
		exists, err := fsService.PathExists(ctx, superviseDir)
		if err != nil {
			return false, fmt.Errorf("failed to check supervise directory %s: %w", superviseDir, err)
		}

		if !exists {
			// No supervise directory means cleanup is complete
			continue
		}

		// Method 1: Check S6 status file for IsFinishing flag
		// Use parseStatusFile() to avoid circular dependency with Status() method
		statusFile := filepath.Join(superviseDir, "status")
		statusExists, err := fsService.FileExists(ctx, statusFile)
		if err != nil {
			return false, fmt.Errorf("failed to check status file %s: %w", statusFile, err)
		}

		if statusExists {
			// Parse status file directly using centralized parser - avoids circular dependency and FSM blocks
			statusData, err := parseS6StatusFile(ctx, statusFile, fsService)
			if err != nil {
				// Can't read status, assume cleanup in progress
				return false, nil
			}

			// If IsFinishing=true, supervisor is still cleaning up
			if statusData.IsFinishing {
				return false, nil
			}

			// If process is still running (PID != 0), cleanup not complete
			if statusData.Pid != 0 {
				return false, nil
			}
		}

		// Method 2: Check if supervisor process is still running via PID file
		pidFile := filepath.Join(superviseDir, "pid")
		pidExists, err := fsService.FileExists(ctx, pidFile)
		if err != nil {
			return false, fmt.Errorf("failed to check PID file %s: %w", pidFile, err)
		}

		if pidExists {
			// Read PID and check if process exists
			data, err := fsService.ReadFile(ctx, pidFile)
			if err != nil {
				// Can't read PID file, assume cleanup in progress
				return false, nil
			}

			pidStr := strings.TrimSpace(string(data))
			if pidStr != "" {
				pid, err := strconv.Atoi(pidStr)
				if err == nil {
					// Check if process still exists (kill -0 doesn't send signal)
					if err := syscall.Kill(pid, 0); err == nil {
						// Process still exists, cleanup not complete
						return false, nil
					}
				}
			}
		}
	}

	// All supervisors have completed cleanup
	return true, nil
}

// killSupervisorProcess directly kills supervisor process when S6 commands fail
// This is a last resort when s6-svc -xd doesn't work (e.g., corrupted supervise state)
func (s *DefaultService) killSupervisorProcess(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	supervisePidFile := filepath.Join(servicePath, "supervise", "pid")
	data, err := fsService.ReadFile(ctx, supervisePidFile)
	if err != nil {
		return err // No pid file means no supervisor to kill
	}

	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return fmt.Errorf("empty pid file for %s", servicePath)
	}

	// Try SIGTERM first for clean shutdown
	if _, err := fsService.ExecuteCommand(ctx, "kill", "-TERM", pidStr); err != nil {
		s.logger.Debugf("Failed to send SIGTERM to supervisor %s: %v", pidStr, err)
	}

	// Wait briefly, then use SIGKILL if process still exists
	select {
	case <-time.After(gracePeriodForTermination):
		// Process didn't terminate gracefully, use SIGKILL
		if _, err := fsService.ExecuteCommand(ctx, "kill", "-KILL", pidStr); err != nil {
			return fmt.Errorf("failed to kill supervisor process %s: %w", pidStr, err)
		}
		s.logger.Debugf("Sent SIGKILL to supervisor process %s", pidStr)
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// removeFileFromArtifacts removes a file from the artifacts tracking
func (s *DefaultService) removeFileFromArtifacts(filePath string) {
	if s.artifacts == nil {
		return
	}

	// Remove from CreatedFiles slice
	for i, createdFile := range s.artifacts.CreatedFiles {
		if createdFile == filePath {
			s.artifacts.CreatedFiles = append(s.artifacts.CreatedFiles[:i], s.artifacts.CreatedFiles[i+1:]...)
			break
		}
	}
}

// addFileToArtifacts adds a file to the artifacts tracking
func (s *DefaultService) addFileToArtifacts(filePath string) {
	if s.artifacts == nil {
		return
	}

	// Check if file is already tracked
	for _, createdFile := range s.artifacts.CreatedFiles {
		if createdFile == filePath {
			return // Already tracked
		}
	}

	// Add to CreatedFiles slice
	s.artifacts.CreatedFiles = append(s.artifacts.CreatedFiles, filePath)
}
