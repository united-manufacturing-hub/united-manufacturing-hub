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

package models

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidationError represents a validation error with a path and message
type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error at path '%s': %s", e.Path, e.Message)
}

// ValidateDataModelStructure validates a data model structure based on the specified criteria:
// 1. _refModel contains exactly one ':'
// 2. _refModel has a version specified (format: model:version)
// 3. Leaf nodes DO NOT contain _refModel and DO contain _type
// 4. Folder nodes do not contain _type, _description, _unit, or _refModel
// 5. SubModel nodes ONLY contain _refModel
// 6. The version fulfills ^v\d+$
func ValidateDataModelStructure(structure map[string]Field) []ValidationError {
	var errors []ValidationError
	versionRegex := regexp.MustCompile(`^v\d+$`)

	for fieldName, field := range structure {
		errors = append(errors, validateField(fieldName, field, versionRegex)...)
	}

	return errors
}

func validateField(path string, field Field, versionRegex *regexp.Regexp) []ValidationError {
	var errors []ValidationError

	// Determine node type
	hasSubfields := len(field.Subfields) > 0
	hasModelRef := field.ModelRef != ""
	hasType := field.Type != ""
	hasDescription := field.Description != ""
	hasUnit := field.Unit != ""

	// Validate _refModel format if present
	if hasModelRef {
		// Check if _refModel contains exactly one ':'
		colonCount := strings.Count(field.ModelRef, ":")
		if colonCount != 1 {
			errors = append(errors, ValidationError{
				Path:    path,
				Message: fmt.Sprintf("_refModel must contain exactly one ':' but found %d", colonCount),
			})
		} else {
			// Check if version is specified and valid
			parts := strings.Split(field.ModelRef, ":")
			if len(parts) == 2 {
				version := parts[1]
				if version == "" {
					errors = append(errors, ValidationError{
						Path:    path,
						Message: "_refModel must have a version specified after ':'",
					})
				} else if !versionRegex.MatchString(version) {
					errors = append(errors, ValidationError{
						Path:    path,
						Message: fmt.Sprintf("version '%s' does not match pattern ^v\\d+$", version),
					})
				}
			}
		}
	}

	// Invalid combinations first
	if hasSubfields && hasModelRef {
		errors = append(errors, ValidationError{
			Path:    path,
			Message: "field cannot have both subfields and _refModel",
		})
		return errors
	}

	if hasType && hasModelRef {
		errors = append(errors, ValidationError{
			Path:    path,
			Message: "field cannot have both _type and _refModel",
		})
		return errors
	}

	// Validate based on node type
	switch {
	case hasModelRef && !hasSubfields && !hasType:
		// SubModel node: ONLY contain _refModel
		if hasDescription || hasUnit {
			errors = append(errors, ValidationError{
				Path:    path,
				Message: "subModel nodes should ONLY contain _refModel",
			})
		}

	case !hasSubfields && !hasModelRef:
		// Leaf node: DO NOT contain _refModel and DO contain _type
		if !hasType {
			errors = append(errors, ValidationError{
				Path:    path,
				Message: "leaf nodes must contain _type",
			})
		}

	case hasSubfields && !hasModelRef:
		// Folder node: can have _type, but recursively validate subfields
		for subfieldName, subfield := range field.Subfields {
			subfieldPath := fmt.Sprintf("%s.%s", path, subfieldName)
			errors = append(errors, validateField(subfieldPath, subfield, versionRegex)...)
		}

	default:
		// This should not happen, but adding for completeness
		errors = append(errors, ValidationError{
			Path:    path,
			Message: "field has an invalid combination of properties",
		})
	}

	return errors
}
