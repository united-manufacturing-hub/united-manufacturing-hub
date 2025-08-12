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
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
)

var ErrBenthosLogLines = []string{
	"unable to infer",
	"Config lint error",
}

// CheckBenthosLogLinesForConfigErrors examines a slice of S6 log entries for
// Benthos configuration errors. It performs case-insensitive checks for known
// error patterns combined with the word "error". Returns true if any errors
// are detected, false otherwise.
// This function is used to detect fatal configuration errors that would cause
// Benthos to enter a CrashLoop. When such errors are detected, we can immediately
// abort the startup process rather than waiting for the full timeout period,
// as these errors require configuration changes to resolve.
func CheckBenthosLogLinesForConfigErrors(logs []s6_shared.LogEntry) bool {
	for _, log := range logs {
		logContent := strings.ToLower(log.Content)
		if !strings.Contains(logContent, "error") {
			continue
		}

		for _, errLine := range ErrBenthosLogLines {
			if strings.Contains(logContent, strings.ToLower(errLine)) {
				return true
			}
		}
	}
	return false
}
