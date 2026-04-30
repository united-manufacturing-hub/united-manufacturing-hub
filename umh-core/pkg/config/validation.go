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

package config

import (
	"errors"
	"fmt"
	"strings"
)

// ValidateComponentName validates that a component name contains only valid characters
// and is not empty. Valid characters are letters (a-z, A-Z), numbers (0-9), underscores (_) and hyphens (-).
func ValidateComponentName(name string) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}

	if name[0] == '-' || name[0] == '_' || name[len(name)-1] == '-' || name[len(name)-1] == '_' {
		return errors.New("name has to start and end with a letter or number")
	}

	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return errors.New("only letters, numbers, dashes and underscores are allowed")
		}
	}

	return nil
}

// FormatConfigValidationMessage projects validation issues into the
// agent health message string. Pure function. Returns empty when no issues.
func FormatConfigValidationMessage(issues []ConfigValidationIssue) string {
	if len(issues) == 0 {
		return ""
	}
	if len(issues) == 1 {
		i := issues[0]
		msg := fmt.Sprintf("Invalid %s value %q", i.Field, i.OffendingValue)
		if len(i.AllowedValues) > 0 {
			msg += ". Allowed: " + strings.Join(i.AllowedValues, ", ")
		}
		return msg + "."
	}
	parts := make([]string, len(issues))
	for n, i := range issues {
		parts[n] = fmt.Sprintf("%s=%q", i.Field, i.OffendingValue)
	}
	return fmt.Sprintf("%d configuration issues: %s", len(issues), strings.Join(parts, "; "))
}
