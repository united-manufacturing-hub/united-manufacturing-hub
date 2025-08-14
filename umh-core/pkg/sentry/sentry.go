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
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/getsentry/sentry-go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"go.uber.org/zap"
)

// Package-level state for debouncing errors.
var shouldDebounceErrors = true

// EnableTestMode disables debouncing for testing.
func EnableTestMode() {
	shouldDebounceErrors = false
}

// DisableTestMode restores normal debouncing behavior.
func DisableTestMode() {
	shouldDebounceErrors = true
}

// InitSentry initializes sentry with the given app name and version
// If debounceErrors is true, errors will be debounced to avoid spamming Sentry.
func InitSentry(appVersion string, debounceErrors bool) {
	// Set debouncing configuration
	shouldDebounceErrors = debounceErrors

	// Disable Sentry for local development (default version)
	// This prevents reporting local test failures to Sentry while still allowing
	// CI pipeline failures to be reported (where VERSION is set via ldflags).
	// The default appVersion "0.0.0-dev" comes from cmd/main.go when not built with proper version tags.
	if appVersion == "" || appVersion == constants.DefaultAppVersion {
		zap.S().Debug("Sentry disabled for local development build")

		return
	}

	environment := constants.DefaultDevelopmentEnvironment

	version, err := semver.NewVersion(appVersion)
	if err != nil {
		zap.S().Errorf("Failed to parse app version, using default environment (development): %s", err)
	} else if version.Prerelease() == "" {
		environment = constants.DefaultProductionEnvironment
	}

	err = sentry.Init(sentry.ClientOptions{
		// management.umh.app is not working anymore, so we use the direct DSN
		Dsn:           "https://1e1f51c30e576ff39d2445e76dc89da7@o4507265932394496.ingest.de.sentry.io/4509039283798097",
		Environment:   environment,
		Release:       "umhcore@" + appVersion,
		EnableTracing: false, // no need for tracing, it doesnt work anyway
	})
	if err != nil {
		zap.S().Error("Failed to initialize Sentry: %s", err)

		return
	}
}

func getMeaningfulErrorTitle(err error) string {
	message := err.Error()

	// Extract the first sentence or phrase(until period, comma or a colon)
	idx := strings.IndexAny(message, ".,:")
	if idx > 0 {
		message = message[:idx]
	}

	// Limit length of Sentry title
	if len(message) > 100 {
		message = message[:97] + "..."
	}

	return message
}

func createSentryEvent(level sentry.Level, err error) *sentry.Event {
	event := sentry.NewEvent()
	event.Level = level
	event.Message = err.Error()

	// Create exception with proper type name
	exception := &sentry.Exception{
		Type:       getMeaningfulErrorTitle(err),
		Value:      err.Error(),
		Module:     "", // Will be filled by stacktrace
		Stacktrace: sentry.ExtractStacktrace(err),
	}
	event.Exception = []sentry.Exception{*exception}

	// Capture all goroutines and convert them to Sentry threads
	if level == sentry.LevelFatal || level == sentry.LevelError {
		threads, stacktrace := captureGoroutinesAsThreads()
		event.Threads = threads
		event.Attachments = append(event.Attachments, &sentry.Attachment{
			Filename:    "stacktrace.txt",
			ContentType: "text/plain",
			Payload:     stacktrace,
		})
	}

	// Let Sentry use its default stack trace-based fingerprinting
	// which is typically more effective for grouping similar errors

	// But let's give it some more hints
	event.Fingerprint = []string{
		"{{ default }}",
		"level: " + getLevelString(level),
	}

	return event
}

// createSentryEventWithContext creates a Sentry event with additional context data.
func createSentryEventWithContext(level sentry.Level, err error, context map[string]interface{}) *sentry.Event {
	event := createSentryEvent(level, err)

	// Add context data as tags and extra data for better grouping and filtering
	if context != nil {
		// Initialize tags map if needed
		if event.Tags == nil {
			event.Tags = make(map[string]string)
		}

		// Add all context values as tags for easy filtering in Sentry
		for key, value := range context {
			// Convert the value to string for tags
			switch convertedValue := value.(type) {
			case string:
				event.Tags[key] = convertedValue
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
				event.Tags[key] = convertToString(convertedValue)
			default:
				// For complex types, add them to the extra data instead
				if event.Extra == nil {
					event.Extra = make(map[string]interface{})
				}

				event.Extra[key] = convertedValue
			}

			// If the tag is called operation, add it to the fingerprint
			if key == "operation" {
				valueStr := fmt.Sprintf("operation: %v", value)
				event.Fingerprint = append(event.Fingerprint, valueStr)
			}

			if key == "fsm_type" {
				valueStr := fmt.Sprintf("fsm_type: %v", value)
				event.Fingerprint = append(event.Fingerprint, valueStr)
			}

			if key == "service_type" {
				valueStr := fmt.Sprintf("service_type: %v", value)
				event.Fingerprint = append(event.Fingerprint, valueStr)
			}
		}
	}

	return event
}

// Helper function to convert sentry.Level to string.
func getLevelString(level sentry.Level) string {
	switch level {
	case sentry.LevelDebug:
		return "debug"
	case sentry.LevelInfo:
		return "info"
	case sentry.LevelWarning:
		return "warning"
	case sentry.LevelError:
		return "error"
	case sentry.LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// Helper function to convert simple values to string for tags.
func convertToString(v interface{}) string {
	return fmt.Sprintf("%v", v)
}

// Helper function to send an event to Sentry.
func sendSentryEvent(event *sentry.Event) {
	localHub := sentry.CurrentHub().Clone()
	localHub.CaptureEvent(event)
}
