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

// ValidateStructureOnly validates a data model's structure without checking references
// It applies the following rules:
// - Field names can only contain letters, numbers, dashes, and underscores
// - A node can either be a leaf node or a non-leaf node
// - A leaf node must have either _type or _refModel (but not both)
// - A non-leaf node (folder) can only have subfields, no _type, _description, or _unit
// - _refModel format and version validation
func (v *Validator) ValidateStructureOnly(ctx context.Context, dataModel config.DataModelVersion) error {
	return v.validateDataModel(ctx, dataModel)
}

// ValidateWithReferences validates a data model and all its references
// It first validates the structure, then checks for circular references and limits recursion depth to 10 levels
// Parameters:
// - ctx: context for cancellation
// - dataModel: the data model to validate
// - allDataModels: map of all available data models for reference resolution
// Returns error if validation fails or circular references are detected
func (v *Validator) ValidateWithReferences(ctx context.Context, dataModel config.DataModelVersion, allDataModels map[string]config.DataModelsConfig) error {
	// First validate the data model structure itself
	if err := v.ValidateStructureOnly(ctx, dataModel); err != nil {
		return err
	}

	// Then validate all references
	visitedModels := make(map[string]bool)
	return v.validateReferences(ctx, dataModel, allDataModels, visitedModels, 0)
}

// validateDataModel validates a data model version (private method)
func (v *Validator) validateDataModel(ctx context.Context, dataModel config.DataModelVersion) error {
	// Check if context is cancelled before starting validation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	var errors []ValidationError
	v.validateStructure(ctx, dataModel.Structure, "", &errors)

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
func (v *Validator) validateStructure(ctx context.Context, structure map[string]config.Field, path string, errors *[]ValidationError) {
	// Check if context is cancelled before processing this level
	select {
	case <-ctx.Done():
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "validation cancelled: " + ctx.Err().Error(),
		})
		return
	default:
	}

	for fieldName, field := range structure {
		// Validate field name first
		v.validateFieldName(fieldName, path, errors)

		currentPath := fieldName
		if path != "" {
			currentPath = path + "." + fieldName
		}

		v.validateField(ctx, field, currentPath, errors)
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
func (v *Validator) validateField(ctx context.Context, field config.Field, path string, errors *[]ValidationError) {
	// Check if context is cancelled before processing this field
	select {
	case <-ctx.Done():
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "validation cancelled: " + ctx.Err().Error(),
		})
		return
	default:
	}

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
		v.validateNonLeafNode(ctx, field, path, errors)
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
func (v *Validator) validateNonLeafNode(ctx context.Context, field config.Field, path string, errors *[]ValidationError) {
	// Non-leaf nodes (folders) should not have _type, _description, or _unit
	if field.Type != "" {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "non-leaf nodes (folders) cannot have _type",
		})
	}

	if field.Description != "" {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "non-leaf nodes (folders) cannot have _description",
		})
	}

	if field.Unit != "" {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "non-leaf nodes (folders) cannot have _unit",
		})
	}

	// For non-leaf nodes, recursively validate subfields
	v.validateStructure(ctx, field.Subfields, path, errors)
}

// validateReferences recursively validates all _refModel references in a data model
func (v *Validator) validateReferences(ctx context.Context, dataModel config.DataModelVersion, allDataModels map[string]config.DataModelsConfig, visitedModels map[string]bool, depth int) error {
	// Check if context is cancelled before starting reference validation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	var errors []ValidationError
	v.validateStructureReferences(ctx, dataModel.Structure, allDataModels, visitedModels, depth, "", &errors)

	if len(errors) > 0 {
		// Build error message with all validation errors
		errorMsg := "data model reference validation failed:"
		for _, validationError := range errors {
			errorMsg += fmt.Sprintf("\n  - %s", validationError.Error())
		}
		return fmt.Errorf("%s", errorMsg)
	}

	return nil
}

// validateStructureReferences recursively validates references in a structure
func (v *Validator) validateStructureReferences(ctx context.Context, structure map[string]config.Field, allDataModels map[string]config.DataModelsConfig, visitedModels map[string]bool, depth int, path string, errors *[]ValidationError) {
	// Check if context is cancelled before processing this level
	select {
	case <-ctx.Done():
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "validation cancelled: " + ctx.Err().Error(),
		})
		return
	default:
	}

	for fieldName, field := range structure {
		currentPath := fieldName
		if path != "" {
			currentPath = path + "." + fieldName
		}

		// Check if this field has a reference
		if field.ModelRef != "" {
			v.validateSingleReference(ctx, field.ModelRef, currentPath, allDataModels, visitedModels, depth, errors)
		}

		// Recursively check subfields
		if field.Subfields != nil {
			v.validateStructureReferences(ctx, field.Subfields, allDataModels, visitedModels, depth, currentPath, errors)
		}
	}
}

// validateSingleReference validates a single _refModel reference
func (v *Validator) validateSingleReference(ctx context.Context, modelRef string, path string, allDataModels map[string]config.DataModelsConfig, visitedModels map[string]bool, depth int, errors *[]ValidationError) {
	// Check if context is cancelled before processing this reference
	select {
	case <-ctx.Done():
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "validation cancelled: " + ctx.Err().Error(),
		})
		return
	default:
	}

	// Safety check: limit recursion depth to 10 levels
	if depth >= 10 {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: "reference validation depth limit exceeded (10 levels) - possible deep nesting or circular reference",
		})
		return
	}

	// Parse the model reference
	parts := strings.Split(modelRef, ":")
	if len(parts) != 2 {
		// This should already be caught by basic validation, but let's be safe
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: fmt.Sprintf("invalid _refModel format: %s", modelRef),
		})
		return
	}

	modelName := parts[0]
	version := parts[1]

	// Check for circular reference
	referenceKey := modelName + ":" + version
	if visitedModels[referenceKey] {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: fmt.Sprintf("circular reference detected: %s", referenceKey),
		})
		return
	}

	// Check if the referenced model exists
	referencedModel, exists := allDataModels[modelName]
	if !exists {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: fmt.Sprintf("referenced model '%s' does not exist", modelName),
		})
		return
	}

	// Check if the referenced version exists
	referencedVersion, versionExists := referencedModel.Versions[version]
	if !versionExists {
		*errors = append(*errors, ValidationError{
			Path:    path,
			Message: fmt.Sprintf("referenced model '%s' version '%s' does not exist", modelName, version),
		})
		return
	}

	// Mark this model as visited to detect cycles
	visitedModels[referenceKey] = true

	// Recursively validate the referenced model (increment depth here)
	v.validateStructureReferences(ctx, referencedVersion.Structure, allDataModels, visitedModels, depth+1, path+"."+referenceKey, errors)

	// Unmark this model after validation (backtrack)
	delete(visitedModels, referenceKey)
}
