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
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/cactus/tai64"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// ServiceStatus represents the status of an S6 service
type ServiceStatus string

const (
	// ServiceUnknown indicates the service status cannot be determined
	ServiceUnknown ServiceStatus = "unknown"
	// ServiceUp indicates the service is running
	ServiceUp ServiceStatus = "up"
	// ServiceDown indicates the service is stopped
	ServiceDown ServiceStatus = "down"
	// ServiceRestarting indicates the service is restarting
	ServiceRestarting ServiceStatus = "restarting"
)

// ServiceInfo contains information about an S6 service
type ServiceInfo struct {
	Status        ServiceStatus // Current status of the service
	Uptime        int64         // Seconds the service has been up
	DownTime      int64         // Seconds the service has been down
	ReadyTime     int64         // Seconds the service has been ready
	Pid           int           // Process ID if service is up
	Pgid          int           // Process group ID if service is up
	ExitCode      int           // Exit code if service is down
	WantUp        bool          // Whether the service wants to be up (based on existence of down file)
	IsPaused      bool          // Whether the service is paused
	IsFinishing   bool          // Whether the service is shutting down
	IsWantingUp   bool          // Whether the service wants to be up (based on flags)
	IsReady       bool          // Whether the service is ready
	ExitHistory   []ExitEvent   // History of exit codes
	LastChangedAt time.Time     // Timestamp when the service status last changed
	LastReadyAt   time.Time     // Timestamp when the service was last ready
}

// ExitEvent represents a service exit event
type ExitEvent struct {
	Timestamp time.Time // timestamp of the exit event
	ExitCode  int       // exit code of the service
	Signal    int       // signal number of the exit event
}

// LogEntry represents a parsed log entry from the S6 logs
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
}

// Service defines the interface for interacting with S6 services
type Service interface {
	// Create creates the service with specific configuration
	Create(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) error
	// Remove removes the service directory structure
	Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Start starts the service
	Start(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Stop stops the service
	Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Restart restarts the service
	Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// Status gets the current status of the service
	Status(ctx context.Context, servicePath string, fsService filesystem.Service) (ServiceInfo, error)
	// ExitHistory gets the exit history of the service
	ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]ExitEvent, error)
	// ServiceExists checks if the service directory exists
	ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error)
	// GetConfig gets the actual service config from s6
	GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (s6serviceconfig.S6ServiceConfig, error)
	// CleanS6ServiceDirectory cleans the S6 service directory, removing non-standard services
	CleanS6ServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error
	// GetS6ConfigFile retrieves a config file for a service
	GetS6ConfigFile(ctx context.Context, servicePath string, configFileName string, fsService filesystem.Service) ([]byte, error)
	// ForceRemove removes a service from the S6 manager
	ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error
	// GetLogs gets the logs of the service
	GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]LogEntry, error)
	// EnsureSupervision checks if the supervise directory exists for a service and notifies
	// s6-svscan if it doesn't, to trigger supervision setup.
	// Returns true if supervise directory exists (ready for supervision), false otherwise.
	EnsureSupervision(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error)
}

// DefaultService is the default implementation of the S6 Service interface
type DefaultService struct {
	logger *zap.SugaredLogger
}

// NewDefaultService creates a new default S6 service
func NewDefaultService() Service {
	return &DefaultService{
		logger: logger.For(logger.ComponentS6Service),
	}
}

