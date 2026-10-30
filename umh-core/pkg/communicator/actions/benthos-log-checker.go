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
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// benthosConfigErrorPatterns match the lines benthos logs when it rejects its
// config before it starts. Such an error repeats on every restart until the
// config changes, so an action can fail at once instead of waiting for the timeout.
var benthosConfigErrorPatterns = []string{
	"unable to infer",
	"Config lint error",
}

// findBenthosConfigError returns the distinct benthos config errors logged at or
// after since, joined with "; ". Benthos logs one line per lint error before it
// exits, so a config with several errors reports all of them at once. Older lines
// are skipped: a stopped flow keeps the lines of its previous config, which say
// nothing about the config being rolled out.
func findBenthosConfigError(logs []s6.LogEntry, since time.Time) (string, bool) {
	var messages []string

	for _, log := range logs {
		if log.Timestamp.Before(since) {
			continue
		}

		if !isBenthosConfigErrorLine(log.Content) {
			continue
		}

		message := benthosConfigErrorText(log.Content)
		if slices.Contains(messages, message) {
			continue
		}

		messages = append(messages, message)
	}

	if len(messages) == 0 {
		return "", false
	}

	return strings.Join(messages, "; "), true
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

// benthosConfigErrorText returns the part of a config error line that names the
// problem: the lint field without its config location, else the msg field, else
// the whole line.
func benthosConfigErrorText(content string) string {
	lint, hasLint := quotedFieldValue(content, "lint")
	if hasLint {
		return withoutConfigLocation(lint)
	}

	message, hasMessage := quotedFieldValue(content, "msg")
	if hasMessage {
		return message
	}

	return content
}

// quotedFieldValue returns the value of a key="value" field in a logfmt line
// (https://brandur.org/logfmt), the format benthos logs in.
func quotedFieldValue(content string, key string) (string, bool) {
	_, afterKey, hasKey := strings.Cut(content, " "+key+"=")
	if !hasKey {
		return "", false
	}

	quotedValue, quoteErr := strconv.QuotedPrefix(afterKey)
	if quoteErr != nil {
		return "", false
	}

	value, unquoteErr := strconv.Unquote(quotedValue)

	return value, unquoteErr == nil
}

// configLocationPrefix matches the "path(line,col) " prefix benthos puts before a
// lint message. It names a line of the rendered benthos config, which the user did not write.
var configLocationPrefix = regexp.MustCompile(`^\S+\(\d+,\d+\) `)

func withoutConfigLocation(lint string) string {
	return configLocationPrefix.ReplaceAllString(lint, "")
}

type flowConfigError struct {
	message string
	flows   DFCType
}

func flowBenthosLogs(flow dataflowcomponent.DataflowComponentObservedState) []s6.LogEntry {
	return flow.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs
}

// findFlowConfigError looks for a config error in the logs of a bridge's read
// and write flow. When both flows logged one, the message names both.
func findFlowConfigError(readLogs []s6.LogEntry, writeLogs []s6.LogEntry, since time.Time) (flowConfigError, bool) {
	readMessage, readHasError := findBenthosConfigError(readLogs, since)
	writeMessage, writeHasError := findBenthosConfigError(writeLogs, since)
	flows := dfcTypeFromPresence(readHasError, writeHasError)

	if readHasError && writeHasError {
		message := fmt.Sprintf("read flow: %s; write flow: %s", readMessage, writeMessage)

		return flowConfigError{message: message, flows: flows}, true
	}

	if readHasError {
		return flowConfigError{message: readMessage, flows: flows}, true
	}

	if writeHasError {
		return flowConfigError{message: writeMessage, flows: flows}, true
	}

	return flowConfigError{}, false
}
