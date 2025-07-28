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
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// ServiceArtifacts represents the essential paths for an S6 service
// Tracks only essential root paths to minimize I/O operations and improve performance
type ServiceArtifacts struct {
	// ServiceDir is the scan directory symlink path (e.g., /run/service/foo)
	ServiceDir string
	// RepositoryDir is the repository directory path (e.g., /data/services/foo)
	RepositoryDir string
	// LogDir is the external log directory (e.g., /data/logs/foo)
	LogDir string
	// TempDir is populated only during Create() for atomic operations (DEPRECATED: no longer used with symlink approach)
	TempDir string
	// CreatedFiles tracks all files created during service creation for health checks (paths point to repository files)
	CreatedFiles []string
	// RemovalProgress tracks what has been completed during removal for idempotent incremental removal
	RemovalProgress *RemovalProgress
}

// RemovalProgress tracks the state of removal operations using the skarnet sequence
// Each field represents a step that has been completed and verified
type RemovalProgress struct {
	// SymlinkRemoved indicates that the symlink has been removed from scan directory
	SymlinkRemoved bool
	// ServiceStopped indicates that s6-svc -wD -d has completed for both main and log services
	ServiceStopped bool
	// ServiceUnsupervised indicates that s6-svunlink has completed and supervisors have exited
	ServiceUnsupervised bool
	// DirectoriesRemoved indicates that both repository and log directories have been removed
	DirectoriesRemoved bool
}

// InitRemovalProgress initializes removal progress tracking if not already present
func (artifacts *ServiceArtifacts) InitRemovalProgress() {
	if artifacts.RemovalProgress == nil {
		artifacts.RemovalProgress = &RemovalProgress{}
	}
}

// IsFullyRemoved checks if all removal steps have been completed using the skarnet sequence
func (artifacts *ServiceArtifacts) IsFullyRemoved() bool {
	if artifacts.RemovalProgress == nil {
		return false
	}
	p := artifacts.RemovalProgress
	return p.SymlinkRemoved && p.ServiceStopped && p.ServiceUnsupervised && p.DirectoriesRemoved
}

// CreateArtifacts creates a complete S6 service using the symlink approach
// Uses symlink-based atomic creation:
// - Creates service files directly in repository directory
// - Creates atomic symlink to make service visible to scanner
// - S6 scanner notification to trigger supervision setup
func (s *DefaultService) CreateArtifacts(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) (*ServiceArtifacts, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	serviceName := filepath.Base(servicePath)
	repositoryDir := filepath.Join(constants.S6RepositoryBaseDir, serviceName)

	// Create artifacts structure
	artifacts := &ServiceArtifacts{
		ServiceDir:    servicePath,   // Scan directory symlink path
		RepositoryDir: repositoryDir, // Repository directory path
		LogDir:        filepath.Join(constants.S6LogBaseDir, serviceName),
		TempDir:       "", // No longer used with symlink approach
	}

	// Remove existing symlink if present
	if exists, _ := fsService.PathExists(ctx, servicePath); exists {
		if err := fsService.RemoveAll(ctx, servicePath); err != nil {
			return nil, fmt.Errorf("failed to remove existing symlink: %w", err)
		}
	}

	// Remove existing repository if present
	if exists, _ := fsService.PathExists(ctx, repositoryDir); exists {
		if err := fsService.RemoveAll(ctx, repositoryDir); err != nil {
			return nil, fmt.Errorf("failed to remove existing repository: %w", err)
		}
	}

	// Create service files directly in repository location
	createdFiles, err := s.createS6FilesInRepository(ctx, repositoryDir, config, fsService)
	if err != nil {
		// Clean up on failure
		_ = fsService.RemoveAll(ctx, repositoryDir)
		return nil, fmt.Errorf("failed to create service files: %w", err)
	}

	// NOW create the atomic symlink - this makes the service visible to scanner
	if err := fsService.Symlink(ctx, repositoryDir, servicePath); err != nil {
		// Clean up repository on symlink failure
		_ = fsService.RemoveAll(ctx, repositoryDir)
		return nil, fmt.Errorf("failed to create scan directory symlink: %w", err)
	}

	// Store the created files in artifacts (repository paths)
	artifacts.CreatedFiles = createdFiles

	s.LastConfigChangeAt = time.Now().UTC()

	// Notify S6 scanner of new service
	if _, err := s.EnsureSupervision(ctx, servicePath, fsService); err != nil {
		s.logger.Warnf("Failed to notify S6 scanner: %v", err)
	}

	s.logger.Debugf("Successfully created service artifacts: %+v", artifacts)
	return artifacts, nil
}

