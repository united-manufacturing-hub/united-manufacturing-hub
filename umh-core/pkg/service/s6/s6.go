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
	"cmp"
	"context"
	"encoding/binary"
	"encoding/hex"
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

	"golang.org/x/sync/errgroup"
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
	// Timestamp in UTC time
	Timestamp time.Time `json:"timestamp"`
	// Content of the log entry
	Content string `json:"content"`
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

// logState is the per-log-file cursor used by GetLogs.
//
// Fields:
//
//	mu     – guards every field in the struct (single-writer, multi-reader)
//	inode  – inode of the file when we last touched it; changes ⇒ rotation
//	offset – next byte to read on disk (monotonically increases until
//	         rotation or truncation)
//

// logs   – backing array that holds *at most* S6MaxLines entries.
//
//	Allocated once; after that, entries are overwritten in place.
//
// head   – index of the slot where the **next** entry will be written.
//
//	When head wraps from max-1 to 0, `full` is set to true.
//
// full   – true once the buffer has wrapped at least once; used to decide
//
//	how to linearise the ring when we copy it out.
type logState struct {
	// mu guards every field in the struct (single-writer, multi-reader)
	mu sync.Mutex
	// inode is the inode of the file when we last touched it; changes ⇒ rotation
	inode uint64
	// offset is the next byte to read on disk (monotonically increases until
	// rotation or truncation)
	offset int64

	// logs is the backing array that holds *at most* S6MaxLines entries.
	// Allocated once; after that, entries are overwritten in place.
	logs []LogEntry
	// head is the index of the slot where the **next** entry will be written.
	// When head wraps from max-1 to 0, `full` is set to true.
	head int
	// full is true once the buffer has wrapped at least once; used to decide
	// how to linearise the ring when we copy it out.
	full bool
}

// DefaultService is the default implementation of the S6 Service interface
type DefaultService struct {
	logger         *zap.SugaredLogger
	logCursors     sync.Map // map[string]*logState (key = abs log path)
	operationMutex sync.Mutex
}

// NewDefaultService creates a new default S6 service
func NewDefaultService() Service {
	return &DefaultService{
		logger: logger.For(logger.ComponentS6Service),
	}
}

