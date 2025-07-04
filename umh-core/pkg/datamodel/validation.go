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

// Package datamodel provides high-performance validation and translation for UMH data models.
//
// This package is organized into two main sub-packages:
//   - validation: High-performance validation of data model structures
//   - translation: Translation of validated data models to JSON schemas
//
// This file provides backward-compatible access to the validation functionality.
// For new code, consider importing the validation sub-package directly.
package datamodel

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/validation"
)

// Validator provides high-performance validation for UMH data models.
// This is a facade that delegates to the validation sub-package.
type Validator = validation.Validator

// ValidationError represents a validation error with a path and message.
// This is a facade that delegates to the validation sub-package.
type ValidationError = validation.ValidationError

// NewValidator creates a new Validator instance.
// This is a facade that delegates to the validation sub-package.
func NewValidator() *Validator {
	return validation.NewValidator()
}

// ValidateStructureOnly validates a data model's structure without checking references.
// This is a convenience function that creates a validator and calls ValidateStructureOnly.
func ValidateStructureOnly(ctx context.Context, dataModel config.DataModelVersion) error {
	validator := NewValidator()
	return validator.ValidateStructureOnly(ctx, dataModel)
}

// ValidateWithReferences validates a data model and all its references.
// This is a convenience function that creates a validator and calls ValidateWithReferences.
func ValidateWithReferences(ctx context.Context, dataModel config.DataModelVersion, allDataModels map[string]config.DataModelsConfig) error {
	validator := NewValidator()
	return validator.ValidateWithReferences(ctx, dataModel, allDataModels)
}