// Create creates the S6 service with specific configuration
func (s *DefaultService) Create(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".create", time.Since(start))
	}()

	s.logger.Debugf("Creating S6 service %s", servicePath)

	// Create service directory if it doesn't exist
	if err := fsService.EnsureDirectory(ctx, servicePath); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}

	// Create down file to prevent automatic startup
	downFilePath := filepath.Join(servicePath, "down")
	exists, err := fsService.FileExists(ctx, downFilePath)
	if err != nil {
		return fmt.Errorf("failed to check if down file exists: %w", err)
	}
	if !exists {
		err := fsService.WriteFile(ctx, downFilePath, []byte{}, 0644)
		if err != nil {
			return fmt.Errorf("failed to create down file: %w", err)
		}
	}

	// Create type file (required for s6-rc)
	typeFile := filepath.Join(servicePath, "type")
	exists, err = fsService.FileExists(ctx, typeFile)
	if err != nil {
		return fmt.Errorf("failed to check if type file exists: %w", err)
	}
	if !exists {
		err := fsService.WriteFile(ctx, typeFile, []byte("longrun"), 0644)
		if err != nil {
			return fmt.Errorf("failed to create type file: %w", err)
		}
	}

	// s6-supervise requires a run script to function properly
	if len(config.Command) > 0 {
		if err := s.createS6RunScript(ctx, servicePath, fsService, config.Command, config.Env, config.MemoryLimit); err != nil {
			return fmt.Errorf("failed to create S6 run script: %w", err)
		}
	} else {
		return fmt.Errorf("no command specified for service %s", servicePath)
	}

	// Create any config files specified
	if err := s.createS6ConfigFiles(ctx, servicePath, fsService, config.ConfigFiles); err != nil {
		return fmt.Errorf("failed to create S6 config files: %w", err)
	}

	// There is no need to register the service in user/contents.d,
	// as s6-overlay expects a static service configuration defined at container build time.
	// S6-overlay works in two phases:
	// 1. At container build time: Services are defined in /etc/s6-overlay/s6-rc.d/
	//    (including the special "user" bundle that lists services to start)
	// 2. At container startup: S6-overlay copies these definitions to /run/service/ and supervises them
	//
	// Attempting to modify /run/service/user/contents.d at runtime will cause errors because:
	// - S6-overlay treats "user" as a special directory and tries to supervise it
	// - This causes the "s6-supervise user: warning: unable to spawn ./run (waiting 60 seconds)" error
	// - S6-overlay won't recognize services added after container initialization
	//
	// For dynamic services, it's better to:
	// 1. Create services in their own directories under /run/service/
	// 2. Use s6-svscanctl to notify s6 about the new service

	// Create a dependency on base services to prevent race conditions
	dependenciesDPath := filepath.Join(servicePath, "dependencies.d")
	if err := fsService.EnsureDirectory(ctx, dependenciesDPath); err != nil {
		return fmt.Errorf("failed to create dependencies.d directory: %w", err)
	}

	baseDepFile := filepath.Join(dependenciesDPath, "base")
	err = fsService.WriteFile(ctx, baseDepFile, []byte{}, 0644)
	if err != nil {
		return fmt.Errorf("failed to create base dependency file: %w", err)
	}

	// Create log service directory and run script
	serviceName := filepath.Base(servicePath)
	logDir := filepath.Join(constants.S6LogBaseDir, serviceName)
	logServicePath := filepath.Join(servicePath, "log")

	// Create log service directory
	if err := fsService.EnsureDirectory(ctx, logServicePath); err != nil {
		return fmt.Errorf("failed to create log service directory: %w", err)
	}

	// Create log run script
	logRunPath := filepath.Join(logServicePath, "run")
	logRunContent := fmt.Sprintf(`#!/command/execlineb -P
fdmove -c 2 1
foreground { mkdir -p %s }
foreground { chown -R nobody:nobody %s }
logutil-service %s
`, logDir, logDir, logDir)

	if err := fsService.WriteFile(ctx, logRunPath, []byte(logRunContent), 0755); err != nil {
		return fmt.Errorf("failed to write log run script: %w", err)
	}

	// Notification is now handled by EnsureSupervision
	// We don't call s6-svscanctl here anymore to avoid duplicating the logic

	s.logger.Debugf("S6 service %s created with logging to %s", servicePath, logDir)

	return nil
}

