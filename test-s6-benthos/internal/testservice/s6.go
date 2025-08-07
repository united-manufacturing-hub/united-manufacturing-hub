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

package testservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	S6BaseDir           = "/run/service"
	S6RepositoryBaseDir = "/data/services"
	S6LogBaseDir        = "/data/logs"
	S6ConfigDirName     = "config"
)

// S6ServiceConfig contains configuration for creating a service
type S6ServiceConfig struct {
	Env         map[string]string
	ConfigFiles map[string]string
	Command     []string
	MemoryLimit int64
	LogFilesize int64
}

// ServiceStatus represents the status of an S6 service
type ServiceStatus string

const (
	ServiceUnknown    ServiceStatus = "unknown"
	ServiceUp         ServiceStatus = "up"
	ServiceDown       ServiceStatus = "down"
	ServiceRestarting ServiceStatus = "restarting"
)

// ServiceInfo contains information about an S6 service
type ServiceInfo struct {
	LastChangedAt      time.Time
	LastReadyAt        time.Time
	LastDeploymentTime time.Time
	Status             ServiceStatus
	Uptime             int64
	DownTime           int64
	ReadyTime          int64
	Pid                int
	Pgid               int
	ExitCode           int
	WantUp             bool
	IsPaused           bool
	IsFinishing        bool
	IsWantingUp        bool
	IsReady            bool
}

// LogEntry represents a parsed log entry from the S6 logs
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
}

// ExitEvent represents a service exit event
type ExitEvent struct {
	Timestamp time.Time
	ExitCode  int
	Signal    int
}

// S6Service provides a simplified S6 service interface for testing
type S6Service struct {
	logger *zap.SugaredLogger
}

// NewS6Service creates a new S6 service for testing
func NewS6Service(logger *zap.SugaredLogger) *S6Service {
	return &S6Service{
		logger: logger,
	}
}

// Create creates the S6 service with specific configuration
func (s *S6Service) Create(ctx context.Context, servicePath string, config S6ServiceConfig, fsService FilesystemInterface) error {
	s.logger.Debugf("Creating S6 service %s", servicePath)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Create service directory structure
	if err := fsService.EnsureDirectory(ctx, servicePath); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}

	// Create log service directory
	logServicePath := filepath.Join(servicePath, "log")
	if err := fsService.EnsureDirectory(ctx, logServicePath); err != nil {
		return fmt.Errorf("failed to create log service directory: %w", err)
	}

	// Create config directory
	configPath := filepath.Join(servicePath, S6ConfigDirName)
	if err := fsService.EnsureDirectory(ctx, configPath); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config files
	for filename, content := range config.ConfigFiles {
		configFilePath := filepath.Join(configPath, filename)
		if err := fsService.WriteFile(ctx, configFilePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write config file %s: %w", filename, err)
		}
	}

	// Create main run script
	runScript := s.generateRunScript(config)
	runPath := filepath.Join(servicePath, "run")
	if err := fsService.WriteFile(ctx, runPath, []byte(runScript), 0755); err != nil {
		return fmt.Errorf("failed to write run script: %w", err)
	}

	// Create log run script
	serviceName := filepath.Base(servicePath)
	logRunScript := s.generateLogRunScript(config, serviceName)
	logRunPath := filepath.Join(logServicePath, "run")
	if err := fsService.WriteFile(ctx, logRunPath, []byte(logRunScript), 0755); err != nil {
		return fmt.Errorf("failed to write log run script: %w", err)
	}

	// Create dependencies directory
	depsPath := filepath.Join(servicePath, "dependencies.d")
	if err := fsService.EnsureDirectory(ctx, depsPath); err != nil {
		return fmt.Errorf("failed to create dependencies directory: %w", err)
	}

	// Create base dependency file
	baseDep := filepath.Join(depsPath, "base")
	if err := fsService.WriteFile(ctx, baseDep, []byte{}, 0644); err != nil {
		return fmt.Errorf("failed to create base dependency: %w", err)
	}

	// Create symlink in scan directory
	scanPath := filepath.Join(S6BaseDir, filepath.Base(servicePath))
	if err := fsService.Symlink(ctx, servicePath, scanPath); err != nil {
		s.logger.Warnf("Failed to create symlink %s -> %s: %v", scanPath, servicePath, err)
		// Continue anyway, s6 might still work
	}

	// Notify s6-svscan to rescan for new services
	_, err := fsService.ExecuteCommand(ctx, "s6-svscanctl", "-a", S6BaseDir)
	if err != nil {
		s.logger.Debugf("Failed to notify s6-svscan: %v", err)
		// This is not critical, s6-svscan will eventually discover the service
	}

	s.logger.Debugf("Successfully created S6 service %s", servicePath)
	return nil
}

