// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
