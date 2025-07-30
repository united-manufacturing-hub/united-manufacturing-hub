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
	"context"
	"fmt"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"go.uber.org/zap"
)

type IssueType string

const (
	IssueTypeWarning IssueType = "warning"
	IssueTypeError   IssueType = "error"
	IssueTypeFatal   IssueType = "fatal"
)

func ReportIssue(err error, issueType IssueType, log *zap.SugaredLogger) {
	if log == nil {
		// If logger initialization failed somehow, create a no-op logger to avoid nil panics
		log = zap.NewNop().Sugar()
	}
	switch issueType {
	case IssueTypeFatal:
		reportFatal(err, log)
	case IssueTypeError:
		reportError(err, log)
	case IssueTypeWarning:
		reportWarning(err, log)
	}
}

func ReportIssuef(issueType IssueType, log *zap.SugaredLogger, template string, args ...interface{}) {
	ReportIssue(fmt.Errorf(template, args...), issueType, log)
}

// ReportIssueWithContext reports an issue with additional context data that will be included in Sentry
func ReportIssueWithContext(err error, issueType IssueType, log *zap.SugaredLogger, context map[string]interface{}) {
	if log == nil {
		// If logger initialization failed somehow, create a no-op logger to avoid nil panics
		log = zap.NewNop().Sugar()
	}
	switch issueType {
	case IssueTypeFatal:
		reportFatalWithContext(err, log, context)
	case IssueTypeError:
		reportErrorWithContext(err, log, context)
	case IssueTypeWarning:
		reportWarningWithContext(err, log, context)
	}
}

// ReportIssuefWithContext formats an error message and reports it with additional context data
func ReportIssuefWithContext(issueType IssueType, log *zap.SugaredLogger, context map[string]interface{}, template string, args ...interface{}) {
	ReportIssueWithContext(fmt.Errorf(template, args...), issueType, log, context)
}

// Helper functions for common error patterns

// ReportFSMError reports an FSM-related error with proper context
func ReportFSMError(log *zap.SugaredLogger, instanceID string, fsmType string, operation string, err error) {
	context := map[string]interface{}{
		"instance_id": instanceID,
		"fsm_type":    fsmType,
		"operation":   operation,
	}
	ReportIssueWithContext(err, IssueTypeError, log, context)
}

// ReportFSMFatal reports an FSM-related fatal error with proper context
func ReportFSMFatal(log *zap.SugaredLogger, instanceID string, fsmType string, operation string, err error) {
	context := map[string]interface{}{
		"instance_id": instanceID,
		"fsm_type":    fsmType,
		"operation":   operation,
	}
	ReportIssueWithContext(err, IssueTypeFatal, log, context)
}

// ReportFSMErrorf formats an FSM-related error message and reports it with proper context
func ReportFSMErrorf(log *zap.SugaredLogger, instanceID string, fsmType string, operation string, template string, args ...interface{}) {
	context := map[string]interface{}{
		"instance_id": instanceID,
		"fsm_type":    fsmType,
		"operation":   operation,
	}
	ReportIssuefWithContext(IssueTypeError, log, context, template, args...)
}

// ReportServiceError reports a service-related error with proper context
func ReportServiceError(log *zap.SugaredLogger, serviceID string, serviceType string, operation string, err error) {
	context := map[string]interface{}{
		"service_id":   serviceID,
		"service_type": serviceType,
		"operation":    operation,
	}
	ReportIssueWithContext(err, IssueTypeError, log, context)
}

// ReportServiceErrorf formats a service-related error message and reports it with proper context
func ReportServiceErrorf(log *zap.SugaredLogger, serviceID string, serviceType string, operation string, template string, args ...interface{}) {
	context := map[string]interface{}{
		"service_id":   serviceID,
		"service_type": serviceType,
		"operation":    operation,
	}
	ReportIssuefWithContext(IssueTypeError, log, context, template, args...)
}

// RecoverAndReport captures panics and sends them to Sentry, then continues execution
// Use it with defer: defer sentry.RecoverAndReport()
func RecoverAndReport() {
	if r := recover(); r != nil {
		// Capture the panic with Sentry including stack trace
		eventID := sentry.CaptureException(fmt.Errorf("recovered panic: %v", r))
		if eventID != nil {
			sentry.Flush(time.Second * 2)
		}
		// Log locally as well
		if log := logger.For(logger.ComponentCore); log != nil {
			log.Errorf("Recovered from panic: %v", r)
		}
		// Continue execution - don't re-panic
	}
}

