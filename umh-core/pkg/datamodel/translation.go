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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel/translation"
)

// Translator is a public alias for the translation package's Translator
type Translator = translation.Translator

// TypeCategory is a public alias for the translation package's TypeCategory
type TypeCategory = translation.TypeCategory

// TypeInfo is a public alias for the translation package's TypeInfo
type TypeInfo = translation.TypeInfo

// PathInfo is a public alias for the translation package's PathInfo
type PathInfo = translation.PathInfo

// SchemaOutput is a public alias for the translation package's SchemaOutput
type SchemaOutput = translation.SchemaOutput

// Public type category constants
const (
	TypeCategoryTimeseries = translation.TypeCategoryTimeseries
	TypeCategoryRelational = translation.TypeCategoryRelational
)

// NewTranslator creates a new Translator instance
func NewTranslator() *Translator {
	return translation.NewTranslator()
}

// ParseTypeInfo parses a field type string into structured information
func ParseTypeInfo(fieldType string) (TypeInfo, error) {
	return translation.ParseTypeInfo(fieldType)
}

// TranslateToJSONSchema is a convenience function that creates a translator and translates to JSON schemas
func TranslateToJSONSchema(ctx context.Context, dataModel config.DataModelVersion, modelName string, version string, allDataModels map[string]config.DataModelsConfig) ([]SchemaOutput, error) {
	translator := NewTranslator()
	return translator.TranslateToJSONSchema(ctx, dataModel, modelName, version, allDataModels)
}
