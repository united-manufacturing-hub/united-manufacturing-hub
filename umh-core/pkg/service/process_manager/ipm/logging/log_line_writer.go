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
	"os"
	"path/filepath"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
	"go.uber.org/zap"
)

// LogLineWriter handles writing log entries to both memory buffer and file system.
// It provides a unified interface for hybrid logging where logs are stored in memory
// for fast retrieval and also persisted to files for durability. The writer handles
// file rotation automatically when size thresholds are exceeded and coordinates
// with the LogManager for proper log file management.
type LogLineWriter struct {
	Identifier   constants.ServiceIdentifier
	LogManager   *LogManager
	MemoryBuffer *MemoryLogBuffer
	logger       *zap.SugaredLogger
	FsService    filesystem.Service

	mu          sync.Mutex
	currentFile *os.File
	LogPath     string
	currentSize int64
	maxFileSize int64
}

// NewLogLineWriter creates a new log line writer for the specified service.
// It initializes both memory and file-based logging components and registers
// the service with the log manager for rotation monitoring. The writer will
// create the current log file if it doesn't exist and track its size for
// rotation purposes.
func NewLogLineWriter(identifier constants.ServiceIdentifier, logPath string, logManager *LogManager, memoryBuffer *MemoryLogBuffer, fsService filesystem.Service) (*LogLineWriter, error) {
	if logManager == nil {
		return nil, fmt.Errorf("logManager cannot be nil")
	}
	if memoryBuffer == nil {
		return nil, fmt.Errorf("memoryBuffer cannot be nil")
	}

	writer := &LogLineWriter{
		Identifier:   identifier,
		LogManager:   logManager,
		MemoryBuffer: memoryBuffer,
		logger:       logManager.Logger, // Use the same logger as LogManager
		FsService:    fsService,
		LogPath:      logPath,
		maxFileSize:  constants.DefaultLogFileSize,
	}

	// Register with log manager for rotation coordination
	logManager.RegisterService(identifier, logPath, writer.maxFileSize)

	// Initialize current file
	if err := writer.initCurrentFile(); err != nil {
		return nil, fmt.Errorf("failed to initialize current log file: %w", err)
	}

	return writer, nil
}

// initCurrentFile opens or creates the current log file and determines its size.
// This method is called during writer initialization and after log rotation
// to ensure the current file is ready for writing. It handles both new file
// creation and existing file size detection for proper rotation tracking.
func (llw *LogLineWriter) initCurrentFile() error {
	currentLogFile := filepath.Join(llw.LogPath, constants.CurrentLogFileName)

	// Check if file exists and get its current size
	var currentSize int64
	if stat, err := llw.FsService.Stat(context.Background(), currentLogFile); err == nil {
		currentSize = stat.Size()
	}

	// Open file for appending (create if it doesn't exist)
	file, err := os.OpenFile(currentLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, constants.LogFilePermission)
	if err != nil {
		return fmt.Errorf("failed to open current log file: %w", err)
	}

	llw.currentFile = file
	llw.currentSize = currentSize

	return nil
}

// WriteLine writes a log entry to both the memory buffer and the current log file.
// This is the main entry point for logging operations. The method is thread-safe
// and handles file rotation automatically when size thresholds are exceeded.
// Log entries are formatted consistently for file storage while preserving
// their original structure in memory.
func (llw *LogLineWriter) WriteLine(entry process_shared.LogEntry) error {
	llw.mu.Lock()
	defer llw.mu.Unlock()

	// Add to memory buffer first (this is fast and always succeeds)
	llw.MemoryBuffer.AddEntry(entry)

	// Format entry for file writing
	// Use the same format as S6 logs: "YYYY-MM-DD HH:MM:SS.NNNNNNNNN  content"
	formattedLine := entry.Timestamp.Format(constants.LogTimestampFormat) + constants.LogEntrySeparator + entry.Content + "\n"

	// Write to current file
	if llw.currentFile != nil {
		n, err := llw.currentFile.WriteString(formattedLine)
		if err != nil {
			llw.logger.Errorf("Failed to write to log file for %s: %v", llw.Identifier, err)
			return fmt.Errorf("failed to write to log file: %w", err)
		}

		// Update size tracking
		llw.currentSize += int64(n)

		// Force flush to ensure data is written (important for crash recovery)
		if err := llw.currentFile.Sync(); err != nil {
			llw.logger.Warnf("Failed to sync log file for %s: %v", llw.Identifier, err)
			// Don't return error for sync failures - the write succeeded
		}

		// Check if rotation is needed
		if llw.currentSize > llw.maxFileSize {
			if err := llw.rotateLogFile(); err != nil {
				llw.logger.Errorf("Failed to rotate log file for %s: %v", llw.Identifier, err)
				return fmt.Errorf("failed to rotate log file: %w", err)
			}
		}
	}

	return nil
}

// rotateLogFile handles log file rotation when the current file exceeds size limits.
// It closes the current file, delegates the rotation logic to the LogManager,
// and initializes a new current file. This operation is coordinated with the
// LogManager to ensure consistent rotation behavior and proper cleanup of old files.
func (llw *LogLineWriter) rotateLogFile() error {
	// Close current file before rotation
	if llw.currentFile != nil {
		if err := llw.currentFile.Close(); err != nil {
			llw.logger.Warnf("Error closing log file during rotation for %s: %v", llw.Identifier, err)
		}
		llw.currentFile = nil
	}

	// Use LogManager to perform the rotation
	// This ensures consistent rotation behavior and cleanup
	if err := llw.LogManager.RotateLogFile(llw.Identifier, llw.FsService); err != nil {
		return fmt.Errorf("LogManager rotation failed: %w", err)
	}

	// Initialize new current file
	if err := llw.initCurrentFile(); err != nil {
		return fmt.Errorf("failed to initialize new current file: %w", err)
	}

	llw.logger.Infof("Log file rotated successfully for %s (previous size: %d bytes)", llw.Identifier, llw.currentSize)

	return nil
}

// Close closes the log file and cleans up resources. This method should be called
// when the service is stopped or removed to ensure proper cleanup. After calling
// Close, the writer should not be used for further logging operations.
func (llw *LogLineWriter) Close() error {
	llw.mu.Lock()
	defer llw.mu.Unlock()

	var closeErr error

	// Close current file if open
	if llw.currentFile != nil {
		if err := llw.currentFile.Close(); err != nil {
			llw.logger.Errorf("Error closing log file for %s: %v", llw.Identifier, err)
			closeErr = err
		}
		llw.currentFile = nil
	}

	// Reset size tracking
	llw.currentSize = 0

	// Note: We don't clear the memory buffer here as it may still be needed
	// for GetLogs() calls. The buffer will be cleaned up when the service is removed.

	return closeErr
}

// GetCurrentFileSize returns the current size of the log file in bytes.
// This method is thread-safe and can be used for monitoring log file growth.
func (llw *LogLineWriter) GetCurrentFileSize() int64 {
	llw.mu.Lock()
	defer llw.mu.Unlock()
	return llw.currentSize
}

// SetMaxFileSize updates the maximum file size threshold for rotation.
// This method allows dynamic adjustment of rotation behavior. The new size
// will be used for subsequent rotation checks.
func (llw *LogLineWriter) SetMaxFileSize(size int64) {
	llw.mu.Lock()
	defer llw.mu.Unlock()

	if size <= 0 {
		size = constants.DefaultLogFileSize
	}

	llw.maxFileSize = size
}
