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

import (
	"io"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/logging"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
)

// UMHCoreLogger captures stdout/stderr and forwards to both original output and memory buffer
type UMHCoreLogger struct {
	originalWriter io.Writer
	memoryBuffer   *logging.MemoryLogBuffer
	logLineWriter  *logging.LogLineWriter
}

// NewUMHCoreLogger creates a new stdout capturer for the umh-core service
func NewUMHCoreLogger(originalWriter io.Writer, memoryBuffer *logging.MemoryLogBuffer, logLineWriter *logging.LogLineWriter) *UMHCoreLogger {
	return &UMHCoreLogger{
		originalWriter: originalWriter,
		memoryBuffer:   memoryBuffer,
		logLineWriter:  logLineWriter,
	}
}

// Write implements io.Writer interface - captures output and forwards to both destinations
func (ucl *UMHCoreLogger) Write(p []byte) (n int, err error) {
	// Write to original output first (preserve existing behavior)
	n, err = ucl.originalWriter.Write(p)
	if err != nil {
		return n, err
	}

	// Create log entry from the written data
	entry := process_shared.LogEntry{
		Timestamp: time.Now(),
		Content:   string(p),
	}

	// Add to memory buffer directly (for immediate availability)
	ucl.memoryBuffer.AddEntry(entry)

	// Also write through LogLineWriter (for file logging and rotation)
	if ucl.logLineWriter != nil {
		if writeErr := ucl.logLineWriter.WriteLine(entry); writeErr != nil {
			// Log error but don't fail the original write
			// We could log this somewhere, but since we're capturing stdout,
			// we can't use regular logging without creating a loop
		}
	}

	return n, nil
}

// Close closes the log line writer
func (ucl *UMHCoreLogger) Close() error {
	if ucl.logLineWriter != nil {
		return ucl.logLineWriter.Close()
	}
	return nil
}
