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

package integration

import (
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestLogger provides a logger that captures logs for verification in integration tests.
type TestLogger struct {
	Logger *zap.SugaredLogger
	Logs   *observer.ObservedLogs
	mu     sync.RWMutex
}

// NewTestLogger creates a new TestLogger with the specified log level.
func NewTestLogger(level zapcore.Level) *TestLogger {
	core, logs := observer.New(level)
	logger := zap.New(core).Sugar()

	return &TestLogger{
		Logger: logger,
		Logs:   logs,
	}
}

// HasLogWithMessage checks if any captured log contains the specified message substring.
func (tl *TestLogger) HasLogWithMessage(msg string) bool {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	entries := tl.Logs.All()
	for _, entry := range entries {
		if strings.Contains(entry.Message, msg) {
			return true
		}
	}

	return false
}

// HasLogWithField checks if any captured log has a field with the specified key and value.
func (tl *TestLogger) HasLogWithField(key string, value interface{}) bool {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	entries := tl.Logs.All()
	for _, entry := range entries {
		for _, field := range entry.Context {
			if field.Key == key && field.Interface == value {
				return true
			}
		}
	}

	return false
}

// GetLogsMatching returns all log entries that contain the specified message substring.
func (tl *TestLogger) GetLogsMatching(msg string) []observer.LoggedEntry {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	var matching []observer.LoggedEntry

	entries := tl.Logs.All()

	for _, entry := range entries {
		if strings.Contains(entry.Message, msg) {
			matching = append(matching, entry)
		}
	}

	return matching
}

// GetErrorsAndWarnings returns all log entries at ERROR or WARN level.
func (tl *TestLogger) GetErrorsAndWarnings() []observer.LoggedEntry {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	var errorsAndWarnings []observer.LoggedEntry

	entries := tl.Logs.All()

	for _, entry := range entries {
		if entry.Level == zapcore.ErrorLevel || entry.Level == zapcore.WarnLevel {
			errorsAndWarnings = append(errorsAndWarnings, entry)
		}
	}

	return errorsAndWarnings
}

// CountLogsWithMessage returns the count of log entries containing the specified message substring.
func (tl *TestLogger) CountLogsWithMessage(msg string) int {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	count := 0

	entries := tl.Logs.All()

	for _, entry := range entries {
		if strings.Contains(entry.Message, msg) {
			count++
		}
	}

	return count
}

// GetLogsWithFieldContaining returns log entries with a field matching the key and value substring.
func (tl *TestLogger) GetLogsWithFieldContaining(key, valueSubstring string) []observer.LoggedEntry {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	var matching []observer.LoggedEntry

	entries := tl.Logs.All()

	for _, entry := range entries {
		for _, field := range entry.Context {
			if field.Key == key {
				if strVal, ok := field.Interface.(string); ok {
					if strings.Contains(strVal, valueSubstring) {
						matching = append(matching, entry)

						break
					}
				}
			}
		}
	}

	return matching
}

// GetLogsMissingField returns all log entries that are missing the specified field.
func (tl *TestLogger) GetLogsMissingField(key string) []observer.LoggedEntry {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	var missing []observer.LoggedEntry

	entries := tl.Logs.All()

	for _, entry := range entries {
		hasField := false

		for _, field := range entry.Context {
			if field.Key == key {
				hasField = true

				break
			}
		}

		if !hasField {
			missing = append(missing, entry)
		}
	}

	return missing
}
