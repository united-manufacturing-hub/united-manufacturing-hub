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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

// SentryHook wraps a zapcore.Core and sends error/warning logs to Sentry.
type SentryHook struct {
	zapcore.Core
	debouncer *FingerprintDebouncer
}

// NewSentryHook creates a new Sentry hook with the specified debounce window.
func NewSentryHook(debounceWindow time.Duration) *SentryHook {
	return &SentryHook{
		debouncer: NewFingerprintDebouncer(debounceWindow),
	}
}

// Debouncer returns the underlying debouncer for testing purposes.
func (h *SentryHook) Debouncer() *FingerprintDebouncer {
	return h.debouncer
}

// Wrap wraps an existing zapcore.Core with Sentry capture functionality.
func (h *SentryHook) Wrap(core zapcore.Core) zapcore.Core {
	h.Core = core

	return h
}

// Write intercepts log writes to capture errors to Sentry.
func (h *SentryHook) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// Only capture WARN and ERROR levels
	if entry.Level >= zapcore.WarnLevel {
		h.captureToSentry(entry, fields)
	}

	return h.Core.Write(entry, fields)
}

// Check returns a CheckedEntry if the log level is enabled.
func (h *SentryHook) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if h.Enabled(entry.Level) {
		return ce.AddCore(entry, h)
	}

	return ce
}

func (h *SentryHook) captureToSentry(entry zapcore.Entry, fields []zapcore.Field) {
	fieldMap := FieldsToMap(fields)

	// Extract feature field - use "unknown" if missing
	feature := ExtractFeature(fieldMap)

	// Extract error DIRECTLY from fields (FieldsToMap converts errors to strings)
	err := ExtractErrorFromFields(fields)

	var errorTypes string

	if err != nil {
		errorTypes = ExtractErrorTypes(err)
	}

	// Build fingerprint using error TYPES (stable), not messages (contain URLs/IPs)
	fingerprint := BuildFingerprint(entry.Level, feature, entry.Message, errorTypes)

	// Check debouncer
	fpKey := strings.Join(fingerprint, "|")
	if !h.debouncer.ShouldCapture(fpKey) {
		return
	}

	// Build event manually for full control
	event := sentry.NewEvent()
	event.Level = ZapLevelToSentry(entry.Level)
	event.Message = entry.Message

	// Add error as exception with event_name as the Type (shows as title in Sentry UI)
	// This makes the title "action_failed" instead of "*fmt.wrapError"
	if err != nil {
		event.Exception = []sentry.Exception{
			{
				Type:  entry.Message, // "action_failed" - becomes the title
				Value: err.Error(),   // "connection failed: ..." - becomes subtitle
			},
		}
	}

	// Set tags - include error_types for searchability
	event.Tags = map[string]string{
		"feature":    feature,
		"event_name": entry.Message,
	}

	if errorTypes != "" {
		event.Tags["error_types"] = errorTypes
	}

	// Auto-derive from hierarchy_path
	if path, ok := fieldMap["hierarchy_path"].(string); ok && path != "" {
		info := ParseHierarchyPath(path)
		event.Tags["fsm_version"] = info.FSMVersion
		event.Tags["worker_type"] = info.WorkerType
	}

	event.Fingerprint = fingerprint

	hub := sentry.CurrentHub().Clone()
	hub.CaptureEvent(event)
}

// FieldsToMap converts zapcore.Field slice to a map for easier access.
func FieldsToMap(fields []zapcore.Field) map[string]interface{} {
	result := make(map[string]interface{})

	enc := zapcore.NewMapObjectEncoder()

	for _, f := range fields {
		f.AddTo(enc)
	}

	for k, v := range enc.Fields {
		result[k] = v
	}

	return result
}

// ExtractErrorTypes walks the error chain and extracts type names.
// Types are STABLE (same across customers), messages are VARIABLE (contain URLs/IPs).
func ExtractErrorTypes(err error) string {
	if err == nil {
		return ""
	}

	var types []string

	current := err

	for current != nil {
		types = append(types, fmt.Sprintf("%T", current))
		current = errors.Unwrap(current)
	}

	return strings.Join(types, "|")
}

// ZapLevelToSentry converts zap log level to Sentry level.
func ZapLevelToSentry(level zapcore.Level) sentry.Level {
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

// BuildFingerprint creates a Sentry fingerprint from log components.
// The fingerprint uses stable components (error types) rather than variable
// components (error messages with URLs/IPs) to ensure proper grouping.
func BuildFingerprint(level zapcore.Level, feature, eventName, errorTypes string) []string {
	fingerprint := []string{
		"level: " + level.String(),
		"feature: " + feature,
		"event_name: " + eventName,
	}

	if errorTypes != "" {
		fingerprint = append(fingerprint, "error_types: "+errorTypes)
	}

	return fingerprint
}

// ExtractFeature extracts the feature field from a field map.
// Returns "unknown" if the feature field is missing, empty, or not a string.
// Note: This function intentionally does NOT log warnings to avoid recursive
// Sentry captures when the warning itself triggers the hook.
func ExtractFeature(fieldMap map[string]interface{}) string {
	feature, ok := fieldMap["feature"].(string)
	if !ok || feature == "" {
		return "unknown"
	}

	return feature
}

// ExtractErrorFromFields extracts the error interface directly from zap fields.
// This is needed because FieldsToMap/MapObjectEncoder converts errors to strings,
// losing the error interface needed for SetException and error type extraction.
func ExtractErrorFromFields(fields []zapcore.Field) error {
	for _, f := range fields {
		if f.Key == "error" && f.Type == zapcore.ErrorType {
			if err, ok := f.Interface.(error); ok {
				return err
			}
		}
	}

	return nil
}
