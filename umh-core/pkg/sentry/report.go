package sentry

import (
	"fmt"
)

type IssueType string

const (
	IssueTypeWarning IssueType = "warning"
	IssueTypeError   IssueType = "error"
	IssueTypeFatal   IssueType = "fatal"
)

func ReportIssue(err error, issueType IssueType) {
	switch issueType {
	case IssueTypeFatal:
		reportFatal(err.Error())
	case IssueTypeError:
		reportError(err.Error())
	case IssueTypeWarning:
		reportWarning(err.Error())
	}
}

func ReportIssuef(err error, issueType IssueType, template string, args ...interface{}) {
	ReportIssue(fmt.Errorf(template, args...), issueType)
}