// createRunScript creates a run script for the service
func (s *DefaultService) createS6RunScript(ctx context.Context, servicePath string, fsService filesystem.Service, command []string, env map[string]string, memoryLimit int64) error {
	runScript := filepath.Join(servicePath, "run")

	// Create template data
	data := struct {
		Command     []string
		Env         map[string]string
		MemoryLimit int64
	}{
		Command:     command,
		Env:         env,
		MemoryLimit: memoryLimit,
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

	// Write the templated content directly to the file
	if err := fsService.WriteFile(ctx, runScript, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write run script: %w", err)
	}

	// Make run script executable
	if err := fsService.Chmod(ctx, runScript, 0755); err != nil {
		return fmt.Errorf("failed to make run script executable: %w", err)
	}

	return nil
}

// createConfigFiles creates config files needed by the service
func (s *DefaultService) createS6ConfigFiles(ctx context.Context, servicePath string, fsService filesystem.Service, configFiles map[string]string) error {
	if len(configFiles) == 0 {
		return nil
	}

	configPath := filepath.Join(servicePath, "config")

	for path, content := range configFiles {
		// If path is relative, make it relative to service directory
		if !filepath.IsAbs(path) {
			path = filepath.Join(configPath, path)
		}

		// Create directory if it doesn't exist
		dir := filepath.Dir(path)
		if err := fsService.EnsureDirectory(ctx, dir); err != nil {
			return fmt.Errorf("failed to create directory for config file: %w", err)
		}

		// Create and write the file
		if err := fsService.WriteFile(ctx, path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write to config file %s: %w", path, err)
		}
	}

	return nil
}

// Remove removes the S6 service directory structure
func (s *DefaultService) Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".remove", time.Since(start))
	}()

	if s.logger != nil {
		s.logger.Debugf("Removing S6 service %s", servicePath)
	}

	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return fmt.Errorf("failed to check if service exists: %w", err)
	}
	if !exists {
		return ErrServiceNotExist
	}

	// Remove the service from contents.d first
	serviceName := filepath.Base(servicePath)
	//contentsFile := filepath.Join(filepath.Dir(servicePath), "user", "contents.d", serviceName)
	//fsService.Remove(ctx, contentsFile) // Ignore errors - file might not exist

	// Remove the service directory
	err = fsService.RemoveAll(ctx, servicePath)
	if err != nil {
		return fmt.Errorf("failed to remove S6 service %s: %w", servicePath, err)
	}

	// Clean up logs directory (best effort - don't block removal if this fails)
	logDir := filepath.Join(constants.S6LogBaseDir, serviceName)
	if logErr := fsService.RemoveAll(ctx, logDir); logErr != nil && s.logger != nil {
		sentry.ReportServiceErrorf(
			s.logger,
			serviceName,
			"s6",
			"cleanup_logs",
			"Failed to clean up log directory: %v",
			logErr,
		)
	}

	if s.logger != nil {
		s.logger.Debugf("Removed S6 service %s and its logs", servicePath)
	}
	return nil
}

// Start starts the S6 service
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
		return ErrServiceNotExist
	}

	_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-u", servicePath)
	if err != nil {
		s.logger.Warnf("Failed to start service: %v", err)
		return fmt.Errorf("failed to start service: %w", err)
	}

	s.logger.Debugf("Started S6 service %s", servicePath)
	return nil
}

// Stop stops the S6 service
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
		return ErrServiceNotExist
	}

	_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-d", servicePath)
	if err != nil {
		s.logger.Warnf("Failed to stop service: %v", err)
		return fmt.Errorf("failed to stop service: %w", err)
	}

	s.logger.Debugf("Stopped S6 service %s", servicePath)
	return nil
}

// Restart restarts the S6 service
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
		return ErrServiceNotExist
	}

	_, err = s.ExecuteS6Command(ctx, servicePath, fsService, "s6-svc", "-r", servicePath)
	if err != nil {
		s.logger.Warnf("Failed to restart service: %v", err)
		return fmt.Errorf("failed to restart service: %w", err)
	}

	s.logger.Debugf("Restarted S6 service %s", servicePath)
	return nil
}

