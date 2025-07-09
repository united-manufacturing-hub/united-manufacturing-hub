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

// Package datamodel provides high-performance validation and JSON Schema translation
// for UMH data models.
//
// The translator converts validated UMH data models into JSON Schema format compatible
// with the Schema Registry, enabling runtime validation of UNS messages by benthos-umh.

package datamodel

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
)

// JSONSchema represents a JSON Schema document
type JSONSchema map[string]interface{}

// SchemaTranslationResult contains the result of translating a data model to JSON schemas
type SchemaTranslationResult struct {
	// Schemas maps schema registry subject names to JSON schemas
	// Subject format: {contract_name}_{version}-{payload_shape}
	// Example: "_pump_v1-timeseries-number"
	Schemas map[string]JSONSchema

	// PayloadShapeUsage maps payload shapes to the virtual paths that use them
	// This helps understand which fields are covered by each schema
	PayloadShapeUsage map[string][]string
}

// Translator provides high-performance translation of UMH data models to JSON Schema format.
// The translator works with validated data models and produces schemas compatible with
// the Schema Registry format expected by benthos-umh UNS output plugin.
//
// Performance characteristics:
//   - Leverages existing validator performance optimizations
//   - Minimal memory allocations for path building
//   - Efficient virtual path extraction and grouping
type Translator struct {
	validator *Validator
}

// NewTranslator creates a new Translator instance.
// The translator includes a validator to ensure only valid data models are translated.
func NewTranslator() *Translator {
	return &Translator{
		validator: NewValidator(),
	}
}

// ensureDefaultPayloadShapes creates a copy of the payload shapes map with default payload shapes injected if not present.
// This ensures that the two fundamental payload shapes (timeseries-number and timeseries-string) are always available.
//
// The function never overrides existing payload shapes, preserving any custom definitions provided by the user.
// Default payload shapes include standard UMH timeseries fields (timestamp_ms and value) with appropriate types.
func ensureDefaultPayloadShapes(payloadShapes map[string]config.PayloadShape) map[string]config.PayloadShape {
	// Create a copy to avoid modifying the original map, pre-size for existing + 2 defaults
	enriched := make(map[string]config.PayloadShape, len(payloadShapes)+2)

	// Copy existing payload shapes
	for name, shape := range payloadShapes {
		enriched[name] = shape
	}

	// Inject default timeseries-number if not present
	if _, exists := enriched["timeseries-number"]; !exists {
		enriched["timeseries-number"] = config.PayloadShape{
			Fields: map[string]config.PayloadField{
				"timestamp_ms": {Type: "number"},
				"value":        {Type: "number"},
			},
		}
	}

	// Inject default timeseries-string if not present
	if _, exists := enriched["timeseries-string"]; !exists {
		enriched["timeseries-string"] = config.PayloadShape{
			Fields: map[string]config.PayloadField{
				"timestamp_ms": {Type: "number"},
				"value":        {Type: "string"},
			},
		}
	}

	return enriched
}

