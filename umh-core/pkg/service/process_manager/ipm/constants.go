//go:build internal_process_manager
// +build internal_process_manager

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

import "time"

// Directory and file structure constants
const (
	// Service directory structure
	LogDirectoryName    = "log"
	ConfigDirectoryName = "config"

	// Standard file names
	PidFileName        = "run.pid"
	RunScriptFileName  = "run.sh"
	CurrentLogFileName = "current"
)

// Memory and storage limits
const (
	// Memory log buffer configuration
	DefaultLogBufferSize = 10000 // 10k lines per service
	MinLogBufferSize     = 1000  // Minimum 1k lines
	MaxLogBufferSize     = 50000 // Maximum 50k lines

	// File size limits for rotation
	DefaultLogFileSize = 1024 * 1024       // 1MB
	MinLogFileSize     = 4096              // 4KB minimum
	MaxLogFileSize     = 256 * 1024 * 1024 // 256MB maximum

	// Log file retention
	DefaultLogFileRetention = 10  // Keep 10 rotated files
	MinLogFileRetention     = 1   // Keep at least 1 file
	MaxLogFileRetention     = 100 // Maximum 100 files

	// Exit event history limits
	MaxExitEvents = 100 // Keep last 100 exit events per service
)

// File permissions and modes
const (
	ConfigFilePermission = 0644 // Standard config file permissions
	LogFilePermission    = 0644 // Standard log file permissions
	ScriptFilePermission = 0755 // Executable script permissions
	DirectoryPermission  = 0755 // Standard directory permissions
)

// Process management constants
const (
	// Memory limits
	MinMemoryLimitBytes = 1024 * 1024 // Minimum 1MB memory limit

	// Process termination timeouts
	GracefulShutdownTimeout = 10 * time.Second       // Time to wait for SIGTERM
	ForceKillTimeout        = 5 * time.Second        // Time to wait before SIGKILL
	ProcessCheckInterval    = 100 * time.Millisecond // Process status check interval
)

// Reconciliation and timing constants
const (
	// Step timing controls
	CleanupTimeReserve = 20 * time.Millisecond // Reserve time for cleanup
	StepTimeThreshold  = 10 * time.Millisecond // Minimum time to start new step

	// Context and deadline management
	DefaultReconcileTimeout = 30 * time.Second // Default reconciliation timeout
	TaskProcessingTimeout   = 10 * time.Second // Individual task timeout
)

// Log formatting constants
const (
	// S6-compatible log format
	LogTimestampFormat = "2006-01-02 15:04:05.000000000" // Nanosecond precision
	LogEntrySeparator  = "  "                            // Double space separator
	LogFileExtension   = ".log"                          // Rotated log file extension

	// Default log line characteristics for estimation
	AverageLogLineLength = 80   // Estimated average log line length
	MaxLogLineLength     = 4096 // Maximum reasonable log line length
)