func (s *DefaultService) Status(ctx context.Context, servicePath string, fsService filesystem.Service) (ServiceInfo, error) {
	debug.PrintStack()
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".status", time.Since(start))
	}()

	// First, check that the service exists.
	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to check if service exists: %w", err)
	}
	if !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// Default info.
	info := ServiceInfo{
		Status: ServiceUnknown,
	}

	// Build supervise directory path.
	superviseDir := filepath.Join(servicePath, "supervise")
	exists, err = fsService.FileExists(ctx, superviseDir)
	if err != nil {
		return info, fmt.Errorf("failed to check if supervise directory exists: %w", err)
	}
	if !exists {
		return info, ErrServiceNotExist // This is a temporary thing that can happen in bufered filesystems, when s6 did not yet have time to create the directory
	}

	// Read the status file.
	statusFile := filepath.Join(superviseDir, S6SuperviseStatusFile)
	exists, err = fsService.FileExists(ctx, statusFile)
	if err != nil {
		return info, fmt.Errorf("failed to check if status file exists: %w", err)
	}
	if !exists {
		return info, ErrServiceNotExist // This is a temporary thing that can happen in bufered filesystems, when s6 did not yet have time to create the file
	}
	statusData, err := fsService.ReadFile(ctx, statusFile)
	if err != nil {
		return info, fmt.Errorf("failed to read status file: %w", err)
	}

	// Check if the status file has the expected size
	if len(statusData) != S6StatusFileSize {
		return info, fmt.Errorf("invalid status file size: got %d bytes, expected %d: %w",
			len(statusData), S6StatusFileSize, ErrInvalidStatus)
	}

	// --- Parse the two TAI64N timestamps ---

	// Stamp: bytes [0:12] - When status last changed
	stampBytes := statusData[S6StatusChangedOffset : S6StatusChangedOffset+12]
	stampHex := "@" + hex.EncodeToString(stampBytes)
	stampTime, err := tai64.Parse(stampHex)
	if err != nil {
		return info, fmt.Errorf("failed to parse stamp (%s): %w", stampHex, err)
	}

	// Readystamp: bytes [12:24] - When service was last ready
	readyStampBytes := statusData[S6StatusReadyOffset : S6StatusReadyOffset+12]
	readyStampHex := "@" + hex.EncodeToString(readyStampBytes)
	readyTime, err := tai64.Parse(readyStampHex)
	if err != nil {
		return info, fmt.Errorf("failed to parse readystamp (%s): %w", readyStampHex, err)
	}

	// --- Parse integer fields using big-endian encoding ---

	// PID: bytes [24:32] (8 bytes)
	pid := binary.BigEndian.Uint64(statusData[S6StatusPidOffset : S6StatusPidOffset+8])

	// PGID: bytes [32:40] (8 bytes)
	pgid := binary.BigEndian.Uint64(statusData[S6StatusPgidOffset : S6StatusPgidOffset+8])

	// Wait status: bytes [40:42] (2 bytes)
	wstat := binary.BigEndian.Uint16(statusData[S6StatusWstatOffset : S6StatusWstatOffset+2])

	// --- Parse flags (1 byte at offset 42) ---
	flags := statusData[S6StatusFlagsOffset]
	flagPaused := (flags & S6FlagPaused) != 0
	flagFinishing := (flags & S6FlagFinishing) != 0
	flagWantUp := (flags & S6FlagWantUp) != 0
	flagReady := (flags & S6FlagReady) != 0

	// --- Determine service status ---
	now := time.Now().UTC()
	if pid != 0 && !flagFinishing {
		info.Status = ServiceUp
		info.Pid = int(pid)
		info.Pgid = int(pgid)
		// uptime is measured from the stamp timestamp
		info.Uptime = int64(now.Sub(stampTime).Seconds())
		info.ReadyTime = int64(now.Sub(readyTime).Seconds())
	} else {
		info.Status = ServiceDown
		// Interpret wstat as a wait status.
		// We convert to syscall.WaitStatus so that we can check if the process exited normally.
		var ws syscall.WaitStatus = syscall.WaitStatus(wstat)
		if ws.Exited() {
			info.ExitCode = ws.ExitStatus()
		} else if ws.Signaled() {
			// You may choose to record the signal number as a negative exit code.
			info.ExitCode = -int(ws.Signal())
		} else {
			info.ExitCode = int(wstat)
		}
		info.DownTime = int64(now.Sub(stampTime).Seconds())
		info.ReadyTime = int64(now.Sub(readyTime).Seconds())
	}

	// Store the timestamps
	info.LastChangedAt = stampTime
	info.LastReadyAt = readyTime

	// Store the flags
	info.IsPaused = flagPaused
	info.IsFinishing = flagFinishing
	info.IsWantingUp = flagWantUp
	info.IsReady = flagReady

	// Determine if service is "wanted up": if no "down" file exists.
	downFile := filepath.Join(servicePath, "down")
	downExists, _ := fsService.FileExists(ctx, downFile)
	info.WantUp = !downExists

	// Optionally update exit history.
	history, histErr := s.ExitHistory(ctx, superviseDir, fsService)
	if histErr == nil {
		info.ExitHistory = history
	} else {
		return info, fmt.Errorf("failed to get exit history: %w", histErr)
	}

	//s.logger.Debugf("Status for S6 service %s: %+v", servicePath, info)

	return info, nil
}