// Create creates the S6 service with specific configuration
func (s *DefaultService) Create(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, fsService filesystem.Service) error {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

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

	// Create logutil-service command line, see also https://skarnet.org/software/s6/s6-log.html
	// logutil-service is a wrapper around s6_log and reads from the S6_LOGGING_SCRIPT environment variable
	// We overwrite the default S6_LOGGING_SCRIPT with our own if config.LogFilesize is set
	logRunContent, err := getLogRunScript(config, logDir)
	if err != nil {
		return fmt.Errorf("failed to get log run script: %w", err)
	}

	// Create log run script
	logRunPath := filepath.Join(logServicePath, "run")
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

// Remove deletes every artefact that `Create` produced:
//
//   - <servicePath>             (the long-run service directory)
//   - <servicePath>/log         (nested logger service)
//   - <logBase>/<name>          (rotated log directory)
//
// The method is called once per reconcile *tick* while the S6-FSM is in the
// *removing* state, therefore it must:
//
//  1. **Return quickly** – never wait or poll.
//  2. **Be idempotent**  – safe to call when directories are half-gone.
//  3. **Return nil only when nothing is left.**
//
// Any remaining file or I/O error leads to a non-nil return so the FSM keeps
// trying (or escalates after the back-off threshold).
func (s *DefaultService) Remove(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service,
			servicePath+".remove", time.Since(start))
	}()

	if ctx.Err() != nil {
		return ctx.Err() // context already cancelled / deadline exceeded
	}

	srvName := filepath.Base(servicePath)
	logDir := filepath.Join(constants.S6LogBaseDir, srvName)

	// ──────────────────────── fast-path ────────────────────────────
	// If both paths are already gone we are done – lets the FSM fire
	// "remove_done" immediately and exit the reconcile loop.
	if gone, _ := fsService.PathExists(ctx, servicePath); !gone {
		if gone2, _ := fsService.PathExists(ctx, logDir); !gone2 {
			return nil
		}
	}

	//----------------------------------------------------------------
	// Two independent best-effort deletions.  We *always* call them,
	// even if the directory is missing, because RemoveAll on a non-
	// existent path is cheap and returns nil – keeps the code simple.
	// We start it in a separate goroutine and wait for constants.S6RemoveTimeout
	// so that we keep the main reconcile loop fast. If this requires a couple
	// of retries, it is fine as long as we do not block the main reconcile loop.
	//----------------------------------------------------------------
	ctxRemoveTimeout, cancel := context.WithTimeout(ctx, constants.S6RemoveTimeout)
	defer cancel()

	g, gctx := errgroup.WithContext(ctxRemoveTimeout)

	var srvErr, logErr error

	g.Go(func() error {
		srvErr = fsService.RemoveAll(gctx, servicePath)
		return srvErr
	})

	g.Go(func() error {
		logErr = fsService.RemoveAll(gctx, logDir)
		return logErr
	})

	// Create a buffered channel to receive the result from g.Wait().
	// The channel is buffered so that the goroutine sending on it doesn't block.
	errc := make(chan error, 1)

	// Run g.Wait() in a separate goroutine.
	// This allows us to use a select statement to return early if the context is canceled.
	go func() {
		// g.Wait() blocks until all goroutines launched with g.Go() have returned.
		// It returns the first non-nil error, if any.
		errc <- g.Wait()
	}()

	// Use a select statement to wait for either the g.Wait() result or the context's cancellation.
	select {
	case err := <-errc:
		// g.Wait() has finished, so check if any goroutine returned an error.
		if err != nil {
			// If there was an error in any sub-call, return that error.
			return fmt.Errorf("s6.Remove incomplete for %q: %s",
				servicePath, err.Error())
		}
		// If err is nil, all goroutines completed successfully.
	case <-ctxRemoveTimeout.Done():
		// The context was canceled or its deadline was exceeded before all goroutines finished.
		// Although some goroutines might still be running in the background,
		// they use a context (gctx) that should cause them to terminate promptly.
		return ctxRemoveTimeout.Err()
	}

	//----------------------------------------------------------------
	// Double-check: report success only when both paths are gone *and*
	// no I/O error occurred.
	//----------------------------------------------------------------
	srvLeft, srvLeftErr := fsService.PathExists(ctx, servicePath)
	logLeft, logLeftErr := fsService.PathExists(ctx, logDir)

	if srvErr != nil || logErr != nil || srvLeft || logLeft {
		// build a helpful composite error message
		var parts []string
		if srvErr != nil {
			parts = append(parts, fmt.Sprintf("serviceDir: %v", srvErr))
		}
		if logErr != nil {
			parts = append(parts, fmt.Sprintf("logDir: %v", logErr))
		}
		if srvLeft {
			parts = append(parts, "serviceDir still exists")
		}
		if logLeft {
			parts = append(parts, "logDir still exists")
		}

		if srvLeftErr != nil {
			parts = append(parts, fmt.Sprintf("serviceDir: %v", srvLeftErr))
		}
		if logLeftErr != nil {
			parts = append(parts, fmt.Sprintf("logDir: %v", logLeftErr))
		}

		return fmt.Errorf("s6.Remove incomplete for %q: %s",
			servicePath, strings.Join(parts, "; "))
	}

	s.logger.Debugf("Removed S6 service %s and its logs", servicePath)
	return nil
}

// Start starts the S6 service
func (s *DefaultService) Start(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".start", time.Since(start))
	}()

	s.logger.Debugf("Starting S6 service %s", servicePath)
	exists, err := s.serviceExists(ctx, servicePath, fsService)
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

// stop is an internal helper that stops the S6 service without acquiring the mutex
func (s *DefaultService) stop(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	s.logger.Debugf("Stopping S6 service %s", servicePath)
	exists, err := s.serviceExists(ctx, servicePath, fsService)
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

// Stop stops the S6 service
func (s *DefaultService) Stop(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".stop", time.Since(start))
	}()

	return s.stop(ctx, servicePath, fsService)
}