// Remove removes the service directory structure
func (s *S6Service) Remove(ctx context.Context, servicePath string, fsService FilesystemInterface) error {
	s.logger.Debugf("Removing S6 service %s", servicePath)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Remove symlink from scan directory
	scanPath := filepath.Join(S6BaseDir, filepath.Base(servicePath))
	if exists, _ := fsService.PathExists(ctx, scanPath); exists {
		if err := fsService.Remove(ctx, scanPath); err != nil {
			s.logger.Warnf("Failed to remove symlink %s: %v", scanPath, err)
		}
	}

	// Remove service directory
	if err := fsService.RemoveAll(ctx, servicePath); err != nil {
		return fmt.Errorf("failed to remove service directory: %w", err)
	}

	s.logger.Debugf("Successfully removed S6 service %s", servicePath)
	return nil
}

// Start starts the service
func (s *S6Service) Start(ctx context.Context, servicePath string, fsService FilesystemInterface) error {
	s.logger.Debugf("Starting S6 service %s", servicePath)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Wait for s6-supervise to start monitoring this service
	if err := s.waitForSupervision(ctx, servicePath, fsService); err != nil {
		return fmt.Errorf("service supervision not ready: %w", err)
	}

	// Remove down files if they exist
	downFiles := []string{
		filepath.Join(servicePath, "down"),
		filepath.Join(servicePath, "log", "down"),
	}

	for _, downFile := range downFiles {
		if exists, _ := fsService.FileExists(ctx, downFile); exists {
			if err := fsService.Remove(ctx, downFile); err != nil {
				s.logger.Warnf("Failed to remove down file %s: %v", downFile, err)
			}
		}
	}

	// Start log service first
	logServicePath := filepath.Join(servicePath, "log")
	_, err := fsService.ExecuteCommand(ctx, "s6-svc", "-u", logServicePath)
	if err != nil {
		s.logger.Warnf("Failed to start log service: %v", err)
	}

	// Start main service
	_, err = fsService.ExecuteCommand(ctx, "s6-svc", "-u", servicePath)
	if err != nil {
		s.logger.Warnf("Failed to start service: %v", err)
		return fmt.Errorf("failed to start service: %w", err)
	}

	s.logger.Debugf("Started S6 service %s", servicePath)
	return nil
}

// Stop stops the service
func (s *S6Service) Stop(ctx context.Context, servicePath string, fsService FilesystemInterface) error {
	s.logger.Debugf("Stopping S6 service %s", servicePath)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Stop main service
	_, err := fsService.ExecuteCommand(ctx, "s6-svc", "-d", servicePath)
	if err != nil {
		s.logger.Warnf("Failed to stop service: %v", err)
	}

	// Stop log service
	logServicePath := filepath.Join(servicePath, "log")
	_, err = fsService.ExecuteCommand(ctx, "s6-svc", "-d", logServicePath)
	if err != nil {
		s.logger.Warnf("Failed to stop log service: %v", err)
	}

	// Create down files
	downFiles := []string{
		filepath.Join(servicePath, "down"),
		filepath.Join(servicePath, "log", "down"),
	}

	for _, downFile := range downFiles {
		if err := fsService.WriteFile(ctx, downFile, []byte{}, 0644); err != nil {
			s.logger.Warnf("Failed to create down file %s: %v", downFile, err)
		}
	}

	s.logger.Debugf("Stopped S6 service %s", servicePath)
	return nil
}

// Status gets the current status of the service
func (s *S6Service) Status(ctx context.Context, servicePath string, fsService FilesystemInterface) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	info := ServiceInfo{
		Status: ServiceUnknown,
	}

	// Check if service exists
	exists, err := fsService.PathExists(ctx, servicePath)
	if err != nil {
		return info, fmt.Errorf("failed to check if service exists: %w", err)
	}
	if !exists {
		return info, fmt.Errorf("service does not exist")
	}

	// Check supervise directory
	superviseDir := filepath.Join(servicePath, "supervise")
	exists, err = fsService.PathExists(ctx, superviseDir)
	if err != nil {
		return info, fmt.Errorf("failed to check supervise directory: %w", err)
	}
	if !exists {
		return info, nil // Service not yet supervised
	}

	// Try to get status from s6-svstat
	output, err := fsService.ExecuteCommand(ctx, "s6-svstat", servicePath)
	if err != nil {
		s.logger.Debugf("Failed to get service status via s6-svstat: %v", err)
		return info, nil
	}

	// Parse s6-svstat output
	statusStr := string(output)
	if strings.Contains(statusStr, "up") {
		info.Status = ServiceUp
	} else if strings.Contains(statusStr, "down") {
		info.Status = ServiceDown
	}

	return info, nil
}

// ServiceExists checks if the service directory exists
func (s *S6Service) ServiceExists(ctx context.Context, servicePath string, fsService FilesystemInterface) (bool, error) {
	return fsService.PathExists(ctx, servicePath)
}