// TranslateDataModel translates a UMH data model to JSON Schema format for Schema Registry.
// The function first validates the data model structure, then extracts virtual paths,
// groups them by payload shape, and generates JSON schemas for each payload shape.
//
// Default payload shapes (timeseries-number and timeseries-string) are automatically
// injected if not present in the payloadShapes map. Existing payload shapes are never
// overridden, ensuring custom definitions take precedence.
//
// If allDataModels is provided (non-nil and non-empty), the function will perform full
// reference validation and resolve model references. If allDataModels is nil or empty,
// only basic structure validation is performed without reference resolution.
//
// Parameters:
//   - ctx: context for cancellation
//   - contractName: the contract name (e.g., "_pump_data")
//   - version: the version (e.g., "v1")
//   - dataModel: the data model to translate
//   - payloadShapes: map of payload shape definitions
//   - allDataModels: optional map of all available data models for reference resolution (can be nil)
//
// Returns SchemaTranslationResult containing:
//   - Schemas: map of subject names to JSON schemas
//   - PayloadShapeUsage: which virtual paths use each payload shape
func (t *Translator) TranslateDataModel(
	ctx context.Context,
	contractName string,
	version string,
	dataModel config.DataModelVersion,
	payloadShapes map[string]config.PayloadShape,
	allDataModels map[string]config.DataModelsConfig,
) (*SchemaTranslationResult, error) {
	// Check if context is cancelled before starting translation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Ensure default payload shapes are available
	enrichedPayloadShapes := ensureDefaultPayloadShapes(payloadShapes)

	// Determine if we should handle references based on allDataModels
	hasReferences := len(allDataModels) > 0

	var pathsByShape map[string][]string
	var err error

	if hasReferences {
		// Validate with full reference checking
		if err := t.validator.ValidateWithReferences(ctx, dataModel, allDataModels); err != nil {
			return nil, fmt.Errorf("data model reference validation failed: %w", err)
		}

		// Resolve all references and flatten to virtual paths
		pathsByShape, err = t.extractVirtualPathsWithReferences(ctx, dataModel.Structure, "", allDataModels, enrichedPayloadShapes, make(map[string]bool), 0)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve virtual paths with references: %w", err)
		}
	} else {
		// Basic structure validation only
		if err := t.validator.ValidateStructureOnly(ctx, dataModel); err != nil {
			return nil, fmt.Errorf("data model validation failed: %w", err)
		}

		// Extract virtual paths without reference resolution
		pathsByShape, err = t.extractVirtualPaths(ctx, dataModel.Structure, "", enrichedPayloadShapes)
		if err != nil {
			return nil, fmt.Errorf("failed to extract virtual paths: %w", err)
		}
	}

	// Generate JSON schemas for each payload shape, pre-size based on number of shapes
	schemas := make(map[string]JSONSchema, len(pathsByShape))
	for payloadShape, virtualPaths := range pathsByShape {
		// Check context cancellation during schema generation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Get payload shape definition
		shapeConfig, exists := enrichedPayloadShapes[payloadShape]
		if !exists {
			return nil, fmt.Errorf("payload shape '%s' not found in configuration", payloadShape)
		}

		// Generate schema registry subject name
		subjectName := generateSubjectName(contractName, version, payloadShape)

		// Generate JSON schema for this payload shape and virtual paths
		schema, err := t.generateJSONSchema(ctx, virtualPaths, shapeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to generate JSON schema for payload shape '%s': %w", payloadShape, err)
		}

		schemas[subjectName] = schema
	}

	return &SchemaTranslationResult{
		Schemas:           schemas,
		PayloadShapeUsage: pathsByShape,
	}, nil
}

// extractVirtualPaths recursively extracts virtual paths from a data model structure
// and groups them by payload shape. Virtual paths are dot-separated strings representing
// the path to each leaf field (e.g., "vibration.x-axis", "motor.speed").
func (t *Translator) extractVirtualPaths(
	ctx context.Context,
	structure map[string]config.Field,
	currentPath string,
	payloadShapes map[string]config.PayloadShape,
) (map[string][]string, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	pathsByShape := make(map[string][]string, len(structure))

	for fieldName, field := range structure {
		// Build the current field path efficiently
		var fieldPath string
		if currentPath == "" {
			fieldPath = fieldName
		} else {
			fieldPath = currentPath + "." + fieldName
		}

		// If this field has a payload shape, add it to the results
		if field.PayloadShape != "" {
			if pathsByShape[field.PayloadShape] == nil {
				pathsByShape[field.PayloadShape] = make([]string, 0, 4) // typical field count per shape
			}
			pathsByShape[field.PayloadShape] = append(pathsByShape[field.PayloadShape], fieldPath)
		}

		// If this field has subfields, recurse into them
		if field.Subfields != nil {
			subPaths, err := t.extractVirtualPaths(ctx, field.Subfields, fieldPath, payloadShapes)
			if err != nil {
				return nil, err
			}

			// Merge the results
			for shape, paths := range subPaths {
				if pathsByShape[shape] == nil {
					pathsByShape[shape] = make([]string, 0, len(paths)+4) // size for incoming + typical growth
				}
				pathsByShape[shape] = append(pathsByShape[shape], paths...)
			}
		}

		// Note: ModelRef fields will be handled by extractVirtualPathsWithReferences
	}

	// Sort virtual paths for consistent output
	for shape := range pathsByShape {
		sort.Strings(pathsByShape[shape])
	}

	return pathsByShape, nil
}