// Restart restarts the S6 service
func (s *DefaultService) Restart(ctx context.Context, servicePath string, fsService filesystem.Service) error {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".restart", time.Since(start))
	}()

	s.logger.Debugf("Restarting S6 service %s", servicePath)
	exists, err := s.serviceExists(ctx, servicePath, fsService)
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
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".status", time.Since(start))
	}()

	// First, check that the service exists.
	exists, err := s.serviceExists(ctx, servicePath, fsService)
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
		ws := syscall.WaitStatus(wstat)
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
	history, histErr := s.exitHistory(ctx, superviseDir, fsService)
	if histErr == nil {
		info.ExitHistory = history
	} else {
		return info, fmt.Errorf("failed to get exit history: %w", histErr)
	}

	// s.logger.Debugf("Status for S6 service %s: %+v", servicePath, info)

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
// exitHistory is an internal helper that retrieves exit history without acquiring the mutex
func (s *DefaultService) exitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]ExitEvent, error) {
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
	if data == nil { // Empty history file
		return nil, nil
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

	// s.logger.Debugf("Exit history for S6 service %s: %+v", superviseDir, history)
	return history, nil
}

// ExitHistory retrieves the service exit history (exported version with mutex)
func (s *DefaultService) ExitHistory(ctx context.Context, superviseDir string, fsService filesystem.Service) ([]ExitEvent, error) {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	return s.exitHistory(ctx, superviseDir, fsService)
}

// serviceExists is an internal helper that checks if the service directory exists without acquiring the mutex
func (s *DefaultService) serviceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
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

// ServiceExists checks if the service directory exists
func (s *DefaultService) ServiceExists(ctx context.Context, servicePath string, fsService filesystem.Service) (bool, error) {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".serviceExists", time.Since(start))
	}()

	return s.serviceExists(ctx, servicePath, fsService)
}

