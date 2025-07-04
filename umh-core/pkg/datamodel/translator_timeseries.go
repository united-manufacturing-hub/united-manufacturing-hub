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
	"encoding/json"
	"fmt"
	"sort"
)

// TimeseriesTranslator handles translation of timeseries data model fields to JSON schemas
type TimeseriesTranslator struct{}

// GetCategory returns the type category this translator handles
func (t *TimeseriesTranslator) GetCategory() TypeCategory {
	return TypeCategoryTimeseries
}

// TranslateToSchema generates a JSON schema for timeseries data with the given paths and subtype
func (t *TimeseriesTranslator) TranslateToSchema(ctx context.Context, paths []PathInfo, modelName string, version string, subType string) (SchemaOutput, error) {
	// Check if context is cancelled before starting schema generation
	select {
	case <-ctx.Done():
		return SchemaOutput{}, ctx.Err()
	default:
	}

	// Map timeseries subtypes to JSON schema types
	jsonType := t.mapTimeseriesType(subType)
	if jsonType == "" {
		return SchemaOutput{}, fmt.Errorf("unsupported timeseries subtype: %s", subType)
	}

	// Generate schema name: _<modelName>-<version>-<jsonType>
	schemaName := fmt.Sprintf("_%s-%s-%s", modelName, version, jsonType)

	// Extract and sort paths for consistent output
	pathStrings := make([]string, len(paths))
	for i, path := range paths {
		pathStrings[i] = path.Path
	}
	sort.Strings(pathStrings)

	// Generate the JSON schema
	schema, err := t.generateTimeseriesSchema(jsonType, pathStrings)
	if err != nil {
		return SchemaOutput{}, err
	}

	return SchemaOutput{
		Name:   schemaName,
		Schema: schema,
	}, nil
}

// mapTimeseriesType maps timeseries subtypes to JSON schema types
func (t *TimeseriesTranslator) mapTimeseriesType(subType string) string {
	mapping := map[string]string{
		"number":  "number",
		"string":  "string",
		"boolean": "boolean",
	}
	return mapping[subType]
}

// generateTimeseriesSchema generates a JSON schema for timeseries data
func (t *TimeseriesTranslator) generateTimeseriesSchema(jsonType string, paths []string) (string, error) {
	schema := TimeseriesJSONSchema{
		Type: "object",
		Properties: TimeseriesProperties{
			VirtualPath: VirtualPathSchema{
				Type: "string",
				Enum: paths,
			},
			Fields: FieldsSchema{
				Type: "object",
				Properties: FieldsProperties{
					Value: ValueSchema{
						Type: "object",
						Properties: ValueProperties{
							TimestampMs: TimestampSchema{
								Type: "number",
							},
							Value: ValueTypeSchema{
								Type: jsonType,
							},
						},
						Required:             []string{"timestamp_ms", "value"},
						AdditionalProperties: false,
					},
				},
				AdditionalProperties: false,
			},
		},
		Required:             []string{"virtual_path", "fields"},
		AdditionalProperties: false,
	}

	// Convert to JSON string
	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal timeseries schema: %w", err)
	}

	return string(jsonBytes), nil
}

// TimeseriesJSONSchema represents the structure of a timeseries JSON schema
type TimeseriesJSONSchema struct {
	Type                 string               `json:"type"`
	Properties           TimeseriesProperties `json:"properties"`
	Required             []string             `json:"required"`
	AdditionalProperties bool                 `json:"additionalProperties"`
}

// TimeseriesProperties represents the properties section of a timeseries schema
type TimeseriesProperties struct {
	VirtualPath VirtualPathSchema `json:"virtual_path"`
	Fields      FieldsSchema      `json:"fields"`
}

// VirtualPathSchema represents the virtual_path property schema
type VirtualPathSchema struct {
	Type string   `json:"type"`
	Enum []string `json:"enum"`
}

// FieldsSchema represents the fields property schema
type FieldsSchema struct {
	Type                 string           `json:"type"`
	Properties           FieldsProperties `json:"properties"`
	AdditionalProperties bool             `json:"additionalProperties"`
}

// FieldsProperties represents the properties within the fields object
type FieldsProperties struct {
	Value ValueSchema `json:"value"`
}

// ValueSchema represents the value property schema
type ValueSchema struct {
	Type                 string          `json:"type"`
	Properties           ValueProperties `json:"properties"`
	Required             []string        `json:"required"`
	AdditionalProperties bool            `json:"additionalProperties"`
}

// ValueProperties represents the properties within the value object
type ValueProperties struct {
	TimestampMs TimestampSchema `json:"timestamp_ms"`
	Value       ValueTypeSchema `json:"value"`
}

// TimestampSchema represents the timestamp_ms property schema
type TimestampSchema struct {
	Type string `json:"type"`
}

// ValueTypeSchema represents the value type property schema
type ValueTypeSchema struct {
	Type string `json:"type"`
}
