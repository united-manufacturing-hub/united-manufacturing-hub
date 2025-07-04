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

package datamodel

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

// ValidationError represents a validation error with a path and message
type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error at path '%s': %s", e.Path, e.Message)
}

// ValidateDataModel validates a data model version
// It applies the following rules:
// A node can either be a leaf node or a non-leaf node.
// A leaf node must have a either:
// - _type (and optionally a _description and _unit)
// - _refModel
// A non-leaf node must not have a _type, _description or _unit
// Additional validation for _refModel format and combinations
func (v *Validator) ValidateDataModel(ctx context.Context, dataModel config.DataModelVersion) error {
	var errors []ValidationError
	v.validateStructure(dataModel.Structure, "", &errors)

	if len(errors) > 0 {
		// Build error message with all validation errors
		errorMsg := "data model structure validation failed:"
		for _, validationError := range errors {
			errorMsg += fmt.Sprintf("\n  - %s", validationError.Error())
		}
		return fmt.Errorf("%s", errorMsg)
	}

	return nil
}

// validateStructure recursively validates the structure of a data model
func (v *Validator) validateStructure(structure map[string]config.Field, path string, errors *[]ValidationError) {
	for fieldName, field := range structure {
		// Validate field name first
		v.validateFieldName(fieldName, path, errors)

		currentPath := fieldName
		if path != "" {
			currentPath = path + "." + fieldName
		}

		v.validateField(field, currentPath, errors)
	}
}

// validateFieldName validates field names according to naming rules
func (v *Validator) validateFieldName(fieldName string, path string, errors *[]ValidationError) {
	currentPath := fieldName
	if path != "" {
		currentPath = path + "." + fieldName
	}

	// Check for empty field names
	if fieldName == "" {
		*errors = append(*errors, ValidationError{
			Path:    currentPath,
			Message: "field name cannot be empty",
		})
		return
	}

	// Check for dots in field names
	if strings.Contains(fieldName, ".") {
		*errors = append(*errors, ValidationError{
			Path:    currentPath,
			Message: "field name cannot contain dots",
		})
	}

	// Check for valid characters: letters, numbers, dashes, and underscores only
	for _, r := range fieldName {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			*errors = append(*errors, ValidationError{
				Path:    currentPath,
				Message: "field name can only contain letters, numbers, dashes, and underscores",
			})
			break
		}
	}
}

// validateField validates a single field according to the rules
func (v *Validator) validateField(field config.Field, path string, errors *[]ValidationError) {
	// First, validate _refModel format if present
	if field.ModelRef != "" {
		v.validateRefModelFormat(field.ModelRef, path, errors)
	}

	// Check for invalid combinations
	v.validateFieldCombinations(field, path, errors)

	isLeaf := v.isLeafNode(field)

	if isLeaf {
		v.validateLeafNode(field, path, errors)
	} else {
		v.validateNonLeafNode(field, path, errors)
	}
}

// validateRefModelFormat validates the _refModel field format
func (v *Validator) validateRefModelFormat(modelRef string, path string, errors *[]ValidationError) {
	// Check if _refModel contains exactly one ':'
	colonCount := strings.Count(modelRef, ":")
	if colonCount != 1 {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: fmt.Sprintf("_refModel must contain exactly one ':' but found %d", colonCount),
		})
		return
	}

	// Check if version is specified and valid
	parts := strings.Split(modelRef, ":")
	if len(parts) == 2 {
		modelName := parts[0]
		version := parts[1]

		// Check for empty model name
		if modelName == "" {
			*errors = append(*errors, ValidationError{
				Path:    path,
				Message: "_refModel must have a model name specified before ':'",
			})
		}

		// Check for empty version
		if version == "" {
			*errors = append(*errors, ValidationError{
				Path:    path,
				Message: "_refModel must have a version specified after ':'",
			})
		} else {
			// Validate version pattern ^v\d+$
			versionRegex := regexp.MustCompile(`^v\d+$`)
			if !versionRegex.MatchString(version) {
				*errors = append(*errors, ValidationError{
					Path:    path,
					Message: fmt.Sprintf("version '%s' does not match pattern ^v\\d+$", version),
				})
			} else {
				// Check that version starts at v1, not v0
				if version == "v0" {
					*errors = append(*errors, ValidationError{
						Path:    path,
						Message: "version must start at v1, v0 is not allowed",
					})
				}
			}
		}
	}
}

// validateFieldCombinations validates invalid field combinations
func (v *Validator) validateFieldCombinations(field config.Field, path string, errors *[]ValidationError) {
	hasType := field.Type != ""
	hasRefModel := field.ModelRef != ""
	hasSubfields := len(field.Subfields) > 0

	// Cannot have both _type and _refModel
	if hasType && hasRefModel {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "field cannot have both _type and _refModel",
		})
	}

	// Cannot have both subfields and _refModel
	if hasSubfields && hasRefModel {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "field cannot have both subfields and _refModel",
		})
	}
}

// isLeafNode determines if a field is a leaf node (has no subfields)
// A field is considered a leaf node only if subfields is nil
// An empty subfields map (len = 0 but not nil) is considered a non-leaf node
func (v *Validator) isLeafNode(field config.Field) bool {
	return field.Subfields == nil
}

// validateLeafNode validates a leaf node
func (v *Validator) validateLeafNode(field config.Field, path string, errors *[]ValidationError) {
	hasType := field.Type != ""
	hasRefModel := field.ModelRef != ""
	hasDescription := field.Description != ""
	hasUnit := field.Unit != ""

	// Determine leaf node type
	if hasRefModel && !hasType {
		// SubModel node: ONLY contain _refModel
		if hasDescription || hasUnit {
			*errors = append(*errors, ValidationError{
				Path:    path,
				Message: "subModel nodes should ONLY contain _refModel",
			})
		}
		return
	}

	if hasType && !hasRefModel {
		// Regular leaf node with _type
		return
	}

	// If neither _type nor _refModel is present
	if !hasType && !hasRefModel {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "leaf nodes must contain _type",
		})
	}
}

// validateNonLeafNode validates a non-leaf node
func (v *Validator) validateNonLeafNode(field config.Field, path string, errors *[]ValidationError) {
	// For non-leaf nodes, recursively validate subfields
	v.validateStructure(field.Subfields, path, errors)
}