// GetConfig gets the actual service config from s6
func (s *DefaultService) GetConfig(ctx context.Context, servicePath string, fsService filesystem.Service) (s6serviceconfig.S6ServiceConfig, error) {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	exists, err := s.serviceExists(ctx, servicePath, fsService)
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
		LogFilesize: 0,
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

	// Extract LogFilesize using regex
	// Fetch run script
	logServicePath := filepath.Join(servicePath, "log")
	logScript := filepath.Join(logServicePath, "run")
	exists, err = fsService.FileExists(ctx, logScript)
	if err != nil {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to check if log run§ script exists: %w", err)
	}
	if !exists {
		return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("log run script not found")
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
		observedS6ServiceConfig.LogFilesize, err = strconv.ParseInt(logSizeMatches[1], 10, 64)
		if err != nil {
			return s6serviceconfig.S6ServiceConfig{}, fmt.Errorf("failed to parse log filesize: %w", err)
		}
	} else {
		// If no match found, default to 0
		observedS6ServiceConfig.LogFilesize = 0
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
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

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
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check if the service exists with the full path
	exists, err := s.serviceExists(ctx, servicePath, fsService)
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

// ForceRemove tears down an S6 long-run service **unconditionally**.
//
// It is the "nuclear option" the manager calls after several failed
// graceful removals.  The function must therefore:
//
//   - **Be idempotent & non-blocking** – safe to invoke every 100 ms;
//     never waits or polls.
//   - **Erase every artefact** the service could have left behind.
//   - **Return nil only when nothing remains.**
//
// Removal sequence
// ----------------
//  1. Best-effort **stop** the daemon *and* its logger with `s6-svc -d`.
//     This is fast and harmless even if they are already down.
//  2. **SIGTERM any lingering `s6-supervise` processes** (main + logger)
//     by reading `<servicedir>/supervise/pid`.  Without this step a still-
//     running supervisor could re-create the `supervise/` directory after
//     we delete the tree.
//  3. **Delete the two artefact trees**
//     <servicePath>                       (includes …/log subdir)
//     <logsBase>/<serviceName>            (rotated log files)
//  4. **Double-check** that both paths are truly gone and that no I/O
//     errors occurred; otherwise return an error so the caller retries.
func (s *DefaultService) ForceRemove(
	ctx context.Context,
	servicePath string,
	fsService filesystem.Service,
) error {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

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

	//--------------------------------------------------------------------
	// 1. Best-effort graceful stop (idempotent, non-blocking)
	//--------------------------------------------------------------------
	mainErr := s.stop(ctx, servicePath, fsService)                         // main
	loggerErr := s.stop(ctx, filepath.Join(servicePath, "log"), fsService) // logger

	//--------------------------------------------------------------------
	// 2. SIGTERM lingering s6-supervise processes to avoid resurrection
	//--------------------------------------------------------------------
	killSupervise := func(dir string) error {
		pidFile := filepath.Join(dir, "supervise", "pid")
		data, err := fsService.ReadFile(ctx, pidFile)
		if err != nil || len(data) == 0 {
			return nil // no supervisor ⇒ nothing to kill
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			err = syscall.Kill(pid, syscall.SIGTERM)
			if err != nil {
				return fmt.Errorf("failed to kill supervisor for service %s: %w", dir, err)
			}
		}
		return nil
	}
	killSuperviseMainErr := killSupervise(servicePath)
	killSuperviseLoggerErr := killSupervise(filepath.Join(servicePath, "log"))

	//--------------------------------------------------------------------
	// 3. Delete artefact trees
	//--------------------------------------------------------------------
	srvErr := fsService.RemoveAll(ctx, servicePath)

	svcName := filepath.Base(servicePath)
	logDir := filepath.Join(constants.S6LogBaseDir, svcName)
	logErr := fsService.RemoveAll(ctx, logDir)

	//--------------------------------------------------------------------
	// 4. Verification – declare success *only* if nothing is left
	//--------------------------------------------------------------------
	svcLeft, pathExistsMainErr := fsService.PathExists(ctx, servicePath)
	logLeft, pathExistsLoggerErr := fsService.PathExists(ctx, logDir)

	if srvErr != nil || logErr != nil || svcLeft || logLeft {
		var parts []string
		if srvErr != nil {
			parts = append(parts, fmt.Sprintf("serviceDir: %v", srvErr))
		}
		if logErr != nil {
			parts = append(parts, fmt.Sprintf("logDir: %v", logErr))
		}
		if svcLeft || pathExistsMainErr != nil {
			parts = append(parts, "serviceDir still exists")
		}
		if logLeft || pathExistsLoggerErr != nil {
			parts = append(parts, "logDir still exists")
		}

		if mainErr != nil {
			parts = append(parts, fmt.Sprintf("stopping service: %v", mainErr))
		}

		if loggerErr != nil {
			parts = append(parts, fmt.Sprintf("stopping logger for service: %v", loggerErr))
		}

		if killSuperviseMainErr != nil {
			parts = append(parts, fmt.Sprintf("killing supervisor for service: %v", killSuperviseMainErr))
		}

		if killSuperviseLoggerErr != nil {
			parts = append(parts, fmt.Sprintf("killing supervisor for logger: %v", killSuperviseLoggerErr))
		}

		if pathExistsMainErr != nil {
			parts = append(parts, fmt.Sprintf("checking if service exists: %v", pathExistsMainErr))
		}

		if pathExistsLoggerErr != nil {
			parts = append(parts, fmt.Sprintf("checking if logger exists: %v", pathExistsLoggerErr))
		}

		aggregatedErr := fmt.Errorf("s6.ForceRemove incomplete for %q: %s",
			servicePath, strings.Join(parts, "; "))

		sentry.ReportIssuef(sentry.IssueTypeWarning, s.logger, "%s", aggregatedErr.Error())

		return aggregatedErr
	}

	s.logger.Infof("Force-removed S6 service %s and its logs", servicePath)
	return nil
}

// appendToRingBuffer appends entries to the ring buffer, extracted from existing GetLogs logic.
func (s *DefaultService) appendToRingBuffer(entries []LogEntry, st *logState) {
	const max = constants.S6MaxLines

	// Preallocate backing storage to full size once - it's recycled at runtime and never dropped
	if st.logs == nil {
		st.logs = make([]LogEntry, max) // len == max, cap == max
		st.head = 0
		st.full = false
	}

	for _, e := range entries {
		st.logs[st.head] = e
		st.head = (st.head + 1) % max
		if !st.full && st.head == 0 {
			st.full = true // wrapped around for the first time
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
// The function keeps one in-memory cursor (`logState`) *per log file*:
//
//	logCursors : map[absLogPath]*logState
//
// That cursor stores where we stopped reading the file last time
// (`offset`) and a fixed-size **ring buffer** with the most recent
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
//  2. **Detect file rotation/truncation (or first-time initialization)**
//     If the inode changed or the file shrank, we:
//     - Reset the ring buffer to start fresh (preserving backing array)
//     - Find the most recent rotated file by TAI64N timestamp
//     - Read any remaining data from that rotated file
//     - Reset inode and offset for the new current file
//
//     **FIRST CALL scenario**: When called on a newly created current file,
//     st.inode == 0, so we simply initialize it to the current file's inode.
//
//  3. **Read content from current file**
//     `ReadFileRange(ctx, path, st.offset)` returns the bytes that
//     appeared since the previous call.  The cursor's `offset` is
//     advanced whether or not anything changed.
//
//     **FIRST CALL**: Reads entire file from beginning (st.offset == 0).
//
//  4. **Process rotated and current content separately**
//     Both rotated file content (if any) and current file content are
//     parsed and appended to the ring buffer in chronological order:
//     rotated content first (older), then current content (newer).
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
//   - One `memmove` operation per call for final copy-out.
//   - Thread-safety via the `logState.mu` mutex.
//
// Errors are returned early and unwrapped where they occur so callers
// see the root cause (e.g. "file disappeared", "permission denied").
func (s *DefaultService) GetLogs(ctx context.Context, servicePath string, fsService filesystem.Service) ([]LogEntry, error) {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()

	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentS6Service, servicePath+".getLogs", time.Since(start))
	}()

	serviceName := filepath.Base(servicePath)

	// Check if the service exists first
	exists, err := s.serviceExists(ctx, servicePath, fsService)
	if err != nil {
		return nil, fmt.Errorf("failed to check if service exists: %w", err)
	}
	if !exists {
		s.logger.Debugf("Service with path %s does not exist, returning empty logs", servicePath)
		return nil, ErrServiceNotExist
	}

	// Get the log file from /data/logs/<service-name>/current
	logFile := filepath.Join(constants.S6LogBaseDir, serviceName, "current")
	exists, err = fsService.PathExists(ctx, logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to check if log file exists: %w", err)
	}
	if !exists {
		s.logger.Debugf("Log file %s does not exist, returning ErrLogFileNotFound", logFile)
		return nil, fmt.Errorf("path: %s err :%w", logFile, ErrLogFileNotFound)
	}

	// ── 1. grab / create state ──────────────────────────────────────
	stAny, _ := s.logCursors.LoadOrStore(logFile, &logState{})
	st := stAny.(*logState)

	st.mu.Lock()
	defer st.mu.Unlock()

	// ── 2. check inode & size (rotation / truncation?) ──────────────
	fi, err := fsService.Stat(ctx, logFile)
	if err != nil {
		return nil, err
	}
	if fi == nil {
		return nil, fmt.Errorf("stat returned nil for log file: %s", logFile)
	}
	sys := fi.Sys().(*syscall.Stat_t) // on Linux / Alpine
	size, ino := fi.Size(), sys.Ino

	// Check for rotation or truncation
	var rotatedContent []byte
	if st.inode != 0 && (st.inode != ino || st.offset > size) {
		s.logger.Debugf("Detected rotation for log file %s (inode: %d->%d, offset: %d, size: %d)",
			logFile, st.inode, ino, st.offset, size)

		// Reset ring buffer on rotation
		if st.logs == nil {
			st.logs = make([]LogEntry, constants.S6MaxLines)
		}
		/*
			Why the previous implementation used st.logs[:0]:
				The old approach allocated with make([]LogEntry, 0, max) and used append(), so the slice would grow from length 0 to some length. On rotation, it needed to reset the length back to 0 with st.logs[:0] while keeping the capacity.

			Why we don't need it now:
				With our optimized approach:
				We preallocate to full size: make([]LogEntry, max) - length is always max
				We use direct indexing: st.logs[st.head] = e - no append operations
				We track valid entries with st.head and st.full: The slice length never changes
		*/

		// Reset ring buffer state but keep the backing array
		st.head = 0
		st.full = false

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
			rotatedContent, _, err = fsService.ReadFileRange(ctx, rotatedFile, st.offset)
			if err != nil {
				s.logger.Warnf("Failed to read rotated file %s from offset %d: %v", rotatedFile, st.offset, err)
			} else if len(rotatedContent) > 0 {
				s.logger.Debugf("Read %d bytes from rotated file %s", len(rotatedContent), rotatedFile)
			}
		}

		// Reset for new current file
		st.inode, st.offset = ino, 0
	} else if st.inode == 0 {
		// First call - initialize inode
		st.inode = ino
	}

	// ── 3. read new bytes from current file ─────────────────────────
	currentContent, newSize, err := fsService.ReadFileRange(ctx, logFile, st.offset)
	if err != nil {
		return nil, err
	}
	st.offset = newSize // advance cursor even if currentContent == nil

	// ── 4. combine rotated and current content into the ring buffer ──────────────────────
	// The order of the content is important: rotated content is older, current content is newer.
	// We want to keep the newest lines at the end of the ring buffer.

	if len(rotatedContent) > 0 {
		entries, err := ParseLogsFromBytes(rotatedContent)
		if err != nil {
			return nil, err
		}
		s.appendToRingBuffer(entries, st)
	}

	if len(currentContent) > 0 {
		entries, err := ParseLogsFromBytes(currentContent)
		if err != nil {
			return nil, err
		}
		s.appendToRingBuffer(entries, st)
	}

	// ── 5. return *copy* so caller can't mutate our cache ───────────
	var length int
	if st.full {
		length = constants.S6MaxLines
	} else {
		length = st.head // number of valid entries written so far
	}

	out := make([]LogEntry, length)

	// `head` always points **to** the slot for the *next* write, so the
	// oldest entry is there, and the newest entry is just before it.
	// We need to lay the data out linearly in time order:
	//
	//	[head … max-1]  followed by  [0 … head-1]
	if st.full {
		// Ring buffer has wrapped - linearize it
		n := copy(out, st.logs[st.head:])
		copy(out[n:], st.logs[:st.head])
	} else {
		// Ring buffer hasn't wrapped yet - simple copy from beginning
		copy(out, st.logs[:st.head])
	}

	return out, nil
}

// ParseLogsFromBytes is a zero-allocation* parser for an s6 "current"
// file.  It scans the buffer **once**, pre-allocates the result slice
// and never calls strings.Split/Index, so the costly
// runtime.growslice/strings.* nodes vanish from the profile.
//
//	*apart from the unavoidable string↔[]byte conversions needed for the
//	LogEntry struct – those are just header copies, no heap memcopy.
func ParseLogsFromBytes(buf []byte) ([]LogEntry, error) {
	// Trim one trailing newline that is always present in rotated logs.
	buf = bytes.TrimSuffix(buf, []byte{'\n'})

	// 1) -------- pre-allocation --------------------------------------
	nLines := bytes.Count(buf, []byte{'\n'}) + 1
	entries := make([]LogEntry, 0, nLines) // avoids  runtime.growslice

	// 2) -------- single pass over the buffer -------------------------
	for start := 0; start < len(buf); {
		// find next '\n'
		nl := bytes.IndexByte(buf[start:], '\n')
		var line []byte
		if nl == -1 {
			line = buf[start:]
			start = len(buf)
		} else {
			line = buf[start : start+nl]
			start += nl + 1
		}
		if len(line) == 0 { // empty line – rotate artefact
			continue
		}

		// 3) -------- parse one line ----------------------------------
		// format: 2025-04-20 13:01:02.123456789␠␠payload
		sep := bytes.Index(line, []byte("  "))
		if sep == -1 || sep < 29 { // malformed – keep raw
			entries = append(entries, LogEntry{Content: string(line)})
			continue
		}

		ts, err := ParseNano(string(line[:sep])) // ParseNano is already fast
		if err != nil {
			entries = append(entries, LogEntry{Content: string(line)})
			continue
		}

		entries = append(entries, LogEntry{
			Timestamp: ts,
			Content:   string(line[sep+2:]),
		})
	}
	return entries, nil
}

// ParseLogsFromBytes_Unoptimized is the more readable not optimized version of ParseLogsFromBytes
func ParseLogsFromBytes_Unoptimized(content []byte) ([]LogEntry, error) {
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
	// Quick check for empty strings or too short lines
	if len(line) < 28 { // Minimum length for "YYYY-MM-DD HH:MM:SS.<9 digit nanoseconds>  content"
		return LogEntry{Content: line}
	}

	// Check if we have the double space separator
	sepIdx := strings.Index(line, "  ")
	if sepIdx == -1 || sepIdx > 29 {
		return LogEntry{Content: line}
	}

	// Extract timestamp part
	timestampStr := line[:sepIdx]

	// Extract content part (after the double space)
	content := ""
	if sepIdx+2 < len(line) {
		content = line[sepIdx+2:]
	}

	// Try to parse the timestamp
	// We are using ParseNano over time.Parse because it is faster for our specific time format
	timestamp, err := ParseNano(timestampStr)
	if err != nil {
		return LogEntry{Content: line}
	}

	return LogEntry{
		Timestamp: timestamp,
		Content:   content,
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

// findLatestRotatedFile finds the most recently rotated file using slices.MaxFunc.
//
// S6 creates rotated files with TAI64N timestamps in their names (e.g., @400000006501234567890abc.s).
// TAI64N timestamps are designed to be lexicographically sortable, so we can use string comparison
// to find the chronologically latest file efficiently.
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