// extractVirtualPathsWithReferences resolves model references and extracts virtual paths
func (t *Translator) extractVirtualPathsWithReferences(
	ctx context.Context,
	structure map[string]config.Field,
	currentPath string,
	allDataModels map[string]config.DataModelsConfig,
	payloadShapes map[string]config.PayloadShape,
	visitedModels map[string]bool,
	depth int,
) (map[string][]string, error) {
	// Check context cancellation and depth limit
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if depth >= 10 {
		return nil, fmt.Errorf("reference resolution depth limit exceeded at path '%s'", currentPath)
	}

	pathsByShape := make(map[string][]string, len(structure))

	for fieldName, field := range structure {
		var fieldPath string
		if currentPath == "" {
			fieldPath = fieldName
		} else {
			fieldPath = currentPath + "." + fieldName
		}

		// Handle payload shape fields
		if field.PayloadShape != "" {
			if pathsByShape[field.PayloadShape] == nil {
				pathsByShape[field.PayloadShape] = make([]string, 0, 4) // typical field count per shape
			}
			pathsByShape[field.PayloadShape] = append(pathsByShape[field.PayloadShape], fieldPath)
		}

		// Handle subfields
		if field.Subfields != nil {
			subPaths, err := t.extractVirtualPathsWithReferences(ctx, field.Subfields, fieldPath, allDataModels, payloadShapes, visitedModels, depth)
			if err != nil {
				return nil, err
			}
			t.mergePathsByShape(pathsByShape, subPaths)
		}

		// Handle model references
		if field.ModelRef != nil {
			refPaths, err := t.resolveModelReference(ctx, field.ModelRef, fieldPath, allDataModels, payloadShapes, visitedModels, depth)
			if err != nil {
				return nil, err
			}
			t.mergePathsByShape(pathsByShape, refPaths)
		}
	}

	// Sort virtual paths for consistent output
	for shape := range pathsByShape {
		sort.Strings(pathsByShape[shape])
	}

	return pathsByShape, nil
}

// resolveModelReference resolves a model reference and extracts its virtual paths
func (t *Translator) resolveModelReference(
	ctx context.Context,
	modelRef *config.ModelRef,
	currentPath string,
	allDataModels map[string]config.DataModelsConfig,
	payloadShapes map[string]config.PayloadShape,
	visitedModels map[string]bool,
	depth int,
) (map[string][]string, error) {
	// Use more efficient string building for reference key
	var referenceKey strings.Builder
	referenceKey.Grow(len(modelRef.Name) + 1 + len(modelRef.Version)) // pre-size for exact length
	referenceKey.WriteString(modelRef.Name)
	referenceKey.WriteByte(':')
	referenceKey.WriteString(modelRef.Version)
	refKey := referenceKey.String()

	// Check for circular references
	if visitedModels[refKey] {
		return nil, fmt.Errorf("circular reference detected: %s at path '%s'", refKey, currentPath)
	}

	// Check if referenced model exists
	referencedModel, exists := allDataModels[modelRef.Name]
	if !exists {
		return nil, fmt.Errorf("referenced model '%s' does not exist at path '%s'", modelRef.Name, currentPath)
	}

	referencedVersion, versionExists := referencedModel.Versions[modelRef.Version]
	if !versionExists {
		return nil, fmt.Errorf("referenced model '%s' version '%s' does not exist at path '%s'", modelRef.Name, modelRef.Version, currentPath)
	}

	// Mark as visited and recurse
	visitedModels[refKey] = true
	defer delete(visitedModels, refKey)

	return t.extractVirtualPathsWithReferences(ctx, referencedVersion.Structure, currentPath, allDataModels, payloadShapes, visitedModels, depth+1)
}

