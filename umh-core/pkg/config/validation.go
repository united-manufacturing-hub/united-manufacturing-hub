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

// ValidateLocation validates that a location hierarchy is well-formed.
//
// It enforces:
// - Required level 0
// - No gaps in the location hierarchy (consecutive levels starting from 0)
// - Non-empty values for all levels
func ValidateLocation(location map[int]string) error {
	if len(location) == 0 {
		return errors.New("location cannot be empty")
	}

	// Level 0 is required
	level0Value, exists := location[0]
	if !exists {
		return errors.New("level 0 is required")
	}
	if level0Value == "" {
		return errors.New("level 0 cannot be empty")
	}

	// Check for consecutive levels starting from 0
	for level := 0; level < len(location); level++ {
		value, exists := location[level]
		if !exists {
			return fmt.Errorf("location hierarchy has a gap at level %d (expected consecutive levels starting from 0)", level)
		}
		if value == "" {
			return fmt.Errorf("location level %d cannot be empty", level)
		}
	}

	return nil
}