// GetLogs gets the logs of the service
func (s *S6Service) GetLogs(ctx context.Context, servicePath string, fsService FilesystemInterface) ([]LogEntry, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	serviceName := filepath.Base(servicePath)
	logFile := filepath.Join(S6LogBaseDir, serviceName, "current")

	exists, err := fsService.PathExists(ctx, logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check if log file exists: %w", err)
	}
	if !exists {
		return []LogEntry{}, nil
	}

	content, err := fsService.ReadFile(ctx, logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	return s.parseLogContent(string(content)), nil
}

// generateRunScript generates the main run script for the service
func (s *S6Service) generateRunScript(config S6ServiceConfig) string {
	var script strings.Builder

	script.WriteString("#!/command/execlineb -P\n\n")

	// Add environment variables
	for key, value := range config.Env {
		script.WriteString(fmt.Sprintf("export %s %s\n", key, value))
	}

	if len(config.Env) > 0 {
		script.WriteString("\n")
	}

	// Add memory limit if specified
	if config.MemoryLimit > 0 {
		script.WriteString(fmt.Sprintf("s6-softlimit -m %d\n", config.MemoryLimit))
	}

	// Drop privileges for the actual service
	script.WriteString("s6-setuidgid nobody\n")

	// Keep stderr and stdout separate but both visible in logs
	script.WriteString("fdmove -c 2 1\n")

	if len(config.Command) > 0 {
		for i, arg := range config.Command {
			if i == len(config.Command)-1 {
				script.WriteString(fmt.Sprintf("%s\n", arg))
			} else {
				script.WriteString(fmt.Sprintf("%s ", arg))
			}
		}
	} else {
		script.WriteString("echo 'No command specified'\n")
	}

	return script.String()
}

// generateLogRunScript generates the log run script for the service
func (s *S6Service) generateLogRunScript(config S6ServiceConfig, serviceName string) string {
	// Use the actual service name (e.g., "benthos-test-0") instead of just "benthos"
	logDir := filepath.Join(S6LogBaseDir, serviceName)

	// Create logutil-service command line, see also https://skarnet.org/software/s6/s6-log.html
	// logutil-service is a wrapper around s6_log and reads from the S6_LOGGING_SCRIPT environment variable
	logutilServiceCmd := ""
	logutilEnv := ""
	if config.LogFilesize > 0 {
		// n20 is currently hardcoded to match the default defined in the Dockerfile
		// using the same export method as in runScriptTemplate for env variables
		// Important: This needs to be T (ISO 8601) as our time parser expects this format
		logutilEnv = fmt.Sprintf("export S6_LOGGING_SCRIPT \"n%d s%d T\"", 20, config.LogFilesize)
	}

	// Keep stdout quiet (stops binary blob leakage), but leave stderr visible
	// in Docker logs for s6-log diagnostics
	logutilServiceCmd = fmt.Sprintf(
		"logutil-service %s 1>/dev/null",
		logDir,
	)

	// Create log run script using the same format as UMH core
	logRunContent := fmt.Sprintf(`#!/command/execlineb -P
fdmove -c 2 1
foreground { mkdir -p %s }
foreground { chown -R nobody:nobody %s }

%s
%s`, logDir, logDir, logutilEnv, logutilServiceCmd)

	return logRunContent
}

// parseLogContent parses log content into LogEntry structs
func (s *S6Service) parseLogContent(content string) []LogEntry {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var entries []LogEntry

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Simple parsing - in real implementation you'd parse TAI64N timestamps
		entry := LogEntry{
			Timestamp: time.Now(),
			Content:   line,
		}
		entries = append(entries, entry)
	}

	return entries
}

// waitForSupervision waits for s6-supervise to start monitoring the service
func (s *S6Service) waitForSupervision(ctx context.Context, servicePath string, fsService FilesystemInterface) error {
	superviseDir := filepath.Join(servicePath, "supervise")
	maxWait := 5 * time.Second
	pollInterval := 50 * time.Millisecond

	timeout := time.After(maxWait)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for supervision of %s", servicePath)
		case <-ticker.C:
			// Check if supervise directory exists (created by s6-supervise)
			exists, err := fsService.PathExists(ctx, superviseDir)
			if err != nil {
				continue // Try again
			}
			if exists {
				// Double-check by testing if we can get status
				_, err := fsService.ExecuteCommand(ctx, "s6-svstat", servicePath)
				if err == nil {
					s.logger.Debugf("Service %s is now under supervision", servicePath)
					return nil
				}
			}
		}
	}
}

// FilesystemInterface defines the filesystem operations needed by S6Service
type FilesystemInterface interface {
	EnsureDirectory(ctx context.Context, path string) error
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error
	PathExists(ctx context.Context, path string) (bool, error)
	FileExists(ctx context.Context, path string) (bool, error)
	Remove(ctx context.Context, path string) error
	RemoveAll(ctx context.Context, path string) error
	ExecuteCommand(ctx context.Context, name string, args ...string) ([]byte, error)
	Symlink(ctx context.Context, target, linkPath string) error
}