// RecoverReportAndRePanic captures panics, sends them to Sentry, then re-panics
// Use when you want to log the panic but still crash the program
func RecoverReportAndRePanic() {
	if r := recover(); r != nil {
		// Get a logger for better structured reporting
		log := logger.For(logger.ComponentCore)
		if log == nil {
			log = logger.For("panic_recovery")
		}

		// Report with context for better Sentry grouping
		ReportIssueWithContext(
			fmt.Errorf("panic: %v", r),
			IssueTypeFatal,
			log,
			map[string]interface{}{
				"panic_value": fmt.Sprintf("%v", r),
				"location":    "goroutine",
			},
		)

		// Give Sentry time to send the event
		sentry.Flush(time.Second * 2)
		// Re-panic to maintain normal Go panic behavior
		panic(r)
	}
}

// RecoverReportAndRePanicWithContext captures panics with additional context
func RecoverReportAndRePanicWithContext(contextData map[string]interface{}) {
	if r := recover(); r != nil {
		// Get a logger for structured reporting
		log := logger.For(logger.ComponentCore)
		if log == nil {
			log = logger.For("panic_recovery")
		}

		// Merge provided context with panic info
		fullContext := make(map[string]interface{})
		for k, v := range contextData {
			fullContext[k] = v
		}
		fullContext["panic_value"] = fmt.Sprintf("%v", r)

		// Report with context
		ReportIssueWithContext(
			fmt.Errorf("panic: %v", r),
			IssueTypeFatal,
			log,
			fullContext,
		)

		// Give Sentry time to send the event
		sentry.Flush(time.Second * 2)
		// Re-panic to maintain normal Go panic behavior
		panic(r)
	}
}

// Recover wraps sentry-go's Recover function for use in main function panic recovery
// Returns the event ID if a panic was captured, nil otherwise
func Recover() *sentry.EventID {
	return sentry.Recover()
}

// Flush wraps sentry-go's Flush function to ensure events are sent before program exit
func Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}

// GlobalPanicRecovery sets up global panic recovery for main()
// Use with: defer sentry.GlobalPanicRecovery(log)()
func GlobalPanicRecovery(log *zap.SugaredLogger) func() {
	return func() {
		if r := recover(); r != nil {
			// Log the panic locally first
			if log != nil {
				log.Errorf("GLOBAL PANIC RECOVERED: %v", r)
			} else {
				fmt.Fprintf(os.Stderr, "GLOBAL PANIC RECOVERED: %v\n", r)
			}

			// Capture the panic with Sentry using our custom function
			ReportIssueWithContext(
				fmt.Errorf("panic: %v", r),
				IssueTypeFatal,
				log,
				map[string]interface{}{
					"panic_value": fmt.Sprintf("%v", r),
					"location":    "main_goroutine",
				},
			)

			// Give Sentry time to send the event before the program exits
			Flush(time.Second * 5)

			// Re-panic to maintain normal crash behavior
			panic(r)
		}
	}
}

// HandleGlobalPanic is a simpler alternative - call directly with defer
// Use with: defer sentry.HandleGlobalPanic(log)
func HandleGlobalPanic(log *zap.SugaredLogger) {
	if r := recover(); r != nil {
		// Log the panic locally first
		if log != nil {
			log.Errorf("GLOBAL PANIC RECOVERED: %v", r)
		} else {
			fmt.Fprintf(os.Stderr, "GLOBAL PANIC RECOVERED: %v\n", r)
		}

		// Capture the panic with Sentry using our custom function
		ReportIssueWithContext(
			fmt.Errorf("panic: %v", r),
			IssueTypeFatal,
			log,
			map[string]interface{}{
				"panic_value": fmt.Sprintf("%v", r),
				"location":    "main_goroutine",
			},
		)

		// Give Sentry time to send the event before the program exits
		Flush(time.Second * 5)

		// Re-panic to maintain normal crash behavior
		panic(r)
	}
}

// SafeGo launches a goroutine with automatic panic recovery
// Use this instead of 'go' throughout the codebase for safer goroutine execution
func SafeGo(fn func()) {
	go func() {
		defer RecoverAndReport()
		fn()
	}()
}

// SafeGoWithContext launches a goroutine with automatic panic recovery and context support
// The function will be called with the provided context
func SafeGoWithContext(ctx context.Context, fn func(context.Context)) {
	go func() {
		defer RecoverAndReport()
		fn(ctx)
	}()
}
