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

// ReportIssueWithContext reports an issue with additional context data that will be included in Sentry.
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

// ReportIssuefWithContext formats an error message and reports it with additional context data.
func ReportIssuefWithContext(issueType IssueType, log *zap.SugaredLogger, context map[string]interface{}, template string, args ...interface{}) {
	ReportIssueWithContext(fmt.Errorf(template, args...), issueType, log, context)
}

// Helper functions for common error patterns

// ReportFSMError reports an FSM-related error with proper context.
func ReportFSMError(log *zap.SugaredLogger, instanceID string, fsmType string, operation string, err error) {
	context := map[string]interface{}{
		"instance_id": instanceID,
		"fsm_type":    fsmType,
		"operation":   operation,
	}
	ReportIssueWithContext(err, IssueTypeError, log, context)
}

// ReportFSMFatal reports an FSM-related fatal error with proper context.
func ReportFSMFatal(log *zap.SugaredLogger, instanceID string, fsmType string, operation string, err error) {
	context := map[string]interface{}{
		"instance_id": instanceID,
		"fsm_type":    fsmType,
		"operation":   operation,
	}
	ReportIssueWithContext(err, IssueTypeFatal, log, context)
}

// ReportFSMErrorf formats an FSM-related error message and reports it with proper context.
func ReportFSMErrorf(log *zap.SugaredLogger, instanceID string, fsmType string, operation string, template string, args ...interface{}) {
	context := map[string]interface{}{
		"instance_id": instanceID,
		"fsm_type":    fsmType,
		"operation":   operation,
	}
	ReportIssuefWithContext(IssueTypeError, log, context, template, args...)
}

// ReportServiceError reports a service-related error with proper context.
func ReportServiceError(log *zap.SugaredLogger, serviceID string, serviceType string, operation string, err error) {
	context := map[string]interface{}{
		"service_id":   serviceID,
		"service_type": serviceType,
		"operation":    operation,
	}
	ReportIssueWithContext(err, IssueTypeError, log, context)
}

// ReportServiceErrorf formats a service-related error message and reports it with proper context.
func ReportServiceErrorf(log *zap.SugaredLogger, serviceID string, serviceType string, operation string, template string, args ...interface{}) {
	context := map[string]interface{}{
		"service_id":   serviceID,
		"service_type": serviceType,
		"operation":    operation,
	}
	ReportIssuefWithContext(IssueTypeError, log, context, template, args...)
}