// ExitHistory retrieves the service exit history by reading the dtally file ("death_tally")
// directly from the supervise directory instead of invoking s6-svdt.
// The dtally file is a binary file containing a sequence of dtally records.
// Each record has the following structure:
//   - Bytes [0:12]: TAI64N timestamp (12 bytes)
//   - Byte 12:      Exit code (1 byte)
//   - Byte 13:      Signal number (1 byte)
//
// If the file size is not a multiple of the record size, it is considered corrupted.
// In that case, you may choose to truncate the file (as the C code does) or return an error.
func (s *DefaultService) ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]ExitEvent, error) {
	// Build the full path to the dtally file.
	dtallyFile := filepath.Join(superviseDir, S6DtallyFileName)

	// Check if the dtally file exists.
	exists, err := fsService.FileExists(ctx, dtallyFile)
	if err != nil {
		return nil, err
	}
	if !exists {
		// If the dtally file does not exist, no exit history is available.
		return nil, nil
	}

	// Read the entire dtally file.
	data, err := fsService.ReadFile(ctx, dtallyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read dtally file: %w", err)
	}

	// Verify that the file size is a multiple of the dtally record size.
	if len(data)%S6_DTALLY_PACK != 0 {
		// The file is considered corrupted or partially written.
		// In the C code, a truncation is attempted in this case.
		// Here, we return an error.
		return nil, fmt.Errorf("dtally file size (%d bytes) is not a multiple of record size (%d)", len(data), S6_DTALLY_PACK)
	}

	// Calculate the number of records.
	numRecords := len(data) / S6_DTALLY_PACK
	var history []ExitEvent

	// Process each dtally record.
	for i := 0; i < numRecords; i++ {
		offset := i * S6_DTALLY_PACK
		record := data[offset : offset+S6_DTALLY_PACK]

		// Unpack the TAI64N timestamp (first 12 bytes) and convert it to a time.Time.
		// The timestamp is encoded as 12 bytes, which we first convert to a hex string,
		// then prepend "@" (as required by the tai64.Parse function) and parse.
		tai64Str := "@" + hex.EncodeToString(record[:12])
		parsedTime, err := tai64.Parse(tai64Str)
		if err != nil {
			// If parsing fails, skip this record.
			continue
		}

		// Unpack the exit code (13th byte).
		exitCode := int(record[12])
		signalNumber := int(record[13])

		history = append(history, ExitEvent{
			Timestamp: parsedTime,
			ExitCode:  exitCode,
			Signal:    signalNumber,
		})
	}

	//s.logger.Debugf("Exit history for S6 service %s: %+v", superviseDir, history)
	return history, nil
}

// ServiceExists checks if the service directory exists
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

// GetConfig gets the actual service config from s6
func (s *DefaultService) GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (s6serviceconfig.S6ServiceConfig, error) {
	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to check if service exists: %w", err)
	}
	if !exists {
		return s6serviceconfig.S6ServiceConfig{}, ErrServiceNotExist
	}

	observedS6ServiceConfig := s6serviceconfig.S6ServiceConfig{
		ConfigFiles: make(map[string]string),
		Env:         make(map[string]string),
		MemoryLimit: 0,
	}

	// Fetch run script
	runScript := filepath.Join(servicePath, "run")
	exists, err = fsService.FileExists(ctx, runScript)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to check if run script exists: %w", err)
	}
	if !exists {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("run script not found")
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
			observedS6ServiceConfig.Env[key] = value
		}
	}

	// Extract command using regex
	cmdMatch := runScriptParser.FindStringSubmatch(scriptContent)

	if len(cmdMatch) >= 2 && cmdMatch[1] != "" {
		// If we captured the command on the same line as fdmove
		cmdLine := strings.TrimSpace(cmdMatch[1])
		observedS6ServiceConfig.Command, err = parseCommandLine(cmdLine)
		if err != nil {
			return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse command: %w", err)
		}
	} else {
		// If the command is on the line after fdmove, or regex didn't match properly
		lines := strings.Split(scriptContent, "\n")
		var commandLine string

		// Find the fdmove line
		for i, line := range lines {
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

				// Otherwise, look for first non-empty line after fdmove
				for j := i + 1; j < len(lines); j++ {
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
			observedS6ServiceConfig.Command, err = parseCommandLine(commandLine)
			if err != nil {
				return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse command: %w", err)
			}
		} else {
			// Absolute fallback - try to look for the command we know should be there
			sentry.ReportIssuef(sentry.IssueTypeWarning, s.logger, "[s6.GetConfig] Could not find command in run script for %s, searching for known paths", servicePath)
			cmdRegex := regexp.MustCompile(`(/[^\s]+)`)
			cmdMatches := cmdRegex.FindAllString(scriptContent, -1)

			if len(cmdMatches) > 0 {
				// Use the first matching path-like string we find as the command
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

				observedS6ServiceConfig.Command = append([]string{cmd}, args...)
			} else {
				return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse run script: no valid command found")
			}
		}
	}

	// Extract memory limit using regex
	memoryLimitMatches := memoryLimitParser.FindStringSubmatch(scriptContent)
	if len(memoryLimitMatches) >= 2 && memoryLimitMatches[1] != "" {
		observedS6ServiceConfig.MemoryLimit, err = strconv.ParseInt(memoryLimitMatches[1], 10, 64)
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
		return observedS6ServiceConfig, nil
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

		observedS6ServiceConfig.ConfigFiles[entry.Name()] = string(content)
	}

	return observedS6ServiceConfig, nil
}

