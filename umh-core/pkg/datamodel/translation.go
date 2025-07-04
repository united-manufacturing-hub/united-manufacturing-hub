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
// This file provides backward-compatible access to the translation functionality.
// For new code, consider importing the translation sub-package directly.
package datamodel

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/translation"
)

// Translator provides translation of validated data models to JSON schemas.
// This is a facade that delegates to the translation sub-package.
type Translator = translation.Translator

// SchemaOutput represents a generated JSON schema with its name.
// This is a facade that delegates to the translation sub-package.
type SchemaOutput = translation.SchemaOutput

// TranslationError represents a translation error with a path and message.
// This is a facade that delegates to the translation sub-package.
type TranslationError = translation.TranslationError

// TypeCategory represents the high-level category of a data model field type.
// This is a facade that delegates to the translation sub-package.
type TypeCategory = translation.TypeCategory

// TypeInfo contains parsed information about a field type.
// This is a facade that delegates to the translation sub-package.
type TypeInfo = translation.TypeInfo

// PathInfo represents a leaf field path with its type information.
// This is a facade that delegates to the translation sub-package.
type PathInfo = translation.PathInfo

// TypeTranslator interface for type-specific translation logic.
// This is a facade that delegates to the translation sub-package.
type TypeTranslator = translation.TypeTranslator

// Type category constants
const (
	TypeCategoryTimeseries = translation.TypeCategoryTimeseries
	TypeCategoryRelational = translation.TypeCategoryRelational
)

// NewTranslator creates a new Translator instance.
// This is a facade that delegates to the translation sub-package.
func NewTranslator() *Translator {
	return translation.NewTranslator()
}

// ParseTypeInfo parses a field type string into structured information.
// This is a facade that delegates to the translation sub-package.
func ParseTypeInfo(fieldType string) (TypeInfo, error) {
	return translation.ParseTypeInfo(fieldType)
}

// TranslateToJSONSchema translates a data model to JSON schemas grouped by type.
// This is a convenience function that creates a translator and calls TranslateToJSONSchema.
func TranslateToJSONSchema(ctx context.Context, dataModel config.DataModelVersion, modelName string, version string, allDataModels map[string]config.DataModelsConfig) ([]SchemaOutput, error) {
	translator := NewTranslator()
	return translator.TranslateToJSONSchema(ctx, dataModel, modelName, version, allDataModels)
}
