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
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

// ValidateDataModel validates a data model version
// It applies the following rules:
// A node can either be a leaf node or a non-leaf node.
// A leaf node must have a either:
// - _type (and optionally a _description and _unit)
// - _refModel
// A non-leaf node must not have a _type, _description or _unit
// The validator does not check if the _refModel is valid
func (v *Validator) ValidateDataModel(ctx context.Context, dataModel config.DataModelVersion) error {
	return v.validateStructure(dataModel.Structure, "")
}

// validateStructure recursively validates the structure of a data model
func (v *Validator) validateStructure(structure map[string]config.Field, path string) error {
	for fieldName, field := range structure {
		currentPath := fieldName
		if path != "" {
			currentPath = path + "." + fieldName
		}

		if err := v.validateField(field, currentPath); err != nil {
			return err
		}
	}
	return nil
}

// validateField validates a single field according to the rules
func (v *Validator) validateField(field config.Field, path string) error {
	isLeaf := v.isLeafNode(field)

	if isLeaf {
		return v.validateLeafNode(field, path)
	}

	return v.validateNonLeafNode(field, path)
}

// isLeafNode determines if a field is a leaf node (has no subfields)
func (v *Validator) isLeafNode(field config.Field) bool {
	return len(field.Subfields) == 0
}

// validateLeafNode validates a leaf node
func (v *Validator) validateLeafNode(field config.Field, path string) error {
	hasType := field.Type != ""
	hasRefModel := field.ModelRef != ""

	// A leaf node must have either _type or _refModel, but not both
	if hasType && hasRefModel {
		return fmt.Errorf("leaf node '%s' cannot have both _type and _refModel", path)
	}

	if !hasType && !hasRefModel {
		return fmt.Errorf("leaf node '%s' must have either _type or _refModel", path)
	}

	return nil
}

// validateNonLeafNode validates a non-leaf node
func (v *Validator) validateNonLeafNode(field config.Field, path string) error {
	// A non-leaf node must not have _type, _description, or _unit
	var violations []string

	if field.Type != "" {
		violations = append(violations, "_type")
	}
	if field.Description != "" {
		violations = append(violations, "_description")
	}
	if field.Unit != "" {
		violations = append(violations, "_unit")
	}

	if len(violations) > 0 {
		return fmt.Errorf("non-leaf node '%s' cannot have %s", path, strings.Join(violations, ", "))
	}

	// Recursively validate subfields
	return v.validateStructure(field.Subfields, path)
}