// parseCommandLine splits a command line into command and arguments, respecting quotes
func parseCommandLine(cmdLine string) ([]string, error) {
	var cmdParts []string
	var currentPart strings.Builder
	inQuote := false
	quoteChar := byte(0)
	escaped := false

	for i := 0; i < len(cmdLine); i++ {
		// Handle escape character
		if cmdLine[i] == '\\' && !escaped {
			escaped = true
			continue
		}

		if (cmdLine[i] == '"' || cmdLine[i] == '\'') && !escaped {
			if inQuote && cmdLine[i] == quoteChar {
				inQuote = false
				quoteChar = 0
			} else if !inQuote {
				inQuote = true
				quoteChar = cmdLine[i]
			} else {
				// This is a different quote character inside a quote
				currentPart.WriteByte(cmdLine[i])
			}
		} else if escaped {
			// Handle the escaped character
			currentPart.WriteByte(cmdLine[i])
			escaped = false
		} else {
			if cmdLine[i] == ' ' && !inQuote {
				if currentPart.Len() > 0 {
					cmdParts = append(cmdParts, currentPart.String())
					currentPart.Reset()
				}
			} else {
				currentPart.WriteByte(cmdLine[i])
			}
		}
	}

	// Check for unclosed quotes
	if inQuote {
		return nil, fmt.Errorf("unclosed quote")
	}

	if currentPart.Len() > 0 {
		cmdParts = append(cmdParts, currentPart.String())
	}

	return cmdParts, nil
}

// These constants define file locations and offsets for direct S6 supervision file access

const (
	// Source: https://github.com/skarnet/s6/blob/main/src/include/s6/supervise.h
	// S6SuperviseStatusFile is the status file in the supervise directory.
	S6SuperviseStatusFile = "status"

	// S6 status file format (43 bytes total):
	// Byte range | Description
	// -----------|------------
	// 0-11       | TAI64N timestamp when status last changed (12 bytes)
	// 12-23      | TAI64N timestamp when service was last ready (12 bytes)
	// 24-31      | Process ID (big-endian uint64, 8 bytes)
	// 32-39      | Process group ID (big-endian uint64, 8 bytes)
	// 40-41      | Wait status (big-endian uint16, 2 bytes)
	// 42         | Flags byte (1 byte: bit 0=paused, bit 1=finishing, bit 2=want up, bit 3=ready)

	// Source: https://github.com/skarnet/s6/blob/main/src/libs6/s6_svstatus_unpack.c

	// Offsets in the status file:
	S6StatusChangedOffset = 0  // TAI64N timestamp when status last changed (12 bytes)
	S6StatusReadyOffset   = 12 // TAI64N timestamp when service was last ready (12 bytes)
	S6StatusPidOffset     = 24 // Process ID (uint64, 8 bytes)
	S6StatusPgidOffset    = 32 // Process group ID (uint64, 8 bytes)
	S6StatusWstatOffset   = 40 // Wait status (uint16, 2 bytes)
	S6StatusFlagsOffset   = 42 // Flags byte (1 byte)

	// Flags in the flags byte:
	S6FlagPaused    = 0x01 // Service is paused
	S6FlagFinishing = 0x02 // Service is shutting down
	S6FlagWantUp    = 0x04 // Service wants to be up
	S6FlagReady     = 0x08 // Service is ready

	// Expected size of the status file:
	S6StatusFileSize = 43 // bytes
)

