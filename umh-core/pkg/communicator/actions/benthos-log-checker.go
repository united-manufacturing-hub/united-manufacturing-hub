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

package actions

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var benthosConfigErrorPatterns = []string{
	"unable to infer",
	"Config lint error",
}

func findBenthosConfigError(logs []s6.LogEntry, since time.Time) (string, bool) {
	for _, log := range logs {
		if log.Timestamp.Before(since) {
			continue
		}

		if isBenthosConfigErrorLine(log.Content) {
			return benthosConfigErrorText(log.Content), true
		}
	}

	return "", false
}

func isBenthosConfigErrorLine(content string) bool {
	lowerContent := strings.ToLower(content)
	if !strings.Contains(lowerContent, "error") {
		return false
	}

	for _, errLine := range benthosConfigErrorPatterns {
		if strings.Contains(lowerContent, strings.ToLower(errLine)) {
			return true
		}
	}

	return false
}

func benthosConfigErrorText(content string) string {
	lint, hasLint := lintValue(content)
	if !hasLint {
		return content
	}

	return withoutConfigLocation(lint)
}

func lintValue(content string) (string, bool) {
	_, afterLintKey, hasLintKey := strings.Cut(content, ` lint=`)
	if !hasLintKey {
		return "", false
	}

	quotedLint, quoteErr := strconv.QuotedPrefix(afterLintKey)
	if quoteErr != nil {
		return "", false
	}

	lint, unquoteErr := strconv.Unquote(quotedLint)

	return lint, unquoteErr == nil
}

var configLocationPrefix = regexp.MustCompile(`^\S+\(\d+,\d+\) `)

func withoutConfigLocation(lint string) string {
	return configLocationPrefix.ReplaceAllString(lint, "")
}

type flowSelection struct {
	read  bool
	write bool
}

type flowConfigError struct {
	message string
	flows   flowSelection
}

func flowBenthosLogs(flow dataflowcomponent.DataflowComponentObservedState) []s6.LogEntry {
	return flow.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs
}

func findFlowConfigError(readLogs []s6.LogEntry, writeLogs []s6.LogEntry, since time.Time) (flowConfigError, bool) {
	readMessage, readHasError := findBenthosConfigError(readLogs, since)
	writeMessage, writeHasError := findBenthosConfigError(writeLogs, since)
	flows := flowSelection{read: readHasError, write: writeHasError}

	if readHasError {
		return flowConfigError{message: readMessage, flows: flows}, true
	}

	if writeHasError {
		return flowConfigError{message: writeMessage, flows: flows}, true
	}

	return flowConfigError{}, false
}
