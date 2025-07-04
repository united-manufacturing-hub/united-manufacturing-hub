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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/validation"
)

// Validator is a public alias for the validation package's Validator
type Validator = validation.Validator

// NewValidator creates a new Validator instance
func NewValidator() *Validator {
	return validation.NewValidator()
}

// ValidateStructureOnly is a convenience function that creates a validator and validates structure only
func ValidateStructureOnly(ctx context.Context, dataModel config.DataModelVersion) error {
	validator := NewValidator()
	return validator.ValidateStructureOnly(ctx, dataModel)
}

// ValidateWithReferences is a convenience function that creates a validator and validates with references
func ValidateWithReferences(ctx context.Context, dataModel config.DataModelVersion, allDataModels map[string]config.DataModelsConfig) error {
	validator := NewValidator()
	return validator.ValidateWithReferences(ctx, dataModel, allDataModels)
}