// Constants for dtally file processing.
// S6DtallyFileName is the filename for the death tally file.
// S6_DTALLY_PACK is the size of a single dtally record (TAI64N timestamp + exitcode + signal).
const (
	S6DtallyFileName = "death_tally"
	// As TAIN_PACK is 12 bytes, then each dtally record is 12 + 1 + 1 = 14 bytes.
	S6_DTALLY_PACK = 14
)

// CleanS6ServiceDirectory cleans the S6 service directory except for the known services
func (s *DefaultService) CleanS6ServiceDirectory(ctx context.Context, path string, fsService filesystem.Service) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
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
			if err := fsService.RemoveAll(ctx, dirPath); err != nil {
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

// IsKnownService checks if a service is known
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
// servicePath should be the full path including S6BaseDir
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
		return nil, ErrServiceNotExist
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

// ForceRemove removes a service from the S6 manager
func (s *DefaultService) ForceRemove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return fsService.RemoveAll(ctx, servicePath)
}

// GetStructuredLogs gets the logs of the service as structured LogEntry objects
func (s *DefaultService) GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]LogEntry, error) {
	serviceName := filepath.Base(servicePath)

	// Check if the service exists first
	exists, err := s.ServiceExists(ctx, servicePath, fsService)
	if err != nil {
		return nil, fmt.Errorf("failed to check if service exists: %w", err)
	}
	if !exists {
		s.logger.Debugf("Service with path %s does not exist, returning empty logs", servicePath)
		return nil, ErrServiceNotExist
	}

	// Get the log file from /data/logs/<service-name>/current
	logFile := filepath.Join(constants.S6LogBaseDir, serviceName, "current")
	exists, err = fsService.FileExists(ctx, logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check if log file exists: %w", err)
	}
	if !exists {
		return nil, ErrLogFileNotFound
	}

	// Read the log file
	content, err := fsService.ReadFile(ctx, logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	// Split logs by newline
	logs := strings.Split(strings.TrimSpace(string(content)), "\n")

	// Parse each log line into structured entries
	var entries []LogEntry
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

// parseLogLine parses a log line from S6 format and returns a LogEntry
func parseLogLine(line string) LogEntry {
	// S6 log format with T flag: YYYY-MM-DD HH:MM:SS.NNNNNNNNN  content
	parts := strings.SplitN(line, "  ", 2)
	if len(parts) != 2 {
		return LogEntry{Content: line}
	}

	timestamp, err := time.Parse("2006-01-02 15:04:05.999999999", parts[0])
	if err != nil {
		return LogEntry{Content: line}
	}

	return LogEntry{
		Timestamp: timestamp,
		Content:   parts[1],
	}
}

// EnsureSupervision checks if the supervise directory exists for a service and notifies
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
		return false, ErrServiceNotExist
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
		s.logger.Debugf("Notified s6-svscan, waiting for supervise directory to be created on next reconcile")
		return false, nil
	}

	s.logger.Debugf("Supervise directory exists for %s", servicePath)
	return true, nil
}

// ExecuteS6Command executes an S6 command and handles its specific exit codes.
// This function simplifies error handling by translating exit codes into appropriate errors:
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
				return "", ErrS6TemporaryError
			case 100:
				s.logger.Debugf("S6 service %s is already being monitored (exit code 100), continuing", servicePath)
				return "", nil
			case 127:
				s.logger.Debugf("S6 command could not find program (exit code 127) for service %s", servicePath)
				return "", ErrS6ProgramNotFound
			case 126:
				s.logger.Debugf("S6 command could not execute program (exit code 126) for service %s", servicePath)
				return "", ErrS6ProgramNotExecutable
			default:
				return "", fmt.Errorf("unknown S6 error (exit code %d) for service %s: %w, output: %s",
					exitErr.ExitCode(), servicePath, err, string(output))
			}
		}
		return "", fmt.Errorf("failed to execute s6 command (name: %s, args: %v): %w, output: %s", name, args, err, string(output))
	}

	return string(output), nil
}