// RemoveArtifacts removes service artifacts using the symlink-aware skarnet sequence
// This approach follows the proper skarnet sequence, then removes the symlink with directories:
// 1. Stop service cleanly (s6-svc -wD -d) - waits for finish scripts
// 2. Unsupervise service (s6-svunlink) - removes from supervision and waits for supervisor exit
// 3. Remove symlink from scan directory + Remove repository and log directories
//
// Each step includes timeout handling using S6's native timeout parameters (-T/-t)
// and leverages S6's idempotent nature for safe operation.
// Retries for exit code 111 (timeout/system call failure) are handled automatically
// by the calling FSM system until the operation succeeds.
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

	// Step 1: Stop service cleanly using s6-svc -wD -d (idempotent, targeting scan directory symlink)
	if !progress.ServiceStopped {
		symlinkExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
		if symlinkExists {
			if err := s.stopServiceCleanly(ctx, artifacts.ServiceDir, fsService); err != nil {
				s.logger.Debugf("Failed to stop service cleanly during removal: %v", err)
				return fmt.Errorf("failed to stop service cleanly: %w", err)
			}
		}
		progress.ServiceStopped = true
		s.logger.Debugf("Service stopped cleanly: %s", artifacts.ServiceDir)
	}

	// Step 2: Unsupervise service using s6-svunlink (idempotent, targeting scan directory symlink)
	if !progress.ServiceUnsupervised {
		symlinkExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
		if symlinkExists {
			if err := s.unsuperviseService(ctx, artifacts.ServiceDir, fsService); err != nil {
				s.logger.Debugf("Failed to unsupervise service during removal: %v", err)
				return fmt.Errorf("failed to unsupervise service: %w", err)
			}
		}
		progress.ServiceUnsupervised = true
		s.logger.Debugf("Service unsupervised: %s", artifacts.ServiceDir)
	}

	// Step 3: Remove symlink + repository and log directories (idempotent)
	if !progress.DirectoriesRemoved {
		// Remove symlink from scan directory
		symlinkExists, _ := fsService.PathExists(ctx, artifacts.ServiceDir)
		if symlinkExists {
			if err := fsService.RemoveAll(ctx, artifacts.ServiceDir); err != nil {
				s.logger.Debugf("Failed to remove scan directory symlink: %v", err)
				return fmt.Errorf("failed to remove scan directory symlink: %w", err)
			}
		}

		// Remove repository directory (actual service files)
		repoExists, _ := fsService.PathExists(ctx, artifacts.RepositoryDir)
		if repoExists {
			s.logDirectoryContentsIfNotEmpty(ctx, artifacts.RepositoryDir, "repository directory", fsService)
			if err := fsService.RemoveAll(ctx, artifacts.RepositoryDir); err != nil {
				s.logger.Debugf("Failed to remove repository directory: %v", err)
				return fmt.Errorf("failed to remove repository directory: %w", err)
			}
		}

		// Remove external log directory
		logExists, _ := fsService.PathExists(ctx, artifacts.LogDir)
		if logExists {
			s.logDirectoryContentsIfNotEmpty(ctx, artifacts.LogDir, "log directory", fsService)
			if err := fsService.RemoveAll(ctx, artifacts.LogDir); err != nil {
				s.logger.Debugf("Failed to remove log directory: %v", err)
				return fmt.Errorf("failed to remove log directory: %w", err)
			}
		}

		progress.DirectoriesRemoved = true
		progress.SymlinkRemoved = true // Mark symlink as removed since it's part of directory removal
		s.logger.Debugf("Directories removed: symlink=%s, repository=%s, log=%s", artifacts.ServiceDir, artifacts.RepositoryDir, artifacts.LogDir)
	}

	s.logger.Debugf("Successfully completed symlink-aware skarnet sequence for service artifacts: %+v", artifacts)
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

	// Check symlink integrity (scan directory -> repository)
	symlinkExists, err := fsService.PathExists(ctx, artifacts.ServiceDir)
	if err != nil {
		s.logger.Debugf("Health check: I/O error checking symlink %s: %v", artifacts.ServiceDir, err)
		return HealthUnknown, err
	}
	if !symlinkExists {
		s.logger.Debugf("Health check: missing symlink %s", artifacts.ServiceDir)
		return HealthBad, nil
	}

	// Check repository directory exists
	repoExists, err := fsService.PathExists(ctx, artifacts.RepositoryDir)
	if err != nil {
		s.logger.Debugf("Health check: I/O error checking repository %s: %v", artifacts.RepositoryDir, err)
		return HealthUnknown, err
	}
	if !repoExists {
		s.logger.Debugf("Health check: missing repository directory %s", artifacts.RepositoryDir)
		return HealthBad, nil
	}

	// Check supervise directory consistency (in repository location)
	superviseMain := filepath.Join(artifacts.RepositoryDir, "supervise")
	superviseLog := filepath.Join(artifacts.RepositoryDir, "log", "supervise")

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

