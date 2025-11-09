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

package types

import (
	"encoding/json"
	"fmt"
)

// WorkerTypeChecker interface for registry lookup (allows testing without full registry)
type WorkerTypeChecker interface {
	ListRegisteredTypes() []string
}

// ValidateChildSpec validates a ChildSpec for common errors
func ValidateChildSpec(spec ChildSpec, registry WorkerTypeChecker) error {
	// Check Name (required for reconciliation)
	if spec.Name == "" {
		return fmt.Errorf("child spec name cannot be empty")
	}

	// Check WorkerType (required, must exist in registry)
	if spec.WorkerType == "" {
		return fmt.Errorf("child spec %q: worker type cannot be empty", spec.Name)
	}

	// Verify WorkerType exists in registry
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

	// Check UserSpec can be marshaled (basic validation)
	if _, err := json.Marshal(spec.UserSpec); err != nil {
		return fmt.Errorf("child spec %q: invalid user spec: %w", spec.Name, err)
	}

	return nil
}

// ValidateChildSpecs validates a slice of ChildSpecs (checks uniqueness too)
func ValidateChildSpecs(specs []ChildSpec, registry WorkerTypeChecker) error {
	names := make(map[string]bool)

	for i, spec := range specs {
		// Validate individual spec
		if err := ValidateChildSpec(spec, registry); err != nil {
			return fmt.Errorf("child spec [%d]: %w", i, err)
		}

		// Check for duplicate names
		if names[spec.Name] {
			return fmt.Errorf("duplicate child spec name %q", spec.Name)
		}
		names[spec.Name] = true
	}

	return nil
}
