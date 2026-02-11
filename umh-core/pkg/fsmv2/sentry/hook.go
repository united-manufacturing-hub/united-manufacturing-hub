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
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/gostackparse"
	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

var sentryEventsDropped atomic.Int64

// SentryHook wraps a zapcore.Core and sends warn- and error-level logs to Sentry.
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

// Stop releases resources held by the hook, including the debouncer's cleanup goroutine.
// Call this when the hook is no longer needed to prevent goroutine leaks.
func (h *SentryHook) Stop() {
	if h.debouncer != nil {
		h.debouncer.Stop()
	}
}

// Wrap wraps an existing zapcore.Core with Sentry capture functionality.
func (h *SentryHook) Wrap(core zapcore.Core) zapcore.Core {
	h.Core = core

	return h
}

// Write intercepts log writes to capture warnings and errors to Sentry.
func (h *SentryHook) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if h.Core == nil {
		return nil
	}

	if entry.Level >= zapcore.WarnLevel {
		h.captureToSentry(entry, fields)
	}

	return h.Core.Write(entry, fields)
}

// Check returns a CheckedEntry if the log level is enabled.
func (h *SentryHook) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if h.Core == nil {
		return ce
	}

	if h.Enabled(entry.Level) {
		return ce.AddCore(entry, h)
	}

	return ce
}

// With returns a SentryHook that wraps the inner core with the given fields.
// Supervisors call logger.With() to add "worker" fields, and this method ensures
// Sentry capture continues for derived loggers.
func (h *SentryHook) With(fields []zapcore.Field) zapcore.Core {
	return &SentryHook{
		Core:      h.Core.With(fields),
		debouncer: h.debouncer, // Share debouncer across derived loggers
	}
}

