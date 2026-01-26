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

package sentry

import (
	"fmt"
	"math"
	"strconv"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

// FingerprintKeys are the field keys that affect Sentry grouping.
// These fields group errors by their semantic meaning rather than instance-specific data.
var FingerprintKeys = []string{"operation", "fsm_type", "service_type", "trigger"}

// SentryHook implements zapcore.Core and automatically captures Error and Warn
// level logs to Sentry. It wraps an existing zapcore.Core and delegates all
// logging operations to it while asynchronously sending events to Sentry.
type SentryHook struct {
	zapcore.Core
}

// NewSentryHook creates a new SentryHook wrapping the given zapcore.Core.
// The hook will capture Error and Warn level logs to Sentry in a non-blocking
// goroutine, while still delegating all logging to the underlying core.
func NewSentryHook(core zapcore.Core) *SentryHook {
	return &SentryHook{Core: core}
}

// With returns a new SentryHook with the given fields added to the context.
func (h *SentryHook) With(fields []zapcore.Field) zapcore.Core {
	return &SentryHook{Core: h.Core.With(fields)}
}

// Check determines whether the entry should be logged.
func (h *SentryHook) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if h.Enabled(entry.Level) {
		return ce.AddCore(entry, h)
	}

	return ce
}

// Write logs the entry to the underlying core and captures Error/Warn to Sentry.
func (h *SentryHook) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// Only capture Error and Warn levels to Sentry
	if entry.Level >= zapcore.WarnLevel {
		// Extract fields for Sentry context (non-blocking goroutine)
		go h.captureToSentry(entry, fields)
	}

	// Always delegate to underlying core
	return h.Core.Write(entry, fields)
}

// captureToSentry sends the log entry to Sentry asynchronously.
func (h *SentryHook) captureToSentry(entry zapcore.Entry, fields []zapcore.Field) {
	context := extractFieldsAsContext(fields)
	fingerprint := extractFingerprintKeys(fields)

	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(zapLevelToSentry(entry.Level))

		// Add fingerprint keys for proper grouping
		baseFingerprint := []string{
			"{{ default }}",
			"level: " + getLevelString(zapLevelToSentry(entry.Level)),
		}
		scope.SetFingerprint(append(baseFingerprint, fingerprint...))

		// Add all context as tags
		for k, v := range context {
			scope.SetTag(k, v)
		}

		sentry.CaptureMessage(entry.Message)
	})
}

// extractFieldsAsContext converts zap fields to a map of string values for Sentry tags.
func extractFieldsAsContext(fields []zapcore.Field) map[string]string {
	context := make(map[string]string)

	for _, field := range fields {
		switch field.Type {
		case zapcore.StringType:
			context[field.Key] = field.String
		case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
			context[field.Key] = strconv.FormatInt(field.Integer, 10)
		case zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type:
			context[field.Key] = strconv.FormatUint(uint64(field.Integer), 10)
		case zapcore.BoolType:
			if field.Integer == 1 {
				context[field.Key] = "true"
			} else {
				context[field.Key] = "false"
			}
		case zapcore.Float64Type, zapcore.Float32Type:
			if field.Type == zapcore.Float64Type {
				context[field.Key] = strconv.FormatFloat(math.Float64frombits(uint64(field.Integer)), 'g', -1, 64)
			} else {
				context[field.Key] = strconv.FormatFloat(float64(math.Float32frombits(uint32(field.Integer))), 'g', -1, 32)
			}
		case zapcore.DurationType:
			context[field.Key] = strconv.FormatInt(field.Integer, 10)
		default:
			// For complex types, convert to string representation
			if field.Interface != nil {
				context[field.Key] = fmt.Sprintf("%v", field.Interface)
			}
		}
	}

	return context
}

// extractFingerprintKeys extracts the fingerprint keys from fields.
func extractFingerprintKeys(fields []zapcore.Field) []string {
	var fingerprint []string

	for _, field := range fields {
		for _, key := range FingerprintKeys {
			if field.Key == key {
				value := ""

				switch field.Type {
				case zapcore.StringType:
					value = field.String
				default:
					if field.Interface != nil {
						value = fmt.Sprintf("%v", field.Interface)
					} else {
						value = strconv.FormatInt(field.Integer, 10)
					}
				}

				fingerprint = append(fingerprint, fmt.Sprintf("%s: %s", key, value))

				break
			}
		}
	}

	return fingerprint
}

// zapLevelToSentry converts a zapcore.Level to a sentry.Level.
func zapLevelToSentry(level zapcore.Level) sentry.Level {
	switch level {
	case zapcore.DebugLevel:
		return sentry.LevelDebug
	case zapcore.InfoLevel:
		return sentry.LevelInfo
	case zapcore.WarnLevel:
		return sentry.LevelWarning
	case zapcore.ErrorLevel:
		return sentry.LevelError
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		return sentry.LevelFatal
	default:
		return sentry.LevelInfo
	}
}
