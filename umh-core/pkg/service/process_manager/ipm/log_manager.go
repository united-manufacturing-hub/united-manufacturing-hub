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

package ipm

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cactus/tai64"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
)

// LogManager manages log file rotation for services
type LogManager struct {
	Logger *zap.SugaredLogger
	mu     sync.RWMutex

	// Track services and their log file sizes for rotation
	services map[serviceIdentifier]logInfo
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
		services: make(map[serviceIdentifier]logInfo),
	}
}

// RegisterService registers a service for log rotation monitoring
func (lm *LogManager) RegisterService(identifier serviceIdentifier, logPath string, maxSize int64) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Enforce size limits (4KB min, ~256MB max)
	if maxSize < 4096 {
		maxSize = 4096
	} else if maxSize > 256*1024*1024 {
		maxSize = 256 * 1024 * 1024
	}

	currentLogFile := filepath.Join(logPath, "current")

	lm.services[identifier] = logInfo{
		LogPath:        logPath,
		MaxSize:        maxSize,
		CurrentLogFile: currentLogFile,
	}

	lm.Logger.Debug("Registered service for log rotation",
		zap.String("identifier", string(identifier)),
		zap.String("logPath", logPath),
		zap.Int64("maxSize", maxSize))
}

// UnregisterService removes a service from log rotation monitoring
func (lm *LogManager) UnregisterService(identifier serviceIdentifier) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	delete(lm.services, identifier)
	lm.Logger.Debug("Unregistered service from log rotation", zap.String("identifier", string(identifier)))
}

// CheckAndRotate checks all registered services and rotates logs if they exceed size limits
func (lm *LogManager) CheckAndRotate(ctx context.Context, fsService filesystem.Service) error {
	lm.mu.RLock()
	services := make(map[serviceIdentifier]logInfo)
	for k, v := range lm.services {
		services[k] = v
	}
	lm.mu.RUnlock()

	for identifier, info := range services {
		if err := lm.rotateIfNeeded(ctx, identifier, info, fsService); err != nil {
			lm.Logger.Error("Error during log rotation",
				zap.String("identifier", string(identifier)),
				zap.Error(err))
			// Continue with other services even if one fails
		}
	}

	return nil
}

// rotateIfNeeded checks if a log file needs rotation and performs it if necessary
func (lm *LogManager) rotateIfNeeded(ctx context.Context, identifier serviceIdentifier, info logInfo, fsService filesystem.Service) error {
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

	// Generate TAI64N timestamp for the rotated file
	tai64nString := tai64.FormatNano(time.Now())

	// Remove the '@' prefix from the TAI64N string if present
	if strings.HasPrefix(tai64nString, "@") {
		tai64nString = tai64nString[1:]
	}

	// Create rotated filename: TAI64N timestamp + .log extension
	rotatedPath := filepath.Join(info.LogPath, tai64nString+".log")

	// Rename current log file to rotated filename
	if err := fsService.Rename(ctx, info.CurrentLogFile, rotatedPath); err != nil {
		return fmt.Errorf("error renaming log file from %s to %s: %w", info.CurrentLogFile, rotatedPath, err)
	}

	lm.Logger.Info("Log file rotated",
		zap.String("identifier", string(identifier)),
		zap.String("rotatedFile", rotatedPath),
		zap.Int64("size", stat.Size()))

	// Note: We don't need to create a new "current" file - the shell redirection will create it
	// automatically when the process writes to it next

	return nil
}

// CloseAll performs cleanup - no file handles to close with shell redirection approach
func (lm *LogManager) CloseAll() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.Logger.Info("Closing LogManager")

	// Clear the services map
	lm.services = make(map[serviceIdentifier]logInfo)

	return nil
}