func (h *SentryHook) captureToSentry(entry zapcore.Entry, fields []zapcore.Field) {
	defer func() {
		if r := recover(); r != nil {
			sentryEventsDropped.Add(1)
			fmt.Fprintf(os.Stderr, "sentry: recovered panic in captureToSentry: %v\n", r)
		}
	}()

	fieldMap := FieldsToMap(fields)

	// Extract feature field; falls back to "unknown" as a safety net (all FSMLogger warn/error methods inject feature)
	feature := ExtractFeature(fieldMap)

	// Extract error DIRECTLY from fields (FieldsToMap converts errors to strings)
	err := ExtractErrorFromFields(fields)

	var errorTypes string

	if err != nil {
		errorTypes = ExtractErrorTypes(err)
	}

	// Build fingerprint using error TYPES (stable), not messages (contain URLs/IPs)
	fingerprint := BuildFingerprint(entry.Level, feature, entry.Message, errorTypes)

	fpKey := strings.Join(fingerprint, "|")
	if !h.debouncer.ShouldCapture(fpKey) {
		return
	}

	event := sentry.NewEvent()
	event.Level = ZapLevelToSentry(entry.Level)
	event.Message = entry.Message

	// Always capture a stacktrace for debugging context.
	// Priority: 1) Explicit "stack" field (from panic recovery), 2) Capture at log point
	// This follows Sentry best practice: errors without stacktraces are hard to debug.
	// See: https://incident.io/blog/golang-errors
	var stacktrace *sentry.Stacktrace

	if stack, ok := fieldMap["stack"].(string); ok && stack != "" {
		// Use explicit stack from panic recovery (already captured at panic site)
		stacktrace = ParseStackTrace(stack)
	} else {
		// Capture stacktrace at the point of logging and filter out internal frames
		stacktrace = newFilteredStacktrace()
	}

	// Add error as exception with event_name as the Type (appears as title in Sentry UI).
	// The title displays "action_failed" instead of "*fmt.wrapError".
	if err != nil {
		exception := sentry.Exception{
			Type:  entry.Message, // "action_failed" - becomes the title
			Value: err.Error(),   // "connection failed: ..." - becomes subtitle
		}

		if stacktrace != nil {
			exception.Stacktrace = stacktrace
		}

		event.Exception = []sentry.Exception{exception}
	} else if stacktrace != nil {
		// For warnings without errors, attach stacktrace to event threads
		// so there's still debugging context
		event.Threads = []sentry.Thread{
			{
				ID:         "main",
				Name:       "main",
				Stacktrace: stacktrace,
				Current:    true,
			},
		}
	}

	event.Tags = map[string]string{
		"feature":    feature,
		"event_name": entry.Message,
	}

	if errorTypes != "" {
		event.Tags["error_types"] = TruncateTag(errorTypes, maxTagLen)
	}

	// Auto-derive from hierarchy_path
	if path, ok := fieldMap["hierarchy_path"].(string); ok && path != "" {
		info := ParseHierarchyPath(path)
		event.Tags["fsm_version"] = info.FSMVersion
		event.Tags["worker_type"] = info.WorkerType
		if info.WorkerChain != "" {
			event.Tags["worker_chain"] = TruncateTag(info.WorkerChain, maxTagLen)
		}
	}

	if actionName, ok := fieldMap["action_name"].(string); ok && actionName != "" {
		event.Tags["action_name"] = actionName
	}

	// Catch-all: remaining fields go to Contexts for per-event troubleshooting.
	// Tags are searchable; Contexts are visible per-event but not searchable.
	handledKeys := map[string]struct{}{
		"feature":        {},
		"stack":          {},
		"panic":          {},
		"action_name":    {},
		"hierarchy_path": {},
	}
	if err != nil {
		handledKeys["error"] = struct{}{}
	}

	umhContext := make(map[string]interface{})
	if panicVal, ok := fieldMap["panic"].(string); ok && panicVal != "" {
		umhContext["panic_value"] = panicVal
	}
	for k, v := range fieldMap {
		if _, handled := handledKeys[k]; handled {
			continue
		}
		if IsSensitiveKey(k) {
			continue
		}
		if s, ok := v.(string); ok && len(s) > 1024 {
			v = s[:1024] + "...[truncated]"
		}
		umhContext[k] = v
	}
	if len(umhContext) > 0 {
		event.Contexts["umh_context"] = umhContext
	}

	event.Fingerprint = fingerprint

	hub := sentry.CurrentHub().Clone()
	eventID := hub.CaptureEvent(event)

	if eventID == nil {
		sentryEventsDropped.Add(1)
	}
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
// Error types remain consistent across customers; error messages vary because they contain URLs and IPs.
func ExtractErrorTypes(err error) string {
	if err == nil {
		return ""
	}

	var types []string

	var walk func(e error)

	walk = func(e error) {
		if e == nil {
			return
		}

		types = append(types, fmt.Sprintf("%T", e))

		// Handle multi-errors from errors.Join()
		if joined, ok := e.(interface{ Unwrap() []error }); ok {
			for _, inner := range joined.Unwrap() {
				walk(inner)
			}

			return
		}

		walk(errors.Unwrap(e))
	}

	walk(err)

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
// The function avoids logging warnings to prevent recursive Sentry captures
// when the warning itself triggers the hook.
func ExtractFeature(fieldMap map[string]interface{}) string {
	feature, ok := fieldMap["feature"].(string)
	if !ok || feature == "" {
		return "unknown"
	}

	return feature
}

const maxTagLen = 200

// TruncateTag truncates a tag value to maxLen characters, adding "..." suffix.
// Sentry silently truncates tags at 200 chars server-side; this makes truncation explicit.
func TruncateTag(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen-3] + "..."
}

var sensitiveKeys = map[string]struct{}{
	"password":            {},
	"secret":              {},
	"token":               {},
	"credential":          {},
	"auth_token":          {},
	"api_key":             {},
	"private_key":         {},
	"access_token":        {},
	"bearer_token":        {},
	"connection_password": {},
	"db_password":         {},
	"jwt_token":           {},
	"session_token":       {},
	"refresh_token":       {},
}

// IsSensitiveKey checks if a field key matches the sensitive-name denylist.
// Uses exact match (not substring) to avoid blocking legitimate fields like "cache_key".
func IsSensitiveKey(key string) bool {
	_, ok := sensitiveKeys[strings.ToLower(key)]
	return ok
}

// ExtractErrorFromFields extracts the error interface directly from zap fields.
// FieldsToMap and MapObjectEncoder convert errors to strings, which loses the error
// interface required for manual exception construction and error type extraction.
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

// ParseStackTrace parses a raw Go stack trace string into a Sentry Stacktrace.
// Returns nil if parsing fails or the stack is empty.
func ParseStackTrace(stack string) *sentry.Stacktrace {
	if stack == "" {
		return nil
	}

	goroutines, err := gostackparse.Parse(bytes.NewReader([]byte(stack)))
	if err != nil || len(goroutines) == 0 {
		return nil
	}

	// Use the first goroutine (the one that panicked)
	g := goroutines[0]
	if len(g.Stack) == 0 {
		return nil
	}

	var frames []sentry.Frame

	for _, gf := range g.Stack {
		absPath := gf.File
		fileName := filepath.Base(absPath)
		frame := sentry.Frame{
			Function: gf.Func,
			Filename: fileName,
			Lineno:   gf.Line,
			AbsPath:  absPath,
			// Mark UMH application code as in_app for Sentry UI highlighting
			InApp: isAppFrame(gf.Func, absPath),
		}
		frames = append(frames, frame)
	}

	if len(frames) == 0 {
		return nil
	}

	return &sentry.Stacktrace{
		Frames: frames,
	}
}

// internalPackagePrefixes lists package paths to filter from stacktraces.
// These internal logging and sentry packages clutter the trace.
var internalPackagePrefixes = []string{
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry",
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/sentry",
	"go.uber.org/zap",
	"github.com/getsentry/sentry-go",
	"runtime/",
	"runtime.",
}

// appPackagePrefix is the prefix for UMH application code.
// Frames with this prefix get in_app=true, helping Sentry highlight relevant code.
const appPackagePrefix = "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core"

// isAppFrame determines if a frame belongs to UMH application code.
// Used to set in_app=true in Sentry, which highlights relevant frames in the UI.
func isAppFrame(module, absPath string) bool {
	return strings.HasPrefix(module, appPackagePrefix) ||
		strings.Contains(absPath, "umh-core/pkg") ||
		strings.Contains(absPath, "umh-core/cmd")
}

// newFilteredStacktrace captures the current stacktrace and filters out internal frames.
// This removes sentry, zap, and runtime frames so the trace shows application code.
// It also sets in_app=true for UMH application code to help Sentry highlight relevant frames.
func newFilteredStacktrace() *sentry.Stacktrace {
	st := sentry.NewStacktrace()
	if st == nil || len(st.Frames) == 0 {
		return st
	}

	filtered := make([]sentry.Frame, 0, len(st.Frames))

	for _, frame := range st.Frames {
		if !IsInternalFrame(frame) {
			frame.InApp = isAppFrame(frame.Module, frame.AbsPath)
			filtered = append(filtered, frame)
		}
	}

	if len(filtered) == 0 {
		return st // Return original if filtering removes everything
	}

	return &sentry.Stacktrace{
		Frames: filtered,
	}
}

// IsInternalFrame checks if a frame belongs to internal logging or sentry packages.
// Tests use this function directly.
func IsInternalFrame(frame sentry.Frame) bool {
	// Check module (package path) - use HasPrefix for module paths
	for _, prefix := range internalPackagePrefixes {
		if strings.HasPrefix(frame.Module, prefix) {
			return true
		}
	}

	// Check AbsPath separately - use Contains since absolute paths start with /
	// e.g., "/usr/local/go/src/runtime/debug/stack.go" contains "runtime/"
	if strings.Contains(frame.AbsPath, "/sentry-go/") ||
		strings.Contains(frame.AbsPath, "/zap/") ||
		strings.Contains(frame.AbsPath, "/runtime/") ||
		strings.Contains(frame.AbsPath, "pkg/sentry/") ||
		strings.Contains(frame.AbsPath, "pkg/fsmv2/sentry/") {
		return true
	}

	if strings.Contains(frame.Function, "captureToSentry") ||
		strings.Contains(frame.Function, "SentryHook.Write") ||
		strings.Contains(frame.Function, "zapcore.") {
		return true
	}

	return false
}