// mergePathsByShape merges two pathsByShape maps
func (t *Translator) mergePathsByShape(dest, src map[string][]string) {
	for shape, paths := range src {
		if dest[shape] == nil {
			dest[shape] = make([]string, 0, len(paths)+4) // size for incoming + typical growth
		}
		dest[shape] = append(dest[shape], paths...)
	}
}

// generateJSONSchema creates a JSON schema for a specific payload shape and virtual paths
func (t *Translator) generateJSONSchema(
	ctx context.Context,
	virtualPaths []string,
	payloadShape config.PayloadShape,
) (JSONSchema, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Generate the fields schema from the payload shape definition
	fieldsSchema, err := t.generateFieldsSchema(payloadShape)
	if err != nil {
		return nil, fmt.Errorf("failed to generate fields schema: %w", err)
	}

	// Create the main schema structure
	schema := JSONSchema{
		"type": "object",
		"properties": map[string]interface{}{
			"virtual_path": map[string]interface{}{
				"type": "string",
				"enum": virtualPaths,
			},
			"fields": fieldsSchema,
		},
		"required":             []string{"virtual_path", "fields"},
		"additionalProperties": false,
	}

	return schema, nil
}

// generateFieldsSchema converts a payload shape definition to JSON schema format
func (t *Translator) generateFieldsSchema(payloadShape config.PayloadShape) (map[string]interface{}, error) {
	fieldCount := len(payloadShape.Fields)
	properties := make(map[string]interface{}, fieldCount)
	required := make([]string, 0, fieldCount)

	for fieldName, field := range payloadShape.Fields {
		// Convert UMH type to JSON Schema type
		jsonType, err := t.convertTypeToJSONSchema(field.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to convert type for field '%s': %w", fieldName, err)
		}

		properties[fieldName] = map[string]interface{}{
			"type": jsonType,
		}

		// For now, assume all fields in payload shapes are required
		// This can be made configurable later if needed
		required = append(required, fieldName)
	}

	// Sort required fields for consistent output
	sort.Strings(required)

	return map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}, nil
}

// convertTypeToJSONSchema converts UMH type names to JSON Schema types
func (t *Translator) convertTypeToJSONSchema(umhType string) (string, error) {
	switch umhType {
	case "number":
		return "number", nil
	case "string":
		return "string", nil
	case "boolean":
		return "boolean", nil
	case "integer":
		return "integer", nil
	default:
		return "", fmt.Errorf("unsupported UMH type: %s", umhType)
	}
}

// generateSubjectName creates a Schema Registry subject name from contract, version, and payload shape
// Format: {contract_name}_{version}-{payload_shape}
// Example: "_pump_v1-timeseries-number"
func generateSubjectName(contractName, version, payloadShape string) string {
	// Ensure contract name starts with underscore
	if !strings.HasPrefix(contractName, "_") {
		contractName = "_" + contractName
	}

	// Ensure version has 'v' prefix
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	return fmt.Sprintf("%s_%s-%s", contractName, version, payloadShape)
}

// GetSchemaAsJSON returns a JSON schema as a JSON byte array
func (result *SchemaTranslationResult) GetSchemaAsJSON(subjectName string) ([]byte, error) {
	schema, exists := result.Schemas[subjectName]
	if !exists {
		return nil, fmt.Errorf("schema not found for subject '%s'", subjectName)
	}

	return json.Marshal(schema)
}

// GetAllSubjectNames returns all schema registry subject names
func (result *SchemaTranslationResult) GetAllSubjectNames() []string {
	subjects := make([]string, 0, len(result.Schemas))
	for subject := range result.Schemas {
		subjects = append(subjects, subject)
	}
	sort.Strings(subjects)
	return subjects
}
