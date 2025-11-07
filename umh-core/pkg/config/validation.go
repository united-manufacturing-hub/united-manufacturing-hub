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

import "errors"

// ValidateComponentName validates that a component name contains only valid characters
// and is not empty. Valid characters are lowercase letters (a-z), numbers (0-9), underscores (_) and hyphens (-).
func ValidateComponentName(name string) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}

	if name[0] == '-' || name[0] == '_' || name[len(name)-1] == '-' || name[len(name)-1] == '_' {
		return errors.New("name has to start and end with a letter or number")
	}

	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return errors.New("only lowercase letters, numbers, dashes and underscores are allowed")
		}
	}

	return nil
}
