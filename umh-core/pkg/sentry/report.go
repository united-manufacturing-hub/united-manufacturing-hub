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
