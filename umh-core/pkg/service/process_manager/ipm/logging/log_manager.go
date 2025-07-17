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

package logging

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cactus/tai64"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
	"go.uber.org/zap"
)

// LogManager manages log file rotation for services
type LogManager struct {
	Logger *zap.SugaredLogger
	mu     sync.RWMutex

	// Track services and their log file sizes for rotation
	services map[constants.ServiceIdentifier]logInfo
}

type logInfo struct {
	LogPath        string
	MaxSize        int64
	CurrentLogFile string
}

// NewLogManager creates a new log manager
func NewLogManager(logger *zap.SugaredLogger) *LogManager {
	return &LogManager{
		Logger:   logger,
		services: make(map[constants.ServiceIdentifier]logInfo),
	}
}

// RegisterService registers a service for log rotation monitoring
func (lm *LogManager) RegisterService(identifier constants.ServiceIdentifier, logPath string, maxSize int64) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Enforce size limits
	if maxSize < constants.MinLogFileSize {
		maxSize = constants.MinLogFileSize
	} else if maxSize > constants.MaxLogFileSize {
		maxSize = constants.MaxLogFileSize
	}

	currentLogFile := filepath.Join(logPath, constants.CurrentLogFileName)

	lm.services[identifier] = logInfo{
		LogPath:        logPath,
		MaxSize:        maxSize,
		CurrentLogFile: currentLogFile,
	}

	lm.Logger.Debugf("Registered service for log rotation: %s (path: %s, maxSize: %d)", identifier, logPath, maxSize)
}

// UnregisterService removes a service from log rotation monitoring
func (lm *LogManager) UnregisterService(identifier constants.ServiceIdentifier) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	delete(lm.services, identifier)
	lm.Logger.Debugf("Unregistered service from log rotation: %s", identifier)
}

// CheckAndRotate checks all registered services and rotates logs if they exceed size limits
func (lm *LogManager) CheckAndRotate(ctx context.Context, fsService filesystem.Service) error {
	lm.mu.RLock()
	services := make(map[constants.ServiceIdentifier]logInfo)
	for k, v := range lm.services {
		services[k] = v
	}
	lm.mu.RUnlock()

	for identifier, info := range services {
		if err := lm.rotateIfNeeded(ctx, identifier, info, fsService); err != nil {
			lm.Logger.Errorf("Error during log rotation for %s: %v", identifier, err)
			// Continue with other services even if one fails
		}
	}

	return nil
}

// rotateIfNeeded checks if a log file needs rotation and performs it if necessary
func (lm *LogManager) rotateIfNeeded(ctx context.Context, identifier constants.ServiceIdentifier, info logInfo, fsService filesystem.Service) error {
	// Check if current log file exists and get its size
	stat, err := fsService.Stat(ctx, info.CurrentLogFile)
	if err != nil {
		// File doesn't exist yet, nothing to rotate
		return nil
	}

	// Check if file exceeds size limit
	if stat.Size() <= info.MaxSize {
		return nil
	}

	// Perform the actual rotation
	return lm.performRotation(ctx, identifier, info, fsService, stat.Size())
}

// RotateLogFile forces rotation for a specific service, regardless of size.
// This method is called by LogLineWriter when it detects that rotation is needed.
func (lm *LogManager) RotateLogFile(identifier constants.ServiceIdentifier, fsService filesystem.Service) error {
	lm.mu.RLock()
	info, exists := lm.services[identifier]
	lm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service not registered: %s", identifier)
	}

	// Get current file size before rotation for logging
	var currentSize int64
	if stat, err := fsService.Stat(context.Background(), info.CurrentLogFile); err == nil {
		currentSize = stat.Size()
	}

	// Perform the actual rotation
	return lm.performRotation(context.Background(), identifier, info, fsService, currentSize)
}

// performRotation performs the actual log file rotation with TAI64N timestamping and cleanup.
// This is the common implementation used by both rotateIfNeeded and RotateLogFile.
func (lm *LogManager) performRotation(ctx context.Context, identifier constants.ServiceIdentifier, info logInfo, fsService filesystem.Service, currentSize int64) error {
	// Generate TAI64N timestamp for the rotated file
	tai64nString := tai64.FormatNano(time.Now())
	// Remove the '@' prefix from the TAI64N string if present
	tai64nString = strings.TrimPrefix(tai64nString, "@")

	// Create rotated filename: TAI64N timestamp + .log extension
	rotatedPath := filepath.Join(info.LogPath, tai64nString+constants.LogFileExtension)

	// Rename current log file to rotated filename
	if err := fsService.Rename(ctx, info.CurrentLogFile, rotatedPath); err != nil {
		return fmt.Errorf("error renaming log file from %s to %s: %w", info.CurrentLogFile, rotatedPath, err)
	}

	lm.Logger.Infof("Log file rotated for %s: %s (size: %d bytes)", identifier, rotatedPath, currentSize)

	// Clean up old log files (keep only last 10)
	if err := lm.cleanupOldLogs(info.LogPath, fsService); err != nil {
		lm.Logger.Warnf("Error cleaning up old logs for %s: %v", identifier, err)
		// Don't return error - rotation succeeded
	}

	return nil
}

// cleanupOldLogs keeps only the 10 most recent log files in the specified directory.
// It sorts log files by modification time and removes the oldest ones beyond the limit.
func (lm *LogManager) cleanupOldLogs(logPath string, fsService filesystem.Service) error {
	files, err := fsService.ReadDir(context.Background(), logPath)
	if err != nil {
		return fmt.Errorf("failed to read log directory: %w", err)
	}

	// Filter .log files (exclude "current")
	var logFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), constants.LogFileExtension) && file.Name() != constants.CurrentLogFileName {
			logFiles = append(logFiles, filepath.Join(logPath, file.Name()))
		}
	}

	if len(logFiles) <= constants.DefaultLogFileRetention {
		return nil // No cleanup needed
	}

	// Sort by modification time (newest first)
	sort.Slice(logFiles, func(i, j int) bool {
		statI, errI := fsService.Stat(context.Background(), logFiles[i])
		statJ, errJ := fsService.Stat(context.Background(), logFiles[j])
		if errI != nil || errJ != nil {
			return false // Keep both files if we can't stat them
		}
		return statI.ModTime().After(statJ.ModTime())
	})

	// Remove old files (keep first N)
	for i := constants.DefaultLogFileRetention; i < len(logFiles); i++ {
		if err := fsService.Remove(context.Background(), logFiles[i]); err != nil {
			lm.Logger.Errorf("Error removing old log file %s: %v", logFiles[i], err)
			// Continue with other files
		} else {
			lm.Logger.Debugf("Removed old log file: %s", logFiles[i])
		}
	}

	return nil
}

// CloseAll performs cleanup - no file handles to close with shell redirection approach
func (lm *LogManager) CloseAll() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.Logger.Info("Closing LogManager")

	// Clear the services map
	lm.services = make(map[constants.ServiceIdentifier]logInfo)

	return nil
}