// createS6FilesInRepository creates the service files directly in the repository directory
func (s *DefaultService) createS6FilesInRepository(ctx context.Context, repositoryDir string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) ([]string, error) {
	var createdFiles []string

	// Create service directory structure
	if err := fsService.EnsureDirectory(ctx, repositoryDir); err != nil {
		return nil, fmt.Errorf("failed to create repository directory: %w", err)
	}

	// Create down file to prevent automatic startup
	downFilePath := filepath.Join(repositoryDir, "down")
	if err := fsService.WriteFile(ctx, downFilePath, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("failed to create down file: %w", err)
	}
	createdFiles = append(createdFiles, downFilePath)

	// Create type file (required for s6-rc)
	typeFile := filepath.Join(repositoryDir, "type")
	if err := fsService.WriteFile(ctx, typeFile, []byte("longrun"), 0644); err != nil {
		return nil, fmt.Errorf("failed to create type file: %w", err)
	}
	createdFiles = append(createdFiles, typeFile)

	// Create log service
	serviceName := filepath.Base(repositoryDir)
	logDir := filepath.Join(constants.S6LogBaseDir, serviceName)
	logServicePath := filepath.Join(repositoryDir, "log")

	if err := fsService.EnsureDirectory(ctx, logServicePath); err != nil {
		return nil, fmt.Errorf("failed to create log service directory: %w", err)
	}

	// Create log service type file (required for S6 to recognize it as a service)
	logTypeFile := filepath.Join(logServicePath, "type")
	if err := fsService.WriteFile(ctx, logTypeFile, []byte("longrun"), 0644); err != nil {
		return nil, fmt.Errorf("failed to create log service type file: %w", err)
	}
	createdFiles = append(createdFiles, logTypeFile)

	// Create log service down file to prevent automatic startup during creation
	logDownFile := filepath.Join(logServicePath, "down")
	if err := fsService.WriteFile(ctx, logDownFile, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("failed to create log service down file: %w", err)
	}
	createdFiles = append(createdFiles, logDownFile)

	// Create log run script immediately after other log service files to avoid race conditions
	logRunContent, err := getLogRunScript(config, logDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get log run script: %w", err)
	}

	logRunPath := filepath.Join(logServicePath, "run")
	if err := fsService.WriteFile(ctx, logRunPath, []byte(logRunContent), 0755); err != nil {
		return nil, fmt.Errorf("failed to write log run script: %w", err)
	}
	createdFiles = append(createdFiles, logRunPath)

	// Create main service run script using proven template system
	if len(config.Command) > 0 {
		if err := s.createS6RunScript(ctx, repositoryDir, fsService, config.Command, config.Env, config.MemoryLimit, repositoryDir); err != nil {
			return nil, fmt.Errorf("failed to create S6 run script: %w", err)
		}
		createdFiles = append(createdFiles, filepath.Join(repositoryDir, "run"))
	} else {
		return nil, fmt.Errorf("no command specified for service")
	}

	// Create config files using proven function
	configFiles, err := s.createS6ConfigFiles(ctx, repositoryDir, fsService, config.ConfigFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to create S6 config files: %w", err)
	}
	createdFiles = append(createdFiles, configFiles...)

	// Create dependencies
	dependenciesDPath := filepath.Join(repositoryDir, "dependencies.d")
	if err := fsService.EnsureDirectory(ctx, dependenciesDPath); err != nil {
		return nil, fmt.Errorf("failed to create dependencies.d directory: %w", err)
	}

	baseDepFile := filepath.Join(dependenciesDPath, "base")
	if err := fsService.WriteFile(ctx, baseDepFile, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("failed to create base dependency file: %w", err)
	}
	createdFiles = append(createdFiles, baseDepFile)

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

// stopServiceCleanly stops the service using S6's recommended approach
// Uses s6-svc -wD -d for clean shutdown and waits for finish scripts to complete
// This is step 1 of the skarnet sequence for proper service removal
// Now includes timeout handling with -T parameter and retry logic for exit code 111
func (s *DefaultService) stopServiceCleanly(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	servicePaths := []string{
		servicePath,                       // Main service
		filepath.Join(servicePath, "log"), // Log service subdirectory
	}

	var lastErr error

	for _, svcPath := range servicePaths {
		// Check if service path exists before attempting to stop
		exists, err := fsService.PathExists(ctx, svcPath)
		if err != nil {
			s.logger.Debugf("Failed to check if service path exists %s: %v", svcPath, err)
			lastErr = err
			continue
		}

		if !exists {
			s.logger.Debugf("Service path does not exist, skipping: %s", svcPath)
			continue
		}

		// Calculate timeout for this command
		timeoutMs := s.calculateS6Timeout(ctx)

		// Use s6-svc with optional timeout parameter
		// -w: wait for state change
		// -D: wait for the service to be down and finish script to complete
		// -d: send the service the "down" signal
		var args []string
		if timeoutMs > 0 {
			// -T: timeout in milliseconds
			args = []string{"-T", fmt.Sprintf("%d", timeoutMs), "-wD", "-d", svcPath}
		} else {
			// No timeout specified, use S6 default behavior
			args = []string{"-wD", "-d", svcPath}
		}

		_, err = s.ExecuteS6Command(ctx, svcPath, fsService, "s6-svc", args...)
		if err != nil {
			s.logger.Debugf("Failed to cleanly stop service %s: %v", svcPath, err)
			lastErr = err
			// Continue trying other services even if one fails
		} else {
			s.logger.Debugf("Successfully stopped service: %s", svcPath)
		}
	}

	return lastErr
}

// unsuperviseService removes the service from S6 supervision using s6-svunlink
// This is step 2 of the skarnet sequence for proper service removal
// s6-svunlink removes the service from s6-svscan supervision and waits for supervisor processes to exit
// Now includes timeout handling with -t parameter and verification that supervision actually ended
func (s *DefaultService) unsuperviseService(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	scanDir := filepath.Dir(servicePath)      // e.g., /run/service
	serviceName := filepath.Base(servicePath) // e.g., benthos-hello-world

	// Calculate timeout for this command
	timeoutMs := s.calculateS6Timeout(ctx)

	// Use s6-svunlink with optional timeout parameter
	// s6-svunlink removes the entire service tree from supervision
	// This handles both main service and log subdirectory supervisors
	var args []string
	if timeoutMs > 0 {
		// -t: timeout in milliseconds
		args = []string{"-t", fmt.Sprintf("%d", timeoutMs), scanDir, serviceName}
	} else {
		// No timeout specified, use S6 default behavior
		args = []string{scanDir, serviceName}
	}

	_, err := s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svunlink", args...)
	if err != nil {
		return fmt.Errorf("s6-svunlink command failed for service %s: %w", servicePath, err)
	}

	// Verify that supervision actually ended by checking the lock file
	// This follows the same logic as s6-svstat to detect if supervisor is still running
	if err := s.verifySupervisionEnded(ctx, servicePath, fsService); err != nil {
		return fmt.Errorf("supervision verification failed for service %s: %w", servicePath, err)
	}

	s.logger.Debugf("Successfully unsupervised service %s (including log subdirectory)", servicePath)
	return nil
}

// verifySupervisionEnded checks if supervision has actually ended using the same logic as s6-svstat
// Returns nil if supervision has ended, error if supervisor is still running or check failed
func (s *DefaultService) verifySupervisionEnded(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	// Check main service supervision
	if err := s.checkSingleSupervisionEnded(ctx, servicePath, fsService); err != nil {
		return fmt.Errorf("main service supervision still active: %w", err)
	}

	// Check log service supervision
	logServicePath := filepath.Join(servicePath, "log")
	if err := s.checkSingleSupervisionEnded(ctx, logServicePath, fsService); err != nil {
		return fmt.Errorf("log service supervision still active: %w", err)
	}

	return nil
}

// checkSingleSupervisionEnded checks if supervision has ended for a single service path
// Uses the same logic as s6_svc_ok() and s6-svstat to detect supervisor presence
func (s *DefaultService) checkSingleSupervisionEnded(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	lockFile := filepath.Join(servicePath, "supervise", "lock")

	// Check if lock file exists
	exists, err := fsService.PathExists(ctx, lockFile)
	if err != nil {
		return fmt.Errorf("failed to check lock file %s: %w", lockFile, err)
	}

	if !exists {
		// Lock file doesn't exist - supervision has ended
		s.logger.Debugf("Lock file %s does not exist - supervision ended", lockFile)
		return nil
	}

	// Lock file exists - check if it's locked (meaning supervisor is still running)
	// We use flock command with -n (non-blocking) to test if the file is locked
	// Exit code 1 means file is locked (supervisor running), 0 means not locked
	_, err = fsService.ExecuteCommand(ctx, "flock", "-n", lockFile, "true")
	if err != nil {
		// flock failed - this typically means the file is locked by supervisor
		s.logger.Debugf("Lock file %s is locked (supervisor still running): %v", lockFile, err)
		return fmt.Errorf("supervisor still running (lock file %s is locked)", lockFile)
	}

	// flock succeeded - file is not locked, supervision has ended
	s.logger.Debugf("Lock file %s exists but is not locked - supervision ended", lockFile)
	return nil
}

// Note: parseStatusFile logic has been moved to status.go as parseS6StatusFile
// This centralizes all S6 status parsing logic in one place

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

// calculateS6Timeout calculates the timeout for S6 commands based on remaining context time
// Uses S6_TIMEOUT_PERCENTAGE of remaining context time
func (s *DefaultService) calculateS6Timeout(ctx context.Context) int {
	deadline, ok := ctx.Deadline()
	if !ok {
		// No deadline set, return 0 to let S6 use its default behavior
		return 0
	}

	remainingTime := time.Until(deadline)
	if remainingTime <= 0 {
		// Context already expired, return 0
		return 0
	}

	// Calculate percentage-based timeout
	timeoutDuration := time.Duration(float64(remainingTime) * constants.S6_TIMEOUT_PERCENTAGE)

	// Convert to milliseconds for S6 -T parameter
	timeoutMs := int(timeoutDuration / time.Millisecond)

	s.logger.Debugf("S6 timeout calculated: %dms (remaining: %v, percentage: %.1f%%)",
		timeoutMs, remainingTime, constants.S6_TIMEOUT_PERCENTAGE*100)

	return timeoutMs
}

// logDirectoryContentsIfNotEmpty logs the contents of a directory if it's not empty
func (s *DefaultService) logDirectoryContentsIfNotEmpty(ctx context.Context, dirPath string, dirType string, fsService filesystem.Service) {
	entries, err := fsService.ReadDir(ctx, dirPath)
	if err != nil {
		s.logger.Debugf("Could not read %s contents before removal: %v", dirType, err)
		return
	}

	if len(entries) > 0 {
		var contents []string
		for _, entry := range entries {
			if entry.IsDir() {
				contents = append(contents, entry.Name()+"/")
			} else {
				contents = append(contents, entry.Name())
			}
		}
		s.logger.Debugf("Removing non-empty %s %s containing: %v", dirType, dirPath, contents)
	}
}
