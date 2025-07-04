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

// TypeCategory represents the high-level category of a data model field type
type TypeCategory string

const (
	TypeCategoryTimeseries TypeCategory = "timeseries"
	TypeCategoryRelational TypeCategory = "relational"
)

// TypeInfo contains parsed information about a field type
type TypeInfo struct {
	Category TypeCategory
	SubType  string // "number", "string", "boolean", "table", etc.
	FullType string // "timeseries-number", "relational-table", etc.
}

// ParseTypeInfo parses a field type string into structured information
func ParseTypeInfo(fieldType string) TypeInfo {
	parts := strings.SplitN(fieldType, "-", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return TypeInfo{} // Invalid type
	}

	return TypeInfo{
		Category: TypeCategory(parts[0]),
		SubType:  parts[1],
		FullType: fieldType,
	}
}

// PathInfo represents a leaf field path with its type information
type PathInfo struct {
	Path      string
	ValueType string // Full type string like "timeseries-number"
}

// SchemaOutput represents a generated JSON schema
type SchemaOutput struct {
	Name   string // Schema name like "_pump-v1-number"
	Schema string // JSON schema content
}

// TranslationError represents a translation error with a path and message
type TranslationError struct {
	Path    string
	Message string
}

func (e TranslationError) Error() string {
	return fmt.Sprintf("translation error at path '%s': %s", e.Path, e.Message)
}

// TypeTranslator interface for type-specific schema generation
type TypeTranslator interface {
	// GetCategory returns the type category this translator handles
	GetCategory() TypeCategory

	// TranslateToSchema generates a JSON schema for the given paths and type
	TranslateToSchema(ctx context.Context, paths []PathInfo, modelName string, version string, subType string) (SchemaOutput, error)
}

// Translator provides high-performance translation of UMH data models to JSON schemas.
// The translator is stateless and thread-safe, allowing concurrent use across
// multiple goroutines. It translates data model structures into JSON schemas
// grouped by type category and subtype.
type Translator struct {
	typeTranslators map[TypeCategory]TypeTranslator
}

// NewTranslator creates a new Translator instance with all supported type translators
func NewTranslator() *Translator {
	return &Translator{
		typeTranslators: map[TypeCategory]TypeTranslator{
			TypeCategoryTimeseries: &TimeseriesTranslator{},
			TypeCategoryRelational: &RelationalTranslator{},
		},
	}
}

// TranslateToJSONSchema translates a data model to JSON schemas grouped by type
// Parameters:
// - ctx: context for cancellation
// - dataModel: the data model to translate
// - modelName: name of the model (for schema naming)
// - version: version of the model (for schema naming)
// - allDataModels: map of all available data models for reference resolution
// Returns slice of SchemaOutput, each containing a schema name and JSON content
func (t *Translator) TranslateToJSONSchema(ctx context.Context, dataModel config.DataModelVersion, modelName string, version string, allDataModels map[string]config.DataModelsConfig) ([]SchemaOutput, error) {
	// Check if context is cancelled before starting translation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Step 1: Collect all leaf paths with their types
	paths, err := t.collectAllLeafPaths(ctx, dataModel.Structure, allDataModels, make(map[string]bool), "")
	if err != nil {
		return nil, err
	}

	// Step 2: Group paths by type category and subtype
	groupedPaths := t.groupPathsByType(paths)

	// Step 3: Generate schemas using appropriate translators
	var allSchemas []SchemaOutput

	for typeKey, typePaths := range groupedPaths {
		typeInfo := ParseTypeInfo(typeKey)

		translator, exists := t.typeTranslators[typeInfo.Category]
		if !exists {
			return nil, fmt.Errorf("unsupported type category: %s", typeInfo.Category)
		}

		schema, err := translator.TranslateToSchema(ctx, typePaths, modelName, version, typeInfo.SubType)
		if err != nil {
			return nil, err
		}

		allSchemas = append(allSchemas, schema)
	}

	return allSchemas, nil
}

// groupPathsByType groups paths by their full type string
func (t *Translator) groupPathsByType(paths []PathInfo) map[string][]PathInfo {
	groups := make(map[string][]PathInfo)
	for _, path := range paths {
		groups[path.ValueType] = append(groups[path.ValueType], path)
	}
	return groups
}

// collectAllLeafPaths recursively collects all leaf field paths with their types
func (t *Translator) collectAllLeafPaths(ctx context.Context, structure map[string]config.Field, allDataModels map[string]config.DataModelsConfig, visitedModels map[string]bool, currentPath string) ([]PathInfo, error) {
	// Check if context is cancelled before processing this level
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var paths []PathInfo

	for fieldName, field := range structure {
		// Build path efficiently
		var fieldPath string
		if currentPath == "" {
			fieldPath = fieldName
		} else {
			fieldPath = currentPath + "." + fieldName
		}

		// Check if this field has a reference
		if field.ModelRef != "" {
			refPaths, err := t.resolveReference(ctx, field.ModelRef, allDataModels, visitedModels, fieldPath)
			if err != nil {
				return nil, err
			}
			paths = append(paths, refPaths...)
		} else if field.Type != "" {
			// This is a leaf field with a type
			paths = append(paths, PathInfo{
				Path:      fieldPath,
				ValueType: field.Type,
			})
		} else if field.Subfields != nil {
			// Recursively process subfields
			subPaths, err := t.collectAllLeafPaths(ctx, field.Subfields, allDataModels, visitedModels, fieldPath)
			if err != nil {
				return nil, err
			}
			paths = append(paths, subPaths...)
		}
	}

	return paths, nil
}

// resolveReference resolves a _refModel reference and returns its leaf paths
func (t *Translator) resolveReference(ctx context.Context, modelRef string, allDataModels map[string]config.DataModelsConfig, visitedModels map[string]bool, prefixPath string) ([]PathInfo, error) {
	// Check if context is cancelled before processing this reference
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Parse the model reference
	colonIndex := strings.IndexByte(modelRef, ':')
	if colonIndex == -1 {
		return nil, TranslationError{
			Path:    prefixPath,
			Message: fmt.Sprintf("invalid _refModel format: %s", modelRef),
		}
	}

	modelName := modelRef[:colonIndex]
	version := modelRef[colonIndex+1:]

	// Check for circular reference
	referenceKey := modelName + ":" + version
	if visitedModels[referenceKey] {
		return nil, TranslationError{
			Path:    prefixPath,
			Message: fmt.Sprintf("circular reference detected: %s", referenceKey),
		}
	}

	// Check if the referenced model exists
	referencedModel, exists := allDataModels[modelName]
	if !exists {
		return nil, TranslationError{
			Path:    prefixPath,
			Message: fmt.Sprintf("referenced model '%s' does not exist", modelName),
		}
	}

	// Check if the referenced version exists
	referencedVersion, versionExists := referencedModel.Versions[version]
	if !versionExists {
		return nil, TranslationError{
			Path:    prefixPath,
			Message: fmt.Sprintf("referenced model '%s' version '%s' does not exist", modelName, version),
		}
	}

	// Mark this model as visited to detect cycles
	visitedModels[referenceKey] = true

	// Recursively collect paths from the referenced model
	refPaths, err := t.collectAllLeafPaths(ctx, referencedVersion.Structure, allDataModels, visitedModels, prefixPath)
	if err != nil {
		return nil, err
	}

	// Unmark this model after processing (backtrack)
	delete(visitedModels, referenceKey)

	return refPaths, nil
}
