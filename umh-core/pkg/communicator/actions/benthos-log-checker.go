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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var ErrBenthosLogLines = []string{
	"unable to infer",
	"Config lint error",
}

func FindBenthosConfigError(logs []s6.LogEntry) (string, bool) {
	for _, log := range logs {
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

	for _, errLine := range ErrBenthosLogLines {
		if strings.Contains(lowerContent, strings.ToLower(errLine)) {
			return true
		}
	}

	return false
}

var lintLocationPrefix = regexp.MustCompile(`^\S+\(\d+,\d+\) `)

func benthosConfigErrorText(content string) string {
	_, afterLintKey, hasLintKey := strings.Cut(content, ` lint=`)
	if !hasLintKey {
		return content
	}

	quotedLint, quoteErr := strconv.QuotedPrefix(afterLintKey)
	if quoteErr != nil {
		return content
	}

	lint, unquoteErr := strconv.Unquote(quotedLint)
	if unquoteErr != nil {
		return content
	}

	return lintLocationPrefix.ReplaceAllString(lint, "")
}
