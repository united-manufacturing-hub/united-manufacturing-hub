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
	"encoding/json"
	"errors"
	"fmt"
)

// WorkerTypeChecker interface for registry lookup (allows testing without full registry).
type WorkerTypeChecker interface {
	ListRegisteredTypes() []string
}

// ValidateChildSpec validates a ChildSpec for common errors.
func ValidateChildSpec(spec ChildSpec, registry WorkerTypeChecker) error {
	if spec.Name == "" {
		return errors.New("child spec name cannot be empty")
	}

	if spec.WorkerType == "" {
		return fmt.Errorf("child spec %q: worker type cannot be empty", spec.Name)
	}

	validTypes := registry.ListRegisteredTypes()
	found := false

	for _, t := range validTypes {
		if t == spec.WorkerType {
			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf("child spec %q: unknown worker type %q (available: %v)",
			spec.Name, spec.WorkerType, validTypes)
	}

	if _, err := json.Marshal(spec.UserSpec); err != nil {
		return fmt.Errorf("child spec %q: invalid user spec: %w", spec.Name, err)
	}

	// Validate ChildStartStates
	if len(spec.ChildStartStates) > 0 {
		seenStates := make(map[string]bool)

		for i, state := range spec.ChildStartStates {
			if state == "" {
				return fmt.Errorf("child spec %q: ChildStartStates[%d] cannot be empty", spec.Name, i)
			}

			if seenStates[state] {
				return fmt.Errorf("child spec %q: duplicate state %q in ChildStartStates", spec.Name, state)
			}

			seenStates[state] = true
		}
	}

	return nil
}

// ValidateChildSpecs validates a slice of ChildSpecs (checks uniqueness too).
func ValidateChildSpecs(specs []ChildSpec, registry WorkerTypeChecker) error {
	names := make(map[string]bool)

	for i, spec := range specs {
		if err := ValidateChildSpec(spec, registry); err != nil {
			return fmt.Errorf("child spec [%d]: %w", i, err)
		}

		if names[spec.Name] {
			return fmt.Errorf("duplicate child spec name %q", spec.Name)
		}

		names[spec.Name] = true
	}

	return nil
}
