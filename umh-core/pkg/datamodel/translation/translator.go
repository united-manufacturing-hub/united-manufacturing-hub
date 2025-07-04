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

package translation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// Translation-specific constants for space-ready deployment
const (
	// Maximum translation timeout to prevent hanging in space
	MAX_TRANSLATION_TIMEOUT = 30 * time.Second
	// Maximum number of schemas generated per translation (sanity check)
	MAX_SCHEMAS_PER_TRANSLATION = 100
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
// SPACE-READY: Returns error for robust error handling
func ParseTypeInfo(fieldType string) (TypeInfo, error) {
	if fieldType == "" {
		return TypeInfo{}, fmt.Errorf("field type cannot be empty")
	}

	parts := strings.SplitN(fieldType, "-", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return TypeInfo{}, fmt.Errorf("invalid field type format: %s (expected format: category-subtype)", fieldType)
	}

	return TypeInfo{
		Category: TypeCategory(parts[0]),
		SubType:  parts[1],
		FullType: fieldType,
	}, nil
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
//
// SPACE-READY ASSUMPTIONS:
// - Input has been validated by the validator
// - Field names, structures, and references are valid
// - Recursion depth is already controlled by validator
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

// normalizeModelName strips leading underscores from model names
// Users sometimes mistakenly include the underscore prefix that's used in schema naming
func (t *Translator) normalizeModelName(modelName string) string {
	// Strip leading underscores
	for len(modelName) > 0 && modelName[0] == '_' {
		modelName = modelName[1:]
	}
	return modelName
}

// normalizeVersion adds "v" prefix to version if it's missing
// Users sometimes forget the "v" prefix (e.g., "1" instead of "v1")
func (t *Translator) normalizeVersion(version string) string {
	// If version is empty, return as-is (will be caught by validation elsewhere)
	if version == "" {
		return version
	}

	// If version already starts with "v", return as-is
	if strings.HasPrefix(version, "v") {
		return version
	}

	// Add "v" prefix
	return "v" + version
}

// TranslateToJSONSchema translates a data model to JSON schemas grouped by type
// Parameters:
// - ctx: context for cancellation
// - dataModel: the data model to translate (MUST be pre-validated)
// - modelName: name of the model (for schema naming) - leading underscores will be stripped
// - version: version of the model (for schema naming) - "v" prefix will be added if missing
// - allDataModels: map of all available data models for reference resolution (MUST be pre-validated)
// Returns slice of SchemaOutput, each containing a schema name and JSON content
//
// SPACE-READY: Assumes input is pre-validated, focuses on translation robustness
func (t *Translator) TranslateToJSONSchema(ctx context.Context, dataModel config.DataModelVersion, modelName string, version string, allDataModels map[string]config.DataModelsConfig) ([]SchemaOutput, error) {
	// SPACE-READY: Add timeout protection to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, MAX_TRANSLATION_TIMEOUT)
	defer cancel()

	// SPACE-READY: Panic recovery for mission-critical deployment
	defer func() {
		if r := recover(); r != nil {
			// Log the panic but don't crash the probe
			// In a real space deployment, this would be logged to telemetry
			fmt.Printf("CRITICAL: Panic recovered in TranslateToJSONSchema: %v", r)
		}
	}()

	// Check if context is cancelled before starting translation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Normalize inputs to handle common user mistakes
	modelName = t.normalizeModelName(modelName)
	version = t.normalizeVersion(version)

	// Step 1: Collect all leaf paths with their types
	paths, err := t.collectAllLeafPaths(ctx, dataModel.Structure, allDataModels, make(map[string]bool), "")
	if err != nil {
		return nil, fmt.Errorf("failed to collect leaf paths: %w", err)
	}

	// SPACE-READY: Sanity check - ensure we have paths to translate
	if len(paths) == 0 {
		return nil, fmt.Errorf("no valid paths found in data model")
	}

	// Step 2: Group paths by type category and subtype
	groupedPaths := t.groupPathsByType(paths)

	// SPACE-READY: Sanity check - prevent generating too many schemas
	if len(groupedPaths) > MAX_SCHEMAS_PER_TRANSLATION {
		return nil, fmt.Errorf("too many schema types (max %d)", MAX_SCHEMAS_PER_TRANSLATION)
	}

	// Step 3: Generate schemas using appropriate translators
	// Pre-allocate with exact capacity since we know the number of type groups
	allSchemas := make([]SchemaOutput, 0, len(groupedPaths))

	for typeKey, typePaths := range groupedPaths {
		// SPACE-READY: Handle ParseTypeInfo error
		typeInfo, err := ParseTypeInfo(typeKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse type %s: %w", typeKey, err)
		}

		translator, exists := t.typeTranslators[typeInfo.Category]
		if !exists {
			return nil, fmt.Errorf("unsupported type category: %s", typeInfo.Category)
		}

		schema, err := translator.TranslateToSchema(ctx, typePaths, modelName, version, typeInfo.SubType)
		if err != nil {
			return nil, fmt.Errorf("failed to translate schema for type %s: %w", typeKey, err)
		}

		allSchemas = append(allSchemas, schema)
	}

	return allSchemas, nil
}

// groupPathsByType groups paths by their full type string
func (t *Translator) groupPathsByType(paths []PathInfo) map[string][]PathInfo {
	// Pre-allocate map with estimated capacity (typically 3-4 types: number, string, boolean, etc.)
	groups := make(map[string][]PathInfo, 4)
	for _, path := range paths {
		groups[path.ValueType] = append(groups[path.ValueType], path)
	}
	return groups
}

// collectAllLeafPaths recursively collects all leaf field paths with their types
// SPACE-READY: Assumes input is pre-validated, focuses on translation robustness
func (t *Translator) collectAllLeafPaths(ctx context.Context, structure map[string]config.Field, allDataModels map[string]config.DataModelsConfig, visitedModels map[string]bool, currentPath string) ([]PathInfo, error) {
	// Check if context is cancelled before processing this level
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Pre-allocate with estimated capacity based on structure size
	// Most fields are either leaf nodes or have a few subfields, so this is a reasonable estimate
	estimatedCapacity := len(structure) * 2 // Conservative estimate for nested structures
	paths := make([]PathInfo, 0, estimatedCapacity)

	for fieldName, field := range structure {
		// Build path efficiently using pre-calculated length to avoid reallocations
		var fieldPath string
		if currentPath == "" {
			fieldPath = fieldName
		} else {
			// Pre-calculate total length for efficient allocation
			totalLen := len(currentPath) + 1 + len(fieldName)
			var builder strings.Builder
			builder.Grow(totalLen)
			builder.WriteString(currentPath)
			builder.WriteByte('.')
			builder.WriteString(fieldName)
			fieldPath = builder.String()
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
// SPACE-READY: Assumes references are pre-validated, focuses on translation robustness
func (t *Translator) resolveReference(ctx context.Context, modelRef string, allDataModels map[string]config.DataModelsConfig, visitedModels map[string]bool, prefixPath string) ([]PathInfo, error) {
	// Check if context is cancelled before processing this reference
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Parse the model reference (format already validated by validator)
	colonIndex := strings.IndexByte(modelRef, ':')
	if colonIndex == -1 {
		return nil, TranslationError{
			Path:    prefixPath,
			Message: fmt.Sprintf("invalid _refModel format: %s", modelRef),
		}
	}

	modelName := modelRef[:colonIndex]
	version := modelRef[colonIndex+1:]

	// Check for circular reference (should be caught by validator, but double-check for safety)
	referenceKey := modelName + ":" + version
	if visitedModels[referenceKey] {
		return nil, TranslationError{
			Path:    prefixPath,
			Message: fmt.Sprintf("circular reference detected: %s", referenceKey),
		}
	}

	// Check if the referenced model exists (should be validated, but check for safety)
	referencedModel, exists := allDataModels[modelName]
	if !exists {
		return nil, TranslationError{
			Path:    prefixPath,
			Message: fmt.Sprintf("referenced model '%s' does not exist", modelName),
		}
	}

	// Check if the referenced version exists (should be validated, but check for safety)
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
